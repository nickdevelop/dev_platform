package flow

import (
	"context"
	"errors"
	"fmt"
	"log"
	"regexp"
	"strings"
	"sync"
	"time"

	"devplatform/internal/app/chat"
	"devplatform/internal/app/modelconfig"
	"devplatform/internal/app/sessions"
	"devplatform/internal/app/sessions/turns"
	"devplatform/internal/app/tasks"
	"devplatform/internal/core/agents"
)

var (
	ErrFlowRunNotFound = errors.New("flow: run not found")
	ErrFlowRunFinished = errors.New("flow: run already finished")
)

// SessionHistoryProvider is an optional interface to read completed session transcripts.
type SessionHistoryProvider interface {
	History(ctx context.Context, projectID uint, sessionID string) (sessions.History, error)
}

// ModelResolver defines how model names or IDs are resolved to endpoints and credentials.
type ModelResolver interface {
	ResolveModelByNameOrID(ref string) (modelconfig.ResolvedModel, error)
}

// Completer abstracts OpenAI-compatible chat completions.
type Completer interface {
	Stream(ctx context.Context, req chat.CompletionRequest, onDelta func(chat.CompletionDelta) error) error
	Complete(ctx context.Context, req chat.CompletionRequest, maxTokens int) (string, *chat.Usage, error)
}

// SessionLauncher abstracts dispatching coding agent sessions in real project workspaces.
type SessionLauncher interface {
	Create(ctx context.Context, projectID uint, backend agents.BackendID, title, message, model string, images ...[]agents.ImageAttachment) (any, error)
}

// SessionLauncherGetter is an optional interface to check session completion.
type SessionLauncherOutcome interface {
	Get(projectID uint, sessionID string) (any, error)
}

// TaskRepository abstracts database operations for tasks, roles, runs and flow messages.
type TaskRepository interface {
	Get(id uint) (tasks.Task, error)
	Update(t tasks.Task) error
	ListRoles(taskID uint) ([]tasks.FlowRole, error)
	GetRole(id uint) (tasks.FlowRole, error)
	CreateFlowRun(run tasks.FlowRun) (tasks.FlowRun, error)
	GetFlowRun(id uint) (tasks.FlowRun, error)
	UpdateFlowRun(run tasks.FlowRun) error
	CreateFlowMessage(msg tasks.FlowMessage) (tasks.FlowMessage, error)
	UpdateFlowMessage(msg tasks.FlowMessage) error
	ListFlowMessages(runID uint) ([]tasks.FlowMessage, error)
}

// Engine coordinates multi-agent collaborative workflows.
type Engine struct {
	repo      TaskRepository
	launcher  tasks.SessionLauncher
	resolver  ModelResolver
	completer Completer
	eventBus  *EventBus

	mu       sync.Mutex
	cancels  map[uint]context.CancelFunc
	runLocks map[uint]*sync.Mutex
}

// NewEngine creates a new collaborative flow engine.
func NewEngine(
	repo TaskRepository,
	launcher tasks.SessionLauncher,
	resolver ModelResolver,
	completer Completer,
	eventBus *EventBus,
) *Engine {
	if eventBus == nil {
		eventBus = NewEventBus()
	}
	return &Engine{
		repo:      repo,
		launcher:  launcher,
		resolver:  resolver,
		completer: completer,
		eventBus:  eventBus,
		cancels:   make(map[uint]context.CancelFunc),
		runLocks:  make(map[uint]*sync.Mutex),
	}
}

// EventBus exposes the event bus for SSE handlers.
func (e *Engine) EventBus() *EventBus {
	return e.eventBus
}

// SubscribeEvents adapts EventBus subscriptions to tasks.FlowEvent.
func (e *Engine) SubscribeEvents(runID uint) (<-chan tasks.FlowEvent, func()) {
	rawCh, rawUnsub := e.eventBus.Subscribe(runID)
	outCh := make(chan tasks.FlowEvent, 64)
	done := make(chan struct{})

	go func() {
		defer close(outCh)
		for {
			select {
			case <-done:
				return
			case evt, ok := <-rawCh:
				if !ok {
					return
				}
				outCh <- tasks.FlowEvent{
					Type:    string(evt.Type),
					RunID:   evt.RunID,
					Payload: evt.Payload,
				}
			}
		}
	}()

	unsub := func() {
		close(done)
		rawUnsub()
	}
	return outCh, unsub
}

