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

	// Run scheduler tick and wait for workers
	scheduler.tick(time.Now().UTC())
	scheduler.Wait()

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

	// Run tick and wait for workers
	scheduler.tick(time.Now().UTC())
	scheduler.Wait()

	reloadedReady, _ := svc.Get(taskReady.ID)
	if reloadedReady.LastRunStatus != RunSuccess {
		t.Fatalf("expected task run success, got %s", reloadedReady.LastRunStatus)
	}
	if reloadedReady.ScheduleEnabled {
		t.Fatalf("expected once schedule to be disabled after run")
	}
}

func TestSchedulerConcurrentDispatch(t *testing.T) {
	db := setupTestDB(t)
	launcher := &fakeSessionLauncher{}
	svc := NewService(NewRepository(db), launcher)
	scheduler := NewScheduler(svc, 100*time.Millisecond)
	ctx := context.Background()

	past := time.Now().Add(-1 * time.Minute).UTC()

	// Create 3 ready tasks
	for i := 1; i <= 3; i++ {
		task, err := svc.Create(ctx, CreateTaskRequest{
			Title:     "并发任务",
			ProjectID: uint(i),
			Backend:   agents.BackendOpenCodeV2,
			Model:     "gpt-4o",
			FlowType:  FlowTransactional,
		})
		if err != nil {
			t.Fatalf("create task %d: %v", i, err)
		}
		_, _ = svc.SaveExecutionPrompt(ctx, task.ID, "并发执行方案")
		task, _ = svc.Get(task.ID)
		task.ScheduleEnabled = true
		task.ScheduleType = ScheduleOnce
		task.NextRunAt = &past
		_ = svc.repo.Update(task)
	}

	// Tick and wait
	scheduler.tick(time.Now().UTC())
	scheduler.Wait()

	tasks, err := svc.List(ListFilter{})
	if err != nil || len(tasks) != 3 {
		t.Fatalf("list tasks: %v", err)
	}
	for _, task := range tasks {
		if task.LastRunStatus != RunSuccess {
			t.Fatalf("expected task %d run success, got %s", task.ID, task.LastRunStatus)
		}
		if task.ScheduleEnabled {
			t.Fatalf("expected task %d schedule disabled", task.ID)
		}
	}
}
