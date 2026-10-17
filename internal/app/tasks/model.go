package tasks

import (
	"errors"
	"time"

	"devplatform/internal/core/agents"
)

// FlowType identifies the orchestration flow kind.
type FlowType string

const (
	FlowTransactional FlowType = "transactional"
	FlowDevelopment   FlowType = "development"
)

// StepStatus represents the progress state of a step.
type StepStatus string

const (
	StepPending    StepStatus = "pending"
	StepInProgress StepStatus = "in_progress"
	StepConfirmed  StepStatus = "confirmed"
	StepCompleted  StepStatus = "completed"
)

// TaskStatus represents the lifecycle state of a task.
type TaskStatus string

const (
	StatusDraft             TaskStatus = "draft"
	StatusInProgress        TaskStatus = "in_progress"
	StatusReadyForExecution TaskStatus = "ready_for_execution"
	StatusExecuting         TaskStatus = "executing"
	StatusCompleted         TaskStatus = "completed"
	StatusFailed            TaskStatus = "failed"
)

// ScheduleType identifies how a task is scheduled.
type ScheduleType string

const (
	ScheduleNone ScheduleType = "none"
	ScheduleOnce ScheduleType = "once"
	ScheduleCron ScheduleType = "cron"
)

// RunStatus reports the outcome of the latest scheduled run.
type RunStatus string

const (
	RunIdle             RunStatus = "idle"
	RunExecuting        RunStatus = "running"
	RunSuccess          RunStatus = "success"
	RunFailed           RunStatus = "failed"
	RunSkippedNotReady  RunStatus = "skipped_not_ready"
)

// StepDef defines one step in an orchestration flow.
type StepDef struct {
	Index       int    `json:"index"`
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Prompt      string `json:"prompt"`
	IsFinal     bool   `json:"isFinal"`
}

