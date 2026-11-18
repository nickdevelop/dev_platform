package groups

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"

	"devplatform/internal/app/events"
	"devplatform/internal/app/tasks"
)

// GroupFlowController defines the execution controller for group collaborative tasks.
type GroupFlowController interface {
	StartGroupRun(ctx context.Context, groupID uint, taskID uint, triggerType string, initialPrompt string) (tasks.FlowRun, error)
	PostUserMessage(ctx context.Context, runID uint, content string) (tasks.FlowMessage, error)
	CancelRun(runID uint) error
	SubscribeEvents(runID uint) (<-chan tasks.FlowEvent, func())
}

// Service coordinates team groups, role assignments, group tasks and collaborative execution.
type Service struct {
	repo           *Repository
	flowController GroupFlowController
	eventsMgr      *events.Manager
	flowSubs       map[uint]*groupFlowSubscription
	subMu          sync.Mutex
}

type groupFlowSubscription struct {
	release func()
}

// NewService creates a new groups domain service.
func NewService(repo *Repository) *Service {
	return &Service{
		repo:     repo,
		flowSubs: make(map[uint]*groupFlowSubscription),
	}
}

// SetFlowController injects the flow engine controller.
func (s *Service) SetFlowController(controller GroupFlowController) {
	s.flowController = controller
}

// SetEventsManager attaches the platform unified event bus.
func (s *Service) SetEventsManager(mgr *events.Manager) {
	s.eventsMgr = mgr
}

// ListGroups returns all team groups with summary stats.
func (s *Service) ListGroups(ctx context.Context) ([]Group, error) {
	return s.repo.ListGroups()
}

// GetGroup returns a single group with its full member roles.
func (s *Service) GetGroup(ctx context.Context, id uint) (GroupDetail, error) {
	g, err := s.repo.GetGroup(id)
	if err != nil {
		return GroupDetail{}, err
	}
	members, err := s.repo.ListGroupMembers(id)
	if err != nil {
		return GroupDetail{}, err
	}
	return GroupDetail{
		Group:   g,
		Members: members,
	}, nil
}

// CreateGroup creates a new group and associates member roles.
func (s *Service) CreateGroup(ctx context.Context, req CreateGroupRequest) (GroupDetail, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return GroupDetail{}, ErrInvalidGroupName
	}
	model := strings.TrimSpace(req.OrchestratorModel)
	if model == "" {
		return GroupDetail{}, ErrInvalidOrchestratorModel
	}

	g := Group{
		Name:               name,
		Description:        strings.TrimSpace(req.Description),
		OrchestratorModel:  model,
		OrchestratorPrompt: strings.TrimSpace(req.OrchestratorPrompt),
		Icon:               strings.TrimSpace(req.Icon),
		Color:              strings.TrimSpace(req.Color),
	}

	created, err := s.repo.CreateGroup(g, req.RoleIDs)
	if err != nil {
		return GroupDetail{}, err
	}
	return s.GetGroup(ctx, created.ID)
}

// UpdateGroup updates group properties.
func (s *Service) UpdateGroup(ctx context.Context, id uint, req UpdateGroupRequest) (Group, error) {
	existing, err := s.repo.GetGroup(id)
	if err != nil {
		return Group{}, err
	}

	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			return Group{}, ErrInvalidGroupName
		}
		existing.Name = name
	}
	if req.Description != nil {
		existing.Description = strings.TrimSpace(*req.Description)
	}
	if req.OrchestratorModel != nil {
		model := strings.TrimSpace(*req.OrchestratorModel)
		if model == "" {
			return Group{}, ErrInvalidOrchestratorModel
		}
		existing.OrchestratorModel = model
	}
	if req.OrchestratorPrompt != nil {
		existing.OrchestratorPrompt = strings.TrimSpace(*req.OrchestratorPrompt)
	}
	if req.Icon != nil {
		existing.Icon = strings.TrimSpace(*req.Icon)
	}
	if req.Color != nil {
		existing.Color = strings.TrimSpace(*req.Color)
	}

	if err := s.repo.UpdateGroup(existing); err != nil {
		return Group{}, err
	}
	return s.repo.GetGroup(id)
}

