// Package agents' registry maps backend ids to the adapters that implement
// them. It is the single lookup the instance lifecycle and the session
// services use to reach an adapter, and it is deliberately a plain value with
// no package-level state: an adapter package exposes its own
// Register(registry) helper that calls Registry.Register, so nothing mutates a
// global registry from init() and tests can build an isolated registry holding
// fakes.
package agents

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
)

var (
	// ErrAdapterRequired reports a registration with a nil adapter.
	ErrAdapterRequired = errors.New("agents: adapter is required")
	// ErrAdapterAlreadyRegistered reports a second registration for a backend
	// id that already has an adapter.
	ErrAdapterAlreadyRegistered = errors.New("agents: adapter already registered")
	// ErrBackendNotRegistered reports a backend id that is valid but has no
	// adapter registered.
	ErrBackendNotRegistered = errors.New("agents: backend has no registered adapter")
)

// Registry is a concurrency-safe lookup from backend id to adapter. The zero
// value is not usable; build one with NewRegistry.
type Registry struct {
	mu       sync.RWMutex
	adapters map[BackendID]AgentAdapter
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{adapters: make(map[BackendID]AgentAdapter)}
}

// Register adds adapter as the implementation of the given backend id. It
// rejects an unknown backend id, a nil adapter, and a second registration for
// a backend that already has one, so a wiring mistake fails loudly instead of
// silently replacing an adapter. Registration is explicit by design: adapters
// are registered by their composition root (a Register helper per adapter
// package or the platform wiring), never from package init().
func (r *Registry) Register(id BackendID, adapter AgentAdapter) error {
	if !id.Valid() {
		return fmt.Errorf("%w: %q", ErrUnknownBackend, id)
	}
	if adapter == nil {
		return fmt.Errorf("%w: backend %q", ErrAdapterRequired, id)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.adapters == nil {
		r.adapters = make(map[BackendID]AgentAdapter)
	}
	if _, exists := r.adapters[id]; exists {
		return fmt.Errorf("%w: %q", ErrAdapterAlreadyRegistered, id)
	}
	r.adapters[id] = adapter
	return nil
}

// Get returns the adapter registered for id, and whether one exists.
func (r *Registry) Get(id BackendID) (AgentAdapter, bool) {
	if r == nil {
		return nil, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	adapter, ok := r.adapters[id]
	return adapter, ok
}

// Backends returns the registered backend ids in sorted order, so callers can
// report the available backends deterministically.
func (r *Registry) Backends() []BackendID {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := make([]BackendID, 0, len(r.adapters))
	for id := range r.adapters {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

// Close stops all registered adapters that implement BackendCloser.
func (r *Registry) Close(ctx context.Context) error {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	adapters := make([]AgentAdapter, 0, len(r.adapters))
	for _, a := range r.adapters {
		adapters = append(adapters, a)
	}
	r.mu.RUnlock()

	var firstErr error
	for _, a := range adapters {
		if closer, ok := a.(BackendCloser); ok {
			if err := closer.Close(ctx); err != nil && firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}
