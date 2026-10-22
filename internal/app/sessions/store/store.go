// Package store persists the platform's session index: the durable mapping from
// a platform session id to the (project, backend, instance, agent-native
// session id) it belongs to. It holds no agent logic and no conversation
// transcript; the platform never persists agent-native history.
package store

import (
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"devplatform/internal/app/database"
)

// ErrNotFound reports an operation on a session that is not indexed for the
// authorized project. Unknown ids and sessions owned by another project are
// indistinguishable on purpose, so the API never confirms the existence of a
// session outside the caller's project.
var ErrNotFound = errors.New("sessions: session not found")

// Store is the GORM-backed store for the session index. Every write runs in its
// own transaction so a partial update is never visible.
type Store struct {
	db *gorm.DB
}

// New builds the session index store over the platform database.
func New(db *gorm.DB) *Store {
	return &Store{db: db}
}

// Create inserts one index row.
func (r *Store) Create(row database.AgentSession) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		return tx.Create(&row).Error
	})
}

// GetByID loads the index row with the given platform id.
func (r *Store) GetByID(id string) (database.AgentSession, error) {
	var row database.AgentSession
	if err := r.db.First(&row, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return database.AgentSession{}, fmt.Errorf("%w: %s", ErrNotFound, id)
		}
		return database.AgentSession{}, fmt.Errorf("sessions: load %s: %w", id, err)
	}
	return row, nil
}

// GetByNative loads the index row mapping one native session in a project.
func (r *Store) GetByNative(projectID uint, backend, nativeID string) (database.AgentSession, error) {
	var row database.AgentSession
	if err := r.db.Where("project_id = ? AND backend = ? AND native_session_id = ?", projectID, backend, nativeID).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return database.AgentSession{}, fmt.Errorf("%w: %d/%s/%s", ErrNotFound, projectID, backend, nativeID)
		}
		return database.AgentSession{}, fmt.Errorf("sessions: load native %d/%s/%s: %w", projectID, backend, nativeID, err)
	}
	return row, nil
}

// List returns a project's indexed sessions in creation order.
func (r *Store) List(projectID uint) ([]database.AgentSession, error) {
	var rows []database.AgentSession
	if err := r.db.Where("project_id = ?", projectID).Order("created_at ASC, id ASC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("sessions: list project %d: %w", projectID, err)
	}
	return rows, nil
}

// ListPaged returns a page of a project's sessions, ordered by updated_at DESC, created_at DESC, id DESC.
// If limit <= 0, it returns all sessions.
// It returns (rows, total, error).
func (r *Store) ListPaged(projectID uint, limit, offset int) ([]database.AgentSession, int, error) {
	var total int64
	if err := r.db.Model(&database.AgentSession{}).Where("project_id = ?", projectID).Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("sessions: count project %d: %w", projectID, err)
	}
	query := r.db.Where("project_id = ?", projectID).Order("updated_at DESC, created_at DESC, id DESC")
	if limit > 0 {
		query = query.Limit(limit).Offset(offset)
	}
	var rows []database.AgentSession
	if err := query.Find(&rows).Error; err != nil {
		return nil, 0, fmt.Errorf("sessions: list paged project %d: %w", projectID, err)
	}
	return rows, int(total), nil
}

// ListByInstance returns every indexed session of one instance.
func (r *Store) ListByInstance(instanceID string) ([]database.AgentSession, error) {
	var rows []database.AgentSession
	if err := r.db.Where("instance_id = ?", instanceID).Order("created_at ASC, id ASC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("sessions: list instance %s: %w", instanceID, err)
	}
	return rows, nil
}

// Update persists the full row.
func (r *Store) Update(row database.AgentSession) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		return tx.Save(&row).Error
	})
}

// UpdateOnContinue persists what one Continue operation may have changed —
// the status, the title the adapter reported, and the instance the session was
// rebound to — while pinning updated_at to the value the row was loaded with.
// The workbench sorts its session list by updated_at as the last-interaction
// time, and opening a session is not an interaction. GORM refreshes that column
// on every Save and on any map update that omits it, so the map must carry the
// loaded value explicitly.
func (r *Store) UpdateOnContinue(row database.AgentSession) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		return tx.Model(&database.AgentSession{}).Where("id = ?", row.ID).
			Updates(map[string]any{
				"status":      row.Status,
				"title":       row.Title,
				"instance_id": row.InstanceID,
				"updated_at":  row.UpdatedAt,
			}).Error
	})
}

// UpdateStatus updates only the status column of one row, preserving its updated_at timestamp.
func (r *Store) UpdateStatus(id, status string) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		return tx.Model(&database.AgentSession{}).Where("id = ?", id).
			UpdateColumn("status", status).Error
	})
}

// TouchUpdatedAt updates only the updated_at column of one row.
func (r *Store) TouchUpdatedAt(id string, at time.Time) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		return tx.Model(&database.AgentSession{}).Where("id = ?", id).
			UpdateColumn("updated_at", at).Error
	})
}

// Delete removes one index row by id. Re-binding a session onto a new instance
// uses it to merge away the duplicate row discovery may have created for the
// same native session under that instance.
func (r *Store) Delete(id string) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		return tx.Delete(&database.AgentSession{}, "id = ?", id).Error
	})
}
