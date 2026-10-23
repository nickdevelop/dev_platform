package tasks

import (
	"context"
	"errors"
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
	Delete(projectID uint, sessionID string) error
}

// Service manages task lifecycles, execution dispatch, and scheduling.
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
	Title           string           `json:"title"`
	Description     string           `json:"description"`
	TaskType        TaskType         `json:"taskType"`
	ProjectID       uint             `json:"projectId"`
	Backend         agents.BackendID `json:"backend"`
	Model           string           `json:"model"`
	PromptTemplate  string           `json:"promptTemplate"`
	ScheduleType    ScheduleType     `json:"scheduleType"`
	ScheduledAt     *time.Time       `json:"scheduledAt"`
	CronExpr        string           `json:"cronExpr"`
	ScheduleEnabled bool             `json:"scheduleEnabled"`
}

// UpdateTaskRequest carries arguments to update task properties.
type UpdateTaskRequest struct {
	Title           *string           `json:"title"`
	Description     *string           `json:"description"`
	ProjectID       *uint             `json:"projectId"`
	Backend         *agents.BackendID `json:"backend"`
	Model           *string           `json:"model"`
	PromptTemplate  *string           `json:"promptTemplate"`
	ScheduleType    *ScheduleType     `json:"scheduleType"`
	ScheduledAt     *time.Time        `json:"scheduledAt"`
	CronExpr        *string           `json:"cronExpr"`
	ScheduleEnabled *bool             `json:"scheduleEnabled"`
	Status          *TaskStatus       `json:"status"`
}

// Create creates a new task (session template).
func (s *Service) Create(ctx context.Context, req CreateTaskRequest) (Task, error) {
	tt := req.TaskType
	if tt == "" {
		tt = TaskTypeStandard
	}
	st := req.ScheduleType
	if st == "" {
		st = ScheduleManual
	}

	t := Task{
		Title:           strings.TrimSpace(req.Title),
		Description:     strings.TrimSpace(req.Description),
		TaskType:        tt,
		ProjectID:       req.ProjectID,
		Backend:         req.Backend,
		Model:           strings.TrimSpace(req.Model),
		PromptTemplate:  strings.TrimSpace(req.PromptTemplate),
		ScheduleType:    st,
		ScheduledAt:     req.ScheduledAt,
		CronExpr:        strings.TrimSpace(req.CronExpr),
		ScheduleEnabled: req.ScheduleEnabled,
		LastRunStatus:   RunIdle,
		Status:          StatusActive,
	}

	// Validate task with its executor
	executor, err := ResolveExecutor(t.TaskType, s.sessions, s.repo)
	if err != nil {
		return Task{}, err
	}
	if err := executor.Validate(t); err != nil {
		return Task{}, err
	}

	// Validate scheduling settings
	if err := ValidateScheduleConfig(t); err != nil {
		return Task{}, err
	}

	if t.ScheduleEnabled && t.ScheduleType != ScheduleManual {
		next, err := CalculateNextRun(t, time.Now().UTC())
		if err != nil {
			return Task{}, err
		}
		t.NextRunAt = next
	}

	return s.repo.Create(t)
}

// Get returns one task by ID, syncing any running executions first.
func (s *Service) Get(id uint) (Task, error) {
	if s.sessions != nil {
		t, err := s.repo.Get(id)
		if err == nil && t.LastRunStatus == RunExecuting {
			if synced, err := s.SyncTaskStatus(context.Background(), id); err == nil {
				return synced, nil
			}
		}
	}
	return s.repo.Get(id)
}

// List returns tasks matching the filter, syncing live runs first.
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
		if t.LastRunStatus == RunExecuting {
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
	if req.PromptTemplate != nil {
		t.PromptTemplate = strings.TrimSpace(*req.PromptTemplate)
	}
	if req.ScheduleType != nil {
		t.ScheduleType = *req.ScheduleType
	}
	if req.ScheduledAt != nil {
		t.ScheduledAt = req.ScheduledAt
	}
	if req.CronExpr != nil {
		t.CronExpr = strings.TrimSpace(*req.CronExpr)
	}
	if req.ScheduleEnabled != nil {
		t.ScheduleEnabled = *req.ScheduleEnabled
	}
	if req.Status != nil {
		t.Status = *req.Status
	}

	if err := ValidateScheduleConfig(t); err != nil {
		return Task{}, err
	}

	if t.ScheduleEnabled && t.ScheduleType != ScheduleManual {
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

// Delete removes a task and cleans up sessions created by this task from the platform's index.
func (s *Service) Delete(id uint) error {
	t, err := s.repo.Get(id)
	if err != nil {
		return err
	}

	// Collect session IDs created by this task
	runs, _ := s.repo.ListRuns(id)
	sessionIDs := make(map[string]bool)
	for _, run := range runs {
		if run.SessionID != "" {
			sessionIDs[run.SessionID] = true
		}
	}

	// Remove these sessions from the platform index (underlying native sessions untouched)
	if s.sessions != nil {
		for sid := range sessionIDs {
			_ = s.sessions.Delete(t.ProjectID, sid)
		}
	}

	return s.repo.Delete(id)
}

// TriggerExecution executes the task using its configured executor strategy.
func (s *Service) TriggerExecution(ctx context.Context, id uint, triggerType ...string) (TaskRun, error) {
	t, err := s.repo.Get(id)
	if err != nil {
		return TaskRun{}, err
	}

	trig := "manual"
	if len(triggerType) > 0 && triggerType[0] != "" {
		trig = triggerType[0]
	}

	executor, err := ResolveExecutor(t.TaskType, s.sessions, s.repo)
	if err != nil {
		return TaskRun{}, err
	}

	return executor.Execute(ctx, t, trig)
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
				_ = s.repo.Update(t)
			case "failed":
				t.LastRunStatus = RunFailed
				_ = s.repo.Update(t)
			case "running":
				t.LastRunStatus = RunExecuting
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

// GetTaskSessionIDs returns all session IDs created by tasks in the given project (or all if projectID is 0).
func (s *Service) GetTaskSessionIDs(projectID uint) ([]string, error) {
	return s.repo.GetTaskSessionIDs(projectID)
}

// ReconcileOrphans marks any lingering 'running' task runs from a previous
// platform process as 'interrupted'. It runs once during platform startup.
func (s *Service) ReconcileOrphans(ctx context.Context) (int64, error) {
	return s.repo.ReconcileOrphaned(time.Now().UTC())
}
