package main

import (
	"testing"

	"devplatform/internal/core/agents"
)

type wiringStubAdapter struct{ agents.AgentAdapter }

func TestCheckBackendWiringAcceptsMatchingSets(t *testing.T) {
	catalog, err := agents.NewCatalog(agents.BackendDescriptor{ID: agents.BackendPi, Binary: "pi"})
	if err != nil {
		t.Fatalf("NewCatalog: %v", err)
	}
	registry := agents.NewRegistry()
	if err := registry.Register(agents.BackendPi, wiringStubAdapter{}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := checkBackendWiring(registry, catalog); err != nil {
		t.Fatalf("checkBackendWiring = %v, want nil", err)
	}
}

func TestCheckBackendWiringRejectsRegisteredWithoutDescriptor(t *testing.T) {
	catalog, err := agents.NewCatalog()
	if err != nil {
		t.Fatalf("NewCatalog: %v", err)
	}
	registry := agents.NewRegistry()
	if err := registry.Register(agents.BackendPi, wiringStubAdapter{}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := checkBackendWiring(registry, catalog); err == nil {
		t.Fatal("checkBackendWiring must reject an adapter with no descriptor")
	}
}

func TestCheckBackendWiringRejectsDescribedWithoutAdapter(t *testing.T) {
	catalog, err := agents.NewCatalog(agents.BackendDescriptor{ID: agents.BackendPi, Binary: "pi"})
	if err != nil {
		t.Fatalf("NewCatalog: %v", err)
	}
	if err := checkBackendWiring(agents.NewRegistry(), catalog); err == nil {
		t.Fatal("checkBackendWiring must reject a descriptor with no adapter")
	}
}
