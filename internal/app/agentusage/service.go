// Package agentusage provides quota and usage statistics for agent backends.
// The package depends only on the agent contracts and the adapter registry.
// It never names a concrete backend.
package agentusage

import (
	"context"
	"errors"
	"fmt"

	"devplatform/internal/core/agents"
)

// Sentinel errors for usage operations.
var (
	// ErrUnknownBackend reports an unknown backend id or one not registered.
	ErrUnknownBackend = errors.New("agentusage: unknown or unregistered backend")
	// ErrUnsupportedCapability reports a backend that does not implement UsageReporter.
	ErrUnsupportedCapability = errors.New("agentusage: backend does not support usage reporting")
	// ErrUsageFailed reports a failure while querying usage from the adapter.
	ErrUsageFailed = errors.New("agentusage: failed to query usage")
)

// Registry resolves a backend id to its adapter. *agents.Registry satisfies it.
type Registry interface {
	Get(id agents.BackendID) (agents.AgentAdapter, bool)
}

// Service queries usage and quota information from agent backends.
type Service struct {
	registry Registry
}

// NewService builds the usage service over the adapter registry.
func NewService(registry Registry) *Service {
	return &Service{registry: registry}
}

// GetUsage retrieves the current usage report from the given backend.
func (s *Service) GetUsage(ctx context.Context, backend agents.BackendID) (*agents.UsageReport, error) {
	adapter, ok := s.registry.Get(backend)
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownBackend, backend)
	}

	reporter, ok := adapter.(agents.UsageReporter)
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedCapability, backend)
	}

	report, err := reporter.GetUsage(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUsageFailed, err)
	}

	return report, nil
}
