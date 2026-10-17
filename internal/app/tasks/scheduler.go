package tasks

import (
	"context"
	"log"
	"sync"
	"time"
)

// Scheduler runs periodic checks and executes schedulable tasks.
type Scheduler struct {
	svc      *Service
	interval time.Duration
	stopCh   chan struct{}
	wg       sync.WaitGroup
}

// NewScheduler creates a task scheduler.
func NewScheduler(svc *Service, checkInterval time.Duration) *Scheduler {
	if checkInterval <= 0 {
		checkInterval = 10 * time.Second
	}
	return &Scheduler{
		svc:      svc,
		interval: checkInterval,
		stopCh:   make(chan struct{}),
	}
}

// Start launches the scheduler loop in a background goroutine.
func (s *Scheduler) Start() {
	s.wg.Add(1)
	go s.run()
}

// Stop signals the scheduler to stop and waits for ongoing tasks to finish.
func (s *Scheduler) Stop() {
	close(s.stopCh)
	s.wg.Wait()
}

func (s *Scheduler) run() {
	defer s.wg.Done()
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			return
		case now := <-ticker.C:
			s.tick(now.UTC())
		}
	}
}

// tick checks for schedulable tasks and dispatches them.
func (s *Scheduler) tick(now time.Time) {
	tasks, err := s.svc.repo.FindSchedulableTasks(now)
	if err != nil {
		log.Printf("scheduler: find schedulable tasks: %v", err)
		return
	}

	for _, task := range tasks {
		s.dispatch(task, now)
	}
}

func (s *Scheduler) dispatch(t Task, now time.Time) {
	// Guard: check if the task is ready for execution (final step and confirmed)
	if ok, err := CanSchedule(t); !ok {
		log.Printf("scheduler: task %d skipped, not ready: %v", t.ID, err)
		t.LastRunStatus = RunSkippedNotReady
		t.LastRunAt = &now
		if t.ScheduleType == ScheduleOnce {
			t.ScheduleEnabled = false
			t.NextRunAt = nil
		} else if t.ScheduleType == ScheduleCron {
			// Calculate next run so it doesn't get stuck in loop
			next, _ := CalculateNextRun(t, now)
			t.NextRunAt = next
		}
		_ = s.svc.repo.Update(t)
		return
	}

	log.Printf("scheduler: executing task %d (%s)...", t.ID, t.Title)
	execCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	if err := s.svc.TriggerExecution(execCtx, t.ID); err != nil {
		log.Printf("scheduler: task %d execution error: %v", t.ID, err)
	}

	// Re-fetch task to update schedule state
	latest, err := s.svc.repo.Get(t.ID)
	if err != nil {
		return
	}

	if latest.ScheduleType == ScheduleOnce {
		latest.ScheduleEnabled = false
		latest.NextRunAt = nil
	} else if latest.ScheduleType == ScheduleCron {
		next, err := CalculateNextRun(latest, now)
		if err == nil {
			latest.NextRunAt = next
		}
	}
	_ = s.svc.repo.Update(latest)
}
