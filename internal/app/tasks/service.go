package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"devplatform/internal/app/sessions"
	"devplatform/internal/core/agents"
)

// SessionLauncher abstracts session creation and messaging to avoid tight coupling.
type SessionLauncher interface {
	Create(ctx context.Context, projectID uint, backend agents.BackendID, title, message, model string, images ...[]agents.ImageAttachment) (sessions.CreateOutcome, error)
	Send(ctx context.Context, projectID uint, sessionID, text, model string, images ...[]agents.ImageAttachment) (sessions.Session, error)
	Get(projectID uint, sessionID string) (sessions.Session, error)
}

// Service manages task lifecycles, flow advancement, scheduling and execution.
type Service struct {
	repo     *Repository
	sessions SessionLauncher
}

// NewService creates a new tasks service.
func NewService(repo *Repository, sessionLauncher SessionLauncher) *Service {
	return &Service{
		repo:     repo,
		sessions: sessionLauncher,
	}
}

// CreateTaskRequest carries arguments to create a task.
type CreateTaskRequest struct {
	Title       string           `json:"title"`
	Description string           `json:"description"`
	ProjectID   uint             `json:"projectId"`
	Backend     agents.BackendID `json:"backend"`
	Model       string           `json:"model"`
	FlowType    FlowType         `json:"flowType"`
}

// UpdateTaskRequest carries arguments to update task properties.
type UpdateTaskRequest struct {
	Title       *string           `json:"title"`
	Description *string           `json:"description"`
	ProjectID   *uint             `json:"projectId"`
	Backend     *agents.BackendID `json:"backend"`
	Model       *string           `json:"model"`
}

// AdvanceStepRequest carries data to confirm and advance the step.
type AdvanceStepRequest struct {
	Confirmed       bool   `json:"confirmed"`
	NextStep        *int   `json:"nextStep,omitempty"`
	Notes           string `json:"notes,omitempty"`
	DriverPrompt    string `json:"driverPrompt,omitempty"`
	ExecutionPrompt string `json:"executionPrompt,omitempty"`
}

// ScheduleConfigRequest carries schedule configuration for a task.
type ScheduleConfigRequest struct {
	ScheduleType    ScheduleType `json:"scheduleType"`
	ScheduledAt     *time.Time   `json:"scheduledAt"`
	CronExpr        string       `json:"cronExpr"`
	ScheduleEnabled bool         `json:"scheduleEnabled"`
}

// StepContextRecord stores notes and metadata for a step.
type StepContextRecord struct {
	StepIndex   int       `json:"stepIndex"`
	Confirmed   bool      `json:"confirmed"`
	Notes       string    `json:"notes,omitempty"`
	ConfirmedAt time.Time `json:"confirmedAt,omitempty"`
}

// Create creates a new task in draft state at step 1.
func (s *Service) Create(ctx context.Context, req CreateTaskRequest) (Task, error) {
	t := Task{
		Title:        strings.TrimSpace(req.Title),
		Description:  strings.TrimSpace(req.Description),
		ProjectID:    req.ProjectID,
		Backend:      req.Backend,
		Model:        strings.TrimSpace(req.Model),
		FlowType:     req.FlowType,
		CurrentStep:  1,
		StepStatus:   StepPending,
		Status:       StatusDraft,
		IsConfirmed:  false,
		ScheduleType: ScheduleNone,
	}

	if err := ValidateTaskCreation(t); err != nil {
		return Task{}, err
	}

	return s.repo.Create(t)
}

// Get returns one task by ID. If the task is currently executing, it syncs
// its status with the underlying agent session first.
func (s *Service) Get(id uint) (Task, error) {
	if s.sessions != nil {
		t, err := s.repo.Get(id)
		if err == nil && (t.Status == StatusExecuting || t.LastRunStatus == RunExecuting) {
			if synced, err := s.SyncTaskStatus(context.Background(), id); err == nil {
				return synced, nil
			}
		}
	}
	return s.repo.Get(id)
}

