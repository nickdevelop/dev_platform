package tasks

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"

	"devplatform/internal/app/events"
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

// FlowController defines operations to interact with active collaborative flows.
type FlowController interface {
	StartRun(ctx context.Context, task Task, triggerType string, initialPrompt string) (FlowRun, error)
	PostUserMessage(ctx context.Context, runID uint, content string) (FlowMessage, error)
	CancelRun(runID uint) error
	SubscribeEvents(runID uint) (<-chan FlowEvent, func())
}

// Service manages task lifecycles, execution dispatch, and scheduling.
type Service struct {
	repo           *Repository
	sessions       SessionLauncher
	flowController FlowController
	eventsMgr      *events.Manager
	flowSubs       map[uint]*flowSubscription
	subMu          sync.Mutex
}

// flowSubscription owns one engine subscription per subscribed run. The
// release func is idempotent: it is safe for both the unified bus
// OnUnsubscribe hook and the forwarding goroutine's exit path to call it,
// whichever happens first.
type flowSubscription struct {
	release func()
}

// NewService creates a new tasks service.
func NewService(repo *Repository, sessionLauncher SessionLauncher) *Service {
	return &Service{
		repo:      repo,
		sessions:  sessionLauncher,
		flowSubs:  make(map[uint]*flowSubscription),
	}
}

// SetFlowController injects the collaborative flow controller.
func (s *Service) SetFlowController(controller FlowController) {
	s.flowController = controller
}

// FlowController returns the active flow controller.
func (s *Service) FlowController() FlowController {
	return s.flowController
}

// CreateTaskRequest carries arguments to create a task.
type CreateTaskRequest struct {
	Title             string           `json:"title"`
	Description       string           `json:"description"`
	TaskType          TaskType         `json:"taskType"`
	ProjectID         uint             `json:"projectId"`
	Backend           agents.BackendID `json:"backend"`
	Model             string           `json:"model"`
	PromptTemplate    string           `json:"promptTemplate"`
	OrchestratorModel string           `json:"orchestratorModel"`
	ScheduleType      ScheduleType     `json:"scheduleType"`
	ScheduledAt       *time.Time       `json:"scheduledAt"`
	CronExpr          string           `json:"cronExpr"`
	ScheduleEnabled   bool             `json:"scheduleEnabled"`
}

