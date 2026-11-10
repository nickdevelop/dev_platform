package tasks

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"devplatform/internal/app/database"
	"devplatform/internal/app/events"
	"devplatform/internal/app/sessions"
	"devplatform/internal/core/agents"
)

type fakeSessionLauncher struct {
	createdSessions []sessions.Session
	sentMessages    []string
	deletedSessions []string
	sendErr         error
}

func (f *fakeSessionLauncher) Create(ctx context.Context, projectID uint, backend agents.BackendID, title, message, model string, images ...[]agents.ImageAttachment) (sessions.CreateOutcome, error) {
	s := sessions.Session{
		ID:        fmt.Sprintf("sess_%d", len(f.createdSessions)+1),
		ProjectID: projectID,
		Backend:   backend,
		Title:     title,
		Status:    sessions.SessionActive,
		Turn:      sessions.TurnRunning,
	}
	f.createdSessions = append(f.createdSessions, s)
	f.sentMessages = append(f.sentMessages, message)
	return sessions.CreateOutcome{Session: s}, nil
}

func (f *fakeSessionLauncher) Send(ctx context.Context, projectID uint, sessionID, text, model string, images ...[]agents.ImageAttachment) (sessions.Session, error) {
	if f.sendErr != nil {
		return sessions.Session{}, f.sendErr
	}
	f.sentMessages = append(f.sentMessages, text)
	return sessions.Session{ID: sessionID, ProjectID: projectID, Status: sessions.SessionActive}, nil
}

func (f *fakeSessionLauncher) Get(projectID uint, sessionID string) (sessions.Session, error) {
	for _, s := range f.createdSessions {
		if s.ID == sessionID && s.ProjectID == projectID {
			return s, nil
		}
	}
	return sessions.Session{}, sessions.ErrSessionNotFound
}

func (f *fakeSessionLauncher) Delete(projectID uint, sessionID string) error {
	f.deletedSessions = append(f.deletedSessions, sessionID)
	return nil
}

func (f *fakeSessionLauncher) setSessionState(sessionID string, status sessions.SessionStatus, turn sessions.TurnState) {
	for i, s := range f.createdSessions {
		if s.ID == sessionID {
			f.createdSessions[i].Status = status
			f.createdSessions[i].Turn = turn
			return
		}
	}
}

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&database.Task{}, &database.TaskRun{}, &database.FlowRole{}, &database.FlowRun{}, &database.FlowMessage{}); err != nil {
		t.Fatalf("migrate task & runs: %v", err)
	}
	return db
}

