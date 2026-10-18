package tasks

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"devplatform/internal/app/database"
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
	if err := db.AutoMigrate(&database.Task{}, &database.TaskRun{}); err != nil {
		t.Fatalf("migrate task & runs: %v", err)
	}
	return db
}

func TestTransactionalTaskFlowAndRuns(t *testing.T) {
	db := setupTestDB(t)
	launcher := &fakeSessionLauncher{}
	svc := NewService(NewRepository(db), launcher)
	ctx := context.Background()

	// 1. Create Transactional Task
	task, err := svc.Create(ctx, CreateTaskRequest{
		Title:       "发布事务任务",
		Description: "生产环境发布",
		ProjectID:   1,
		Backend:     agents.BackendOpenCodeV2,
		Model:       "gpt-4o",
		FlowType:    FlowTransactional,
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	// Step 1 is "analysis"
	step1Def, ok := GetStepDef(FlowTransactional, 1)
	if !ok || step1Def.Key != "analysis" || step1Def.Name != "任务分析" {
		t.Fatalf("expected step 1 to be analysis, got: %+v", step1Def)
	}

	// 2. Start initial analysis session
	task, sess1, err := svc.CreateSessionForTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("create analysis session: %v", err)
	}
	if task.ActiveSessionID != sess1.ID {
		t.Fatalf("expected active session to be %s", sess1.ID)
	}

	// 3. Save execution prompt (task analysis finished, output direct execution plan)
	task, err = svc.SaveExecutionPrompt(ctx, task.ID, "请按以下步骤执行部署：1.拉取镜像 2.重启容器 3.健康检查")
	if err != nil {
		t.Fatalf("save execution prompt: %v", err)
	}
	if !task.IsConfirmed || task.Status != StatusReadyForExecution || task.CurrentStep != 2 {
		t.Fatalf("expected task ready for execution at step 2, got: %+v", task)
	}

	// 4. Trigger Execution 1: should spawn a NEW independent session and create TaskRun
	run1, err := svc.TriggerExecution(ctx, task.ID, "manual")
	if err != nil {
		t.Fatalf("trigger execution 1: %v", err)
	}
	if run1.SessionID == sess1.ID {
		t.Fatalf("expected run 1 to have a new session, but got analysis session id %s", run1.SessionID)
	}

	// 5. Trigger Execution 2 (e.g. periodic or manual repeat): should spawn ANOTHER new session
	run2, err := svc.TriggerExecution(ctx, task.ID, "cron")
	if err != nil {
		t.Fatalf("trigger execution 2: %v", err)
	}
	if run2.SessionID == run1.SessionID {
		t.Fatalf("expected run 2 to have a distinct session from run 1")
	}

	// 6. Check runs history
	runs, err := svc.ListRuns(ctx, task.ID)
	if err != nil || len(runs) != 2 {
		t.Fatalf("expected 2 runs, got %d (err: %v)", len(runs), err)
	}
	if runs[0].TriggerType != "cron" || runs[1].TriggerType != "manual" {
		t.Fatalf("unexpected runs order or trigger type: %+v", runs)
	}
}

func TestDevelopmentTaskFlowAndCompletionLock(t *testing.T) {
	db := setupTestDB(t)
	launcher := &fakeSessionLauncher{}
	svc := NewService(NewRepository(db), launcher)
	ctx := context.Background()

	// 1. Create Development Task (4 steps)
	task, err := svc.Create(ctx, CreateTaskRequest{
		Title:       "新特性开发",
		Description: "用户中心重构",
		ProjectID:   2,
		Backend:     agents.BackendPi,
		Model:       "claude-3-5-sonnet",
		FlowType:    FlowDevelopment,
	})
	if err != nil {
		t.Fatalf("create dev task: %v", err)
	}

	// 2. Start initial session for step 1
	task, sess, err := svc.CreateSessionForTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	// 3. Step 1 -> Step 2: system sends DriverPrompt to the SAME session to invoke Agent
	task, err = svc.AdvanceStep(ctx, task.ID, AdvanceStepRequest{
		Confirmed:    true,
		DriverPrompt: "【进入需求设计】请输出技术架构设计方案",
	})
	if err != nil || task.CurrentStep != 2 {
		t.Fatalf("advance to step 2 failed: %v", err)
	}
	if len(launcher.sentMessages) < 2 {
		t.Fatalf("expected driver prompt to be sent to session, got %d messages", len(launcher.sentMessages))
	}

	// 4. Step 2 -> Step 3
	task, err = svc.AdvanceStep(ctx, task.ID, AdvanceStepRequest{
		Confirmed:    true,
		DriverPrompt: "【进入制定计划】请输出分步执行计划并澄清所有确认事项",
	})
	if err != nil || task.CurrentStep != 3 {
		t.Fatalf("advance to step 3 failed: %v", err)
	}

	// 5. Step 3 -> Step 4 (Ready for execution)
	task, err = svc.AdvanceStep(ctx, task.ID, AdvanceStepRequest{
		Confirmed: true,
		Notes:     "计划已确认无歧义",
	})
	if err != nil || task.CurrentStep != 4 || !task.IsConfirmed || task.Status != StatusReadyForExecution {
		t.Fatalf("advance to step 4 failed: %v, task: %+v", err, task)
	}

	// 6. Trigger execution in existing session
	run, err := svc.TriggerExecution(ctx, task.ID, "manual")
	if err != nil {
		t.Fatalf("trigger execution: %v", err)
	}
	if run.SessionID != sess.ID {
		t.Fatalf("expected development run to execute in existing session %s, got %s", sess.ID, run.SessionID)
	}

	// 7. Check task status: must be StatusCompleted
	task, _ = svc.Get(task.ID)
	if task.Status != StatusCompleted {
		t.Fatalf("expected development task status to be completed, got %s", task.Status)
	}

	// 8. Try to trigger again: must be refused with ErrTaskAlreadyCompleted!
	_, err = svc.TriggerExecution(ctx, task.ID, "manual")
	if err == nil || err != ErrTaskAlreadyCompleted {
		t.Fatalf("expected ErrTaskAlreadyCompleted, got: %v", err)
	}
}

func TestTaskExecutionTurnSync(t *testing.T) {
	db := setupTestDB(t)
	launcher := &fakeSessionLauncher{}
	svc := NewService(NewRepository(db), launcher)
	ctx := context.Background()

	// 1. Create a confirmed transactional task
	task, err := svc.Create(ctx, CreateTaskRequest{
		Title:       "定时同步任务",
		ProjectID:   1,
		Backend:     agents.BackendOpenCodeV2,
		Model:       "gpt-4o",
		FlowType:    FlowTransactional,
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	task, err = svc.SaveExecutionPrompt(ctx, task.ID, "同步脚本提示词")
	if err != nil {
		t.Fatalf("save prompt: %v", err)
	}

	// 2. Trigger execution
	run, err := svc.TriggerExecution(ctx, task.ID, "manual")
	if err != nil {
		t.Fatalf("trigger execution: %v", err)
	}
	if run.Status != "running" {
		t.Fatalf("expected initial run status to be 'running', got %s", run.Status)
	}

	// Task should be in executing state while turn is running
	launcher.setSessionState(run.SessionID, sessions.SessionActive, sessions.TurnRunning)
	syncedTask, err := svc.SyncTaskStatus(ctx, task.ID)
	if err != nil {
		t.Fatalf("sync status: %v", err)
	}
	if syncedTask.Status != StatusExecuting || syncedTask.LastRunStatus != RunExecuting {
		t.Fatalf("expected task executing while turn running, got task=%s run=%s", syncedTask.Status, syncedTask.LastRunStatus)
	}

	// 3. Agent turn completes
	launcher.setSessionState(run.SessionID, sessions.SessionActive, sessions.TurnIdle)
	syncedTask, err = svc.SyncTaskStatus(ctx, task.ID)
	if err != nil {
		t.Fatalf("sync status: %v", err)
	}
	if syncedTask.Status != StatusReadyForExecution || syncedTask.LastRunStatus != RunSuccess {
		t.Fatalf("expected task ready and run success, got task=%s run=%s", syncedTask.Status, syncedTask.LastRunStatus)
	}

	// Verify run was updated in repository
	runs, err := svc.ListRuns(ctx, task.ID)
	if err != nil || len(runs) != 1 {
		t.Fatalf("list runs: %v", err)
	}
	if runs[0].Status != "success" || runs[0].FinishedAt == nil {
		t.Fatalf("expected run status to be success with finished_at, got: %+v", runs[0])
	}
}

func TestTaskExecutionStoppedByUser(t *testing.T) {
	db := setupTestDB(t)
	launcher := &fakeSessionLauncher{}
	svc := NewService(NewRepository(db), launcher)
	ctx := context.Background()

	task, _ := svc.Create(ctx, CreateTaskRequest{
		Title:       "长耗时分析任务",
		ProjectID:   1,
		Backend:     agents.BackendOpenCodeV2,
		Model:       "gpt-4o",
		FlowType:    FlowTransactional,
	})
	task, _ = svc.SaveExecutionPrompt(ctx, task.ID, "执行长任务")

	run, err := svc.TriggerExecution(ctx, task.ID, "manual")
	if err != nil {
		t.Fatalf("trigger execution: %v", err)
	}

	// User clicked "Stop" on session
	launcher.setSessionState(run.SessionID, sessions.SessionStopped, sessions.TurnIdle)

	syncedTask, err := svc.SyncTaskStatus(ctx, task.ID)
	if err != nil {
		t.Fatalf("sync status: %v", err)
	}
	if syncedTask.LastRunStatus != RunFailed {
		t.Fatalf("expected last run status failed, got %s", syncedTask.LastRunStatus)
	}

	runs, _ := svc.ListRuns(ctx, task.ID)
	if runs[0].Status != "failed" || runs[0].Error != "execution stopped by user" {
		t.Fatalf("expected run status failed with reason, got %+v", runs[0])
	}
}

func TestTaskAdvanceStepSendFailure(t *testing.T) {
	db := setupTestDB(t)
	launcher := &fakeSessionLauncher{
		sendErr: fmt.Errorf("connection refused"),
	}
	svc := NewService(NewRepository(db), launcher)
	ctx := context.Background()

	task, _ := svc.Create(ctx, CreateTaskRequest{
		Title:       "网络异常任务",
		ProjectID:   1,
		Backend:     agents.BackendPi,
		Model:       "claude-3-5-sonnet",
		FlowType:    FlowDevelopment,
	})
	task, _, _ = svc.CreateSessionForTask(ctx, task.ID)

	// Advancing step should fail when sending driver prompt fails, and NOT advance current step
	_, err := svc.AdvanceStep(ctx, task.ID, AdvanceStepRequest{
		Confirmed:    true,
		DriverPrompt: "下一阶段提示",
	})
	if err == nil {
		t.Fatal("expected advance step to fail when send driver prompt fails")
	}

	latest, _ := svc.repo.Get(task.ID)
	if latest.CurrentStep != 1 {
		t.Fatalf("step should not advance on failure, got step %d", latest.CurrentStep)
	}
}

func TestDeleteTaskCleansUpSessions(t *testing.T) {
	db := setupTestDB(t)
	launcher := &fakeSessionLauncher{}
	svc := NewService(NewRepository(db), launcher)
	ctx := context.Background()

	// 创建一个任务并拉起会话
	task, err := svc.Create(ctx, CreateTaskRequest{
		Title:     "待删除任务",
		ProjectID: 1,
		Backend:   agents.BackendPi,
		Model:     "claude-3-5-sonnet",
		FlowType:  FlowTransactional,
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	task, _, err = svc.CreateSessionForTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	activeSessionID := task.ActiveSessionID
	if activeSessionID == "" {
		t.Fatal("expected active session ID")
	}

	// 确认并就绪，再触发一次执行生成一个 run 会话
	_, _ = svc.AdvanceStep(ctx, task.ID, AdvanceStepRequest{Confirmed: true})
	task, _ = svc.SaveExecutionPrompt(ctx, task.ID, "测试执行")
	run, err := svc.TriggerExecution(ctx, task.ID)
	if err != nil {
		t.Fatalf("trigger execution: %v", err)
	}

	// 执行删除任务
	if err := svc.Delete(task.ID); err != nil {
		t.Fatalf("delete task: %v", err)
	}

	// 验证任务已被删除
	if _, err := svc.Get(task.ID); !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("expected ErrTaskNotFound, got %v", err)
	}

	// 验证任务关联的 activeSessionID 和 run.SessionID 都被传递给了 launcher.Delete
	deletedMap := make(map[string]bool)
	for _, sid := range launcher.deletedSessions {
		deletedMap[sid] = true
	}

	if !deletedMap[activeSessionID] {
		t.Errorf("expected activeSessionID %s to be deleted, got %v", activeSessionID, launcher.deletedSessions)
	}
	if !deletedMap[run.SessionID] {
		t.Errorf("expected run session ID %s to be deleted, got %v", run.SessionID, launcher.deletedSessions)
	}
}