// List returns tasks matching the filter, syncing any executing tasks first.
func (s *Service) List(filter ListFilter) ([]Task, error) {
	list, err := s.repo.List(filter)
	if err != nil {
		return nil, err
	}
	if s.sessions == nil {
		return list, nil
	}

	hasExecuting := false
	for _, t := range list {
		if t.Status == StatusExecuting || t.LastRunStatus == RunExecuting {
			hasExecuting = true
			_, _ = s.SyncTaskStatus(context.Background(), t.ID)
		}
	}
	if hasExecuting {
		return s.repo.List(filter)
	}
	return list, nil
}

// Update updates task fields.
func (s *Service) Update(ctx context.Context, id uint, req UpdateTaskRequest) (Task, error) {
	t, err := s.repo.Get(id)
	if err != nil {
		return Task{}, err
	}

	if req.Title != nil {
		title := strings.TrimSpace(*req.Title)
		if title == "" {
			return Task{}, errors.New("tasks: title cannot be empty")
		}
		t.Title = title
	}
	if req.Description != nil {
		t.Description = strings.TrimSpace(*req.Description)
	}
	if req.ProjectID != nil {
		if *req.ProjectID == 0 {
			return Task{}, errors.New("tasks: invalid project id")
		}
		t.ProjectID = *req.ProjectID
	}
	if req.Backend != nil {
		if !req.Backend.Valid() {
			return Task{}, errors.New("tasks: invalid backend")
		}
		t.Backend = *req.Backend
	}
	if req.Model != nil {
		m := strings.TrimSpace(*req.Model)
		if m == "" {
			return Task{}, errors.New("tasks: model cannot be empty")
		}
		t.Model = m
	}

	if err := s.repo.Update(t); err != nil {
		return Task{}, err
	}
	return s.repo.Get(id)
}

// Delete removes a task.
func (s *Service) Delete(id uint) error {
	return s.repo.Delete(id)
}

// SaveExecutionPrompt saves the validated execution prompt for a transactional task,
// advancing it to ready_for_execution state.
func (s *Service) SaveExecutionPrompt(ctx context.Context, id uint, prompt string) (Task, error) {
	t, err := s.repo.Get(id)
	if err != nil {
		return Task{}, err
	}
	if t.FlowType != FlowTransactional {
		return Task{}, errors.New("tasks: execution prompt can only be set for transactional tasks")
	}
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return Task{}, errors.New("tasks: execution prompt cannot be empty")
	}

	t.ExecutionPrompt = prompt
	t.CurrentStep = 2
	t.StepStatus = StepConfirmed
	t.IsConfirmed = true
	t.Status = StatusReadyForExecution

	if err := s.repo.Update(t); err != nil {
		return Task{}, err
	}
	return s.repo.Get(id)
}

