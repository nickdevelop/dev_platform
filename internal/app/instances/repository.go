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
	ListLive() ([]Instance, error)
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
	var rows []database.AgentInstance
	if err := r.db.Where("project_id = ?", projectID).Order("created_at ASC, id ASC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("instances: list project %d: %w", projectID, err)
	}
	out := make([]Instance, 0, len(rows))
	for _, row := range rows {
		out = append(out, instanceFromModel(row))
	}
	return out, nil
}

func (r *gormRepository) ListLive() ([]Instance, error) {
	var rows []database.AgentInstance
	if err := r.db.Where("status IN ?", []string{string(agents.InstanceStarting), string(agents.InstanceRunning)}).
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("instances: list live: %w", err)
	}
	out := make([]Instance, 0, len(rows))
	for _, row := range rows {
		out = append(out, instanceFromModel(row))
	}
	return out, nil
}

func (r *gormRepository) UpdateStatus(instanceID string, status agents.InstanceStatus, errText string, updatedAt time.Time) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		return tx.Model(&database.AgentInstance{}).Where("id = ?", instanceID).
			Updates(map[string]any{"status": string(status), "error": errText, "updated_at": updatedAt}).Error
	})
}