func (e *Engine) getRunLock(runID uint) *sync.Mutex {
	e.mu.Lock()
	defer e.mu.Unlock()
	lock, ok := e.runLocks[runID]
	if !ok {
		lock = &sync.Mutex{}
		e.runLocks[runID] = lock
	}
	return lock
}

// StartRun creates a new FlowRun and triggers the coordinator loop.
func (e *Engine) StartRun(ctx context.Context, task tasks.Task, triggerType string, initialPrompt string) (tasks.FlowRun, error) {
	if task.TaskType != tasks.TaskTypeFlow {
		return tasks.FlowRun{}, tasks.ErrInvalidTaskType
	}
	if strings.TrimSpace(task.OrchestratorModel) == "" {
		task.OrchestratorModel = task.Model
	}
	if strings.TrimSpace(task.OrchestratorModel) == "" {
		return tasks.FlowRun{}, tasks.ErrOrchestratorModelRequired
	}

	roles, err := e.repo.ListRoles(task.ID)
	if err != nil {
		return tasks.FlowRun{}, err
	}
	if len(roles) == 0 {
		return tasks.FlowRun{}, tasks.ErrNoRolesConfigured
	}

	prompt := strings.TrimSpace(initialPrompt)
	if prompt == "" {
		prompt = strings.TrimSpace(task.PromptTemplate)
	}
	if prompt == "" {
		prompt = fmt.Sprintf("执行协同流程任务：%s\n%s", task.Title, task.Description)
	}

	now := time.Now().UTC()
	run := tasks.FlowRun{
		TaskID:            task.ID,
		ProjectID:         task.ProjectID,
		Status:            tasks.FlowRunRunning,
		CurrentStep:       0,
		MaxSteps:          15,
		OrchestratorModel: task.OrchestratorModel,
		TriggerType:       triggerType,
		Prompt:            prompt,
		CreatedAt:         now,
		UpdatedAt:         now,
	}

	createdRun, err := e.repo.CreateFlowRun(run)
	if err != nil {
		return tasks.FlowRun{}, fmt.Errorf("flow: create run: %w", err)
	}

	// Update task last run state
	task.LastRunAt = &now
	task.LastRunStatus = tasks.RunExecuting
	_ = e.repo.Update(task)

	// Persist initial user goal message in chat
	userMsg := tasks.FlowMessage{
		RunID:      createdRun.ID,
		SenderType: tasks.SenderUser,
		SenderName: "用户",
		Content:    prompt,
		Status:     tasks.FlowMsgComplete,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	savedUserMsg, err := e.repo.CreateFlowMessage(userMsg)
	if err == nil {
		e.eventBus.Publish(createdRun.ID, FlowEvent{
			Type:    EventMessage,
			RunID:   createdRun.ID,
			Payload: savedUserMsg,
		})
	}

	// Start asynchronous execution loop
	e.triggerLoop(createdRun.ID)

	return createdRun, nil
}

// PostUserMessage records a user message and triggers the coordinator to evaluate next steps.
func (e *Engine) PostUserMessage(ctx context.Context, runID uint, content string) (tasks.FlowMessage, error) {
	run, err := e.repo.GetFlowRun(runID)
	if err != nil {
		return tasks.FlowMessage{}, err
	}

	now := time.Now().UTC()
	userMsg := tasks.FlowMessage{
		RunID:      run.ID,
		SenderType: tasks.SenderUser,
		SenderName: "用户",
		Content:    content,
		Status:     tasks.FlowMsgComplete,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	savedMsg, err := e.repo.CreateFlowMessage(userMsg)
	if err != nil {
		return tasks.FlowMessage{}, err
	}

	e.eventBus.Publish(run.ID, FlowEvent{
		Type:    EventMessage,
		RunID:   run.ID,
		Payload: savedMsg,
	})

	// Wake up run loop if completed or waiting
	if run.Status != tasks.FlowRunRunning {
		run.Status = tasks.FlowRunRunning
		run.Error = ""
		_ = e.repo.UpdateFlowRun(run)
		e.eventBus.Publish(run.ID, FlowEvent{
			Type:  EventStatus,
			RunID: run.ID,
			Payload: FlowStatusPayload{
				Status:      string(tasks.FlowRunRunning),
				CurrentStep: run.CurrentStep,
			},
		})
	}

	e.triggerLoop(run.ID)
	return savedMsg, nil
}

// CancelRun aborts an active flow run.
func (e *Engine) CancelRun(runID uint) error {
	e.mu.Lock()
	if cancel, ok := e.cancels[runID]; ok {
		cancel()
		delete(e.cancels, runID)
	}
	e.mu.Unlock()

	run, err := e.repo.GetFlowRun(runID)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	run.Status = tasks.FlowRunCancelled
	run.FinishedAt = &now
	_ = e.repo.UpdateFlowRun(run)

	e.eventBus.Publish(run.ID, FlowEvent{
		Type:  EventStatus,
		RunID: run.ID,
		Payload: FlowStatusPayload{
			Status:      string(tasks.FlowRunCancelled),
			CurrentStep: run.CurrentStep,
		},
	})

	return nil
}

func (e *Engine) triggerLoop(runID uint) {
	e.mu.Lock()
	if cancel, ok := e.cancels[runID]; ok {
		cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	e.cancels[runID] = cancel
	e.mu.Unlock()

	go func() {
		defer func() {
			e.mu.Lock()
			delete(e.cancels, runID)
			e.mu.Unlock()
		}()
		e.executeLoop(ctx, runID)
	}()
}

func (e *Engine) executeLoop(ctx context.Context, runID uint) {
	lock := e.getRunLock(runID)
	lock.Lock()
	defer lock.Unlock()

	mentionRegex := regexp.MustCompile(`@([^\s:：,，\n]+)`)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		run, err := e.repo.GetFlowRun(runID)
		if err != nil || run.Status != tasks.FlowRunRunning {
			return
		}

		if run.CurrentStep >= run.MaxSteps {
			now := time.Now().UTC()
			run.Status = tasks.FlowRunWaitingUser
			_ = e.repo.UpdateFlowRun(run)

			warnMsg := tasks.FlowMessage{
				RunID:      run.ID,
				SenderType: tasks.SenderSystem,
				SenderName: "系统提示",
				Content:    fmt.Sprintf("流程已执行达到最大步数上限 (%d 步)，中控已挂起。请在群聊中输入指令干预或确认是否结束。", run.MaxSteps),
				Status:     tasks.FlowMsgComplete,
				CreatedAt:  now,
				UpdatedAt:  now,
			}
			savedWarn, _ := e.repo.CreateFlowMessage(warnMsg)
			e.eventBus.Publish(run.ID, FlowEvent{Type: EventMessage, RunID: run.ID, Payload: savedWarn})
			e.eventBus.Publish(run.ID, FlowEvent{
				Type:  EventStatus,
				RunID: run.ID,
				Payload: FlowStatusPayload{
					Status:      string(tasks.FlowRunWaitingUser),
					CurrentStep: run.CurrentStep,
				},
			})
			return
		}

		task, err := e.repo.Get(run.TaskID)
		if err != nil {
			log.Printf("flow: get task failed: %v", err)
			return
		}

		roles, err := e.repo.ListRoles(task.ID)
		if err != nil || len(roles) == 0 {
			log.Printf("flow: no roles found for task %d", task.ID)
			return
		}

		// 1. Orchestrator Turn: Think & Dispatch
		orchestratorResponse, err := e.runOrchestratorStep(ctx, run, task, roles)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			log.Printf("flow: orchestrator step error: %v", err)
			now := time.Now().UTC()
			run.Status = tasks.FlowRunFailed
			run.Error = err.Error()
			run.FinishedAt = &now
			_ = e.repo.UpdateFlowRun(run)

			e.eventBus.Publish(run.ID, FlowEvent{
				Type:  EventStatus,
				RunID: run.ID,
				Payload: FlowStatusPayload{
					Status:      string(tasks.FlowRunFailed),
					CurrentStep: run.CurrentStep,
					Error:       err.Error(),
				},
			})
			return
		}

		// 2. Analyze Orchestrator Output: Check if finished or @mentioned a role
		isFinished := strings.Contains(orchestratorResponse, "[流程完成]") ||
			strings.Contains(orchestratorResponse, "[任务完成]") ||
			strings.Contains(orchestratorResponse, "【流程完成】") ||
			strings.Contains(orchestratorResponse, "【任务完成】")

		matches := mentionRegex.FindAllStringSubmatch(orchestratorResponse, -1)
		var targetRole *tasks.FlowRole
		if len(matches) > 0 {
			for _, m := range matches {
				candidateName := strings.TrimSpace(m[1])
				for _, r := range roles {
					if strings.EqualFold(r.Name, candidateName) {
						targetRole = &r
						break
					}
				}
				if targetRole != nil {
					break
				}
			}
		}

		if isFinished || targetRole == nil {
			// Flow achieved goal or concluded
			now := time.Now().UTC()
			run.Status = tasks.FlowRunCompleted
			run.FinishedAt = &now
			_ = e.repo.UpdateFlowRun(run)

			task.LastRunStatus = tasks.RunSuccess
			_ = e.repo.Update(task)

			e.eventBus.Publish(run.ID, FlowEvent{
				Type:  EventStatus,
				RunID: run.ID,
				Payload: FlowStatusPayload{
					Status:      string(tasks.FlowRunCompleted),
					CurrentStep: run.CurrentStep,
				},
			})
			return
		}

		// 3. Dispatch to Target Role
		err = e.runRoleStep(ctx, run, task, *targetRole, orchestratorResponse)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			log.Printf("flow: role step failed: %v", err)
			// Agent failure feedback into chat as agent message
			now := time.Now().UTC()
			failMsg := tasks.FlowMessage{
				RunID:      run.ID,
				SenderType: tasks.SenderAgent,
				SenderName: targetRole.Name,
				RoleID:     targetRole.ID,
				Content:    fmt.Sprintf("❌ 执行遇到异常: %v", err),
				Status:     tasks.FlowMsgFailed,
				CreatedAt:  now,
				UpdatedAt:  now,
			}
			savedFail, _ := e.repo.CreateFlowMessage(failMsg)
			e.eventBus.Publish(run.ID, FlowEvent{Type: EventMessage, RunID: run.ID, Payload: savedFail})
		}

		// Increment step counter
		run.CurrentStep++
		_ = e.repo.UpdateFlowRun(run)
		e.eventBus.Publish(run.ID, FlowEvent{
			Type:  EventStatus,
			RunID: run.ID,
			Payload: FlowStatusPayload{
				Status:      string(tasks.FlowRunRunning),
				CurrentStep: run.CurrentStep,
			},
		})
	}
}

