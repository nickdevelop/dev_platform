package tasks

import (
	"errors"
	"time"

	"gorm.io/gorm"

	"devplatform/internal/app/database"
	"devplatform/internal/core/agents"
)

// Repository manages persistence for tasks.
type Repository struct {
	db *gorm.DB
}

// NewRepository creates a new task repository.
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// ListFilter carries filters for querying tasks.
type ListFilter struct {
	ProjectID *uint
	FlowType  *FlowType
	Status    *TaskStatus
}

func toDB(t Task) database.Task {
	return database.Task{
		ID:              t.ID,
		Title:           t.Title,
		Description:     t.Description,
		ProjectID:       t.ProjectID,
		Backend:         string(t.Backend),
		Model:           t.Model,
		FlowType:        string(t.FlowType),
		CurrentStep:     t.CurrentStep,
		StepStatus:      string(t.StepStatus),
		Status:          string(t.Status),
		IsConfirmed:     t.IsConfirmed,
		StepsData:       t.StepsData,
		ActiveSessionID: t.ActiveSessionID,
		ScheduleType:    string(t.ScheduleType),
		ScheduledAt:     t.ScheduledAt,
		CronExpr:        t.CronExpr,
		ScheduleEnabled: t.ScheduleEnabled,
		LastRunAt:       t.LastRunAt,
		NextRunAt:       t.NextRunAt,
		LastRunStatus:   string(t.LastRunStatus),
		CreatedAt:       t.CreatedAt,
		UpdatedAt:       t.UpdatedAt,
	}
}

func fromDB(row database.Task) Task {
	return Task{
		ID:              row.ID,
		Title:           row.Title,
		Description:     row.Description,
		ProjectID:       row.ProjectID,
		Backend:         agents.BackendID(row.Backend),
		Model:           row.Model,
		FlowType:        FlowType(row.FlowType),
		CurrentStep:     row.CurrentStep,
		StepStatus:      StepStatus(row.StepStatus),
		Status:          TaskStatus(row.Status),
		IsConfirmed:     row.IsConfirmed,
		StepsData:       row.StepsData,
		ActiveSessionID: row.ActiveSessionID,
		ScheduleType:    ScheduleType(row.ScheduleType),
		ScheduledAt:     row.ScheduledAt,
		CronExpr:        row.CronExpr,
		ScheduleEnabled: row.ScheduleEnabled,
		LastRunAt:       row.LastRunAt,
		NextRunAt:       row.NextRunAt,
		LastRunStatus:   RunStatus(row.LastRunStatus),
		CreatedAt:       row.CreatedAt,
		UpdatedAt:       row.UpdatedAt,
	}
}

// Create inserts a new task.
func (r *Repository) Create(t Task) (Task, error) {
	row := toDB(t)
	if err := r.db.Create(&row).Error; err != nil {
		return Task{}, err
	}
	return fromDB(row), nil
}

// Get finds a task by ID.
func (r *Repository) Get(id uint) (Task, error) {
	var row database.Task
	if err := r.db.First(&row, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Task{}, ErrTaskNotFound
		}
		return Task{}, err
	}
	return fromDB(row), nil
}

// Update updates an existing task.
func (r *Repository) Update(t Task) error {
	row := toDB(t)
	res := r.db.Model(&database.Task{}).Where("id = ?", t.ID).Updates(map[string]any{
		"title":             row.Title,
		"description":       row.Description,
		"project_id":        row.ProjectID,
		"backend":           row.Backend,
		"model":             row.Model,
		"flow_type":         row.FlowType,
		"current_step":      row.CurrentStep,
		"step_status":       row.StepStatus,
		"status":            row.Status,
		"is_confirmed":      row.IsConfirmed,
		"steps_data":        row.StepsData,
		"active_session_id": row.ActiveSessionID,
		"schedule_type":     row.ScheduleType,
		"scheduled_at":      row.ScheduledAt,
		"cron_expr":         row.CronExpr,
		"schedule_enabled":  row.ScheduleEnabled,
		"last_run_at":       row.LastRunAt,
		"next_run_at":       row.NextRunAt,
		"last_run_status":   row.LastRunStatus,
	})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrTaskNotFound
	}
	return nil
}

// Delete removes a task by ID.
func (r *Repository) Delete(id uint) error {
	res := r.db.Delete(&database.Task{}, "id = ?", id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrTaskNotFound
	}
	return nil
}

// List queries tasks with optional filtering.
func (r *Repository) List(filter ListFilter) ([]Task, error) {
	query := r.db.Model(&database.Task{})
	if filter.ProjectID != nil {
		query = query.Where("project_id = ?", *filter.ProjectID)
	}
	if filter.FlowType != nil {
		query = query.Where("flow_type = ?", string(*filter.FlowType))
	}
	if filter.Status != nil {
		query = query.Where("status = ?", string(*filter.Status))
	}

	var rows []database.Task
	if err := query.Order("id desc").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]Task, len(rows))
	for i, row := range rows {
		out[i] = fromDB(row)
	}
	return out, nil
}

// FindSchedulableTasks finds all tasks that have scheduling enabled and next_run_at <= now.
func (r *Repository) FindSchedulableTasks(now time.Time) ([]Task, error) {
	var rows []database.Task
	err := r.db.Where("schedule_enabled = ? AND next_run_at IS NOT NULL AND next_run_at <= ?", true, now).Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]Task, len(rows))
	for i, row := range rows {
		out[i] = fromDB(row)
	}
	return out, nil
}
