package tasks

import (
	"context"
	"testing"
	"time"

	"devplatform/internal/app/sessions"
	"devplatform/internal/core/agents"
)

func TestSchedulerTick(t *testing.T) {
	db := setupTestDB(t)
	launcher := &fakeSessionLauncher{}
	svc := NewService(NewRepository(db), launcher)
	scheduler := NewScheduler(svc, 100*time.Millisecond)

	ctx := context.Background()

	past := time.Now().Add(-1 * time.Minute).UTC()

	// 1. Task with schedule_enabled = true and next_run_at in past
	task, err := svc.Create(ctx, CreateTaskRequest{
		Title:          "定时执行模板任务",
		Description:    "测试自动执行",
		TaskType:       TaskTypeStandard,
		ProjectID:      1,
		Backend:        agents.BackendOpenCodeV2,
		Model:          "gpt-4o",
		PromptTemplate: "执行系统检查",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	task.ScheduleEnabled = true
	task.ScheduleType = ScheduleOnce
	task.NextRunAt = &past
	_ = svc.repo.Update(task)

	// Run tick and wait for dispatch workers
	scheduler.tick(time.Now().UTC())
	scheduler.Wait()

	reloaded, _ := svc.Get(task.ID)
	// Because fake launcher creates sessions with turn=running, status starts as executing
	if reloaded.LastRunStatus != RunExecuting {
		t.Fatalf("expected task run executing, got %s", reloaded.LastRunStatus)
	}
	if reloaded.ScheduleEnabled {
		t.Fatalf("expected once schedule to be disabled after run")
	}

	// Session completes
	launcher.setSessionState("sess_1", sessions.SessionActive, sessions.TurnIdle)
	reloaded, _ = svc.Get(task.ID)
	if reloaded.LastRunStatus != RunSuccess {
		t.Fatalf("expected task run success, got %s", reloaded.LastRunStatus)
	}
}

func TestSchedulerConcurrentDispatch(t *testing.T) {
	db := setupTestDB(t)
	launcher := &fakeSessionLauncher{}
	svc := NewService(NewRepository(db), launcher)
	scheduler := NewScheduler(svc, 100*time.Millisecond)
	ctx := context.Background()

	past := time.Now().Add(-1 * time.Minute).UTC()

	// Create 3 scheduled tasks
	for i := 1; i <= 3; i++ {
		task, err := svc.Create(ctx, CreateTaskRequest{
			Title:          "并发任务",
			TaskType:       TaskTypeStandard,
			ProjectID:      uint(i),
			Backend:        agents.BackendOpenCodeV2,
			Model:          "gpt-4o",
			PromptTemplate: "并发测试提示词",
		})
		if err != nil {
			t.Fatalf("create task %d: %v", i, err)
		}
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
		if task.LastRunStatus != RunExecuting {
			t.Fatalf("expected task %d run executing, got %s", task.ID, task.LastRunStatus)
		}
		if task.ScheduleEnabled {
			t.Fatalf("expected once schedule disabled for task %d", task.ID)
		}
	}
}
