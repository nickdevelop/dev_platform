package groups

import (
	"errors"
	"fmt"
	"time"

	"devplatform/internal/app/database"
	"devplatform/internal/app/tasks"
	"devplatform/internal/core/agents"

	"gorm.io/gorm"
)

// Repository provides persistence for groups, team members, and group tasks.
type Repository struct {
	db *gorm.DB
}

// NewRepository creates a new groups repository.
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// ListGroups returns all team groups ordered by updated_at descending, with member and task counts.
func (r *Repository) ListGroups() ([]Group, error) {
	var rows []database.Group
	if err := r.db.Order("updated_at DESC, id DESC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("groups: list: %w", err)
	}

	result := make([]Group, len(rows))
	for i, row := range rows {
		var memberCount int64
		_ = r.db.Model(&database.GroupMember{}).Where("group_id = ?", row.ID).Count(&memberCount).Error

		var taskCount int64
		_ = r.db.Model(&database.GroupTask{}).Where("group_id = ?", row.ID).Count(&taskCount).Error

		result[i] = Group{
			ID:                 row.ID,
			Name:               row.Name,
			Description:        row.Description,
			OrchestratorModel:  row.OrchestratorModel,
			OrchestratorPrompt: row.OrchestratorPrompt,
			Icon:               row.Icon,
			Color:              row.Color,
			MemberCount:        int(memberCount),
			TaskCount:          int(taskCount),
			CreatedAt:          row.CreatedAt,
			UpdatedAt:          row.UpdatedAt,
		}
	}
	return result, nil
}

// GetGroup returns a single team group by id.
func (r *Repository) GetGroup(id uint) (Group, error) {
	var row database.Group
	if err := r.db.First(&row, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Group{}, ErrGroupNotFound
		}
		return Group{}, fmt.Errorf("groups: get: %w", err)
	}

	var memberCount int64
	_ = r.db.Model(&database.GroupMember{}).Where("group_id = ?", row.ID).Count(&memberCount).Error

	var taskCount int64
	_ = r.db.Model(&database.GroupTask{}).Where("group_id = ?", row.ID).Count(&taskCount).Error

	return Group{
		ID:                 row.ID,
		Name:               row.Name,
		Description:        row.Description,
		OrchestratorModel:  row.OrchestratorModel,
		OrchestratorPrompt: row.OrchestratorPrompt,
		Icon:               row.Icon,
		Color:              row.Color,
		MemberCount:        int(memberCount),
		TaskCount:          int(taskCount),
		CreatedAt:          row.CreatedAt,
		UpdatedAt:          row.UpdatedAt,
	}, nil
}

// CreateGroup persists a new group and associates initial member roles in a single transaction.
func (r *Repository) CreateGroup(g Group, roleIDs []uint) (Group, error) {
	now := time.Now().UTC()
	row := database.Group{
		Name:               g.Name,
		Description:        g.Description,
		OrchestratorModel:  g.OrchestratorModel,
		OrchestratorPrompt: g.OrchestratorPrompt,
		Icon:               g.Icon,
		Color:              g.Color,
		CreatedAt:          now,
		UpdatedAt:          now,
	}

	err := r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		for idx, roleID := range roleIDs {
			m := database.GroupMember{
				GroupID:   row.ID,
				RoleID:    roleID,
				SortOrder: idx,
				CreatedAt: now,
			}
			if err := tx.Create(&m).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return Group{}, fmt.Errorf("groups: create: %w", err)
	}

	g.ID = row.ID
	g.CreatedAt = row.CreatedAt
	g.UpdatedAt = row.UpdatedAt
	g.MemberCount = len(roleIDs)
	return g, nil
}

// UpdateGroup updates group properties.
func (r *Repository) UpdateGroup(g Group) error {
	updates := map[string]interface{}{
		"name":                g.Name,
		"description":         g.Description,
		"orchestrator_model":  g.OrchestratorModel,
		"orchestrator_prompt": g.OrchestratorPrompt,
		"icon":                g.Icon,
		"color":               g.Color,
		"updated_at":          time.Now().UTC(),
	}
	res := r.db.Model(&database.Group{}).Where("id = ?", g.ID).Updates(updates)
	if res.Error != nil {
		return fmt.Errorf("groups: update: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrGroupNotFound
	}
	return nil
}

// DeleteGroup deletes a group and cascades its members, tasks, and flow runs.
func (r *Repository) DeleteGroup(id uint) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		var runIDs []uint
		if err := tx.Model(&database.FlowRun{}).Where("group_id = ?", id).Pluck("id", &runIDs).Error; err != nil {
			return err
		}
		if len(runIDs) > 0 {
			if err := tx.Where("run_id IN ?", runIDs).Delete(&database.FlowMessage{}).Error; err != nil {
				return err
			}
			if err := tx.Where("id IN ?", runIDs).Delete(&database.FlowRun{}).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("group_id = ?", id).Delete(&database.GroupMember{}).Error; err != nil {
			return err
		}
		if err := tx.Where("group_id = ?", id).Delete(&database.GroupTask{}).Error; err != nil {
			return err
		}
		res := tx.Delete(&database.Group{}, id)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrGroupNotFound
		}
		return nil
	})
}

