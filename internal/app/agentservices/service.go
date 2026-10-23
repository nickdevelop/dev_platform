// Package agentservices controls the machine-level services of agent backends
// (for example the `opencode serve --service` process): detection, start and
// stop on an operator's request, and the cheap availability gate the session
// layer consults before it starts an instance.
//
// The package depends only on the agent contracts and the adapter registry. It
// never starts a service from a business path: start and stop are reachable
// only through the platform settings routes.
package agentservices

import (
	"context"
	"errors"
	"fmt"
	"time"

	"devplatform/internal/core/agents"
)

// ErrUnknownBackend reports a backend id that is unknown or has no registered
// adapter.
var ErrUnknownBackend = errors.New("agentservices: unknown or unregistered backend")

// Registry resolves a backend id to its adapter. *agents.Registry satisfies it.
type Registry interface {
	Get(id agents.BackendID) (agents.AgentAdapter, bool)
}

// Service controls the machine-level services of registered backends.
type Service struct {
	registry Registry
	now      func() time.Time
}

// NewService builds the service facade over the adapter registry.
func NewService(registry Registry) *Service {
	return &Service{registry: registry, now: func() time.Time { return time.Now().UTC() }}
}

// controller resolves the optional machine-service control of a backend. The
// boolean reports whether the backend has one at all.
func (s *Service) controller(backend agents.BackendID) (agents.ServiceController, bool, error) {
	adapter, ok := s.registry.Get(backend)
	if !ok {
		return nil, false, fmt.Errorf("%w: %q (%w)", ErrUnknownBackend, backend, agents.ErrBackendNotRegistered)
	}
	controller, ok := adapter.(agents.ServiceController)
	return controller, ok, nil
}

// Detect reports the machine-level service of a backend. A backend with no such
// service is reported as `supported: false` rather than as an error: the
// settings page shows the service block only where one exists.
func (s *Service) Detect(ctx context.Context, backend agents.BackendID) (agents.ServiceStatus, error) {
	controller, ok, err := s.controller(backend)
	if err != nil {
		return agents.ServiceStatus{}, err
	}
	if !ok {
		return agents.ServiceStatus{Backend: backend, Supported: false, State: agents.ServiceStopped, CheckedAt: s.now()}, nil
	}
	status, err := controller.DetectService(ctx)
	if err != nil {
		return agents.ServiceStatus{}, err
	}
	status.Supported = true
	status.Backend = backend
	return status, nil
}

// Start starts a backend's machine-level service on an operator's request.
func (s *Service) Start(ctx context.Context, backend agents.BackendID) (agents.ServiceStatus, error) {
	controller, ok, err := s.controller(backend)
	if err != nil {
		return agents.ServiceStatus{}, err
	}
	if !ok {
		return agents.ServiceStatus{}, fmt.Errorf("%w: %q", agents.ErrServiceUnsupported, backend)
	}
	status, err := controller.StartService(ctx)
	return status, err
}

// Stop stops a backend's machine-level service on an operator's request. The
// operator owns the consequences: the service may be serving other clients.
func (s *Service) Stop(ctx context.Context, backend agents.BackendID) (agents.ServiceStatus, error) {
	controller, ok, err := s.controller(backend)
	if err != nil {
		return agents.ServiceStatus{}, err
	}
	if !ok {
		return agents.ServiceStatus{}, fmt.Errorf("%w: %q", agents.ErrServiceUnsupported, backend)
	}
	status, err := controller.StopService(ctx)
	return status, err
}

// Available is the gate the session layer consults before it starts an
// instance: a backend that runs a machine-level service refuses when that
// service is down, and the caller must pass the refusal on rather than bring the
// service up itself. A backend with no service is always available.
func (s *Service) Available(backend agents.BackendID) error {
	controller, ok, err := s.controller(backend)
	if err != nil || !ok {
		return err
	}
	return controller.ServiceAvailable()
}
