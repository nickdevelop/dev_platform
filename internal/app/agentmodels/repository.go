package agentmodels

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"devplatform/internal/app/database"
	"devplatform/internal/core/agents"
)

// Repository stores one backend's model snapshot and its sync time. It is an
// interface so the service can be tested without a database.
type Repository interface {
	// List returns the persisted models in their reported order and when the
	// snapshot was last synced. A backend that was never synced returns an
	// empty slice and a nil time.
	List(backend agents.BackendID) ([]agents.ModelInfo, *time.Time, error)

	// Replace atomically replaces a backend's snapshot. A failed write leaves
	// the previous snapshot untouched.
	Replace(backend agents.BackendID, models []agents.ModelInfo, syncedAt time.Time) error
}

type gormRepository struct {
	db *gorm.DB
}

// NewGORMRepository builds the repository over the platform database.
func NewGORMRepository(db *gorm.DB) Repository {
	return &gormRepository{db: db}
}

func (r *gormRepository) List(backend agents.BackendID) ([]agents.ModelInfo, *time.Time, error) {
	var rows []database.AgentModel
	if err := r.db.Where("backend = ?", string(backend)).
		Order("sort_order asc, id asc").
		Find(&rows).Error; err != nil {
		return nil, nil, fmt.Errorf("list agent models: %w", err)
	}

	models := make([]agents.ModelInfo, 0, len(rows))
	for _, row := range rows {
		model := agents.ModelInfo{
			ID:             row.ModelID,
			Name:           row.Name,
			Provider:       row.Provider,
			ContextLimit:   row.ContextLimit,
			SupportsImages: row.SupportsImages,
		}
		if row.Variants != "" {
			if err := json.Unmarshal([]byte(row.Variants), &model.Variants); err != nil {
				return nil, nil, fmt.Errorf("decode variants of %q: %w", row.ModelID, err)
			}
		}
		models = append(models, model)
	}

	var catalog database.AgentModelCatalog
	err := r.db.Where("backend = ?", string(backend)).Take(&catalog).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return models, nil, nil
	case err != nil:
		return nil, nil, fmt.Errorf("read agent model catalog: %w", err)
	default:
		syncedAt := catalog.SyncedAt
		return models, &syncedAt, nil
	}
}

func (r *gormRepository) Replace(backend agents.BackendID, models []agents.ModelInfo, syncedAt time.Time) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("backend = ?", string(backend)).Delete(&database.AgentModel{}).Error; err != nil {
			return fmt.Errorf("clear agent models: %w", err)
		}

		if len(models) > 0 {
			rows := make([]database.AgentModel, 0, len(models))
			for index, model := range models {
				variants := ""
				if len(model.Variants) > 0 {
					encoded, err := json.Marshal(model.Variants)
					if err != nil {
						return fmt.Errorf("encode variants of %q: %w", model.ID, err)
					}
					variants = string(encoded)
				}
				row := database.AgentModel{
					Backend:        string(backend),
					ModelID:        model.ID,
					Name:           model.Name,
					Provider:       model.Provider,
					Variants:       variants,
					ContextLimit:   model.ContextLimit,
					SupportsImages: model.SupportsImages,
					SortOrder:      index,
				}
				rows = append(rows, row)
			}
			if err := tx.Create(&rows).Error; err != nil {
				return fmt.Errorf("store agent models: %w", err)
			}
		}

		catalog := database.AgentModelCatalog{Backend: string(backend), SyncedAt: syncedAt}
		if err := tx.Save(&catalog).Error; err != nil {
			return fmt.Errorf("store agent model catalog: %w", err)
		}
		return nil
	})
}
