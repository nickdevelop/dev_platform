package flow

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"devplatform/internal/app/chat"
	"devplatform/internal/app/modelconfig"
	"devplatform/internal/app/sessions"
	"devplatform/internal/app/sessions/turns"
	"devplatform/internal/app/tasks"
	"devplatform/internal/core/agents"
)

type fakeModelResolver struct {
	model modelconfig.ResolvedModel
	err   error
}

func (f *fakeModelResolver) ResolveModelByNameOrID(ref string) (modelconfig.ResolvedModel, error) {
	if f.err != nil {
		return modelconfig.ResolvedModel{}, f.err
	}
	return f.model, nil
}

type fakeCompleter struct {
	mu        sync.Mutex
	responses []string
	callCount int
}

func (f *fakeCompleter) Stream(ctx context.Context, req chat.CompletionRequest, onDelta func(chat.CompletionDelta) error) error {
	f.mu.Lock()
	idx := f.callCount
	f.callCount++
	var resp string
	if idx < len(f.responses) {
		resp = f.responses[idx]
	} else {
		resp = "默认完成答复"
	}
	f.mu.Unlock()

	// Stream chunks
	words := strings.Split(resp, " ")
	for _, w := range words {
		if err := onDelta(chat.CompletionDelta{Text: w + " "}); err != nil {
			return err
		}
	}
	return nil
}

func (f *fakeCompleter) Complete(ctx context.Context, req chat.CompletionRequest, maxTokens int) (string, *chat.Usage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return "ok", &chat.Usage{}, nil
}

type fakeTaskRepo struct {
	mu       sync.Mutex
	task     tasks.Task
	roles    []tasks.FlowRole
	runs     map[uint]tasks.FlowRun
	messages map[uint][]tasks.FlowMessage
	nextID   uint
}

func newFakeTaskRepo() *fakeTaskRepo {
	return &fakeTaskRepo{
		runs:     make(map[uint]tasks.FlowRun),
		messages: make(map[uint][]tasks.FlowMessage),
		nextID:   1,
	}
}

func (r *fakeTaskRepo) Get(id uint) (tasks.Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.task, nil
}

func (r *fakeTaskRepo) Update(t tasks.Task) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.task = t
	return nil
}

func (r *fakeTaskRepo) ListRoles(taskID uint) ([]tasks.FlowRole, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.roles, nil
}

func (r *fakeTaskRepo) GetRole(id uint) (tasks.FlowRole, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, role := range r.roles {
		if role.ID == id {
			return role, nil
		}
	}
	return tasks.FlowRole{}, tasks.ErrRoleNotFound
}

func (r *fakeTaskRepo) CreateFlowRun(run tasks.FlowRun) (tasks.FlowRun, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	run.ID = r.nextID
	r.nextID++
	r.runs[run.ID] = run
	return run, nil
}

func (r *fakeTaskRepo) GetFlowRun(id uint) (tasks.FlowRun, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	run, ok := r.runs[id]
	if !ok {
		return tasks.FlowRun{}, tasks.ErrFlowRunNotFound
	}
	return run, nil
}

func (r *fakeTaskRepo) UpdateFlowRun(run tasks.FlowRun) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.runs[run.ID] = run
	return nil
}

func (r *fakeTaskRepo) CreateFlowMessage(msg tasks.FlowMessage) (tasks.FlowMessage, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	msg.ID = r.nextID
	r.nextID++
	r.messages[msg.RunID] = append(r.messages[msg.RunID], msg)
	return msg, nil
}

func (r *fakeTaskRepo) UpdateFlowMessage(msg tasks.FlowMessage) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	list := r.messages[msg.RunID]
	for i, m := range list {
		if m.ID == msg.ID {
			list[i] = msg
			r.messages[msg.RunID] = list
			return nil
		}
	}
	return nil
}

func (r *fakeTaskRepo) ListFlowMessages(runID uint) ([]tasks.FlowMessage, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.messages[runID], nil
}

type fakeSessionLauncher struct {
	created []string
}

func (f *fakeSessionLauncher) Create(ctx context.Context, projectID uint, backend agents.BackendID, title, message, model string, images ...[]agents.ImageAttachment) (sessions.CreateOutcome, error) {
	sessID := fmt.Sprintf("native_sess_%d", len(f.created)+1)
	f.created = append(f.created, sessID)
	return sessions.CreateOutcome{
		Session: sessions.Session{
			ID:        sessID,
			ProjectID: projectID,
			Backend:   backend,
			Status:    sessions.SessionActive,
		},
	}, nil
}

