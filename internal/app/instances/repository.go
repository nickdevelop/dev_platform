package instances

import (
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"devplatform/internal/app/database"
	"devplatform/internal/core/agents"
)

// Repository persists and retrieves agent instance records.
type Repository interface {
	Create(inst Instance) error
	Get(instanceID string) (Instance, error)
	List(projectID uint) ([]Instance, error)
	ListFiltered(projectID uint, filter string) ([]Instance, error)
	ListLive() ([]Instance, error)
	PruneDead(projectID uint, keepLast int) error
	UpdateStatus(instanceID string, status agents.InstanceStatus, errText string, updatedAt time.Time) error
}

type gormRepository struct {
	db *gorm.DB
}

// NewGORMRepository creates a GORM-backed Repository for agent instances.
func NewGORMRepository(db *gorm.DB) Repository {
	return &gormRepository{db: db}
}

func (r *gormRepository) Create(inst Instance) error {
	row := database.AgentInstance{
		ID:        inst.ID,
		ProjectID: inst.ProjectID,
		Backend:   string(inst.Backend),
		Directory: inst.Directory,
		Status:    string(inst.Status),
		Endpoint:  inst.Endpoint,
		Error:     inst.Error,
		CreatedAt: inst.CreatedAt,
		UpdatedAt: inst.UpdatedAt,
	}
	return r.db.Transaction(func(tx *gorm.DB) error {
		return tx.Create(&row).Error
	})
}

func (r *gormRepository) Get(instanceID string) (Instance, error) {
	var row database.AgentInstance
	if err := r.db.First(&row, "id = ?", instanceID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Instance{}, fmt.Errorf("%w: %s", ErrInstanceNotFound, instanceID)
		}
		return Instance{}, fmt.Errorf("instances: load %s: %w", instanceID, err)
	}
	return instanceFromModel(row), nil
}

func (r *gormRepository) List(projectID uint) ([]Instance, error) {
	return r.ListFiltered(projectID, "")
}

func (r *gormRepository) ListFiltered(projectID uint, filter string) ([]Instance, error) {
	query := r.db.Model(&database.AgentInstance{})
	if projectID > 0 {
		query = query.Where("project_id = ?", projectID)
	}
	switch filter {
	case "live":
		query = query.Where("status IN ?", []string{string(agents.InstanceStarting), string(agents.InstanceRunning)})
	case "running":
		query = query.Where("status = ?", string(agents.InstanceRunning))
	case "stopped":
		query = query.Where("status = ?", string(agents.InstanceStopped))
	case "failed":
		query = query.Where("status = ?", string(agents.InstanceFailed))
	}
	var rows []database.AgentInstance
	if err := query.Order("created_at ASC, id ASC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("instances: list project %d: %w", projectID, err)
	}
	out := make([]Instance, 0, len(rows))
	for _, row := range rows {
		out = append(out, instanceFromModel(row))
	}
	return out, nil
}

func (r *gormRepository) ListLive() ([]Instance, error) {
	return r.ListFiltered(0, "live")
}

func (r *gormRepository) PruneDead(projectID uint, keepLast int) error {
	if keepLast <= 0 {
		keepLast = 5
	}
	var deadIDs []string
	query := r.db.Model(&database.AgentInstance{}).
		Where("status IN ?", []string{string(agents.InstanceStopped), string(agents.InstanceFailed)})
	if projectID > 0 {
		query = query.Where("project_id = ?", projectID)
	}
	err := query.Order("created_at DESC, id DESC").Pluck("id", &deadIDs).Error
	if err != nil {
		return fmt.Errorf("instances: prune dead ids: %w", err)
	}
	if len(deadIDs) <= keepLast {
		return nil
	}
	toDelete := deadIDs[keepLast:]
	return r.db.Transaction(func(tx *gorm.DB) error {
		return tx.Where("id IN ?", toDelete).Delete(&database.AgentInstance{}).Error
	})
}

func (r *gormRepository) UpdateStatus(instanceID string, status agents.InstanceStatus, errText string, updatedAt time.Time) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		return tx.Model(&database.AgentInstance{}).Where("id = ?", instanceID).
			Updates(map[string]any{"status": string(status), "error": errText, "updated_at": updatedAt}).Error
	})
}
