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
	OrchestratorModel string         `json:"orchestratorModel,omitempty"`

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

// FlowRole defines an agent role in a collaborative flow task.
type FlowRole struct {
	ID           uint             `json:"id"`
	TaskID       uint             `json:"taskId"` // 0 = global/preset, >0 = task-scoped
	Name         string           `json:"name"`
	Description  string           `json:"description"`
	Backend      agents.BackendID `json:"backend"`
	Model        string           `json:"model"`
	PromptID     uint             `json:"promptId"`
	SystemPrompt string           `json:"systemPrompt"`
	Icon         string           `json:"icon,omitempty"`
	Color        string           `json:"color,omitempty"`
	CreatedAt    time.Time        `json:"createdAt"`
	UpdatedAt    time.Time        `json:"updatedAt"`
}

// FlowRunStatus identifies the lifecycle state of a collaborative flow run.
type FlowRunStatus string

const (
	FlowRunRunning     FlowRunStatus = "running"
	FlowRunWaitingUser FlowRunStatus = "waiting_user"
	FlowRunCompleted   FlowRunStatus = "completed"
	FlowRunFailed      FlowRunStatus = "failed"
	FlowRunCancelled   FlowRunStatus = "cancelled"
)

// FlowRun records one concrete execution run of a collaborative flow task.
type FlowRun struct {
	ID                uint          `json:"id"`
	TaskID            uint          `json:"taskId"`
	ProjectID         uint          `json:"projectId"`
	Status            FlowRunStatus `json:"status"`
	CurrentStep       int           `json:"currentStep"`
	MaxSteps          int           `json:"maxSteps"`
	OrchestratorModel string        `json:"orchestratorModel"`
	TriggerType       string        `json:"triggerType"`
	Prompt            string        `json:"prompt"`
	Error             string        `json:"error,omitempty"`
	CreatedAt         time.Time     `json:"createdAt"`
	UpdatedAt         time.Time     `json:"updatedAt"`
	FinishedAt        *time.Time    `json:"finishedAt,omitempty"`
}

// FlowSenderType distinguishes participants in the collaborative chat.
type FlowSenderType string

const (
	SenderUser         FlowSenderType = "user"
	SenderOrchestrator FlowSenderType = "orchestrator"
	SenderAgent        FlowSenderType = "agent"
	SenderSystem       FlowSenderType = "system"
)

// FlowMessageStatus tracks the generation state of a flow message.
type FlowMessageStatus string

const (
	FlowMsgPending   FlowMessageStatus = "pending"
	FlowMsgStreaming FlowMessageStatus = "streaming"
	FlowMsgComplete  FlowMessageStatus = "complete"
	FlowMsgFailed    FlowMessageStatus = "failed"
)

// FlowMessage is one persisted message inside a collaborative flow conversation.
type FlowMessage struct {
	ID           uint              `json:"id"`
	RunID        uint              `json:"runId"`
	SenderType   FlowSenderType    `json:"senderType"`
	SenderName   string            `json:"senderName"`
	RoleID       uint              `json:"roleId,omitempty"`
	TargetRoleID uint              `json:"targetRoleId,omitempty"`
	Content      string            `json:"content"`
	Reasoning    string            `json:"reasoning,omitempty"`
	SessionID    string            `json:"sessionId,omitempty"`
	Status       FlowMessageStatus `json:"status"`
	CreatedAt    time.Time         `json:"createdAt"`
	UpdatedAt    time.Time         `json:"updatedAt"`
}

// FlowEvent carries one realtime update over SSE for a collaborative flow run.
type FlowEvent struct {
	Type    string `json:"type"`
	RunID   uint   `json:"runId"`
	Payload any    `json:"payload"`
}

// Validation errors.
var (
	ErrTaskNotFound               = errors.New("tasks: task not found")
	ErrRoleNotFound               = errors.New("tasks: flow role not found")
	ErrFlowRunNotFound            = errors.New("tasks: flow run not found")
	ErrInvalidTaskType            = errors.New("tasks: invalid task type")
	ErrFlowTasksNotImplementedYet = errors.New("tasks: flow tasks are not implemented yet (extension point ready)")
	ErrInvalidScheduleTime        = errors.New("tasks: invalid scheduled time")
	ErrInvalidCronExpr            = errors.New("tasks: invalid cron expression")
	ErrOrchestratorModelRequired  = errors.New("tasks: orchestrator model is required for flow tasks")
	ErrNoRolesConfigured          = errors.New("tasks: at least one agent role is required for flow tasks")
)