func (f *fakeSessionLauncher) Send(ctx context.Context, projectID uint, sessionID, text, model string, images ...[]agents.ImageAttachment) (sessions.Session, error) {
	return sessions.Session{ID: sessionID}, nil
}

func (f *fakeSessionLauncher) Get(projectID uint, sessionID string) (sessions.Session, error) {
	return sessions.Session{ID: sessionID}, nil
}

func (f *fakeSessionLauncher) Delete(projectID uint, sessionID string) error {
	return nil
}

func TestFlowEngineTextOnlyTurnFailsRun(t *testing.T) {
	// The collaborative flow protocol is tool-only: a plain-language turn that
	// neither invokes a tool nor asks the user must fail the run. Legacy text
	// markers ("[任务完成]", "@角色") must not resurrect the old fallback.
	repo := newFakeTaskRepo()
	task := tasks.Task{
		ID:                10,
		Title:             "构建多Agent协作功能",
		TaskType:          tasks.TaskTypeFlow,
		ProjectID:         1,
		OrchestratorModel: "gpt-4o",
		PromptTemplate:    "实现协同中控与角色分工",
	}
	repo.task = task
	repo.roles = []tasks.FlowRole{
		{
			ID:           1,
			TaskID:       10,
			Name:         "架构师",
			Description:  "负责架构规划",
			Backend:      "llm",
			Model:        "gpt-4o",
			SystemPrompt: "你是首席架构师",
		},
	}

	resolver := &fakeModelResolver{
		model: modelconfig.ResolvedModel{
			ID:       1,
			ModelID:  "gpt-4o",
			BaseURL:  "https://api.openai.com/v1",
			APIKey:   "test-key",
		},
	}

	completer := &fakeCompleter{
		responses: []string{
			// Turn 1: the model only narrates and mentions the role in text —
			// no flow_dispatch_role tool call, no completion marker semantics.
			"收到目标，设计的方案已达成，[任务完成] @架构师 请输出核心模块设计方案",
		},
	}

	launcher := &fakeSessionLauncher{}
	engine := NewEngine(repo, launcher, resolver, completer, NewEventBus())

	run, err := engine.StartRun(context.Background(), task, "manual", "")
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}

	if run.Status != tasks.FlowRunRunning {
		t.Fatalf("expected run status running, got %s", run.Status)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		r, err := repo.GetFlowRun(run.ID)
		if err == nil && r.Status == tasks.FlowRunCompleted {
			t.Fatal("a text-only turn must never complete the run")
		}
		if err == nil && r.Status == tasks.FlowRunFailed {
			if !strings.Contains(r.Error, "no tool call") {
				t.Fatalf("expected no-tool-call error, got %q", r.Error)
			}

			updatedTask, err := repo.Get(task.ID)
			if err != nil {
				t.Fatalf("repo.Get: %v", err)
			}
			if updatedTask.LastRunStatus != tasks.RunFailed {
				t.Fatalf("expected task last run status failed, got %s", updatedTask.LastRunStatus)
			}

			// Text mentions must not dispatch roles: no agent session launched.
			if len(launcher.created) != 0 {
				t.Fatalf("expected no dispatched role sessions, got %v", launcher.created)
			}

			// The orchestrator's own message recorded the narration, so the
			// chat holds the user goal plus that single turn.
			msgs, err := repo.ListFlowMessages(run.ID)
			if err != nil {
				t.Fatalf("ListFlowMessages: %v", err)
			}
			if len(msgs) != 2 {
				t.Fatalf("expected 2 messages (user goal + orchestrator turn), got %d", len(msgs))
			}
			if msgs[1].SenderType != tasks.SenderOrchestrator {
				t.Errorf("msg[1] should be orchestrator, got %s", msgs[1].SenderType)
			}
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("timed out waiting for flow run to fail")
}

func TestFlowEngineCancel(t *testing.T) {
	repo := newFakeTaskRepo()
	task := tasks.Task{
		ID:                10,
		Title:             "长期运行流程",
		TaskType:          tasks.TaskTypeFlow,
		ProjectID:         1,
		OrchestratorModel: "gpt-4o",
	}
	repo.task = task
	repo.roles = []tasks.FlowRole{
		{ID: 1, TaskID: 10, Name: "角色A", Backend: "llm", Model: "gpt-4o"},
	}

	resolver := &fakeModelResolver{
		model: modelconfig.ResolvedModel{ModelID: "gpt-4o"},
	}
	completer := &fakeCompleter{}
	launcher := &fakeSessionLauncher{}
	engine := NewEngine(repo, launcher, resolver, completer, nil)

	run, err := engine.StartRun(context.Background(), task, "manual", "任务目标")
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}

	if err := engine.CancelRun(run.ID); err != nil {
		t.Fatalf("CancelRun: %v", err)
	}

	updated, err := repo.GetFlowRun(run.ID)
	if err != nil {
		t.Fatalf("GetFlowRun: %v", err)
	}
	if updated.Status != tasks.FlowRunCancelled {
		t.Fatalf("expected cancelled, got %s", updated.Status)
	}
}