// ListGroupMembers returns member roles belonging to the group, joined with agent_roles.
func (r *Repository) ListGroupMembers(groupID uint) ([]GroupMemberDetail, error) {
	type memberJoinedRow struct {
		MemberID     uint      `gorm:"column:member_id"`
		GroupID      uint      `gorm:"column:group_id"`
		RoleID       uint      `gorm:"column:role_id"`
		SortOrder    int       `gorm:"column:sort_order"`
		MemberCreated time.Time `gorm:"column:member_created"`
		RoleName     string    `gorm:"column:role_name"`
		Description  string    `gorm:"column:description"`
		Backend      string    `gorm:"column:backend"`
		Model        string    `gorm:"column:model"`
		PromptID     uint      `gorm:"column:prompt_id"`
		SystemPrompt string    `gorm:"column:system_prompt"`
		Icon         string    `gorm:"column:icon"`
		Color        string    `gorm:"column:color"`
	}

	var rows []memberJoinedRow
	err := r.db.Table("group_members").
		Select("group_members.id as member_id, group_members.group_id, group_members.role_id, group_members.sort_order, group_members.created_at as member_created, agent_roles.name as role_name, agent_roles.description, agent_roles.backend, agent_roles.model, agent_roles.prompt_id, agent_roles.system_prompt, agent_roles.icon, agent_roles.color").
		Joins("INNER JOIN agent_roles ON agent_roles.id = group_members.role_id").
		Where("group_members.group_id = ?", groupID).
		Order("group_members.sort_order ASC, group_members.id ASC").
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("groups: list members: %w", err)
	}

	result := make([]GroupMemberDetail, len(rows))
	for i, r := range rows {
		result[i] = GroupMemberDetail{
			ID:           r.MemberID,
			GroupID:      r.GroupID,
			RoleID:       r.RoleID,
			SortOrder:    r.SortOrder,
			Name:         r.RoleName,
			Description:  r.Description,
			Backend:      agents.BackendID(r.Backend),
			Model:        r.Model,
			PromptID:     r.PromptID,
			SystemPrompt: r.SystemPrompt,
			Icon:         r.Icon,
			Color:        r.Color,
			CreatedAt:    r.MemberCreated,
		}
	}
	return result, nil
}

// SetGroupMembers replaces all member roles for a group in a transaction.
func (r *Repository) SetGroupMembers(groupID uint, roleIDs []uint) error {
	now := time.Now().UTC()
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("group_id = ?", groupID).Delete(&database.GroupMember{}).Error; err != nil {
			return err
		}
		for idx, roleID := range roleIDs {
			m := database.GroupMember{
				GroupID:   groupID,
				RoleID:    roleID,
				SortOrder: idx,
				CreatedAt: now,
			}
			if err := tx.Create(&m).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// ListGroupTasks returns all tasks belonging to a group.
func (r *Repository) ListGroupTasks(groupID uint) ([]GroupTask, error) {
	var rows []database.GroupTask
	err := r.db.Where("group_id = ?", groupID).Order("updated_at DESC, id DESC").Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("groups: list tasks: %w", err)
	}

	result := make([]GroupTask, len(rows))
	for i, row := range rows {
		result[i] = toGroupTask(row)
	}
	return result, nil
}

// GetGroupTask returns a single group task by ID.
func (r *Repository) GetGroupTask(id uint) (GroupTask, error) {
	var row database.GroupTask
	if err := r.db.First(&row, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return GroupTask{}, ErrGroupTaskNotFound
		}
		return GroupTask{}, fmt.Errorf("groups: get task: %w", err)
	}
	return toGroupTask(row), nil
}

