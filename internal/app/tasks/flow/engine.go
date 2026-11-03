package flow

import (
	"context"
	"encoding/json"
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
	Create(ctx context.Context, projectID uint, backend agents.BackendID, title, message, model string, images ...[]agents.ImageAttachment) (sessions.CreateOutcome, error)
	Send(ctx context.Context, projectID uint, sessionID, text, model string, images ...[]agents.ImageAttachment) (sessions.Session, error)
	Get(projectID uint, sessionID string) (sessions.Session, error)
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

// Engine coordinates multi-agent collaborative workflows driven by the orchestrator brain.
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
		MaxSteps:          20,
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

// executeLoop runs the ReAct orchestrator loop driven by Function Calling tools.
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
				Content:    fmt.Sprintf("流程已执行达到最大步数上限 (%d 步)，中枢大脑已挂起。请在群聊中输入指令干预或确认是否结束。", run.MaxSteps),
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

		// 1. Orchestrator Step: Think & Call Tools
		stepOutcome, err := e.runOrchestratorStep(ctx, run, task, roles)
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

		// 2. Handle Step Outcome
		if stepOutcome.Finished {
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

		if stepOutcome.WaitingUser {
			now := time.Now().UTC()
			run.Status = tasks.FlowRunWaitingUser
			run.UpdatedAt = now
			_ = e.repo.UpdateFlowRun(run)

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

		// 3. Fallback text parsing if no tool was invoked (backward compatibility)
		if !stepOutcome.ToolInvoked {
			isFinished := strings.Contains(stepOutcome.Content, "[流程完成]") ||
				strings.Contains(stepOutcome.Content, "[任务完成]") ||
				strings.Contains(stepOutcome.Content, "【流程完成】") ||
				strings.Contains(stepOutcome.Content, "【任务完成】")

			matches := mentionRegex.FindAllStringSubmatch(stepOutcome.Content, -1)
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

			// Dispatch text to target role
			_ = e.executeRoleStep(ctx, run, task, *targetRole, stepOutcome.Content, "")
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

type orchestratorStepOutcome struct {
	Content     string
	ToolInvoked bool
	Finished    bool
	WaitingUser bool
}

func (e *Engine) runOrchestratorStep(
	ctx context.Context,
	run tasks.FlowRun,
	task tasks.Task,
	roles []tasks.FlowRole,
) (orchestratorStepOutcome, error) {
	resolvedModel, err := e.resolver.ResolveModelByNameOrID(run.OrchestratorModel)
	if err != nil {
		return orchestratorStepOutcome{}, fmt.Errorf("resolve orchestrator model (%s): %w", run.OrchestratorModel, err)
	}

	// Load existing messages to discover active role sessions
	messages, err := e.repo.ListFlowMessages(run.ID)
	if err != nil {
		return orchestratorStepOutcome{}, fmt.Errorf("list flow messages: %w", err)
	}

	roleSessions := make(map[string]string) // roleName -> latest sessionID
	for i := len(messages) - 1; i >= 0; i-- {
		m := messages[i]
		if m.SenderType == tasks.SenderAgent && m.SessionID != "" && roleSessions[m.SenderName] == "" {
			roleSessions[m.SenderName] = m.SessionID
		}
	}

	// Build Orchestrator System Prompt
	roleDescriptions := make([]string, 0, len(roles))
	for _, r := range roles {
		extra := ""
		if sessID := roleSessions[r.Name]; sessID != "" {
			extra = fmt.Sprintf(", 当前已有会话 ID: `%s`（可继续追问对话）", sessID)
		}
		roleDescriptions = append(roleDescriptions, fmt.Sprintf(
			"- @%s (后端类型: %s, 模型: %s%s): %s。专业设定: %s",
			r.Name, r.Backend, r.Model, extra, r.Description, r.SystemPrompt,
		))
	}

	sysPrompt := fmt.Sprintf(`你是一个软件开发团队的【中控调度大脑】(Orchestrator)。
你的职责是统揽全局，分析用户的总体目标，拆解执行计划，通过内置的流程工具调度团队中的专业 Agent 角色分工执行，并在各角色执行完成后进行结果评估与验收。

【当前团队可用角色列表】
%s

【中枢大脑工作规范】
1. 界面以类似团队 IM 群聊形式呈现给用户。你的每次发言应当清晰阐明分析思路或下一步规划。
2. 调度角色时，请使用工具 flow_dispatch_role 明确指派具体哪位角色执行（传入 role_name 与 instruction）。
   - 如果该角色已有会话，你可以传入 session_id 继续向该会话发送消息（多轮连续对话）。
   - flow_dispatch_role 工具会自动监听该角色的执行流并在完成后返回其产出摘要。
3. 如果需要用户人工决策、澄清疑问或进行敏感确认，请调用 flow_ask_user 工具。
4. 当所有子任务执行完成、代码或目标已达成、并经你检验合格后，请调用 flow_complete_task 提交最终成果总结。
5. 保持严谨、专业、高效。`, strings.Join(roleDescriptions, "\n"))

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
		return orchestratorStepOutcome{}, err
	}

	req := chat.CompletionRequest{
		BaseURL:  resolvedModel.BaseURL,
		APIKey:   resolvedModel.APIKey,
		Model:    resolvedModel.ModelID,
		Messages: completionMessages,
		Tools:    FlowTools(),
	}

	var sb strings.Builder
	var receivedToolCalls []chat.ToolCall

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
		if len(delta.ToolCalls) > 0 {
			receivedToolCalls = append(receivedToolCalls, delta.ToolCalls...)
		}
		return nil
	}

	if err := e.completer.Stream(ctx, req, onDelta); err != nil {
		savedMsg.Status = tasks.FlowMsgFailed
		savedMsg.Content = sb.String()
		_ = e.repo.UpdateFlowMessage(savedMsg)
		return orchestratorStepOutcome{}, err
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

	outcome := orchestratorStepOutcome{
		Content: fullContent,
	}

	if len(receivedToolCalls) == 0 {
		return outcome, nil
	}

	outcome.ToolInvoked = true

	// Execute received tool calls
	for _, tc := range receivedToolCalls {
		switch tc.Name {
		case ToolCompleteTask:
			var args CompleteTaskArgs
			_ = json.Unmarshal([]byte(tc.Arguments), &args)
			outcome.Finished = true
			if strings.TrimSpace(args.Summary) != "" {
				now := time.Now().UTC()
				summaryMsg := tasks.FlowMessage{
					RunID:      run.ID,
					SenderType: tasks.SenderSystem,
					SenderName: "任务验收",
					Content:    fmt.Sprintf("🎉 任务顺利完成！\n\n【验收总结】:\n%s", args.Summary),
					Status:     tasks.FlowMsgComplete,
					CreatedAt:  now,
					UpdatedAt:  now,
				}
				savedSummary, _ := e.repo.CreateFlowMessage(summaryMsg)
				e.eventBus.Publish(run.ID, FlowEvent{Type: EventMessage, RunID: run.ID, Payload: savedSummary})
			}
			return outcome, nil

		case ToolAskUser:
			var args AskUserArgs
			_ = json.Unmarshal([]byte(tc.Arguments), &args)
			outcome.WaitingUser = true
			if strings.TrimSpace(args.Question) != "" {
				now := time.Now().UTC()
				questionMsg := tasks.FlowMessage{
					RunID:      run.ID,
					SenderType: tasks.SenderOrchestrator,
					SenderName: "中控调度员",
					Content:    fmt.Sprintf("❓ 需要用户确认:\n%s", args.Question),
					Status:     tasks.FlowMsgComplete,
					CreatedAt:  now,
					UpdatedAt:  now,
				}
				savedQ, _ := e.repo.CreateFlowMessage(questionMsg)
				e.eventBus.Publish(run.ID, FlowEvent{Type: EventMessage, RunID: run.ID, Payload: savedQ})
			}
			return outcome, nil

		case ToolDispatchRole:
			var args DispatchRoleArgs
			_ = json.Unmarshal([]byte(tc.Arguments), &args)
			var targetRole *tasks.FlowRole
			for _, r := range roles {
				if strings.EqualFold(r.Name, args.RoleName) {
					targetRole = &r
					break
				}
			}
			if targetRole == nil {
				// Role not found, record system feedback
				now := time.Now().UTC()
				errFeedback := tasks.FlowMessage{
					RunID:      run.ID,
					SenderType: tasks.SenderSystem,
					SenderName: "系统错误",
					Content:    fmt.Sprintf("调度失败: 未找到名为 %q 的角色", args.RoleName),
					Status:     tasks.FlowMsgFailed,
					CreatedAt:  now,
					UpdatedAt:  now,
				}
				savedErr, _ := e.repo.CreateFlowMessage(errFeedback)
				e.eventBus.Publish(run.ID, FlowEvent{Type: EventMessage, RunID: run.ID, Payload: savedErr})
				continue
			}

			if err := e.executeRoleStep(ctx, run, task, *targetRole, args.Instruction, args.SessionID); err != nil {
				log.Printf("flow: execute role %s failed: %v", targetRole.Name, err)
			}

		case ToolWaitSession:
			var args WaitSessionArgs
			_ = json.Unmarshal([]byte(tc.Arguments), &args)
			if args.SessionID != "" && e.launcher != nil {
				timeout := time.Duration(args.TimeoutSeconds) * time.Second
				if timeout <= 0 {
					timeout = 5 * time.Minute
				}
				telemetry := NewTelemetryBus(e.launcher, e.eventBus, run.ID)
				_, _ = telemetry.CollectSessionTurn(ctx, task.ProjectID, args.SessionID, "Agent", 0, timeout)
				telemetry.Close()
			}
		}
	}

	return outcome, nil
}

func (e *Engine) executeRoleStep(
	ctx context.Context,
	run tasks.FlowRun,
	task tasks.Task,
	role tasks.FlowRole,
	instruction string,
	existingSessionID string,
) error {
	now := time.Now().UTC()
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

	// Check if this role is a pure LLM or a Coding Agent backend
	if strings.EqualFold(string(role.Backend), "llm") || role.Backend == "" {
		resolvedModel, err := e.resolver.ResolveModelByNameOrID(role.Model)
		if err != nil {
			savedMsg.Status = tasks.FlowMsgFailed
			savedMsg.Content = fmt.Sprintf("解析模型失败: %v", err)
			_ = e.repo.UpdateFlowMessage(savedMsg)
			return err
		}

		req := chat.CompletionRequest{
			BaseURL: resolvedModel.BaseURL,
			APIKey:  resolvedModel.APIKey,
			Model:   resolvedModel.ModelID,
			Messages: []chat.CompletionMessage{
				{Role: "system", Text: role.SystemPrompt},
				{Role: "user", Text: instruction},
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

	// Coding Agent execution via telemetry bus
	if e.launcher == nil {
		savedMsg.Status = tasks.FlowMsgFailed
		savedMsg.Content = "未配置 Agent 会话服务"
		_ = e.repo.UpdateFlowMessage(savedMsg)
		return errors.New("coding agent session launcher not configured")
	}

	var sessID string
	if existingSessionID != "" {
		// Reuse existing session for multi-turn dialogue
		sessID = existingSessionID
		savedMsg.SessionID = sessID
		savedMsg.Content = fmt.Sprintf("正在复用已有 Agent 会话 (`%s`) 进行交互...", sessID)
		_ = e.repo.UpdateFlowMessage(savedMsg)

		_, err := e.launcher.Send(ctx, task.ProjectID, sessID, instruction, role.Model)
		if err != nil {
			savedMsg.Status = tasks.FlowMsgFailed
			savedMsg.Content = fmt.Sprintf("向会话 `%s` 发送指令失败: %v", sessID, err)
			_ = e.repo.UpdateFlowMessage(savedMsg)
			return err
		}
	} else {
		// Launch new native session
		sessionTitle := fmt.Sprintf("[%s协同] %s 步骤 %d", task.Title, role.Name, run.CurrentStep+1)
		combinedPrompt := fmt.Sprintf("%s\n\n【角色指派任务】:\n%s", role.SystemPrompt, instruction)

		outcome, err := e.launcher.Create(ctx, task.ProjectID, role.Backend, sessionTitle, combinedPrompt, role.Model)
		if err != nil {
			savedMsg.Status = tasks.FlowMsgFailed
			savedMsg.Content = fmt.Sprintf("启动 Agent 会话失败: %v", err)
			_ = e.repo.UpdateFlowMessage(savedMsg)
			return fmt.Errorf("launch agent session: %w", err)
		}
		sessID = outcome.Session.ID
		savedMsg.SessionID = sessID
		savedMsg.Content = fmt.Sprintf("已在工作区启动 Agent 会话 (%s / %s)。\n会话 ID: `%s`\n正在监听执行数据流...", role.Backend, role.Model, sessID)
		_ = e.repo.UpdateFlowMessage(savedMsg)
	}

	e.eventBus.Publish(run.ID, FlowEvent{
		Type:    EventMessage,
		RunID:   run.ID,
		Payload: savedMsg,
	})

	// Collect session turn stream and aggregate final outcome
	telemetry := NewTelemetryBus(e.launcher, e.eventBus, run.ID)
	defer telemetry.Close()

	turnOutcome, err := telemetry.CollectSessionTurn(ctx, task.ProjectID, sessID, role.Name, savedMsg.ID, 10*time.Minute)
	if err != nil {
		savedMsg.Status = tasks.FlowMsgFailed
		savedMsg.Content = fmt.Sprintf("❌ Agent 执行异常（会话 ID: `%s`）: %v", sessID, err)
		_ = e.repo.UpdateFlowMessage(savedMsg)
		e.eventBus.Publish(run.ID, FlowEvent{Type: EventMessage, RunID: run.ID, Payload: savedMsg})
		return err
	}

	summary := turnOutcome.Summary
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
