package tasks

import (
	"context"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
)

var (
	cron5Parser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
	cron6Parser = cron.NewParser(cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
	// Backward-compatible alias for 5-field parser
	cronParser = cron5Parser
)

type yearSchedule struct {
	inner    cron.Schedule
	yearSpec string
}

func (y *yearSchedule) Next(t time.Time) time.Time {
	curr := t
	for i := 0; i < 200; i++ {
		next := y.inner.Next(curr)
		if next.IsZero() {
			return time.Time{}
		}
		if matchYear(next.Year(), y.yearSpec) {
			return next
		}
		if isPastMaxYear(next.Year(), y.yearSpec) {
			return time.Time{}
		}
		curr = next
	}
	return time.Time{}
}

func matchYear(year int, spec string) bool {
	if spec == "*" || spec == "?" {
		return true
	}
	parts := strings.Split(spec, ",")
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if strings.Contains(p, "-") {
			rangeParts := strings.Split(p, "-")
			if len(rangeParts) == 2 {
				start, err1 := strconv.Atoi(rangeParts[0])
				end, err2 := strconv.Atoi(rangeParts[1])
				if err1 == nil && err2 == nil && year >= start && year <= end {
					return true
				}
			}
		} else {
			val, err := strconv.Atoi(p)
			if err == nil && year == val {
				return true
			}
		}
	}
	return false
}

func isPastMaxYear(year int, spec string) bool {
	if spec == "*" || spec == "?" {
		return false
	}
	maxYear := 0
	parts := strings.Split(spec, ",")
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if strings.Contains(p, "-") {
			rangeParts := strings.Split(p, "-")
			if len(rangeParts) == 2 {
				end, err := strconv.Atoi(rangeParts[1])
				if err == nil && end > maxYear {
					maxYear = end
				}
			}
		} else {
			val, err := strconv.Atoi(p)
			if err == nil && val > maxYear {
				maxYear = val
			}
		}
	}
	return maxYear > 0 && year > maxYear
}

func validateYearSpec(spec string) bool {
	if spec == "*" || spec == "?" {
		return true
	}
	parts := strings.Split(spec, ",")
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if strings.Contains(p, "-") {
			rangeParts := strings.Split(p, "-")
			if len(rangeParts) != 2 {
				return false
			}
			start, err1 := strconv.Atoi(rangeParts[0])
			end, err2 := strconv.Atoi(rangeParts[1])
			if err1 != nil || err2 != nil || start > end || start < 1970 || end > 2199 {
				return false
			}
		} else {
			val, err := strconv.Atoi(p)
			if err != nil || val < 1970 || val > 2199 {
				return false
			}
		}
	}
	return true
}

// ParseCronSchedule parses 5, 6, or 7-field cron expressions.
// 5 fields: minute hour day month week
// 6 fields: second minute hour day month week
// 7 fields: second minute hour day month week year
func ParseCronSchedule(expr string) (cron.Schedule, error) {
	trimmed := strings.TrimSpace(expr)
	if trimmed == "" {
		return nil, ErrInvalidCronExpr
	}

	if strings.HasPrefix(trimmed, "@") {
		sched, err := cron5Parser.Parse(trimmed)
		if err != nil {
			return nil, ErrInvalidCronExpr
		}
		return sched, nil
	}

	fields := strings.Fields(trimmed)
	switch len(fields) {
	case 5:
		sched, err := cron5Parser.Parse(trimmed)
		if err != nil {
			return nil, ErrInvalidCronExpr
		}
		return sched, nil
	case 6:
		sched, err := cron6Parser.Parse(trimmed)
		if err != nil {
			return nil, ErrInvalidCronExpr
		}
		return sched, nil
	case 7:
		yearSpec := fields[6]
		if !validateYearSpec(yearSpec) {
			return nil, ErrInvalidCronExpr
		}
		sixFieldSpec := strings.Join(fields[:6], " ")
		sched, err := cron6Parser.Parse(sixFieldSpec)
		if err != nil {
			return nil, ErrInvalidCronExpr
		}
		if yearSpec == "*" || yearSpec == "?" {
			return sched, nil
		}
		return &yearSchedule{inner: sched, yearSpec: yearSpec}, nil
	default:
		return nil, ErrInvalidCronExpr
	}
}

// CalculateNextRun computes the next run time after 'from' based on schedule settings.
func CalculateNextRun(t Task, from time.Time) (*time.Time, error) {
	switch t.ScheduleType {
	case ScheduleOnce:
		if t.ScheduledAt != nil && t.ScheduledAt.After(from.Add(-30*time.Second)) {
			return t.ScheduledAt, nil
		}
		return nil, nil
	case ScheduleCron:
		if t.CronExpr == "" {
			return nil, ErrInvalidCronExpr
		}
		sched, err := ParseCronSchedule(t.CronExpr)
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
		if t.ScheduledAt == nil || t.ScheduledAt.Before(time.Now().Add(-30*time.Second)) {
			return ErrInvalidScheduleTime
		}
	case ScheduleCron:
		if t.CronExpr == "" {
			return ErrInvalidCronExpr
		}
		if _, err := ParseCronSchedule(t.CronExpr); err != nil {
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