// CreateGroupTask persists a new task for a group.
func (r *Repository) CreateGroupTask(task GroupTask) (GroupTask, error) {
	now := time.Now().UTC()
	row := database.GroupTask{
		GroupID:         task.GroupID,
		Title:           task.Title,
		Description:     task.Description,
		ProjectID:       task.ProjectID,
		WorktreeID:      task.WorktreeID,
		Prompt:          task.Prompt,
		ScheduleType:    task.ScheduleType,
		ScheduledAt:     task.ScheduledAt,
		CronExpr:        task.CronExpr,
		ScheduleEnabled: task.ScheduleEnabled,
		LastRunStatus:   "idle",
		Status:          "active",
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := r.db.Create(&row).Error; err != nil {
		return GroupTask{}, fmt.Errorf("groups: create task: %w", err)
	}
	return toGroupTask(row), nil
}

// UpdateGroupTask updates group task fields.
func (r *Repository) UpdateGroupTask(task GroupTask) error {
	updates := map[string]interface{}{
		"title":            task.Title,
		"description":      task.Description,
		"project_id":       task.ProjectID,
		"worktree_id":      task.WorktreeID,
		"prompt":           task.Prompt,
		"schedule_type":    task.ScheduleType,
		"scheduled_at":     task.ScheduledAt,
		"cron_expr":        task.CronExpr,
		"schedule_enabled": task.ScheduleEnabled,
		"status":           task.Status,
		"updated_at":       time.Now().UTC(),
	}
	if task.LastRunAt != nil {
		updates["last_run_at"] = task.LastRunAt
	}
	if task.LastRunStatus != "" {
		updates["last_run_status"] = task.LastRunStatus
	}
	if task.NextRunAt != nil {
		updates["next_run_at"] = task.NextRunAt
	}

	res := r.db.Model(&database.GroupTask{}).Where("id = ?", task.ID).Updates(updates)
	if res.Error != nil {
		return fmt.Errorf("groups: update task: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrGroupTaskNotFound
	}
	return nil
}

// DeleteGroupTask deletes a task and cascades its runs and messages.
func (r *Repository) DeleteGroupTask(id uint) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		var runIDs []uint
		if err := tx.Model(&database.FlowRun{}).Where("task_id = ? AND group_id > 0", id).Pluck("id", &runIDs).Error; err != nil {
			return err
		}
		if len(runIDs) > 0 {
			if err := tx.Where("run_id IN ?", runIDs).Delete(&database.FlowMessage{}).Error; err != nil {
				return err
			}
			if err := tx.Where("id IN ?", runIDs).Delete(&database.FlowRun{}).Error; err != nil {
				return err
			}
		}
		res := tx.Delete(&database.GroupTask{}, id)
		if res.Error != nil {
			return fmt.Errorf("groups: delete task: %w", res.Error)
		}
		if res.RowsAffected == 0 {
			return ErrGroupTaskNotFound
		}
		return nil
	})
}

// ListFlowRuns returns execution runs for a given group task ID.
func (r *Repository) ListFlowRuns(taskID uint) ([]tasks.FlowRun, error) {
	var rows []database.FlowRun
	err := r.db.Where("task_id = ? AND group_id > 0", taskID).Order("created_at DESC, id DESC").Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("groups: list flow runs: %w", err)
	}
	result := make([]tasks.FlowRun, len(rows))
	for i, r := range rows {
		result[i] = tasks.FlowRun{
			ID:                r.ID,
			TaskID:            r.TaskID,
			GroupID:           r.GroupID,
			ProjectID:         r.ProjectID,
			WorktreeID:        r.WorktreeID,
			Status:            tasks.FlowRunStatus(r.Status),
			CurrentStep:       r.CurrentStep,
			MaxSteps:          r.MaxSteps,
			OrchestratorModel: r.OrchestratorModel,
			TriggerType:       r.TriggerType,
			Prompt:            r.Prompt,
			Error:             r.Error,
			CreatedAt:         r.CreatedAt,
			UpdatedAt:         r.UpdatedAt,
			FinishedAt:        r.FinishedAt,
		}
	}
	return result, nil
}

// ListFlowMessages returns all flow messages for a given run ID in chronological order.
func (r *Repository) ListFlowMessages(runID uint) ([]tasks.FlowMessage, error) {
	var rows []database.FlowMessage
	err := r.db.Where("run_id = ?", runID).Order("id ASC").Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("groups: list flow messages: %w", err)
	}
	result := make([]tasks.FlowMessage, len(rows))
	for i, row := range rows {
		result[i] = tasks.FlowMessage{
			ID:           row.ID,
			RunID:        row.RunID,
			SenderType:   tasks.FlowSenderType(row.SenderType),
			SenderName:   row.SenderName,
			RoleID:       row.RoleID,
			TargetRoleID: row.TargetRoleID,
			Content:      row.Content,
			Reasoning:    row.Reasoning,
			SessionID:    row.SessionID,
			Status:       tasks.FlowMessageStatus(row.Status),
			CreatedAt:    row.CreatedAt,
			UpdatedAt:    row.UpdatedAt,
		}
	}
	return result, nil
}

func toGroupTask(row database.GroupTask) GroupTask {
	return GroupTask{
		ID:              row.ID,
		GroupID:         row.GroupID,
		Title:           row.Title,
		Description:     row.Description,
		ProjectID:       row.ProjectID,
		WorktreeID:      row.WorktreeID,
		Prompt:          row.Prompt,
		ScheduleType:    row.ScheduleType,
		ScheduledAt:     row.ScheduledAt,
		CronExpr:        row.CronExpr,
		ScheduleEnabled: row.ScheduleEnabled,
		LastRunAt:       row.LastRunAt,
		NextRunAt:       row.NextRunAt,
		LastRunStatus:   row.LastRunStatus,
		Status:          row.Status,
		CreatedAt:       row.CreatedAt,
		UpdatedAt:       row.UpdatedAt,
	}
}