// UpdateTaskRequest carries arguments to update task properties.
type UpdateTaskRequest struct {
	Title             *string           `json:"title"`
	Description       *string           `json:"description"`
	ProjectID         *uint             `json:"projectId"`
	Backend           *agents.BackendID `json:"backend"`
	Model             *string           `json:"model"`
	PromptTemplate    *string           `json:"promptTemplate"`
	OrchestratorModel *string           `json:"orchestratorModel"`
	ScheduleType      *ScheduleType     `json:"scheduleType"`
	ScheduledAt       *time.Time        `json:"scheduledAt"`
	CronExpr          *string           `json:"cronExpr"`
	ScheduleEnabled   *bool             `json:"scheduleEnabled"`
	Status            *TaskStatus       `json:"status"`
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

	orchestratorModel := strings.TrimSpace(req.OrchestratorModel)
	if orchestratorModel == "" && tt == TaskTypeFlow {
		orchestratorModel = strings.TrimSpace(req.Model)
	}

	t := Task{
		Title:             strings.TrimSpace(req.Title),
		Description:       strings.TrimSpace(req.Description),
		TaskType:          tt,
		ProjectID:         req.ProjectID,
		Backend:           req.Backend,
		Model:             strings.TrimSpace(req.Model),
		PromptTemplate:    strings.TrimSpace(req.PromptTemplate),
		OrchestratorModel: orchestratorModel,
		ScheduleType:      st,
		ScheduledAt:       req.ScheduledAt,
		CronExpr:          strings.TrimSpace(req.CronExpr),
		ScheduleEnabled:   req.ScheduleEnabled,
		LastRunStatus:     RunIdle,
		Status:            StatusActive,
	}

	// Validate task with its executor
	executor, err := ResolveExecutor(t.TaskType, s.sessions, s.repo, s.flowController)
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
	if req.OrchestratorModel != nil {
		t.OrchestratorModel = strings.TrimSpace(*req.OrchestratorModel)
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

	executor, err := ResolveExecutor(t.TaskType, s.sessions, s.repo, s.flowController)
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

// ListRoles queries roles for a task.
func (s *Service) ListRoles(taskID uint) ([]FlowRole, error) {
	return s.repo.ListRoles(taskID)
}

// GetRole finds a role by ID.
func (s *Service) GetRole(id uint) (FlowRole, error) {
	return s.repo.GetRole(id)
}

// CreateRole adds a new role.
func (s *Service) CreateRole(role FlowRole) (FlowRole, error) {
	if strings.TrimSpace(role.Name) == "" {
		return FlowRole{}, errors.New("tasks: role name cannot be empty")
	}
	if strings.TrimSpace(role.Model) == "" {
		return FlowRole{}, errors.New("tasks: role model cannot be empty")
	}
	now := time.Now().UTC()
	role.CreatedAt = now
	role.UpdatedAt = now
	return s.repo.CreateRole(role)
}

// UpdateRole updates an existing role.
func (s *Service) UpdateRole(id uint, role FlowRole) (FlowRole, error) {
	if strings.TrimSpace(role.Name) == "" {
		return FlowRole{}, errors.New("tasks: role name cannot be empty")
	}
	if strings.TrimSpace(role.Model) == "" {
		return FlowRole{}, errors.New("tasks: role model cannot be empty")
	}
	role.ID = id
	role.UpdatedAt = time.Now().UTC()
	if err := s.repo.UpdateRole(role); err != nil {
		return FlowRole{}, err
	}
	return s.repo.GetRole(id)
}

// DeleteRole removes a role.
func (s *Service) DeleteRole(id uint) error {
	return s.repo.DeleteRole(id)
}

// GetFlowRun retrieves a collaborative flow run by ID.
func (s *Service) GetFlowRun(id uint) (FlowRun, error) {
	return s.repo.GetFlowRun(id)
}

// ListFlowRuns retrieves all flow runs for a task.
func (s *Service) ListFlowRuns(taskID uint) ([]FlowRun, error) {
	return s.repo.ListFlowRuns(taskID)
}

// ListFlowMessages retrieves all chat messages for a flow run.
func (s *Service) ListFlowMessages(runID uint) ([]FlowMessage, error) {
	return s.repo.ListFlowMessages(runID)
}

// PostUserFlowMessage records user input and nudges the orchestrator.
func (s *Service) PostUserFlowMessage(ctx context.Context, runID uint, content string) (FlowMessage, error) {
	if strings.TrimSpace(content) == "" {
		return FlowMessage{}, errors.New("tasks: message content cannot be empty")
	}
	if s.flowController != nil {
		return s.flowController.PostUserMessage(ctx, runID, content)
	}

	now := time.Now().UTC()
	msg := FlowMessage{
		RunID:      runID,
		SenderType: SenderUser,
		SenderName: "用户",
		Content:    content,
		Status:     FlowMsgComplete,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	return s.repo.CreateFlowMessage(msg)
}

// CancelFlowRun aborts an active flow run.
func (s *Service) CancelFlowRun(runID uint) error {
	if s.flowController != nil {
		return s.flowController.CancelRun(runID)
	}
	run, err := s.repo.GetFlowRun(runID)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	run.Status = FlowRunCancelled
	run.FinishedAt = &now
	return s.repo.UpdateFlowRun(run)
}

// SubscribeFlowEvents opens an event stream for a flow run.
func (s *Service) SubscribeFlowEvents(runID uint) (<-chan FlowEvent, func(), error) {
	if s.flowController == nil {
		return nil, nil, errors.New("tasks: flow engine not configured")
	}
	ch, unsub := s.flowController.SubscribeEvents(runID)
	return ch, unsub, nil
}

// SetEventsManager attaches the platform-wide unified event manager.
func (s *Service) SetEventsManager(mgr *events.Manager) {
	s.eventsMgr = mgr
}

func (s *Service) ensureFlowSubscription(runID uint) {
	if s.flowController == nil || s.eventsMgr == nil {
		return
	}

	s.subMu.Lock()
	defer s.subMu.Unlock()

	if _, exists := s.flowSubs[runID]; exists {
		return
	}

	ch, unsub := s.flowController.SubscribeEvents(runID)
	var releaseOnce sync.Once
	sub := &flowSubscription{
		release: func() {
			releaseOnce.Do(unsub)
		},
	}
	s.flowSubs[runID] = sub

	go func() {
		defer func() {
			s.subMu.Lock()
			// Only reclaim the entry we own: a faster unsubscribe may have
			// already dropped it and a new subscription may have taken its
			// place meanwhile.
			if s.flowSubs[runID] == sub {
				delete(s.flowSubs, runID)
			}
			s.subMu.Unlock()
			sub.release()
		}()

		runIDStr := strconv.FormatUint(uint64(runID), 10)
		for ev := range ch {
			if s.eventsMgr != nil {
				_ = s.eventsMgr.BroadcastPayload(
					events.ChannelTasks,
					runIDStr,
					string(ev.Type),
					ev.Payload,
				)
			}
		}
	}()
}

// OnSubscribe implements events.ChannelHandler for the "tasks" channel.
func (s *Service) OnSubscribe(ctx context.Context, busID string, req events.SubscribeRequest) (any, error) {
	runID, err := strconv.ParseUint(req.TargetID, 10, 64)
	if err != nil || runID == 0 {
		return nil, events.ErrInvalidRequest
	}

	s.ensureFlowSubscription(uint(runID))
	return s.GetFlowRun(uint(runID))
}

// OnUnsubscribe implements events.ChannelHandler for the "tasks" channel.
// Dropping the last subscriber for a run tears down the flow engine
// subscription: unsub() closes the upstream channel, which ends the
// ensureFlowSubscription forwarding goroutine and removes its flowSubs entry,
// so long-lived services do not accumulate one goroutine per subscribed run.
func (s *Service) OnUnsubscribe(ctx context.Context, busID string, channel string, targetID string) error {
	if channel != events.ChannelTasks {
		return nil
	}
	runID, err := strconv.ParseUint(targetID, 10, 64)
	if err != nil || runID == 0 {
		return nil
	}

	s.subMu.Lock()
	sub, ok := s.flowSubs[uint(runID)]
	if ok {
		delete(s.flowSubs, uint(runID))
	}
	s.subMu.Unlock()

	if ok {
		sub.release()
	}
	return nil
}



