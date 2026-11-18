package groups

import (
	"time"

	"devplatform/internal/app/tasks"
	"devplatform/internal/app/tasks/flow"
)

// Provider implements flow.GroupProvider to allow flow.Engine to load group task metadata.
type Provider struct {
	repo *Repository
}

// NewProvider creates a new flow group provider.
func NewProvider(repo *Repository) *Provider {
	return &Provider{repo: repo}
}

// GetGroupTask loads the task, its parent group, and the group's member roles adapted to tasks.FlowRole.
func (p *Provider) GetGroupTask(taskID uint) (flow.GroupTaskInfo, flow.GroupInfo, []tasks.FlowRole, error) {
	task, err := p.repo.GetGroupTask(taskID)
	if err != nil {
		return flow.GroupTaskInfo{}, flow.GroupInfo{}, nil, err
	}
	group, err := p.repo.GetGroup(task.GroupID)
	if err != nil {
		return flow.GroupTaskInfo{}, flow.GroupInfo{}, nil, err
	}
	members, err := p.repo.ListGroupMembers(task.GroupID)
	if err != nil {
		return flow.GroupTaskInfo{}, flow.GroupInfo{}, nil, err
	}

	roles := make([]tasks.FlowRole, len(members))
	for i, m := range members {
		roles[i] = tasks.FlowRole{
			ID:           m.RoleID,
			TaskID:       task.ID,
			Name:         m.Name,
			Description:  m.Description,
			Backend:      m.Backend,
			Model:        m.Model,
			PromptID:     m.PromptID,
			SystemPrompt: m.SystemPrompt,
			Icon:         m.Icon,
			Color:        m.Color,
		}
	}

	taskInfo := flow.GroupTaskInfo{
		ID:          task.ID,
		GroupID:     task.GroupID,
		Title:       task.Title,
		Description: task.Description,
		ProjectID:   task.ProjectID,
		WorktreeID:  task.WorktreeID,
		Prompt:      task.Prompt,
	}

	groupInfo := flow.GroupInfo{
		ID:                 group.ID,
		Name:               group.Name,
		OrchestratorModel:  group.OrchestratorModel,
		OrchestratorPrompt: group.OrchestratorPrompt,
	}

	return taskInfo, groupInfo, roles, nil
}

// UpdateTaskLastRun updates execution status on the group task.
func (p *Provider) UpdateTaskLastRun(taskID uint, status string, runAt time.Time) error {
	task, err := p.repo.GetGroupTask(taskID)
	if err != nil {
		return err
	}
	task.LastRunAt = &runAt
	task.LastRunStatus = status
	return p.repo.UpdateGroupTask(task)
}
