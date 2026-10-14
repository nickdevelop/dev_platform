// Package sessions owns the platform-side session layer of the agent
// workbench: the durable index that maps a platform session id to the
// (project, backend, instance, agent-native session id) it belongs to, the
// discovery/create/continue/send/stop operations built on the adapter
// registry, and the normalized Server-Sent Events stream the workbench
// consumes.
//
// The package persists only the index and ID mappings. Agent-native session
// history stays inside the agent; the platform never stores it.
package sessions

import (
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"devplatform/internal/database"
)

// ErrSessionNotFound reports an operation on a session that is not indexed for
// the authorized project. Unknown ids and sessions owned by another project
// are indistinguishable on purpose, so the API never confirms the existence of
// a session outside the caller's project.
var ErrSessionNotFound = errors.New("sessions: session not found")

// repository is the GORM-backed store for the session index. Every write runs
// in its own transaction so a partial update is never visible.
type repository struct {
	db *gorm.DB
}

func newRepository(db *gorm.DB) *repository {
	return &repository{db: db}
}

// create inserts one index row.
func (r *repository) create(row database.AgentSession) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		return tx.Create(&row).Error
	})
}

// getByID loads the index row with the given platform id.
func (r *repository) getByID(id string) (database.AgentSession, error) {
	var row database.AgentSession
	if err := r.db.First(&row, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return database.AgentSession{}, fmt.Errorf("%w: %s", ErrSessionNotFound, id)
		}
		return database.AgentSession{}, fmt.Errorf("sessions: load %s: %w", id, err)
	}
	return row, nil
}

// getByNative loads the index row mapping one instance's native session.
func (r *repository) getByNative(instanceID, nativeID string) (database.AgentSession, error) {
	var row database.AgentSession
	if err := r.db.Where("instance_id = ? AND native_session_id = ?", instanceID, nativeID).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return database.AgentSession{}, fmt.Errorf("%w: %s/%s", ErrSessionNotFound, instanceID, nativeID)
		}
		return database.AgentSession{}, fmt.Errorf("sessions: load native %s/%s: %w", instanceID, nativeID, err)
	}
	return row, nil
}

// list returns a project's indexed sessions in creation order.
func (r *repository) list(projectID uint) ([]database.AgentSession, error) {
	var rows []database.AgentSession
	if err := r.db.Where("project_id = ?", projectID).Order("created_at ASC, id ASC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("sessions: list project %d: %w", projectID, err)
	}
	return rows, nil
}

// listByInstance returns every indexed session of one instance.
func (r *repository) listByInstance(instanceID string) ([]database.AgentSession, error) {
	var rows []database.AgentSession
	if err := r.db.Where("instance_id = ?", instanceID).Order("created_at ASC, id ASC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("sessions: list instance %s: %w", instanceID, err)
	}
	return rows, nil
}

// update persists the full row.
func (r *repository) update(row database.AgentSession) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		return tx.Save(&row).Error
	})
}

// updateOnContinue persists what one Continue operation may have changed —
// the status, the title the adapter reported, and the instance the session was
// rebound to — while pinning updated_at to the value the row was loaded with.
// The workbench sorts its session list by updated_at as the last-interaction
// time, and opening a session is not an interaction. GORM refreshes that column
// on every Save and on any map update that omits it, so the map must carry the
// loaded value explicitly.
func (r *repository) updateOnContinue(row database.AgentSession) error {
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

// updateStatus updates only the status and updated-at columns of one row.
func (r *repository) updateStatus(id, status string, at time.Time) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		return tx.Model(&database.AgentSession{}).Where("id = ?", id).
			Updates(map[string]any{"status": status, "updated_at": at}).Error
	})
}

// delete removes one index row by id. Re-binding a session onto a new instance
// uses it to merge away the duplicate row discovery may have created for the
// same native session under that instance.
func (r *repository) delete(id string) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		return tx.Delete(&database.AgentSession{}, "id = ?", id).Error
	})
}
