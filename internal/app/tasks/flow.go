package tasks

import (
	"errors"
	"fmt"
	"time"

	"github.com/robfig/cron/v3"
)

var cronParser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)

// GetStepDef finds the step definition for a given flow and step index.
func GetStepDef(flowType FlowType, stepIndex int) (StepDef, bool) {
	flow, ok := GetFlowDef(flowType)
	if !ok {
		return StepDef{}, false
	}
	for _, step := range flow.Steps {
		if step.Index == stepIndex {
			return step, true
		}
	}
	return StepDef{}, false
}

// FinalStepIndex returns the index of the final step for a flow.
func FinalStepIndex(flowType FlowType) int {
	flow, ok := GetFlowDef(flowType)
	if !ok {
		return 0
	}
	return len(flow.Steps)
}

// IsFinalStep reports whether stepIndex is the final step of the flow.
func IsFinalStep(flowType FlowType, stepIndex int) bool {
	return stepIndex == FinalStepIndex(flowType)
}

// ValidateTaskCreation checks initial task fields.
func ValidateTaskCreation(t Task) error {
	if t.Title == "" {
		return errors.New("tasks: title is required")
	}
	if t.ProjectID == 0 {
		return errors.New("tasks: project is required")
	}
	if !t.Backend.Valid() {
		return errors.New("tasks: backend is required and must be valid")
	}
	if t.Model == "" {
		return errors.New("tasks: model is required")
	}
	if _, ok := GetFlowDef(t.FlowType); !ok {
		return ErrInvalidFlowType
	}
	return nil
}

// CanSchedule checks whether the task is eligible for scheduling.
// Rule: Task must be in the final step AND confirmed, so that no prompts or
// user questions interrupt execution.
func CanSchedule(t Task) (bool, error) {
	if !IsFinalStep(t.FlowType, t.CurrentStep) {
		return false, fmt.Errorf("%w: task is at step %d, but must be at final step %d",
			ErrScheduleNotAllowed, t.CurrentStep, FinalStepIndex(t.FlowType))
	}
	if !t.IsConfirmed {
		return false, fmt.Errorf("%w: final step has not been marked as confirmed", ErrScheduleNotAllowed)
	}
	if t.FlowType == FlowDevelopment && t.ScheduleType == ScheduleCron {
		return false, ErrCronNotSupported
	}
	return true, nil
}

// ValidateScheduleConfig validates scheduling settings for a task.
func ValidateScheduleConfig(t Task) error {
	if !t.ScheduleEnabled || t.ScheduleType == ScheduleNone {
		return nil
	}

	if ok, err := CanSchedule(t); !ok {
		return err
	}

	switch t.ScheduleType {
	case ScheduleOnce:
		if t.ScheduledAt == nil || t.ScheduledAt.Before(time.Now()) {
			return ErrInvalidScheduleTime
		}
	case ScheduleCron:
		if t.FlowType != FlowTransactional {
			return ErrCronNotSupported
		}
		if t.CronExpr == "" {
			return ErrInvalidCronExpr
		}
		if _, err := cronParser.Parse(t.CronExpr); err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidCronExpr, err)
		}
	default:
		return errors.New("tasks: unknown schedule type")
	}
	return nil
}

// CalculateNextRun calculates the next run time for a task given the base time.
func CalculateNextRun(t Task, base time.Time) (*time.Time, error) {
	if !t.ScheduleEnabled {
		return nil, nil
	}
	switch t.ScheduleType {
	case ScheduleOnce:
		if t.ScheduledAt == nil {
			return nil, ErrInvalidScheduleTime
		}
		return t.ScheduledAt, nil
	case ScheduleCron:
		sched, err := cronParser.Parse(t.CronExpr)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalidCronExpr, err)
		}
		next := sched.Next(base)
		if next.IsZero() {
			return nil, nil
		}
		return &next, nil
	default:
		return nil, nil
	}
}
