package tasks

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// TaskExecutor defines the contract for task execution strategies across heterogeneous task types.
type TaskExecutor interface {
	Execute(ctx context.Context, task Task, triggerType string) (TaskRun, error)
	Validate(task Task) error
}

// StandardTaskExecutor executes standard tasks (session templates).
type StandardTaskExecutor struct {
	launcher SessionLauncher
	repo     *Repository
}

// NewStandardTaskExecutor creates a StandardTaskExecutor.
func NewStandardTaskExecutor(launcher SessionLauncher, repo *Repository) *StandardTaskExecutor {
	return &StandardTaskExecutor{
		launcher: launcher,
		repo:     repo,
	}
}

// Validate checks whether the standard task template fields are valid.
func (e *StandardTaskExecutor) Validate(t Task) error {
	if strings.TrimSpace(t.Title) == "" {
		return errors.New("tasks: title is required")
	}
	if t.ProjectID == 0 {
		return errors.New("tasks: project is required")
	}
	if !t.Backend.Valid() {
		return errors.New("tasks: backend is required and must be valid")
	}
	if strings.TrimSpace(t.Model) == "" {
		return errors.New("tasks: model is required")
	}
	return nil
}

// Execute triggers execution of a standard task by creating a new Agent session.
func (e *StandardTaskExecutor) Execute(ctx context.Context, task Task, triggerType string) (TaskRun, error) {
	if e.launcher == nil {
		return TaskRun{}, errors.New("tasks: session service not configured")
	}

	if triggerType == "" {
		triggerType = "manual"
	}

	now := time.Now().UTC()
	task.LastRunAt = &now
	task.LastRunStatus = RunExecuting

	// Formulate session title and prompt
	sessTitle := fmt.Sprintf("[%s执行] %s", triggerType, task.Title)
	execPrompt := strings.TrimSpace(task.PromptTemplate)
	if execPrompt == "" {
		execPrompt = fmt.Sprintf("执行任务：%s\n%s", task.Title, task.Description)
	}

	outcome, err := e.launcher.Create(ctx, task.ProjectID, task.Backend, sessTitle, execPrompt, task.Model)
	if err != nil {
		task.LastRunStatus = RunFailed
		_ = e.repo.Update(task)
		return TaskRun{}, fmt.Errorf("tasks: execute create session: %w", err)
	}

	run := TaskRun{
		TaskID:      task.ID,
		SessionID:   outcome.Session.ID,
		TriggerType: triggerType,
		Status:      "running",
		Prompt:      execPrompt,
		TriggeredAt: now,
	}
	createdRun, err := e.repo.CreateRun(run)
	if err != nil {
		return TaskRun{}, fmt.Errorf("tasks: create run: %w", err)
	}

	if err := e.repo.Update(task); err != nil {
		return createdRun, fmt.Errorf("tasks: update task: %w", err)
	}

	return createdRun, nil
}

// FlowStarter defines the capability to initiate a collaborative flow run.
type FlowStarter interface {
	StartRun(ctx context.Context, task Task, triggerType string, initialPrompt string) (FlowRun, error)
}

// FlowTaskExecutor represents the executor for collaborative multi-agent flow tasks.
type FlowTaskExecutor struct {
	flowStarter FlowStarter
	repo        *Repository
}

// NewFlowTaskExecutor creates a FlowTaskExecutor.
func NewFlowTaskExecutor(flowStarter FlowStarter, repo *Repository) *FlowTaskExecutor {
	return &FlowTaskExecutor{
		flowStarter: flowStarter,
		repo:        repo,
	}
}

// Validate checks whether the flow task fields are valid.
func (e *FlowTaskExecutor) Validate(t Task) error {
	if strings.TrimSpace(t.Title) == "" {
		return errors.New("tasks: title is required")
	}
	if t.ProjectID == 0 {
		return errors.New("tasks: project is required")
	}
	if strings.TrimSpace(t.OrchestratorModel) == "" && strings.TrimSpace(t.Model) == "" {
		return ErrOrchestratorModelRequired
	}
	if e.repo != nil && t.ID > 0 {
		roles, err := e.repo.ListRoles(t.ID)
		if err == nil && len(roles) == 0 {
			return ErrNoRolesConfigured
		}
	}
	return nil
}

// Execute triggers execution of a collaborative flow task.
func (e *FlowTaskExecutor) Execute(ctx context.Context, task Task, triggerType string) (TaskRun, error) {
	if e.flowStarter == nil {
		return TaskRun{}, errors.New("tasks: flow engine not configured")
	}

	if triggerType == "" {
		triggerType = "manual"
	}

	now := time.Now().UTC()
	task.LastRunAt = &now
	task.LastRunStatus = RunExecuting

	run, err := e.flowStarter.StartRun(ctx, task, triggerType, task.PromptTemplate)
	if err != nil {
		task.LastRunStatus = RunFailed
		_ = e.repo.Update(task)
		return TaskRun{}, fmt.Errorf("tasks: execute start flow run: %w", err)
	}

	taskRun := TaskRun{
		TaskID:      task.ID,
		SessionID:   fmt.Sprintf("flow_%d", run.ID),
		TriggerType: triggerType,
		Status:      "running",
		Prompt:      run.Prompt,
		TriggeredAt: now,
	}
	createdRun, err := e.repo.CreateRun(taskRun)
	if err != nil {
		return TaskRun{}, fmt.Errorf("tasks: create task run for flow: %w", err)
	}

	return createdRun, nil
}

// ResolveExecutor returns the appropriate TaskExecutor based on TaskType.
func ResolveExecutor(taskType TaskType, launcher SessionLauncher, repo *Repository, flowStarter ...FlowStarter) (TaskExecutor, error) {
	switch taskType {
	case TaskTypeStandard, "":
		return NewStandardTaskExecutor(launcher, repo), nil
	case TaskTypeFlow:
		var starter FlowStarter
		if len(flowStarter) > 0 {
			starter = flowStarter[0]
		}
		return NewFlowTaskExecutor(starter, repo), nil
	default:
		return nil, ErrInvalidTaskType
	}
}