type fakeDelayedSessionLauncher struct {
	mu        sync.Mutex
	callCount int
	events    []agents.NormalizedEvent
}

func (f *fakeDelayedSessionLauncher) Create(ctx context.Context, projectID uint, backend agents.BackendID, title, message, model string, images ...[]agents.ImageAttachment) (sessions.CreateOutcome, error) {
	return sessions.CreateOutcome{
		Session: sessions.Session{
			ID:        "delayed_sess_1",
			ProjectID: projectID,
			Backend:   backend,
			Status:    sessions.SessionActive,
		},
	}, nil
}

func (f *fakeDelayedSessionLauncher) Send(ctx context.Context, projectID uint, sessionID, text, model string, images ...[]agents.ImageAttachment) (sessions.Session, error) {
	return sessions.Session{ID: sessionID}, nil
}

func (f *fakeDelayedSessionLauncher) Get(projectID uint, sessionID string) (sessions.Session, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.callCount++

	turnState := turns.Idle
	if f.callCount == 2 {
		// Second call enters Running
		turnState = turns.Running
	} else if f.callCount >= 3 {
		// Third call settles back to Idle with finished history
		turnState = turns.Idle
		if len(f.events) == 0 {
			f.events = []agents.NormalizedEvent{
				{
					Type: agents.EventMessage,
					Message: &agents.MessagePayload{
						Role: "assistant",
						Text: "已完成所有前端模块重构并执行单元测试通过。",
					},
				},
			}
		}
	}

	return sessions.Session{
		ID:        sessionID,
		ProjectID: projectID,
		Backend:   agents.BackendDeepSeek,
		Status:    sessions.SessionActive,
		Turn:      turnState,
	}, nil
}

func (f *fakeDelayedSessionLauncher) Delete(projectID uint, sessionID string) error {
	return nil
}

func (f *fakeDelayedSessionLauncher) History(ctx context.Context, projectID uint, sessionID string) (sessions.History, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return sessions.History{
		Events: f.events,
	}, nil
}

func TestFlowEngineWaitsForDelayedAgentSession(t *testing.T) {
	repo := newFakeTaskRepo()
	task := tasks.Task{
		ID:                20,
		Title:             "延迟启动多Agent流程",
		TaskType:          tasks.TaskTypeFlow,
		ProjectID:         1,
		OrchestratorModel: "gpt-4o",
	}
	repo.task = task
	repo.roles = []tasks.FlowRole{
		{
			ID:           1,
			TaskID:       20,
			Name:         "DSH开发者",
			Backend:      agents.BackendDeepSeek,
			Model:        "deepseek-chat",
			SystemPrompt: "你是代码开发专家",
		},
	}

	resolver := &fakeModelResolver{
		model: modelconfig.ResolvedModel{ModelID: "gpt-4o"},
	}
	completer := &fakeToolCompleter{
		roleName:    "DSH开发者",
		instruction: "执行前端重构",
		summary:     "所有前端模块重构已完成并验收通过",
	}
	launcher := &fakeDelayedSessionLauncher{}
	engine := NewEngine(repo, launcher, resolver, completer, nil)

	run, err := engine.StartRun(context.Background(), task, "manual", "任务目标")
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}

	// Wait briefly for step execution to finish
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		r, _ := repo.GetFlowRun(run.ID)
		if r.Status == tasks.FlowRunCompleted || r.Status == tasks.FlowRunFailed {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	msgs, err := repo.ListFlowMessages(run.ID)
	if err != nil {
		t.Fatalf("ListFlowMessages: %v", err)
	}

	// Verify that the agent message contains the actual assistant summary
	var agentMsg *tasks.FlowMessage
	for _, m := range msgs {
		if m.SenderType == tasks.SenderAgent {
			agentMsg = &m
			break
		}
	}

	if agentMsg == nil {
		t.Fatal("expected agent message in flow chat")
	}

	if agentMsg.Status != tasks.FlowMsgComplete {
		t.Fatalf("expected agent message complete, got %s", agentMsg.Status)
	}

	if !strings.Contains(agentMsg.Content, "已完成所有前端模块重构并执行单元测试通过") {
		t.Fatalf("expected real assistant output in message content, got: %s", agentMsg.Content)
	}
}

