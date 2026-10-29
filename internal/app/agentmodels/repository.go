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

	// Replace atomically updates a backend's snapshot, merging new and existing
	// models so that user-defined configurations (such as enabled status and custom name)
	// are preserved across refreshes. Obsolete models not reported by the backend are purged.
	Replace(backend agents.BackendID, models []agents.ModelInfo, syncedAt time.Time) error

	// Update updates configuration fields of a single persisted agent model.
	Update(backend agents.BackendID, modelID string, patch ModelPatch) (agents.ModelInfo, error)
}

// ModelPatch carries the fields that can be updated on an agent model.
type ModelPatch struct {
	Enabled        *bool   `json:"enabled,omitempty"`
	Name           *string `json:"name,omitempty"`
	ContextLimit   *int    `json:"contextLimit,omitempty"`
	SupportsImages *bool   `json:"supportsImages,omitempty"`
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
			Enabled:        row.Enabled,
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
		var existingRows []database.AgentModel
		if err := tx.Where("backend = ?", string(backend)).Find(&existingRows).Error; err != nil {
			return fmt.Errorf("query existing agent models: %w", err)
		}

		existingMap := make(map[string]database.AgentModel, len(existingRows))
		for _, row := range existingRows {
			existingMap[row.ModelID] = row
		}

		activeIDs := make(map[string]struct{}, len(models))

		for index, model := range models {
			activeIDs[model.ID] = struct{}{}

			variants := ""
			if len(model.Variants) > 0 {
				encoded, err := json.Marshal(model.Variants)
				if err != nil {
					return fmt.Errorf("encode variants of %q: %w", model.ID, err)
				}
				variants = string(encoded)
			}

			if existing, ok := existingMap[model.ID]; ok {
				// Preserve user customizations (name, contextLimit, supportsImages, enabled)
				// while updating backend-reported fields and order.
				name := existing.Name
				if name == "" {
					name = model.Name
				}
				contextLimit := existing.ContextLimit
				if contextLimit == nil {
					contextLimit = model.ContextLimit
				}

				existing.SortOrder = index
				existing.Provider = model.Provider
				existing.Variants = variants
				existing.Name = name
				existing.ContextLimit = contextLimit

				if err := tx.Save(&existing).Error; err != nil {
					return fmt.Errorf("update agent model %q: %w", model.ID, err)
				}
			} else {
				row := database.AgentModel{
					Backend:        string(backend),
					ModelID:        model.ID,
					Name:           model.Name,
					Provider:       model.Provider,
					Variants:       variants,
					ContextLimit:   model.ContextLimit,
					SupportsImages: model.SupportsImages,
					SortOrder:      index,
					Enabled:        true,
				}
				if err := tx.Create(&row).Error; err != nil {
					return fmt.Errorf("store agent model %q: %w", model.ID, err)
				}
			}
		}

		// Purge obsolete models not reported by the backend anymore.
		for modelID, oldRow := range existingMap {
			if _, keep := activeIDs[modelID]; !keep {
				if err := tx.Delete(&oldRow).Error; err != nil {
					return fmt.Errorf("purge obsolete agent model %q: %w", modelID, err)
				}
			}
		}

		catalog := database.AgentModelCatalog{Backend: string(backend), SyncedAt: syncedAt}
		if err := tx.Save(&catalog).Error; err != nil {
			return fmt.Errorf("store agent model catalog: %w", err)
		}
		return nil
	})
}

func (r *gormRepository) Update(backend agents.BackendID, modelID string, patch ModelPatch) (agents.ModelInfo, error) {
	var row database.AgentModel
	if err := r.db.Where("backend = ? AND model_id = ?", string(backend), modelID).Take(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return agents.ModelInfo{}, ErrModelNotFound
		}
		return agents.ModelInfo{}, fmt.Errorf("find agent model: %w", err)
	}

	if patch.Enabled != nil {
		row.Enabled = *patch.Enabled
	}
	if patch.Name != nil {
		row.Name = *patch.Name
	}
	if patch.ContextLimit != nil {
		row.ContextLimit = patch.ContextLimit
	}
	if patch.SupportsImages != nil {
		row.SupportsImages = *patch.SupportsImages
	}

	if err := r.db.Save(&row).Error; err != nil {
		return agents.ModelInfo{}, fmt.Errorf("save agent model: %w", err)
	}

	model := agents.ModelInfo{
		ID:             row.ModelID,
		Name:           row.Name,
		Provider:       row.Provider,
		ContextLimit:   row.ContextLimit,
		SupportsImages: row.SupportsImages,
		Enabled:        row.Enabled,
	}
	if row.Variants != "" {
		_ = json.Unmarshal([]byte(row.Variants), &model.Variants)
	}
	return model, nil
}
