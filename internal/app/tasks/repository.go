package tasks

import (
	"errors"
	"time"

	"gorm.io/gorm"

	"devplatform/internal/app/database"
	"devplatform/internal/core/agents"
)

// Repository manages persistence for tasks and runs.
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
	TaskType  *TaskType
	Status    *TaskStatus
	Keyword   string
}

func toDB(t Task) database.Task {
	return database.Task{
		ID:              t.ID,
		Title:           t.Title,
		Description:     t.Description,
		TaskType:        string(t.TaskType),
		ProjectID:       t.ProjectID,
		Backend:         string(t.Backend),
		Model:           t.Model,
		PromptTemplate:  t.PromptTemplate,
		ScheduleType:    string(t.ScheduleType),
		ScheduledAt:     t.ScheduledAt,
		CronExpr:        t.CronExpr,
		ScheduleEnabled: t.ScheduleEnabled,
		LastRunAt:       t.LastRunAt,
		NextRunAt:       t.NextRunAt,
		LastRunStatus:   string(t.LastRunStatus),
		Status:          string(t.Status),
		CreatedAt:       t.CreatedAt,
		UpdatedAt:       t.UpdatedAt,
	}
}

func fromDB(row database.Task) Task {
	tt := TaskType(row.TaskType)
	if tt == "" {
		tt = TaskTypeStandard
	}
	st := ScheduleType(row.ScheduleType)
	if st == "" {
		st = ScheduleManual
	}
	status := TaskStatus(row.Status)
	if status == "" {
		status = StatusActive
	}

	return Task{
		ID:              row.ID,
		Title:           row.Title,
		Description:     row.Description,
		TaskType:        tt,
		ProjectID:       row.ProjectID,
		Backend:         agents.BackendID(row.Backend),
		Model:           row.Model,
		PromptTemplate:  row.PromptTemplate,
		ScheduleType:    st,
		ScheduledAt:     row.ScheduledAt,
		CronExpr:        row.CronExpr,
		ScheduleEnabled: row.ScheduleEnabled,
		LastRunAt:       row.LastRunAt,
		NextRunAt:       row.NextRunAt,
		LastRunStatus:   RunStatus(row.LastRunStatus),
		Status:          status,
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
		"title":            row.Title,
		"description":      row.Description,
		"task_type":        row.TaskType,
		"project_id":       row.ProjectID,
		"backend":          row.Backend,
		"model":            row.Model,
		"prompt_template":  row.PromptTemplate,
		"schedule_type":    row.ScheduleType,
		"scheduled_at":     row.ScheduledAt,
		"cron_expr":        row.CronExpr,
		"schedule_enabled": row.ScheduleEnabled,
		"last_run_at":      row.LastRunAt,
		"next_run_at":      row.NextRunAt,
		"last_run_status":  row.LastRunStatus,
		"status":           row.Status,
	})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrTaskNotFound
	}
	return nil
}

// Delete removes a task by ID and its runs.
func (r *Repository) Delete(id uint) error {
	_ = r.db.Delete(&database.TaskRun{}, "task_id = ?", id).Error
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
	if filter.ProjectID != nil && *filter.ProjectID > 0 {
		query = query.Where("project_id = ?", *filter.ProjectID)
	}
	if filter.TaskType != nil && *filter.TaskType != "" {
		query = query.Where("task_type = ?", string(*filter.TaskType))
	}
	if filter.Status != nil && *filter.Status != "" {
		query = query.Where("status = ?", string(*filter.Status))
	}
	if filter.Keyword != "" {
		keyword := "%" + filter.Keyword + "%"
		query = query.Where("title LIKE ? OR description LIKE ?", keyword, keyword)
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

func runToDB(r TaskRun) database.TaskRun {
	return database.TaskRun{
		ID:          r.ID,
		TaskID:      r.TaskID,
		SessionID:   r.SessionID,
		TriggerType: r.TriggerType,
		Status:      r.Status,
		Prompt:      r.Prompt,
		Error:       r.Error,
		TriggeredAt: r.TriggeredAt,
		FinishedAt:  r.FinishedAt,
		CreatedAt:   r.CreatedAt,
	}
}

func runFromDB(row database.TaskRun) TaskRun {
	return TaskRun{
		ID:          row.ID,
		TaskID:      row.TaskID,
		SessionID:   row.SessionID,
		TriggerType: row.TriggerType,
		Status:      row.Status,
		Prompt:      row.Prompt,
		Error:       row.Error,
		TriggeredAt: row.TriggeredAt,
		FinishedAt:  row.FinishedAt,
		CreatedAt:   row.CreatedAt,
	}
}

// CreateRun inserts a new TaskRun.
func (r *Repository) CreateRun(run TaskRun) (TaskRun, error) {
	row := runToDB(run)
	if err := r.db.Create(&row).Error; err != nil {
		return TaskRun{}, err
	}
	return runFromDB(row), nil
}

// UpdateRun updates an existing TaskRun.
func (r *Repository) UpdateRun(run TaskRun) error {
	row := runToDB(run)
	return r.db.Model(&database.TaskRun{}).Where("id = ?", run.ID).Updates(map[string]any{
		"status":      row.Status,
		"error":       row.Error,
		"finished_at": row.FinishedAt,
	}).Error
}

// ListRuns returns all runs for a task in descending order.
func (r *Repository) ListRuns(taskID uint) ([]TaskRun, error) {
	var rows []database.TaskRun
	err := r.db.Where("task_id = ?", taskID).Order("id desc").Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]TaskRun, len(rows))
	for i, row := range rows {
		out[i] = runFromDB(row)
	}
	return out, nil
}

// GetTaskSessionIDs returns all session IDs created by tasks in the given project (or all if projectID is 0).
func (r *Repository) GetTaskSessionIDs(projectID uint) ([]string, error) {
	query := r.db.Model(&database.TaskRun{}).Select("task_runs.session_id").
		Joins("JOIN tasks ON tasks.id = task_runs.task_id")
	if projectID > 0 {
		query = query.Where("tasks.project_id = ?", projectID)
	}

	var sessionIDs []string
	if err := query.Pluck("task_runs.session_id", &sessionIDs).Error; err != nil {
		return nil, err
	}
	return sessionIDs, nil
}

// ReconcileOrphaned marks lingering 'running' runs as 'interrupted'.
func (r *Repository) ReconcileOrphaned(now time.Time) (int64, error) {
	res := r.db.Model(&database.TaskRun{}).
		Where("status = ?", "running").
		Updates(map[string]any{
			"status":      "interrupted",
			"error":       "interrupted by platform restart",
			"finished_at": &now,
		})
	return res.RowsAffected, res.Error
}