type fakeToolCompleter struct {
	mu   sync.Mutex
	step int
	// roleName / instruction drive the flow_dispatch_role call issued on the
	// first step; summary drives the flow_complete_task call on the second.
	roleName    string
	instruction string
	summary     string
}

func (f *fakeToolCompleter) Stream(ctx context.Context, req chat.CompletionRequest, onDelta func(chat.CompletionDelta) error) error {
	f.mu.Lock()
	step := f.step
	f.step++
	f.mu.Unlock()

	if step == 0 {
		_ = onDelta(chat.CompletionDelta{Text: fmt.Sprintf("正在分析整体方案，指派%s进行执行。", f.roleName)})
		argsJSON := fmt.Sprintf(`{"role_name":%q,"instruction":%q}`, f.roleName, f.instruction)
		_ = onDelta(chat.CompletionDelta{
			ToolCalls: []chat.ToolCall{
				{
					ID:        "call_1",
					Type:      "function",
					Name:      ToolDispatchRole,
					Arguments: argsJSON,
				},
			},
		})
		return nil
	}

	if step == 1 {
		_ = onDelta(chat.CompletionDelta{Text: "角色产出质量合格，满足验收标准。"})
		argsJSON := fmt.Sprintf(`{"summary":%q}`, f.summary)
		_ = onDelta(chat.CompletionDelta{
			ToolCalls: []chat.ToolCall{
				{
					ID:        "call_2",
					Type:      "function",
					Name:      ToolCompleteTask,
					Arguments: argsJSON,
				},
			},
		})
		return nil
	}

	return nil
}

func (f *fakeToolCompleter) Complete(ctx context.Context, req chat.CompletionRequest, maxTokens int) (string, *chat.Usage, error) {
	return "ok", &chat.Usage{}, nil
}

func TestFlowEngineToolCallingOrchestration(t *testing.T) {
	repo := newFakeTaskRepo()
	task := tasks.Task{
		ID:                30,
		Title:             "基于内置大脑ToolCalling协同流程",
		TaskType:          tasks.TaskTypeFlow,
		ProjectID:         1,
		OrchestratorModel: "gpt-4o",
	}
	repo.task = task
	repo.roles = []tasks.FlowRole{
		{
			ID:           1,
			TaskID:       30,
			Name:         "开发工程师",
			Backend:      agents.BackendOpenCodeV2,
			Model:        "gpt-4o",
			SystemPrompt: "你是资深开发工程师",
		},
	}

	resolver := &fakeModelResolver{
		model: modelconfig.ResolvedModel{ModelID: "gpt-4o"},
	}
	completer := &fakeToolCompleter{
		roleName:    "开发工程师",
		instruction: "编写用户认证模块核心逻辑",
		summary:     "用户认证模块已全部开发完毕并验收通过",
	}
	launcher := &fakeDelayedSessionLauncher{}
	bus := NewEventBus()
	engine := NewEngine(repo, launcher, resolver, completer, bus)

	run, err := engine.StartRun(context.Background(), task, "manual", "实现认证模块")
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	completed := false
	for time.Now().Before(deadline) {
		r, err := repo.GetFlowRun(run.ID)
		if err == nil && r.Status == tasks.FlowRunCompleted {
			completed = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if !completed {
		t.Fatal("timed out waiting for flow run to complete via tool calling")
	}

	msgs, err := repo.ListFlowMessages(run.ID)
	if err != nil {
		t.Fatalf("ListFlowMessages: %v", err)
	}

	hasRoleFeedback := false
	hasCompletionSummary := false
	for _, m := range msgs {
		if m.SenderType == tasks.SenderAgent && m.SenderName == "开发工程师" {
			hasRoleFeedback = true
		}
		if m.SenderType == tasks.SenderSystem && strings.Contains(m.Content, "用户认证模块已全部开发完毕并验收通过") {
			hasCompletionSummary = true
		}
	}

	if !hasRoleFeedback {
		t.Error("expected role feedback message in flow history")
	}
	if !hasCompletionSummary {
		t.Error("expected completion summary message in flow history")
	}
}

