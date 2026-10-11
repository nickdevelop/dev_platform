package resources

import (
	"errors"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"devplatform/internal/database"
)

// repository owns the GORM access for resources, their project associations
// and their session selection. It maps persistence errors to the service's
// sentinel errors so the service layer never sees gorm.ErrRecordNotFound.
type repository struct {
	db *gorm.DB
}

func (r *repository) create(row *database.Resource) error {
	if err := r.db.Create(row).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return fmt.Errorf("%w: %s %q", ErrResourceConflict, row.Kind, row.Name)
		}
		return fmt.Errorf("resources: create: %w", err)
	}
	return nil
}

func (r *repository) get(id uint) (database.Resource, error) {
	var row database.Resource
	if err := r.db.First(&row, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return database.Resource{}, ErrResourceNotFound
		}
		return database.Resource{}, fmt.Errorf("resources: load %d: %w", id, err)
	}
	return row, nil
}

// list returns resources in creation order, optionally filtered to one kind.
func (r *repository) list(kind string) ([]database.Resource, error) {
	var rows []database.Resource
	query := r.db.Order("id ASC")
	if kind != "" {
		query = query.Where("kind = ?", kind)
	}
	if err := query.Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("resources: list: %w", err)
	}
	return rows, nil
}

func (r *repository) update(row *database.Resource) error {
	if err := r.db.Save(row).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return fmt.Errorf("%w: %s %q", ErrResourceConflict, row.Kind, row.Name)
		}
		return fmt.Errorf("resources: update %d: %w", row.ID, err)
	}
	return nil
}

// delete removes a resource and every association that points at it in one
// transaction, so no dangling link survives.
func (r *repository) delete(id uint) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		result := tx.Delete(&database.Resource{}, id)
		if result.Error != nil {
			return fmt.Errorf("resources: delete %d: %w", id, result.Error)
		}
		if result.RowsAffected == 0 {
			return ErrResourceNotFound
		}
		if err := tx.Where("resource_id = ?", id).Delete(&database.ProjectResource{}).Error; err != nil {
			return fmt.Errorf("resources: delete associations for %d: %w", id, err)
		}
		if err := tx.Where("resource_id = ?", id).Delete(&database.SessionPrompt{}).Error; err != nil {
			return fmt.Errorf("resources: delete session selections for %d: %w", id, err)
		}
		return nil
	})
}

func (r *repository) projectExists(id uint) (bool, error) {
	var count int64
	if err := r.db.Model(&database.Project{}).Where("id = ?", id).Count(&count).Error; err != nil {
		return false, fmt.Errorf("resources: check project %d: %w", id, err)
	}
	return count > 0, nil
}

func (r *repository) sessionExistsInProject(projectID uint, sessionID string) (bool, error) {
	var count int64
	if err := r.db.Model(&database.AgentSession{}).
		Where("id = ? AND project_id = ?", sessionID, projectID).
		Count(&count).Error; err != nil {
		return false, fmt.Errorf("resources: check session %q: %w", sessionID, err)
	}
	return count > 0, nil
}

// associate links a resource to a project idempotently.
func (r *repository) associate(projectID, resourceID uint) error {
	row := database.ProjectResource{ProjectID: projectID, ResourceID: resourceID}
	if err := r.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
		return fmt.Errorf("resources: associate %d with project %d: %w", resourceID, projectID, err)
	}
	return nil
}

// disassociate removes a link idempotently.
func (r *repository) disassociate(projectID, resourceID uint) error {
	if err := r.db.Where("project_id = ? AND resource_id = ?", projectID, resourceID).
		Delete(&database.ProjectResource{}).Error; err != nil {
		return fmt.Errorf("resources: disassociate %d from project %d: %w", resourceID, projectID, err)
	}
	return nil
}

func (r *repository) listProjectResources(projectID uint) ([]database.Resource, error) {
	var rows []database.Resource
	if err := r.db.
		Joins("JOIN project_resources ON project_resources.resource_id = resources.id").
		Where("project_resources.project_id = ?", projectID).
		Order("resources.id ASC").
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("resources: list project %d resources: %w", projectID, err)
	}
	return rows, nil
}

// selectSessionPrompt records a prompt selection idempotently.
func (r *repository) selectSessionPrompt(sessionID string, resourceID uint) error {
	row := database.SessionPrompt{SessionID: sessionID, ResourceID: resourceID}
	if err := r.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
		return fmt.Errorf("resources: select %d for session %q: %w", resourceID, sessionID, err)
	}
	return nil
}

func (r *repository) removeSessionPrompt(sessionID string, resourceID uint) error {
	if err := r.db.Where("session_id = ? AND resource_id = ?", sessionID, resourceID).
		Delete(&database.SessionPrompt{}).Error; err != nil {
		return fmt.Errorf("resources: remove %d from session %q: %w", resourceID, sessionID, err)
	}
	return nil
}

func (r *repository) listSessionPrompts(sessionID string) ([]database.Resource, error) {
	var rows []database.Resource
	if err := r.db.
		Joins("JOIN session_prompts ON session_prompts.resource_id = resources.id").
		Where("session_prompts.session_id = ?", sessionID).
		Order("resources.id ASC").
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("resources: list session %q prompts: %w", sessionID, err)
	}
	return rows, nil
}
