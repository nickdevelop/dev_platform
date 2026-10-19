package tasks

import (
	"errors"
	"time"

	"devplatform/internal/core/agents"
)

// TaskType identifies the task kind.
type TaskType string

const (
	TaskTypeStandard TaskType = "standard"
	TaskTypeFlow     TaskType = "flow"
)

// TaskStatus represents the lifecycle state of a task.
type TaskStatus string

const (
	StatusActive   TaskStatus = "active"
	StatusDisabled TaskStatus = "disabled"
)

// ScheduleType identifies how a task is scheduled.
type ScheduleType string

const (
	ScheduleManual ScheduleType = "manual"
	ScheduleOnce   ScheduleType = "once"
	ScheduleCron   ScheduleType = "cron"
)

// RunStatus reports the outcome of the latest run.
type RunStatus string

const (
	RunIdle      RunStatus = "idle"
	RunExecuting RunStatus = "running"
	RunSuccess   RunStatus = "success"
	RunFailed    RunStatus = "failed"
)

// Task represents the application task entity (session template).
type Task struct {
	ID              uint             `json:"id"`
	Title           string           `json:"title"`
	Description     string           `json:"description"`
	TaskType        TaskType         `json:"taskType"`
	ProjectID       uint             `json:"projectId"`
	Backend         agents.BackendID `json:"backend"`
	Model           string           `json:"model"`
	PromptTemplate  string           `json:"promptTemplate"`

	// Scheduling configuration
	ScheduleType    ScheduleType `json:"scheduleType"`
	ScheduledAt     *time.Time   `json:"scheduledAt,omitempty"`
	CronExpr        string       `json:"cronExpr,omitempty"`
	ScheduleEnabled bool         `json:"scheduleEnabled"`
	LastRunAt       *time.Time   `json:"lastRunAt,omitempty"`
	NextRunAt       *time.Time   `json:"nextRunAt,omitempty"`
	LastRunStatus   RunStatus    `json:"lastRunStatus,omitempty"`
	Status          TaskStatus   `json:"status"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// TaskRun represents one concrete execution of a task.
type TaskRun struct {
	ID          uint       `json:"id"`
	TaskID      uint       `json:"taskId"`
	SessionID   string     `json:"sessionId"`
	TriggerType string     `json:"triggerType"` // "manual" | "once" | "cron"
	Status      string     `json:"status"`      // "running" | "success" | "failed"
	Prompt      string     `json:"prompt"`
	Error       string     `json:"error,omitempty"`
	TriggeredAt time.Time  `json:"triggeredAt"`
	FinishedAt  *time.Time `json:"finishedAt,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
}

// Validation errors.
var (
	ErrTaskNotFound               = errors.New("tasks: task not found")
	ErrInvalidTaskType            = errors.New("tasks: invalid task type")
	ErrFlowTasksNotImplementedYet = errors.New("tasks: flow tasks are not implemented yet (extension point ready)")
	ErrInvalidScheduleTime        = errors.New("tasks: invalid scheduled time")
	ErrInvalidCronExpr            = errors.New("tasks: invalid cron expression")
)