// AdvanceStep confirms the current step and advances to the next step if requested.
// For development tasks, when moving to a new step, it automatically delivers the stage
// driver prompt to the active session so the Agent steps in immediately.
func (s *Service) AdvanceStep(ctx context.Context, id uint, req AdvanceStepRequest) (Task, error) {
	t, err := s.repo.Get(id)
	if err != nil {
		return Task{}, err
	}

	finalStep := FinalStepIndex(t.FlowType)

	// Update step notes/data
	var records []StepContextRecord
	if t.StepsData != "" {
		_ = json.Unmarshal([]byte(t.StepsData), &records)
	}
	records = append(records, StepContextRecord{
		StepIndex:   t.CurrentStep,
		Confirmed:   req.Confirmed,
		Notes:       req.Notes,
		ConfirmedAt: time.Now().UTC(),
	})
	encodedData, _ := json.Marshal(records)
	t.StepsData = string(encodedData)

	if req.Confirmed {
		t.StepStatus = StepConfirmed
	}

	// For transactional task, if execution prompt is supplied on confirmation
	if t.FlowType == FlowTransactional && req.ExecutionPrompt != "" {
		t.ExecutionPrompt = strings.TrimSpace(req.ExecutionPrompt)
	}

	// Advance step
	oldStep := t.CurrentStep
	targetStep := t.CurrentStep
	if req.NextStep != nil {
		targetStep = *req.NextStep
	} else if req.Confirmed && t.CurrentStep < finalStep {
		targetStep = t.CurrentStep + 1
	}

	if targetStep < 1 || targetStep > finalStep {
		return Task{}, ErrInvalidStep
	}

	if targetStep > t.CurrentStep && !req.Confirmed {
		return Task{}, ErrStepNotConfirmed
	}

	t.CurrentStep = targetStep
	stepChanged := targetStep != oldStep

	if targetStep == finalStep && req.Confirmed {
		t.IsConfirmed = true
		t.Status = StatusReadyForExecution
	} else if targetStep == finalStep {
		// Just reached final step, waiting for final confirmation
		t.Status = StatusInProgress
	} else {
		t.Status = StatusInProgress
		t.IsConfirmed = false
	}

	// For development tasks, if step changed and session is active, invoke Agent with DriverPrompt
	if t.FlowType == FlowDevelopment && stepChanged && t.ActiveSessionID != "" && s.sessions != nil {
		stepDef, _ := GetStepDef(t.FlowType, targetStep)
		driverText := strings.TrimSpace(req.DriverPrompt)
		if driverText == "" {
			driverText = fmt.Sprintf("【推进至第 %d 步：%s】\n%s", targetStep, stepDef.Name, stepDef.Prompt)
		}
		// Send prompt to existing active session so Agent intervenes in this step
		if _, err := s.sessions.Send(ctx, t.ProjectID, t.ActiveSessionID, driverText, t.Model); err != nil {
			return Task{}, fmt.Errorf("tasks: advance send driver prompt: %w", err)
		}
		t.StepStatus = StepInProgress
	}

	if err := s.repo.Update(t); err != nil {
		return Task{}, err
	}
	return s.repo.Get(id)
}

// UpdateSchedule updates and validates scheduling settings for a task.
func (s *Service) UpdateSchedule(ctx context.Context, id uint, req ScheduleConfigRequest) (Task, error) {
	t, err := s.repo.Get(id)
	if err != nil {
		return Task{}, err
	}

	t.ScheduleType = req.ScheduleType
	t.ScheduledAt = req.ScheduledAt
	t.CronExpr = strings.TrimSpace(req.CronExpr)
	t.ScheduleEnabled = req.ScheduleEnabled

	if err := ValidateScheduleConfig(t); err != nil {
		return Task{}, err
	}

	if t.ScheduleEnabled && t.ScheduleType != ScheduleNone {
		next, err := CalculateNextRun(t, time.Now().UTC())
		if err != nil {
			return Task{}, err
		}
		t.NextRunAt = next
	} else {
		t.NextRunAt = nil
	}

	if err := s.repo.Update(t); err != nil {
		return Task{}, err
	}
	return s.repo.Get(id)
}

// CreateSessionForTask creates an AgentSession for the task or returns the active one.
func (s *Service) CreateSessionForTask(ctx context.Context, id uint) (Task, sessions.Session, error) {
	t, err := s.repo.Get(id)
	if err != nil {
		return Task{}, sessions.Session{}, err
	}

	if s.sessions == nil {
		return Task{}, sessions.Session{}, errors.New("tasks: session service not configured")
	}

	// If already has an active session, verify it
	if t.ActiveSessionID != "" {
		existing, err := s.sessions.Get(t.ProjectID, t.ActiveSessionID)
		if err == nil {
			return t, existing, nil
		}
	}

	// Create new session with prompt for the current step
	stepDef, _ := GetStepDef(t.FlowType, t.CurrentStep)
	title := fmt.Sprintf("[%s] %s", stepDef.Name, t.Title)
	initialPrompt := fmt.Sprintf("【%s】\n任务名称：%s\n任务描述：%s\n\n阶段指引：\n%s",
		stepDef.Name, t.Title, t.Description, stepDef.Prompt)

	outcome, err := s.sessions.Create(ctx, t.ProjectID, t.Backend, title, initialPrompt, t.Model)
	if err != nil {
		return Task{}, sessions.Session{}, fmt.Errorf("tasks: create session: %w", err)
	}

	t.ActiveSessionID = outcome.Session.ID
	t.Status = StatusInProgress
	t.StepStatus = StepInProgress

	if err := s.repo.Update(t); err != nil {
		return Task{}, sessions.Session{}, err
	}

	updated, _ := s.repo.Get(id)
	return updated, outcome.Session, nil
}

