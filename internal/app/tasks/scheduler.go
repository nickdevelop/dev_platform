package tasks

import (
	"context"
	"log"
	"sync"
	"time"
)

// maxConcurrentTasks bounds how many tasks the scheduler may execute in parallel.
const maxConcurrentTasks = 5

// Scheduler runs periodic checks and executes schedulable tasks.
type Scheduler struct {
	svc      *Service
	interval time.Duration
	stopCh   chan struct{}
	sem      chan struct{}
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
		sem:      make(chan struct{}, maxConcurrentTasks),
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

// Wait blocks until all currently executing dispatch workers have finished.
func (s *Scheduler) Wait() {
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

// tick checks for schedulable tasks and dispatches them concurrently.
func (s *Scheduler) tick(now time.Time) {
	tasks, err := s.svc.repo.FindSchedulableTasks(now)
	if err != nil {
		log.Printf("scheduler: find schedulable tasks: %v", err)
		return
	}

	for _, task := range tasks {
		t := task
		select {
		case <-s.stopCh:
			return
		case s.sem <- struct{}{}:
			s.wg.Add(1)
			go func() {
				defer func() {
					<-s.sem
					s.wg.Done()
				}()
				s.dispatch(t, now)
			}()
		}
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
		if err := s.svc.repo.Update(t); err != nil {
			log.Printf("scheduler: update skipped task %d failed: %v", t.ID, err)
		}
		return
	}

	log.Printf("scheduler: executing task %d (%s)...", t.ID, t.Title)
	execCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	if _, err := s.svc.TriggerExecution(execCtx, t.ID, string(t.ScheduleType)); err != nil {
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
	if err := s.svc.repo.Update(latest); err != nil {
		log.Printf("scheduler: update scheduled task %d failed: %v", latest.ID, err)
	}
}
