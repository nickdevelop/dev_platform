package tasks

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
)

var cronParser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)

// CalculateNextRun computes the next run time after 'from' based on schedule settings.
func CalculateNextRun(t Task, from time.Time) (*time.Time, error) {
	switch t.ScheduleType {
	case ScheduleOnce:
		if t.ScheduledAt != nil && t.ScheduledAt.After(from) {
			return t.ScheduledAt, nil
		}
		return nil, nil
	case ScheduleCron:
		if t.CronExpr == "" {
			return nil, ErrInvalidCronExpr
		}
		sched, err := cronParser.Parse(t.CronExpr)
		if err != nil {
			return nil, ErrInvalidCronExpr
		}
		next := sched.Next(from)
		if next.IsZero() {
			return nil, nil
		}
		return &next, nil
	default:
		return nil, nil
	}
}

// ValidateScheduleConfig validates scheduling settings for a task.
func ValidateScheduleConfig(t Task) error {
	if !t.ScheduleEnabled || t.ScheduleType == ScheduleManual || t.ScheduleType == "" {
		return nil
	}

	switch t.ScheduleType {
	case ScheduleOnce:
		if t.ScheduledAt == nil || t.ScheduledAt.Before(time.Now()) {
			return ErrInvalidScheduleTime
		}
	case ScheduleCron:
		if t.CronExpr == "" {
			return ErrInvalidCronExpr
		}
		if _, err := cronParser.Parse(t.CronExpr); err != nil {
			return ErrInvalidCronExpr
		}
	}
	return nil
}

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