// DeleteGroup deletes a group and cascades its members and tasks.
func (s *Service) DeleteGroup(ctx context.Context, id uint) error {
	return s.repo.DeleteGroup(id)
}

// SetGroupMembers sets all member roles for a group.
func (s *Service) SetGroupMembers(ctx context.Context, groupID uint, roleIDs []uint) ([]GroupMemberDetail, error) {
	if _, err := s.repo.GetGroup(groupID); err != nil {
		return nil, err
	}
	if err := s.repo.SetGroupMembers(groupID, roleIDs); err != nil {
		return nil, err
	}
	return s.repo.ListGroupMembers(groupID)
}

// ListGroupTasks returns all tasks belonging to a group.
func (s *Service) ListGroupTasks(ctx context.Context, groupID uint) ([]GroupTask, error) {
	if _, err := s.repo.GetGroup(groupID); err != nil {
		return nil, err
	}
	return s.repo.ListGroupTasks(groupID)
}

// GetGroupTask returns a single group task.
func (s *Service) GetGroupTask(ctx context.Context, id uint) (GroupTask, error) {
	return s.repo.GetGroupTask(id)
}

// CreateGroupTask creates a task assigned to a group.
func (s *Service) CreateGroupTask(ctx context.Context, groupID uint, req CreateGroupTaskRequest) (GroupTask, error) {
	if _, err := s.repo.GetGroup(groupID); err != nil {
		return GroupTask{}, err
	}

	title := strings.TrimSpace(req.Title)
	if title == "" {
		return GroupTask{}, ErrInvalidTaskTitle
	}
	if req.ProjectID == 0 {
		return GroupTask{}, ErrInvalidProject
	}
	prompt := strings.TrimSpace(req.Prompt)
	if prompt == "" {
		return GroupTask{}, ErrInvalidPrompt
	}

	st := req.ScheduleType
	if st == "" {
		st = "manual"
	}

	task := GroupTask{
		GroupID:         groupID,
		Title:           title,
		Description:     strings.TrimSpace(req.Description),
		ProjectID:       req.ProjectID,
		WorktreeID:      req.WorktreeID,
		Prompt:          prompt,
		ScheduleType:    st,
		ScheduledAt:     req.ScheduledAt,
		CronExpr:        strings.TrimSpace(req.CronExpr),
		ScheduleEnabled: req.ScheduleEnabled,
	}

	return s.repo.CreateGroupTask(task)
}

// UpdateGroupTask updates group task fields.
func (s *Service) UpdateGroupTask(ctx context.Context, id uint, req UpdateGroupTaskRequest) (GroupTask, error) {
	existing, err := s.repo.GetGroupTask(id)
	if err != nil {
		return GroupTask{}, err
	}

	if req.Title != nil {
		title := strings.TrimSpace(*req.Title)
		if title == "" {
			return GroupTask{}, ErrInvalidTaskTitle
		}
		existing.Title = title
	}
	if req.Description != nil {
		existing.Description = strings.TrimSpace(*req.Description)
	}
	if req.ProjectID != nil {
		if *req.ProjectID == 0 {
			return GroupTask{}, ErrInvalidProject
		}
		existing.ProjectID = *req.ProjectID
	}
	if req.WorktreeID != nil {
		existing.WorktreeID = *req.WorktreeID
	}
	if req.Prompt != nil {
		prompt := strings.TrimSpace(*req.Prompt)
		if prompt == "" {
			return GroupTask{}, ErrInvalidPrompt
		}
		existing.Prompt = prompt
	}
	if req.ScheduleType != nil {
		existing.ScheduleType = *req.ScheduleType
	}
	if req.ScheduledAt != nil {
		existing.ScheduledAt = req.ScheduledAt
	}
	if req.CronExpr != nil {
		existing.CronExpr = strings.TrimSpace(*req.CronExpr)
	}
	if req.ScheduleEnabled != nil {
		existing.ScheduleEnabled = *req.ScheduleEnabled
	}
	if req.Status != nil {
		existing.Status = *req.Status
	}

	if err := s.repo.UpdateGroupTask(existing); err != nil {
		return GroupTask{}, err
	}
	return s.repo.GetGroupTask(id)
}