// FlowDef defines an orchestration flow.
type FlowDef struct {
	Type        FlowType  `json:"type"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Steps       []StepDef `json:"steps"`
}

// FlowTransactionalDef is the 2-step transactional workflow definition.
var FlowTransactionalDef = FlowDef{
	Type:        FlowTransactional,
	Name:        "事务型",
	Description: "包含任务分析与任务执行两阶段。任务分析阶段由 Agent 分析任务、识别歧义与不确定点并输出可直接执行计划；执行阶段每次新起会话免打断执行。",
	Steps: []StepDef{
		{
			Index:       1,
			Key:         "analysis",
			Name:        "任务分析",
			Description: "Agent 分析任务需求，识别信息是否有歧义、执行过程是否存在不确定点与潜在风险，全面与用户确认清楚，并输出可直接执行的执行计划。",
			Prompt:      "【任务分析阶段】请仔细分析当前任务需求，重点识别用户提供的信息是否存在歧义、不明确或遗漏之处，全面审查后续执行过程中可能遇到的不确定点、环境依赖、破坏性变更与潜在风险。请列出所有需要向用户提问和确认的事项清单，并在对话中逐一确认清楚。确认完成后，请输出一份结构清晰、步骤具体、可以直接执行的执行方案/计划，作为下一步自动执行的依据。",
			IsFinal:     false,
		},
		{
			Index:       2,
			Key:         "execute",
			Name:        "任务执行",
			Description: "使用任务分析阶段沉淀的执行方案提示词，新启动独立会话免打断执行任务。",
			Prompt:      "【任务执行阶段】前期任务分析与澄清已全部完成。请严格依据已确认的执行方案开始执行，全程自主处理常规问题，避免额外交互打断。",
			IsFinal:     true,
		},
	},
}

// FlowDevelopmentDef is the 4-step development workflow definition.
var FlowDevelopmentDef = FlowDef{
	Type:        FlowDevelopment,
	Name:        "开发类",
	Description: "包含需求分析、需求设计、制定计划、执行四个阶段，在制定计划阶段完成全部事项确认。",
	Steps: []StepDef{
		{
			Index:       1,
			Key:         "analysis",
			Name:        "需求分析",
			Description: "分析需求背景、目标及边界。",
			Prompt:      "【需求分析阶段】请深入分析本次开发任务的业务背景、功能目标、输入输出和边界条件，输出清晰的需求分析总结。",
			IsFinal:     false,
		},
		{
			Index:       2,
			Key:         "design",
			Name:        "需求设计",
			Description: "技术方案与架构设计。",
			Prompt:      "【需求设计阶段】请基于需求分析，进行系统与技术设计，包括技术方案、架构分层、模块改动点、接口或数据结构定义，并说明影响面。",
			IsFinal:     false,
		},
		{
			Index:       3,
			Key:         "plan",
			Name:        "制定计划",
			Description: "制定执行计划，并将所有确认的事项确认清楚，避免在最后执行过程中打断。",
			Prompt:      "【制定计划与确认阶段】请制定详尽的分步执行计划与测试验证步骤。特别注意：必须在这一阶段将所有技术选型、修改策略、依赖安装、破坏性变更等所有需要确认的事项与用户确认清楚，确保进入下一阶段后能够完全自主执行、不再打断。",
			IsFinal:     false,
		},
		{
			Index:       4,
			Key:         "execute",
			Name:        "执行",
			Description: "按照已制定的计划自动执行开发与测试。",
			Prompt:      "【执行阶段】前期需求分析、设计方案及执行计划均已明确且确认完毕。请严格依据已确认的执行计划进行代码修改、构建与验证，免打断完成开发任务。",
			IsFinal:     true,
		},
	},
}

// GetFlowDef returns the flow definition for a given flow type.
func GetFlowDef(t FlowType) (FlowDef, bool) {
	switch t {
	case FlowTransactional:
		return FlowTransactionalDef, true
	case FlowDevelopment:
		return FlowDevelopmentDef, true
	default:
		return FlowDef{}, false
	}
}

// Task represents the application task entity.
type Task struct {
	ID              uint             `json:"id"`
	Title           string           `json:"title"`
	Description     string           `json:"description"`
	ProjectID       uint             `json:"projectId"`
	Backend         agents.BackendID `json:"backend"`
	Model           string           `json:"model"`
	FlowType        FlowType         `json:"flowType"`
	CurrentStep     int              `json:"currentStep"`
	StepStatus      StepStatus       `json:"stepStatus"`
	Status          TaskStatus       `json:"status"`
	IsConfirmed     bool             `json:"isConfirmed"`
	StepsData       string           `json:"stepsData"`
	ActiveSessionID string           `json:"activeSessionId"`

	// Scheduling configuration
	ScheduleType    ScheduleType `json:"scheduleType"`
	ScheduledAt     *time.Time   `json:"scheduledAt,omitempty"`
	CronExpr        string       `json:"cronExpr,omitempty"`
	ScheduleEnabled bool         `json:"scheduleEnabled"`
	LastRunAt       *time.Time   `json:"lastRunAt,omitempty"`
	NextRunAt       *time.Time   `json:"nextRunAt,omitempty"`
	LastRunStatus   RunStatus    `json:"lastRunStatus,omitempty"`
	ExecutionPrompt string       `json:"executionPrompt,omitempty"`

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
	ErrTaskNotFound         = errors.New("tasks: task not found")
	ErrInvalidFlowType      = errors.New("tasks: invalid flow type")
	ErrInvalidStep          = errors.New("tasks: invalid step")
	ErrStepNotConfirmed     = errors.New("tasks: previous steps not confirmed")
	ErrScheduleNotAllowed   = errors.New("tasks: scheduling not allowed until final step is confirmed")
	ErrCronNotSupported     = errors.New("tasks: cron scheduling only supported for transactional tasks")
	ErrInvalidScheduleTime  = errors.New("tasks: invalid scheduled time")
	ErrInvalidCronExpr      = errors.New("tasks: invalid cron expression")
	ErrTaskAlreadyCompleted = errors.New("tasks: development task is completed and cannot be executed again")
)