// TriggerExecution executes the final step of the task.
// For transactional tasks, it spawns a NEW independent AgentSession on each execution run.
// For development tasks, it executes in the existing active session.
// In both cases, the run is initially marked as "running" and the task status becomes "executing"
// until the agent's turn completes or fails.
func (s *Service) TriggerExecution(ctx context.Context, id uint, triggerType ...string) (TaskRun, error) {
	t, err := s.repo.Get(id)
	if err != nil {
		return TaskRun{}, err
	}

	trig := "manual"
	if len(triggerType) > 0 && triggerType[0] != "" {
		trig = triggerType[0]
	}

	// Development task cannot be executed again once completed
	if t.FlowType == FlowDevelopment && t.Status == StatusCompleted {
		return TaskRun{}, ErrTaskAlreadyCompleted
	}

	// Gatekeeper check: must be final step and confirmed
	if ok, checkErr := CanSchedule(t); !ok {
		t.LastRunStatus = RunSkippedNotReady
		now := time.Now().UTC()
		t.LastRunAt = &now
		if err := s.repo.Update(t); err != nil {
			return TaskRun{}, fmt.Errorf("tasks: update task status: %w", err)
		}
		return TaskRun{}, checkErr
	}

	if s.sessions == nil {
		return TaskRun{}, errors.New("tasks: session service not configured")
	}

	now := time.Now().UTC()
	t.LastRunAt = &now

	if t.FlowType == FlowTransactional {
		// TRANSACTIONAL: Spawn a new independent session on every run!
		execPrompt := t.ExecutionPrompt
		if strings.TrimSpace(execPrompt) == "" {
			finalStep := FinalStepIndex(t.FlowType)
			stepDef, _ := GetStepDef(t.FlowType, finalStep)
			execPrompt = fmt.Sprintf("【执行方案】\n任务：%s\n%s", t.Title, stepDef.Prompt)
		}

		sessTitle := fmt.Sprintf("[%s执行] %s", trig, t.Title)
		outcome, err := s.sessions.Create(ctx, t.ProjectID, t.Backend, sessTitle, execPrompt, t.Model)
		if err != nil {
			t.LastRunStatus = RunFailed
			t.Status = StatusFailed
			_ = s.repo.Update(t)
			return TaskRun{}, fmt.Errorf("tasks: execute create session: %w", err)
		}

		run := TaskRun{
			TaskID:      t.ID,
			SessionID:   outcome.Session.ID,
			TriggerType: trig,
			Status:      "running",
			Prompt:      execPrompt,
			TriggeredAt: now,
		}
		createdRun, err := s.repo.CreateRun(run)
		if err != nil {
			return TaskRun{}, fmt.Errorf("tasks: create run: %w", err)
		}

		t.ActiveSessionID = outcome.Session.ID
		t.LastRunStatus = RunExecuting
		t.Status = StatusExecuting
		if err := s.repo.Update(t); err != nil {
			return createdRun, fmt.Errorf("tasks: update task: %w", err)
		}

		return createdRun, nil
	}

	// DEVELOPMENT: Execute in existing active session
	finalStep := FinalStepIndex(t.FlowType)
	stepDef, _ := GetStepDef(t.FlowType, finalStep)
	execPrompt := fmt.Sprintf("【自动执行 - %s】\n任务：%s\n前期所有需求分析、设计与计划已明确确认完毕，执行过程免打断。\n请严格按照已确认的执行方案开始自动执行：\n%s",
		stepDef.Name, t.Title, stepDef.Prompt)

	t.LastRunStatus = RunExecuting
	t.Status = StatusExecuting

	var sessionID = t.ActiveSessionID
	if sessionID == "" {
		outcome, err := s.sessions.Create(ctx, t.ProjectID, t.Backend, t.Title+" (执行)", execPrompt, t.Model)
		if err != nil {
			t.LastRunStatus = RunFailed
			t.Status = StatusFailed
			_ = s.repo.Update(t)
			return TaskRun{}, fmt.Errorf("tasks: execute create session: %w", err)
		}
		sessionID = outcome.Session.ID
		t.ActiveSessionID = sessionID
	} else {
		_, err := s.sessions.Send(ctx, t.ProjectID, sessionID, execPrompt, t.Model)
		if err != nil {
			t.LastRunStatus = RunFailed
			t.Status = StatusFailed
			_ = s.repo.Update(t)
			return TaskRun{}, fmt.Errorf("tasks: execute send message: %w", err)
		}
	}

	run := TaskRun{
		TaskID:      t.ID,
		SessionID:   sessionID,
		TriggerType: trig,
		Status:      "running",
		Prompt:      execPrompt,
		TriggeredAt: now,
	}
	createdRun, err := s.repo.CreateRun(run)
	if err != nil {
		return TaskRun{}, fmt.Errorf("tasks: create run: %w", err)
	}

	if err := s.repo.Update(t); err != nil {
		return createdRun, fmt.Errorf("tasks: update task: %w", err)
	}

	return createdRun, nil
}