func TestStandardTaskLifecycleAndRuns(t *testing.T) {
	db := setupTestDB(t)
	launcher := &fakeSessionLauncher{}
	svc := NewService(NewRepository(db), launcher)
	ctx := context.Background()

	// 1. Create standard task (session template)
	task, err := svc.Create(ctx, CreateTaskRequest{
		Title:          "日常巡检任务",
		Description:    "定时或手动检查系统日志与指标",
		TaskType:       TaskTypeStandard,
		ProjectID:      1,
		Backend:        agents.BackendOpenCodeV2,
		Model:          "gpt-4o",
		PromptTemplate: "请检查系统当前的运行指标、错误日志并汇总报告。",
		ScheduleType:   ScheduleManual,
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if task.ID == 0 || task.TaskType != TaskTypeStandard {
		t.Fatalf("expected standard task, got %+v", task)
	}

	// 2. Trigger first manual run
	run1, err := svc.TriggerExecution(ctx, task.ID, "manual")
	if err != nil {
		t.Fatalf("trigger execution 1: %v", err)
	}
	if run1.Status != "running" || run1.SessionID != "sess_1" {
		t.Fatalf("unexpected run1: %+v", run1)
	}
	if len(launcher.createdSessions) != 1 {
		t.Fatalf("expected 1 session created, got %d", len(launcher.createdSessions))
	}
	if launcher.sentMessages[0] != "请检查系统当前的运行指标、错误日志并汇总报告。" {
		t.Fatalf("expected template prompt sent, got %s", launcher.sentMessages[0])
	}

	// 3. Status sync while session is running
	task, err = svc.Get(task.ID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if task.LastRunStatus != RunExecuting {
		t.Fatalf("expected task running, got %s", task.LastRunStatus)
	}

	// 4. Session completes successfully
	launcher.setSessionState("sess_1", sessions.SessionActive, sessions.TurnIdle)
	task, err = svc.Get(task.ID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if task.LastRunStatus != RunSuccess {
		t.Fatalf("expected task success after turn idle, got %s", task.LastRunStatus)
	}

	// 5. Trigger second run (independent new session!)
	run2, err := svc.TriggerExecution(ctx, task.ID, "manual")
	if err != nil {
		t.Fatalf("trigger execution 2: %v", err)
	}
	if run2.SessionID != "sess_2" {
		t.Fatalf("expected independent session sess_2, got %s", run2.SessionID)
	}

	// Check runs list
	runs, err := svc.ListRuns(ctx, task.ID)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("expected 2 runs, got %d", len(runs))
	}

	// 6. Delete task cleans up sessions
	if err := svc.Delete(task.ID); err != nil {
		t.Fatalf("delete task: %v", err)
	}
	if len(launcher.deletedSessions) != 2 {
		t.Fatalf("expected 2 deleted sessions in platform index, got %d", len(launcher.deletedSessions))
	}
}

func TestCollaborativeFlowTaskLifecycle(t *testing.T) {
	db := setupTestDB(t)
	launcher := &fakeSessionLauncher{}
	svc := NewService(NewRepository(db), launcher)
	ctx := context.Background()

	// 1. Create a Collaborative Flow Task
	task, err := svc.Create(ctx, CreateTaskRequest{
		Title:             "多Agent协同任务原型",
		Description:       "架构设计与代码实现闭环",
		TaskType:          TaskTypeFlow,
		ProjectID:         1,
		Model:             "gpt-4o",
		OrchestratorModel: "gpt-4o",
		PromptTemplate:    "实现一个多Agent流程协同原型",
	})
	if err != nil {
		t.Fatalf("create flow task: %v", err)
	}
	if task.TaskType != TaskTypeFlow {
		t.Fatalf("expected task type flow, got %s", task.TaskType)
	}

	// 2. Add Roles
	role1, err := svc.CreateRole(FlowRole{
		TaskID:       task.ID,
		Name:         "架构师",
		Description:  "负责架构规划与模块拆解",
		Backend:      "llm",
		Model:        "gpt-4o",
		SystemPrompt: "你是一名资深架构师...",
	})
	if err != nil {
		t.Fatalf("create role 1: %v", err)
	}
	if role1.ID == 0 {
		t.Fatalf("expected non-zero role ID")
	}

	role2, err := svc.CreateRole(FlowRole{
		TaskID:       task.ID,
		Name:         "编码专家",
		Description:  "负责代码编写与实现",
		Backend:      agents.BackendOpenCodeV2,
		Model:        "gpt-4o",
		SystemPrompt: "你是一名资深开发工程师...",
	})
	if err != nil {
		t.Fatalf("create role 2: %v", err)
	}

	// 3. List Roles
	roles, err := svc.ListRoles(task.ID)
	if err != nil {
		t.Fatalf("list roles: %v", err)
	}
	if len(roles) != 2 {
		t.Fatalf("expected 2 roles, got %d", len(roles))
	}

	// 4. Update Role
	role1.Description = "负责全局架构与接口规约"
	updatedRole, err := svc.UpdateRole(role1.ID, role1)
	if err != nil {
		t.Fatalf("update role: %v", err)
	}
	if updatedRole.Description != "负责全局架构与接口规约" {
		t.Fatalf("expected updated description, got %s", updatedRole.Description)
	}

	// 5. Delete Role
	if err := svc.DeleteRole(role2.ID); err != nil {
		t.Fatalf("delete role: %v", err)
	}
	rolesAfterDel, err := svc.ListRoles(task.ID)
	if err != nil || len(rolesAfterDel) != 1 {
		t.Fatalf("expected 1 role after deletion, got %d", len(rolesAfterDel))
	}
}

func TestScheduleValidationAndNextRun(t *testing.T) {
	db := setupTestDB(t)
	launcher := &fakeSessionLauncher{}
	svc := NewService(NewRepository(db), launcher)
	ctx := context.Background()

	// 1. Cron scheduling
	// 1. Valid legacy 5-part cron
	task, err := svc.Create(ctx, CreateTaskRequest{
		Title:           "每日定时巡检",
		TaskType:        TaskTypeStandard,
		ProjectID:       1,
		Backend:         agents.BackendOpenCodeV2,
		Model:           "gpt-4o",
		PromptTemplate:  "每日自动汇报",
		ScheduleType:    ScheduleCron,
		CronExpr:        "0 9 * * *",
		ScheduleEnabled: true,
	})
	if err != nil {
		t.Fatalf("create cron task: %v", err)
	}
	if task.NextRunAt == nil {
		t.Fatalf("expected nextRunAt calculated for cron task")
	}

	// 1.1 Valid 6-part cron (with seconds)
	task6, err := svc.Create(ctx, CreateTaskRequest{
		Title:           "6位秒级任务",
		TaskType:        TaskTypeStandard,
		ProjectID:       1,
		Backend:         agents.BackendOpenCodeV2,
		Model:           "gpt-4o",
		PromptTemplate:  "秒级测试",
		ScheduleType:    ScheduleCron,
		CronExpr:        "0 30 9 * * ?",
		ScheduleEnabled: true,
	})
	if err != nil {
		t.Fatalf("create 6-part cron task: %v", err)
	}
	if task6.NextRunAt == nil {
		t.Fatalf("expected nextRunAt calculated for 6-part cron task")
	}

	// 1.2 Valid 7-part cron (with seconds and year)
	currentYear := time.Now().Year()
	futureYear := currentYear + 1
	task7, err := svc.Create(ctx, CreateTaskRequest{
		Title:           "7位年度任务",
		TaskType:        TaskTypeStandard,
		ProjectID:       1,
		Backend:         agents.BackendOpenCodeV2,
		Model:           "gpt-4o",
		PromptTemplate:  "年度测试",
		ScheduleType:    ScheduleCron,
		CronExpr:        "0 0 12 ? * 1-5 " + strconv.Itoa(futureYear),
		ScheduleEnabled: true,
	})
	if err != nil {
		t.Fatalf("create 7-part cron task: %v", err)
	}
	if task7.NextRunAt == nil {
		t.Fatalf("expected nextRunAt calculated for 7-part cron task")
	}
	if task7.NextRunAt.Year() != futureYear {
		t.Fatalf("expected nextRunAt year %d, got %d", futureYear, task7.NextRunAt.Year())
	}

	// 2. Invalid cron
	_, err = svc.Create(ctx, CreateTaskRequest{
		Title:           "无效 Cron",
		TaskType:        TaskTypeStandard,
		ProjectID:       1,
		Backend:         agents.BackendOpenCodeV2,
		Model:           "gpt-4o",
		ScheduleType:    ScheduleCron,
		CronExpr:        "invalid-cron",
		ScheduleEnabled: true,
	})
	if err == nil {
		t.Fatalf("expected error for invalid cron")
	}

	// 3. Once schedule in past should fail
	past := time.Now().Add(-1 * time.Hour)
	_, err = svc.Create(ctx, CreateTaskRequest{
		Title:           "过去时间一次性",
		TaskType:        TaskTypeStandard,
		ProjectID:       1,
		Backend:         agents.BackendOpenCodeV2,
		Model:           "gpt-4o",
		ScheduleType:    ScheduleOnce,
		ScheduledAt:     &past,
		ScheduleEnabled: true,
	})
	if err == nil {
		t.Fatalf("expected error for once schedule in past")
	}
}

type stubFlowController struct {
	subscribes int
	unsubs     int
}

func (f *stubFlowController) StartRun(ctx context.Context, task Task, triggerType string, initialPrompt string) (FlowRun, error) {
	return FlowRun{}, nil
}

func (f *stubFlowController) PostUserMessage(ctx context.Context, runID uint, content string) (FlowMessage, error) {
	return FlowMessage{}, nil
}

func (f *stubFlowController) CancelRun(runID uint) error { return nil }

func (f *stubFlowController) SubscribeEvents(runID uint) (<-chan FlowEvent, func()) {
	f.subscribes++
	ch := make(chan FlowEvent, 4)
	var once sync.Once
	return ch, func() {
		once.Do(func() {
			f.unsubs++
			close(ch)
		})
	}
}

// TestFlowSubscriptionRecycledOnUnsubscribe pins the events-channel hygiene:
// subscribing to a flow run registers exactly one engine subscription and one
// forwarding goroutine, and the unified bus unsubscribe hook must release both
// so a long-lived service does not accumulate one goroutine per subscribed run.
func TestFlowSubscriptionRecycledOnUnsubscribe(t *testing.T) {
	db := setupTestDB(t)
	svc := NewService(NewRepository(db), &fakeSessionLauncher{})
	controller := &stubFlowController{}
	svc.SetFlowController(controller)
	svc.SetEventsManager(events.NewManager())

	run, err := NewRepository(db).CreateFlowRun(FlowRun{
		TaskID:    1,
		ProjectID: 1,
		Status:    FlowRunRunning,
	})
	if err != nil {
		t.Fatalf("create flow run: %v", err)
	}
	target := strconv.FormatUint(uint64(run.ID), 10)

	if _, err := svc.OnSubscribe(context.Background(), "bus-1", events.SubscribeRequest{Channel: events.ChannelTasks, TargetID: target}); err != nil {
		t.Fatalf("OnSubscribe: %v", err)
	}
	if controller.subscribes != 1 {
		t.Fatalf("expected 1 flow controller subscribe, got %d", controller.subscribes)
	}

	svc.subMu.Lock()
	registered := len(svc.flowSubs)
	svc.subMu.Unlock()
	if registered != 1 {
		t.Fatalf("expected 1 registered flow subscription, got %d", registered)
	}

	// Re-subscribing the same run must not stack a second goroutine.
	if _, err := svc.OnSubscribe(context.Background(), "bus-1", events.SubscribeRequest{Channel: events.ChannelTasks, TargetID: target}); err != nil {
		t.Fatalf("OnSubscribe (repeat): %v", err)
	}
	controller.unsubs = 0
	svc.OnUnsubscribe(context.Background(), "bus-1", events.ChannelTasks, target)
	if controller.unsubs != 1 {
		t.Fatalf("expected exactly 1 unsubscribe after repeat subscribe, got %d", controller.unsubs)
	}

	if err := svc.OnUnsubscribe(context.Background(), "bus-1", events.ChannelTasks, target); err != nil {
		t.Fatalf("OnUnsubscribe: %v", err)
	}
	if controller.unsubs != 1 {
		t.Fatalf("expected exactly 1 unsubscribe call, got %d", controller.unsubs)
	}

	svc.subMu.Lock()
	registered = len(svc.flowSubs)
	svc.subMu.Unlock()
	if registered != 0 {
		t.Fatalf("expected flow subscriptions to be recycled, got %d remaining", registered)
	}

	// Unknown targets and foreign channels are harmless no-ops.
	if err := svc.OnUnsubscribe(context.Background(), "bus-1", events.ChannelTasks, "424242"); err != nil {
		t.Fatalf("OnUnsubscribe (unknown target): %v", err)
	}
	if err := svc.OnUnsubscribe(context.Background(), "bus-1", events.ChannelSessions, target); err != nil {
		t.Fatalf("OnUnsubscribe (foreign channel): %v", err)
	}
}

func TestTasksReconcileOrphans(t *testing.T) {
	db := setupTestDB(t)
	launcher := &fakeSessionLauncher{}
	repo := NewRepository(db)
	svc := NewService(repo, launcher)
	ctx := context.Background()

	task, err := svc.Create(ctx, CreateTaskRequest{
		Title:          "Reconcile Test Task",
		TaskType:       TaskTypeStandard,
		ProjectID:      1,
		Backend:        agents.BackendOpenCodeV2,
		Model:          "gpt-4o",
		PromptTemplate: "echo test",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	now := time.Now().UTC()
	// Insert orphaned running run and completed run
	runningRun := database.TaskRun{
		TaskID:      task.ID,
		SessionID:   "sess_orphan",
		TriggerType: "manual",
		Status:      "running",
		Prompt:      "echo test",
		TriggeredAt: now.Add(-10 * time.Minute),
		CreatedAt:   now.Add(-10 * time.Minute),
	}
	if err := db.Create(&runningRun).Error; err != nil {
		t.Fatalf("create running run: %v", err)
	}

	completedRun := database.TaskRun{
		TaskID:      task.ID,
		SessionID:   "sess_completed",
		TriggerType: "manual",
		Status:      "success",
		Prompt:      "echo test",
		TriggeredAt: now.Add(-20 * time.Minute),
		FinishedAt:  &now,
		CreatedAt:   now.Add(-20 * time.Minute),
	}
	if err := db.Create(&completedRun).Error; err != nil {
		t.Fatalf("create completed run: %v", err)
	}

	reconciled, err := svc.ReconcileOrphans(ctx)
	if err != nil {
		t.Fatalf("ReconcileOrphans: %v", err)
	}
	if reconciled != 1 {
		t.Errorf("ReconcileOrphans returned %d, want 1", reconciled)
	}

	runs, err := repo.ListRuns(task.ID)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	for _, r := range runs {
		if r.ID == runningRun.ID {
			if r.Status != "interrupted" {
				t.Errorf("run.Status = %q, want 'interrupted'", r.Status)
			}
			if r.Error != "interrupted by platform restart" {
				t.Errorf("run.Error = %q, want 'interrupted by platform restart'", r.Error)
			}
			if r.FinishedAt == nil {
				t.Errorf("run.FinishedAt is nil, want non-nil")
			}
		}
		if r.ID == completedRun.ID {
			if r.Status != "success" {
				t.Errorf("completed run.Status = %q, want 'success'", r.Status)
			}
		}
	}
}
