package settings

import (
	"testing"

	"devplatform/internal/core/agents"
	"devplatform/internal/providers/opencode"
	"devplatform/internal/providers/pi"
)

// testCatalog is the catalog the composition root builds, from the real
// adapter descriptors.
func testCatalog(t testing.TB) *agents.Catalog {
	t.Helper()
	catalog, err := agents.NewCatalog(opencode.Descriptor, pi.Descriptor)
	if err != nil {
		t.Fatalf("NewCatalog: %v", err)
	}
	return catalog
}

// TestUnknownBackendIsNeverResolved proves settings does not fall back to any
// default executable for a backend the catalog does not describe.
func TestUnknownBackendIsNeverResolved(t *testing.T) {
	svc := NewService(openTestDB(t), testCatalog(t))
	path, source := svc.ResolvePath(agents.BackendID("claude-code"))
	if path != "" || source != "none" {
		t.Fatalf("ResolvePath(unknown) = %q, %q; want empty and none", path, source)
	}
}

// TestCatalogDrivesAgentDefaults proves the defaults come from the catalog,
// not from a table inside settings: a descriptor's enablement is the default.
func TestCatalogDrivesAgentDefaults(t *testing.T) {
	catalog, err := agents.NewCatalog(agents.BackendDescriptor{ID: agents.BackendPi, Binary: "pi", DefaultEnabled: false, Order: 1})
	if err != nil {
		t.Fatalf("NewCatalog: %v", err)
	}
	svc := NewService(openTestDB(t), catalog)
	got, err := svc.Agents()
	if err != nil {
		t.Fatalf("Agents: %v", err)
	}
	if len(got) != 1 || got[agents.BackendPi].Enabled {
		t.Fatalf("Agents() = %+v, want only pi, disabled by its descriptor", got)
	}
}
