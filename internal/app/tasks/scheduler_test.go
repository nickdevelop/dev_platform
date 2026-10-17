package tasks

import (
	"context"
	"testing"
	"time"

	"devplatform/internal/core/agents"
)

func TestSchedulerTick(t *testing.T) {
	db := setupTestDB(t)
	launcher := &fakeSessionLauncher{}
	svc := NewService(NewRepository(db), launcher)
	scheduler := NewScheduler(svc, 100*time.Millisecond)

	ctx := context.Background()

	// 1. Task not ready (step 1) with schedule_enabled = true in DB
	past := time.Now().Add(-1 * time.Minute).UTC()
	taskNotReady, err := svc.Create(ctx, CreateTaskRequest{
		Title:       "未确认任务",
		Description: "测试用",
		ProjectID:   1,
		Backend:     agents.BackendOpenCodeV2,
		Model:       "gpt-4o",
		FlowType:    FlowTransactional,
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	// Force schedule settings into DB directly to test scheduler guard
	taskNotReady.ScheduleEnabled = true
	taskNotReady.ScheduleType = ScheduleOnce
	taskNotReady.NextRunAt = &past
	_ = svc.repo.Update(taskNotReady)

	// Run scheduler tick
	scheduler.tick(time.Now().UTC())

	reloaded, _ := svc.Get(taskNotReady.ID)
	if reloaded.LastRunStatus != RunSkippedNotReady {
		t.Fatalf("expected task to be skipped_not_ready, got %s", reloaded.LastRunStatus)
	}

	// 2. Task ready (at step 2 and confirmed)
	taskReady, err := svc.Create(ctx, CreateTaskRequest{
		Title:       "已确认就绪任务",
		Description: "测试自动执行",
		ProjectID:   1,
		Backend:     agents.BackendOpenCodeV2,
		Model:       "gpt-4o",
		FlowType:    FlowTransactional,
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	_, _ = svc.AdvanceStep(ctx, taskReady.ID, AdvanceStepRequest{Confirmed: true})
	taskReady, _ = svc.Get(taskReady.ID)
	taskReady.ScheduleEnabled = true
	taskReady.ScheduleType = ScheduleOnce
	taskReady.NextRunAt = &past
	_ = svc.repo.Update(taskReady)

	// Run tick
	scheduler.tick(time.Now().UTC())

	reloadedReady, _ := svc.Get(taskReady.ID)
	if reloadedReady.LastRunStatus != RunSuccess {
		t.Fatalf("expected task run success, got %s", reloadedReady.LastRunStatus)
	}
	if reloadedReady.ScheduleEnabled {
		t.Fatalf("expected once schedule to be disabled after run")
	}
}