// DeleteGroupTask deletes a group task.
func (s *Service) DeleteGroupTask(ctx context.Context, id uint) error {
	return s.repo.DeleteGroupTask(id)
}

// ListGroupTaskRuns returns execution runs for a group task.
func (s *Service) ListGroupTaskRuns(ctx context.Context, taskID uint) ([]tasks.FlowRun, error) {
	return s.repo.ListFlowRuns(taskID)
}

// TriggerGroupTask triggers the collaborative workflow for a group task.
func (s *Service) TriggerGroupTask(ctx context.Context, groupID uint, taskID uint, prompt string) (tasks.FlowRun, error) {
	task, err := s.repo.GetGroupTask(taskID)
	if err != nil {
		return tasks.FlowRun{}, err
	}
	if task.GroupID != groupID {
		return tasks.FlowRun{}, errors.New("groups: task does not belong to group")
	}

	if s.flowController == nil {
		return tasks.FlowRun{}, errors.New("groups: flow controller not configured")
	}

	run, err := s.flowController.StartGroupRun(ctx, groupID, taskID, "manual", prompt)
	if err != nil {
		return tasks.FlowRun{}, err
	}

	now := time.Now().UTC()
	task.LastRunAt = &now
	task.LastRunStatus = "running"
	_ = s.repo.UpdateGroupTask(task)

	return run, nil
}

// PostUserMessage delivers a message from the user into the active flow chat.
func (s *Service) PostUserMessage(ctx context.Context, runID uint, content string) (tasks.FlowMessage, error) {
	if s.flowController == nil {
		return tasks.FlowMessage{}, errors.New("groups: flow controller not configured")
	}
	return s.flowController.PostUserMessage(ctx, runID, content)
}

// CancelRun aborts an active collaborative run.
func (s *Service) CancelRun(ctx context.Context, runID uint) error {
	if s.flowController == nil {
		return errors.New("groups: flow controller not configured")
	}
	return s.flowController.CancelRun(runID)
}

// OnSubscribe implements events.ChannelHandler for the "groups" (or "tasks") channel.
func (s *Service) OnSubscribe(ctx context.Context, busID string, req events.SubscribeRequest) (any, error) {
	runID, err := strconv.ParseUint(req.TargetID, 10, 64)
	if err != nil || runID == 0 {
		return nil, events.ErrInvalidRequest
	}

	s.ensureForwarding(uint(runID))
	return nil, nil
}

// OnUnsubscribe implements events.ChannelHandler.
func (s *Service) OnUnsubscribe(ctx context.Context, busID string, channel string, targetID string) error {
	runID, err := strconv.ParseUint(targetID, 10, 64)
	if err != nil || runID == 0 {
		return nil
	}

	s.subMu.Lock()
	defer s.subMu.Unlock()
	if sub, ok := s.flowSubs[uint(runID)]; ok {
		delete(s.flowSubs, uint(runID))
		sub.release()
	}
	return nil
}

// ListFlowMessages retrieves all chat messages for a group task run.
func (s *Service) ListFlowMessages(ctx context.Context, runID uint) ([]tasks.FlowMessage, error) {
	return s.repo.ListFlowMessages(runID)
}

func (s *Service) ensureForwarding(runID uint) {
	if s.flowController == nil || s.eventsMgr == nil {
		return
	}
	s.subMu.Lock()
	defer s.subMu.Unlock()
	if _, exists := s.flowSubs[runID]; exists {
		return
	}

	eventCh, unsub := s.flowController.SubscribeEvents(runID)
	sub := &groupFlowSubscription{release: sync.OnceFunc(unsub)}
	s.flowSubs[runID] = sub

	runIDStr := strconv.FormatUint(uint64(runID), 10)
	go func() {
		defer func() {
			s.subMu.Lock()
			if s.flowSubs[runID] == sub {
				delete(s.flowSubs, runID)
			}
			s.subMu.Unlock()
			sub.release()
		}()

		for evt := range eventCh {
			if s.eventsMgr != nil {
				_ = s.eventsMgr.BroadcastPayload(
					events.ChannelTasks,
					runIDStr,
					string(evt.Type),
					evt.Payload,
				)
			}
		}
	}()
}
