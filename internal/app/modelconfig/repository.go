package modelconfig

import (
	"errors"
	"fmt"

	"gorm.io/gorm"

	"devplatform/internal/app/database"
)

// Repository persists and retrieves model services and model definitions.
type Repository interface {
	ListServices() ([]database.ModelService, error)
	ListModels() ([]database.ModelDef, error)
	ListModelsByService(serviceID uint) ([]database.ModelDef, error)

	GetService(id uint) (database.ModelService, error)
	CreateService(row *database.ModelService) error
	UpdateService(row *database.ModelService) error
	DeleteService(id uint) error

	GetModel(id uint) (database.ModelDef, error)
	CreateModel(row *database.ModelDef) error
	UpdateModel(row *database.ModelDef) error
	DeleteModel(id uint) error
}

type gormRepository struct {
	db *gorm.DB
}

// NewGORMRepository creates a GORM-backed Repository for model configuration.
func NewGORMRepository(db *gorm.DB) Repository {
	return &gormRepository{db: db}
}

func (r *gormRepository) ListServices() ([]database.ModelService, error) {
	var services []database.ModelService
	if err := r.db.Order("id ASC").Find(&services).Error; err != nil {
		return nil, fmt.Errorf("modelconfig: list services: %w", err)
	}
	return services, nil
}

func (r *gormRepository) ListModels() ([]database.ModelDef, error) {
	var models []database.ModelDef
	if err := r.db.Order("id ASC").Find(&models).Error; err != nil {
		return nil, fmt.Errorf("modelconfig: list models: %w", err)
	}
	return models, nil
}

func (r *gormRepository) ListModelsByService(serviceID uint) ([]database.ModelDef, error) {
	var rows []database.ModelDef
	if err := r.db.Where("service_id = ?", serviceID).Order("id ASC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("modelconfig: load models for service %d: %w", serviceID, err)
	}
	return rows, nil
}

func (r *gormRepository) GetService(id uint) (database.ModelService, error) {
	var row database.ModelService
	if err := r.db.First(&row, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return database.ModelService{}, ErrServiceNotFound
		}
		return database.ModelService{}, fmt.Errorf("modelconfig: load service %d: %w", id, err)
	}
	return row, nil
}

func (r *gormRepository) CreateService(row *database.ModelService) error {
	if err := r.db.Create(row).Error; err != nil {
		return fmt.Errorf("modelconfig: create service: %w", err)
	}
	return nil
}

func (r *gormRepository) UpdateService(row *database.ModelService) error {
	if err := r.db.Save(row).Error; err != nil {
		return fmt.Errorf("modelconfig: update service: %w", err)
	}
	return nil
}

func (r *gormRepository) DeleteService(id uint) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("service_id = ?", id).Delete(&database.ModelDef{}).Error; err != nil {
			return err
		}
		return tx.Delete(&database.ModelService{}, id).Error
	})
}

func (r *gormRepository) GetModel(id uint) (database.ModelDef, error) {
	var row database.ModelDef
	if err := r.db.First(&row, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return database.ModelDef{}, ErrModelNotFound
		}
		return database.ModelDef{}, fmt.Errorf("modelconfig: load model %d: %w", id, err)
	}
	return row, nil
}

func (r *gormRepository) CreateModel(row *database.ModelDef) error {
	if err := r.db.Create(row).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return fmt.Errorf("%w: %q", ErrModelConflict, row.ModelID)
		}
		return fmt.Errorf("modelconfig: add model: %w", err)
	}
	return nil
}

func (r *gormRepository) UpdateModel(row *database.ModelDef) error {
	if err := r.db.Save(row).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return fmt.Errorf("%w: %q", ErrModelConflict, row.ModelID)
		}
		return fmt.Errorf("modelconfig: update model: %w", err)
	}
	return nil
}

func (r *gormRepository) DeleteModel(id uint) error {
	result := r.db.Delete(&database.ModelDef{}, id)
	if result.Error != nil {
		return fmt.Errorf("modelconfig: delete model: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrModelNotFound
	}
	return nil
}
