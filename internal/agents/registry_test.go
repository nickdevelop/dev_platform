package agents

import (
	"errors"
	"testing"
)

// stubAdapter (defined in contracts_test.go) is the smallest AgentAdapter. The
// registry tests use distinct pointers so adapter identity is observable.
func TestRegistryRegisterAndGet(t *testing.T) {
	registry := NewRegistry()
	pi := &stubAdapter{}

	if _, ok := registry.Get(BackendPi); ok {
		t.Fatal("Get on an empty registry returned an adapter")
	}
	if _, ok := registry.Get("unknown"); ok {
		t.Fatal("Get on an unknown backend returned an adapter")
	}

	if err := registry.Register(BackendPi, pi); err != nil {
		t.Fatalf("Register: %v", err)
	}

	got, ok := registry.Get(BackendPi)
	if !ok {
		t.Fatal("Get(pi) = false after Register")
	}
	if got != AgentAdapter(pi) {
		t.Errorf("Get(pi) returned %#v, want the registered adapter", got)
	}
	if _, ok := registry.Get(BackendOpenCodeV2); ok {
		t.Error("Get(opencode-v2) returned an adapter that was never registered")
	}
}

func TestRegistryRejectsInvalidRegistrations(t *testing.T) {
	registry := NewRegistry()

	if err := registry.Register("claude-code", &stubAdapter{}); !errors.Is(err, ErrUnknownBackend) {
		t.Errorf("Register(unknown backend) error = %v, want ErrUnknownBackend", err)
	}

	if err := registry.Register(BackendPi, nil); err == nil {
		t.Error("Register(nil adapter) succeeded, want error")
	}

	if err := registry.Register(BackendPi, &stubAdapter{}); err != nil {
		t.Fatalf("first Register: %v", err)
	}
	if err := registry.Register(BackendPi, &stubAdapter{}); !errors.Is(err, ErrAdapterAlreadyRegistered) {
		t.Errorf("duplicate Register error = %v, want ErrAdapterAlreadyRegistered", err)
	}
}

// TestRegistryAdaptersAreIndependent proves one backend's adapter can be
// resolved without observing another's registration.
func TestRegistryAdaptersAreIndependent(t *testing.T) {
	registry := NewRegistry()
	pi := &stubAdapter{}
	opencode := &stubAdapter{}

	if err := registry.Register(BackendPi, pi); err != nil {
		t.Fatalf("register pi: %v", err)
	}
	if err := registry.Register(BackendOpenCodeV2, opencode); err != nil {
		t.Fatalf("register opencode: %v", err)
	}

	if got, _ := registry.Get(BackendPi); got != AgentAdapter(pi) {
		t.Error("Get(pi) returned the wrong adapter")
	}
	if got, _ := registry.Get(BackendOpenCodeV2); got != AgentAdapter(opencode) {
		t.Error("Get(opencode-v2) returned the wrong adapter")
	}
}
