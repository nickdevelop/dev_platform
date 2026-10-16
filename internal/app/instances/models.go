package instances

import (
	"context"
	"errors"
	"fmt"
	"time"

	"devplatform/internal/core/agents"
)

// ErrUnsupportedCapability reports a backend that does not declare the
// capability an operation needs. An unsupported control must never be presented
// as available, so the caller is told instead of being handed an empty list.
var ErrUnsupportedCapability = errors.New("instances: backend does not support this operation")

// defaultModelTTL is how long a backend's model list is served from cache. A
// catalog changes when the operator changes the backend's configuration, not
// when the platform asks, so the cache exists to keep the list off the critical
// path of opening a session rather than to track the backend closely.
const defaultModelTTL = 60 * time.Second

// BackendModels returns the models a backend offers to one project, served from
// the per-(backend, directory) cache.
//
// A backend's catalog is the backend's own, scoped to the project's authorized
// directory: no instance has to exist, because the workbench needs the list to
// offer a model while creating a session. refresh asks the backend now instead
// of accepting a snapshot; the response says whether what came back is a
// snapshot at all, so the workbench can show models instantly and still learn
// the list moved.
func (s *Service) BackendModels(ctx context.Context, projectID uint, backend agents.BackendID, refresh bool) (agents.ModelList, error) {
	if !backend.Valid() {
		return agents.ModelList{}, fmt.Errorf("%w: %q", ErrUnknownBackend, backend)
	}
	adapter, ok := s.registry.Get(backend)
	if !ok {
		return agents.ModelList{}, fmt.Errorf("%w: %q (%w)", ErrUnknownBackend, backend, agents.ErrBackendNotRegistered)
	}
	if !adapter.Capabilities().ModelSelection {
		return agents.ModelList{}, fmt.Errorf("%w: model selection on %q", ErrUnsupportedCapability, backend)
	}
	directory, err := s.resolveProject(projectID)
	if err != nil {
		return agents.ModelList{}, err
	}
	return s.modelCatalog.List(ctx, agents.ModelCacheKey(backend, directory), refresh, func(ctx context.Context) ([]agents.ModelInfo, error) {
		return adapter.ListModels(ctx, directory)
	})
}

// InvalidateModels drops a backend's cached model lists. The composition root
// wires it to the agent service controller: a machine-level service starting or
// stopping really does change what the backend can answer, so a list held across
// that event is not just old — it is about a state of the backend that no longer
// exists.
func (s *Service) InvalidateModels(backend agents.BackendID) {
	if s.modelCatalog != nil {
		s.modelCatalog.Invalidate(backend)
	}
}
