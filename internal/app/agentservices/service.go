// Package agentservices controls the machine-level services of agent backends
// (for example the `opencode serve --service` process): detection, start and
// stop on an operator's request, and the cheap availability gate the session
// layer consults before it starts an instance.
//
// The package depends only on the agent contracts and the adapter registry. It
// never starts a service from a business path: start and stop are reachable
// only through the platform settings routes, and a change invalidates the
// backend's cached model lists through the hook the composition root installs.
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

// Option configures a Service.
type Option func(*Service)

// WithChangeHook installs a function called after every start or stop attempt,
// whatever its outcome. The composition root uses it to drop the backend's
// cached model lists: a service that started or stopped can change what the
// backend answers.
func WithChangeHook(fn func(agents.BackendID)) Option {
	return func(s *Service) { s.onChange = fn }
}

// Service controls the machine-level services of registered backends.
type Service struct {
	registry Registry
	onChange func(agents.BackendID)
	now      func() time.Time
}

// NewService builds the service facade over the adapter registry.
func NewService(registry Registry, opts ...Option) *Service {
	s := &Service{registry: registry, now: func() time.Time { return time.Now().UTC() }}
	for _, opt := range opts {
		opt(s)
	}
	return s
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
	s.changed(backend)
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
	s.changed(backend)
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

// changed runs the change hook, when one is installed.
func (s *Service) changed(backend agents.BackendID) {
	if s.onChange != nil {
		s.onChange(backend)
	}
}