// SyncTaskStatus checks the underlying session for any running runs and updates
// the Task and TaskRun states accordingly.
func (s *Service) SyncTaskStatus(ctx context.Context, id uint) (Task, error) {
	t, err := s.repo.Get(id)
	if err != nil {
		return Task{}, err
	}
	if s.sessions == nil {
		return t, nil
	}

	runs, err := s.repo.ListRuns(t.ID)
	if err != nil {
		return t, nil
	}

	hasChanges := false
	now := time.Now().UTC()

	for _, run := range runs {
		if run.Status != "running" {
			continue
		}
		sess, err := s.sessions.Get(t.ProjectID, run.SessionID)
		if err != nil {
			if errors.Is(err, sessions.ErrSessionNotFound) {
				run.Status = "failed"
				run.Error = "session not found"
				run.FinishedAt = &now
				_ = s.repo.UpdateRun(run)
				hasChanges = true
			}
			continue
		}

		if sess.Turn == sessions.TurnRunning {
			// Agent is still running the turn
			continue
		}

		// Turn has ended (TurnIdle or unstated)
		if sess.Status == sessions.SessionStopped {
			run.Status = "failed"
			run.Error = "execution stopped by user"
			run.FinishedAt = &now
			_ = s.repo.UpdateRun(run)
			hasChanges = true
		} else if sess.Status == sessions.SessionDisconnected {
			run.Status = "failed"
			run.Error = "instance disconnected"
			run.FinishedAt = &now
			_ = s.repo.UpdateRun(run)
			hasChanges = true
		} else {
			// Succeeded
			run.Status = "success"
			run.FinishedAt = &now
			_ = s.repo.UpdateRun(run)
			hasChanges = true
		}
	}

	if hasChanges {
		updatedRuns, _ := s.repo.ListRuns(t.ID)
		if len(updatedRuns) > 0 {
			latest := updatedRuns[0]
			switch latest.Status {
			case "success":
				t.LastRunStatus = RunSuccess
				if t.FlowType == FlowDevelopment {
					t.Status = StatusCompleted
					t.StepStatus = StepCompleted
				} else {
					t.Status = StatusReadyForExecution
				}
				_ = s.repo.Update(t)
			case "failed":
				t.LastRunStatus = RunFailed
				t.Status = StatusFailed
				_ = s.repo.Update(t)
			case "running":
				t.LastRunStatus = RunExecuting
				t.Status = StatusExecuting
				_ = s.repo.Update(t)
			}
		}
	}

	return s.repo.Get(id)
}

// ListRuns returns all execution runs for a task, syncing status for live runs first.
func (s *Service) ListRuns(ctx context.Context, taskID uint) ([]TaskRun, error) {
	if s.sessions != nil {
		_, _ = s.SyncTaskStatus(ctx, taskID)
	}
	return s.repo.ListRuns(taskID)
}