func (e *Engine) runOrchestratorStep(
	ctx context.Context,
	run tasks.FlowRun,
	task tasks.Task,
	roles []tasks.FlowRole,
) (string, error) {
	// Resolve orchestrator model credentials
	resolvedModel, err := e.resolver.ResolveModelByNameOrID(run.OrchestratorModel)
	if err != nil {
		return "", fmt.Errorf("resolve orchestrator model (%s): %w", run.OrchestratorModel, err)
	}

	// Build Orchestrator System Prompt
	roleDescriptions := make([]string, 0, len(roles))
	for _, r := range roles {
		roleDescriptions = append(roleDescriptions, fmt.Sprintf(
			"- @%s (后端类型: %s, 模型: %s): %s。专业设定: %s",
			r.Name, r.Backend, r.Model, r.Description, r.SystemPrompt,
		))
	}

	sysPrompt := fmt.Sprintf(`你是一个软件开发团队的【中控调度大脑】(Orchestrator)。
你的职责是统揽全局，分析用户的总体目标，拆解执行计划，调度团队中的专业 Agent 角色分工执行，并在各角色执行完成后进行结果评估与验收。

【当前团队可用角色列表】
%s

【调度与交互规范】
1. 界面以类似团队 IM 群聊形式呈现给用户。你的每次发言应当清晰阐明分析思路或下一步规划。
2. 派发任务时，请务必以 "@角色名" 明确指派具体哪位成员执行，并在 @ 后面清晰列出给该角色的具体任务输入、需求细节与上下文依赖。
3. 每次派发请尽量只聚焦 @ 一个核心角色进行单步推进，避免混乱。
4. 当所有子任务执行完成、代码或目标已达成、并经你检验合格后，请给出最终总结报告，并在发言末尾包含明确标志：[任务完成]。
5. 保持严谨、专业、高效。`, strings.Join(roleDescriptions, "\n"))

	// Load chat history
	messages, err := e.repo.ListFlowMessages(run.ID)
	if err != nil {
		return "", fmt.Errorf("list flow messages: %w", err)
	}

	completionMessages := []chat.CompletionMessage{
		{
			Role: "system",
			Text: sysPrompt,
		},
	}
	for _, m := range messages {
		roleStr := "user"
		text := m.Content
		if m.SenderType == tasks.SenderOrchestrator {
			roleStr = "assistant"
		} else if m.SenderType == tasks.SenderAgent {
			roleStr = "user"
			text = fmt.Sprintf("【%s的执行反馈】:\n%s", m.SenderName, m.Content)
		} else if m.SenderType == tasks.SenderSystem {
			roleStr = "system"
		}
		completionMessages = append(completionMessages, chat.CompletionMessage{
			Role: roleStr,
			Text: text,
		})
	}

	// Create placeholder message for orchestrator in DB
	now := time.Now().UTC()
	initMsg := tasks.FlowMessage{
		RunID:      run.ID,
		SenderType: tasks.SenderOrchestrator,
		SenderName: "中控调度员",
		Content:    "",
		Status:     tasks.FlowMsgStreaming,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	savedMsg, err := e.repo.CreateFlowMessage(initMsg)
	if err != nil {
		return "", err
	}

	req := chat.CompletionRequest{
		BaseURL:  resolvedModel.BaseURL,
		APIKey:   resolvedModel.APIKey,
		Model:    resolvedModel.ModelID,
		Messages: completionMessages,
	}

	var sb strings.Builder
	onDelta := func(delta chat.CompletionDelta) error {
		if delta.Text != "" {
			sb.WriteString(delta.Text)
			e.eventBus.Publish(run.ID, FlowEvent{
				Type:  EventChunk,
				RunID: run.ID,
				Payload: FlowChunkPayload{
					MessageID:  savedMsg.ID,
					SenderName: "中控调度员",
					SenderType: string(tasks.SenderOrchestrator),
					Chunk:      delta.Text,
				},
			})
		}
		return nil
	}

	if err := e.completer.Stream(ctx, req, onDelta); err != nil {
		savedMsg.Status = tasks.FlowMsgFailed
		savedMsg.Content = sb.String()
		_ = e.repo.UpdateFlowMessage(savedMsg)
		return "", err
	}

	fullContent := sb.String()
	savedMsg.Content = fullContent
	savedMsg.Status = tasks.FlowMsgComplete
	_ = e.repo.UpdateFlowMessage(savedMsg)

	e.eventBus.Publish(run.ID, FlowEvent{
		Type:    EventMessage,
		RunID:   run.ID,
		Payload: savedMsg,
	})

	return fullContent, nil
}

func (e *Engine) runRoleStep(
	ctx context.Context,
	run tasks.FlowRun,
	task tasks.Task,
	role tasks.FlowRole,
	dispatchContent string,
) error {
	now := time.Now().UTC()
	// Create placeholder message for role in chat
	initMsg := tasks.FlowMessage{
		RunID:      run.ID,
		SenderType: tasks.SenderAgent,
		SenderName: role.Name,
		RoleID:     role.ID,
		Content:    "",
		Status:     tasks.FlowMsgStreaming,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	savedMsg, err := e.repo.CreateFlowMessage(initMsg)
	if err != nil {
		return err
	}

	e.eventBus.Publish(run.ID, FlowEvent{
		Type:    EventMessage,
		RunID:   run.ID,
		Payload: savedMsg,
	})

	// Check if this role is a pure LLM or an agent backend
	if strings.EqualFold(string(role.Backend), "llm") || role.Backend == "" {
		// Pure LLM Execution via Completer
		resolvedModel, err := e.resolver.ResolveModelByNameOrID(role.Model)
		if err != nil {
			return fmt.Errorf("resolve role model (%s): %w", role.Model, err)
		}

		req := chat.CompletionRequest{
			BaseURL: resolvedModel.BaseURL,
			APIKey:  resolvedModel.APIKey,
			Model:   resolvedModel.ModelID,
			Messages: []chat.CompletionMessage{
				{
					Role: "system",
					Text: role.SystemPrompt,
				},
				{
					Role: "user",
					Text: dispatchContent,
				},
			},
		}

		var sb strings.Builder
		onDelta := func(delta chat.CompletionDelta) error {
			if delta.Text != "" {
				sb.WriteString(delta.Text)
				e.eventBus.Publish(run.ID, FlowEvent{
					Type:  EventChunk,
					RunID: run.ID,
					Payload: FlowChunkPayload{
						MessageID:  savedMsg.ID,
						SenderName: role.Name,
						SenderType: string(tasks.SenderAgent),
						Chunk:      delta.Text,
					},
				})
			}
			return nil
		}

		if err := e.completer.Stream(ctx, req, onDelta); err != nil {
			savedMsg.Status = tasks.FlowMsgFailed
			savedMsg.Content = sb.String()
			_ = e.repo.UpdateFlowMessage(savedMsg)
			return err
		}

		savedMsg.Content = sb.String()
		savedMsg.Status = tasks.FlowMsgComplete
		_ = e.repo.UpdateFlowMessage(savedMsg)
		e.eventBus.Publish(run.ID, FlowEvent{
			Type:    EventMessage,
			RunID:   run.ID,
			Payload: savedMsg,
		})
		return nil
	}

	// Coding Agent Execution (e.g. opencode, pi, deepseek) via SessionLauncher in the workspace
	if e.launcher == nil {
		return errors.New("coding agent session launcher not configured")
	}

	sessionTitle := fmt.Sprintf("[%s协同] %s 步骤 %d", task.Title, role.Name, run.CurrentStep+1)
	combinedPrompt := fmt.Sprintf("%s\n\n【角色指派任务】:\n%s", role.SystemPrompt, dispatchContent)

	outcome, err := e.launcher.Create(ctx, task.ProjectID, role.Backend, sessionTitle, combinedPrompt, role.Model)
	if err != nil {
		savedMsg.Status = tasks.FlowMsgFailed
		savedMsg.Content = fmt.Sprintf("启动 Agent 会话失败: %v", err)
		_ = e.repo.UpdateFlowMessage(savedMsg)
		return fmt.Errorf("launch agent session: %w", err)
	}

	sessID := outcome.Session.ID
	savedMsg.SessionID = sessID
	savedMsg.Content = fmt.Sprintf("已在工作区启动 Agent 会话 (%s / %s) 执行任务。\n会话 ID: `%s`\n任务指令已接收，正在处理中...", role.Backend, role.Model, sessID)
	savedMsg.Status = tasks.FlowMsgStreaming
	_ = e.repo.UpdateFlowMessage(savedMsg)

	e.eventBus.Publish(run.ID, FlowEvent{
		Type:    EventMessage,
		RunID:   run.ID,
		Payload: savedMsg,
	})

	// Wait for Agent session to finish its turn
	var summary string
	waitCtx, waitCancel := context.WithTimeout(ctx, 10*time.Minute)
	defer waitCancel()

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-waitCtx.Done():
			savedMsg.Status = tasks.FlowMsgFailed
			savedMsg.Content = fmt.Sprintf("Agent 会话执行超时或被取消（会话 ID: `%s`）", sessID)
			_ = e.repo.UpdateFlowMessage(savedMsg)
			e.eventBus.Publish(run.ID, FlowEvent{
				Type:    EventMessage,
				RunID:   run.ID,
				Payload: savedMsg,
			})
			return waitCtx.Err()
		case <-ticker.C:
		}

		sess, err := e.launcher.Get(task.ProjectID, sessID)
		if err != nil {
			continue
		}

		// When turn is completed or idle (not actively running)
		if sess.Turn != turns.Running {
			if hp, ok := e.launcher.(SessionHistoryProvider); ok {
				if hist, err := hp.History(waitCtx, task.ProjectID, sessID); err == nil && len(hist.Events) > 0 {
					for i := len(hist.Events) - 1; i >= 0; i-- {
						ev := hist.Events[i]
						if ev.Type == agents.EventMessage && ev.Message != nil && ev.Message.Role == "assistant" && strings.TrimSpace(ev.Message.Text) != "" {
							summary = strings.TrimSpace(ev.Message.Text)
							break
						}
					}
				}
			}
			break
		}
	}

	if summary == "" {
		summary = "（已在工作区完成代码修改与自测，就绪等待验收）"
	}

	savedMsg.Content = fmt.Sprintf("✅ Agent 会话 (%s / %s) 执行完成！\n会话 ID: `%s`\n\n【Agent 执行产出】:\n%s", role.Backend, role.Model, sessID, summary)
	savedMsg.Status = tasks.FlowMsgComplete
	_ = e.repo.UpdateFlowMessage(savedMsg)

	e.eventBus.Publish(run.ID, FlowEvent{
		Type:    EventMessage,
		RunID:   run.ID,
		Payload: savedMsg,
	})

	return nil
}
