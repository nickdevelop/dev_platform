package tasks

import (
	"context"
	"fmt"
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

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&database.Task{}); err != nil {
		t.Fatalf("migrate task: %v", err)
	}
	return db
}

func TestTaskCRUDAndFlowAdvance(t *testing.T) {
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
	if task.CurrentStep != 1 || task.StepStatus != StepPending || task.IsConfirmed {
		t.Fatalf("unexpected initial task state: %+v", task)
	}

	// 2. Cannot schedule when at step 1
	future := time.Now().Add(1 * time.Hour)
	_, err = svc.UpdateSchedule(ctx, task.ID, ScheduleConfigRequest{
		ScheduleType:    ScheduleOnce,
		ScheduledAt:     &future,
		ScheduleEnabled: true,
	})
	if err == nil {
		t.Fatalf("expected error when scheduling unconfirmed task at step 1")
	}

	// 3. Advance to Step 2 (Task Execution) without confirmation should fail
	_, err = svc.AdvanceStep(ctx, task.ID, AdvanceStepRequest{
		Confirmed: false,
		NextStep:  intPtr(2),
	})
	if err == nil {
		t.Fatalf("expected error advancing without confirmation")
	}

	// 4. Confirm step 1 and advance to step 2
	task, err = svc.AdvanceStep(ctx, task.ID, AdvanceStepRequest{
		Confirmed: true,
		Notes:     "参数已全部确认无误",
	})
	if err != nil {
		t.Fatalf("advance step: %v", err)
	}
	if task.CurrentStep != 2 || !task.IsConfirmed || task.Status != StatusReadyForExecution {
		t.Fatalf("expected task at step 2 ready for execution, got %+v", task)
	}

	// 5. Now at step 2 and confirmed: scheduling should succeed!
	// Transactional supports Cron
	task, err = svc.UpdateSchedule(ctx, task.ID, ScheduleConfigRequest{
		ScheduleType:    ScheduleCron,
		CronExpr:        "0 2 * * *", // 2 AM every day
		ScheduleEnabled: true,
	})
	if err != nil {
		t.Fatalf("schedule cron failed: %v", err)
	}
	if task.NextRunAt == nil {
		t.Fatalf("expected nextRunAt to be calculated")
	}

	// 6. Test session creation from task
	task, sess, err := svc.CreateSessionForTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("create session for task: %v", err)
	}
	if task.ActiveSessionID != sess.ID {
		t.Fatalf("active session id mismatch: %s != %s", task.ActiveSessionID, sess.ID)
	}

	// 7. Trigger execution
	err = svc.TriggerExecution(ctx, task.ID)
	if err != nil {
		t.Fatalf("trigger execution: %v", err)
	}
	if task.LastRunStatus != RunSuccess {
		updated, _ := svc.Get(task.ID)
		if updated.LastRunStatus != RunSuccess {
			t.Fatalf("expected RunSuccess, got %s", updated.LastRunStatus)
		}
	}
}

func TestDevelopmentTaskFlowAndCronForbidden(t *testing.T) {
	db := setupTestDB(t)
	launcher := &fakeSessionLauncher{}
	svc := NewService(NewRepository(db), launcher)
	ctx := context.Background()

	// Create Development Task (4 steps)
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

	// Step 1 -> Step 2
	task, err = svc.AdvanceStep(ctx, task.ID, AdvanceStepRequest{Confirmed: true})
	if err != nil || task.CurrentStep != 2 {
		t.Fatalf("advance to step 2 failed: %v", err)
	}
	// Step 2 -> Step 3
	task, err = svc.AdvanceStep(ctx, task.ID, AdvanceStepRequest{Confirmed: true})
	if err != nil || task.CurrentStep != 3 {
		t.Fatalf("advance to step 3 failed: %v", err)
	}
	// Step 3 -> Step 4
	task, err = svc.AdvanceStep(ctx, task.ID, AdvanceStepRequest{Confirmed: true, Notes: "计划制定完毕，所有事项已确认"})
	if err != nil || task.CurrentStep != 4 || !task.IsConfirmed {
		t.Fatalf("advance to step 4 failed: %v, task: %+v", err, task)
	}

	// Development task must NOT allow Cron
	_, err = svc.UpdateSchedule(ctx, task.ID, ScheduleConfigRequest{
		ScheduleType:    ScheduleCron,
		CronExpr:        "0 3 * * *",
		ScheduleEnabled: true,
	})
	if err == nil {
		t.Fatalf("expected cron to be forbidden for development tasks")
	}

	// Development task DOES allow once scheduling
	future := time.Now().Add(2 * time.Hour)
	task, err = svc.UpdateSchedule(ctx, task.ID, ScheduleConfigRequest{
		ScheduleType:    ScheduleOnce,
		ScheduledAt:     &future,
		ScheduleEnabled: true,
	})
	if err != nil {
		t.Fatalf("scheduled once failed: %v", err)
	}
	if task.ScheduleType != ScheduleOnce || !task.ScheduleEnabled {
		t.Fatalf("unexpected schedule state: %+v", task)
	}
}

func intPtr(i int) *int {
	return &i
}
