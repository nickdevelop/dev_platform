package tasks

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

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
	if err := db.AutoMigrate(&database.Task{}, &database.TaskRun{}); err != nil {
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

func TestFlowTaskExtensionPoint(t *testing.T) {
	db := setupTestDB(t)
	launcher := &fakeSessionLauncher{}
	svc := NewService(NewRepository(db), launcher)
	ctx := context.Background()

	// Flow task should be intercepted by validator
	_, err := svc.Create(ctx, CreateTaskRequest{
		Title:     "流程编排任务",
		TaskType:  TaskTypeFlow,
		ProjectID: 1,
		Backend:   agents.BackendOpenCodeV2,
		Model:     "gpt-4o",
	})
	if err == nil || err != ErrFlowTasksNotImplementedYet {
		t.Fatalf("expected ErrFlowTasksNotImplementedYet, got %v", err)
	}
}

func TestScheduleValidationAndNextRun(t *testing.T) {
	db := setupTestDB(t)
	launcher := &fakeSessionLauncher{}
	svc := NewService(NewRepository(db), launcher)
	ctx := context.Background()

	// 1. Cron scheduling
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
