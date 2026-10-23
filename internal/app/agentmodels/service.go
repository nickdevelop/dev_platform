// Package agentmodels owns the platform-wide model catalogs of agent
// backends. A catalog is a persisted snapshot of what the backend reported on
// the last explicit refresh: reads never touch the backend, and a refresh
// replaces the snapshot atomically. The package depends only on the agent
// contracts, the adapter registry and the platform database; it never names a
// concrete backend.
package agentmodels

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"gorm.io/gorm"

	"devplatform/internal/core/agents"
)

// Service errors, mapped to HTTP statuses by the handlers.
var (
	// ErrUnknownBackend reports an unknown backend id, or a valid one with no
	// registered adapter.
	ErrUnknownBackend = errors.New("agentmodels: unknown or unregistered backend")
	// ErrUnsupportedCapability reports a backend that does not declare model
	// selection.
	ErrUnsupportedCapability = errors.New("agentmodels: backend does not support model selection")
	// ErrModelListFailed reports that the adapter failed to read its catalog.
	// The persisted snapshot is left untouched.
	ErrModelListFailed = errors.New("agentmodels: backend failed to list models")
	// ErrStorage reports a database failure while reading or replacing a
	// snapshot.
	ErrStorage = errors.New("agentmodels: storage failure")
)

// ModelList is one backend's persisted catalog snapshot. UpdatedAt is nil when
// the backend has never been refreshed; an empty Models slice with a non-nil
// UpdatedAt means the backend reported no models.
type ModelList struct {
	Models    []agents.ModelInfo `json:"models"`
	UpdatedAt *time.Time         `json:"updatedAt"`
}

// Service reads and refreshes platform-wide agent model snapshots.
type Service struct {
	registry *agents.Registry
	repo     Repository

	// refreshMu serializes snapshot replacement. Refreshes are rare manual
	// actions, and serializing them keeps two clicks from interleaving a
	// delete/insert pair for the same backend.
	refreshMu sync.Mutex
	now       func() time.Time
}

// NewService builds the service over the platform database and the adapter
// registry.
func NewService(db *gorm.DB, registry *agents.Registry) *Service {
	return NewServiceWithRepository(NewGORMRepository(db), registry)
}

// NewServiceWithRepository builds the service over a custom repository.
func NewServiceWithRepository(repo Repository, registry *agents.Registry) *Service {
	return &Service{
		registry: registry,
		repo:     repo,
		now:      func() time.Time { return time.Now().UTC() },
	}
}

// List returns the persisted snapshot for a backend. It checks the backend id
// and its declared capability, but never asks the adapter for models: an agent
// that is stopped or unreachable still serves its last snapshot.
func (s *Service) List(backend agents.BackendID) (ModelList, error) {
	if !backend.Valid() {
		return ModelList{}, fmt.Errorf("%w: %q", ErrUnknownBackend, backend)
	}
	adapter, ok := s.registry.Get(backend)
	if !ok {
		return ModelList{}, fmt.Errorf("%w: %q (%w)", ErrUnknownBackend, backend, agents.ErrBackendNotRegistered)
	}
	if !adapter.Capabilities().ModelSelection {
		return ModelList{}, fmt.Errorf("%w: %q", ErrUnsupportedCapability, backend)
	}

	models, updatedAt, err := s.repo.List(backend)
	if err != nil {
		return ModelList{}, fmt.Errorf("%w: %w", ErrStorage, err)
	}
	if models == nil {
		models = []agents.ModelInfo{}
	}
	return ModelList{Models: models, UpdatedAt: updatedAt}, nil
}

// Refresh asks the backend for its current catalog and replaces the persisted
// snapshot in one transaction. A failed adapter call leaves the previous
// snapshot untouched.
func (s *Service) Refresh(ctx context.Context, backend agents.BackendID) (ModelList, error) {
	if !backend.Valid() {
		return ModelList{}, fmt.Errorf("%w: %q", ErrUnknownBackend, backend)
	}
	adapter, ok := s.registry.Get(backend)
	if !ok {
		return ModelList{}, fmt.Errorf("%w: %q (%w)", ErrUnknownBackend, backend, agents.ErrBackendNotRegistered)
	}
	if !adapter.Capabilities().ModelSelection {
		return ModelList{}, fmt.Errorf("%w: %q", ErrUnsupportedCapability, backend)
	}

	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()

	// An empty directory is the platform-wide scope: the backend uses its own
	// default context instead of a project directory.
	models, err := adapter.ListModels(ctx, "")
	if err != nil {
		return ModelList{}, fmt.Errorf("%w: %w", ErrModelListFailed, err)
	}
	models = normalizeModels(models)

	syncedAt := s.now()
	if err := s.repo.Replace(backend, models, syncedAt); err != nil {
		return ModelList{}, fmt.Errorf("%w: %w", ErrStorage, err)
	}
	return ModelList{Models: models, UpdatedAt: &syncedAt}, nil
}

// normalizeModels drops empty ids and duplicate ids while preserving the
// backend's order. The first occurrence of an id wins.
func normalizeModels(models []agents.ModelInfo) []agents.ModelInfo {
	out := make([]agents.ModelInfo, 0, len(models))
	seen := make(map[string]bool, len(models))
	for _, model := range models {
		if model.ID == "" || seen[model.ID] {
			continue
		}
		seen[model.ID] = true
		out = append(out, model)
	}
	return out
}
