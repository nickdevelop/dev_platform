package pi

import (
	"devplatform/internal/agents"
	"testing"
)

// TestDescriptorMatchesCompatibilityMatrix pins the pi descriptor to the tested
// range documented in docs/agent-compatibility.md.
func TestDescriptorMatchesCompatibilityMatrix(t *testing.T) {
	d := Descriptor
	if d.ID != agents.BackendPi || d.Binary != "pi" || d.Label != "Pi Agent" {
		t.Fatalf("identity = %+v", d)
	}
	if d.Tested != "1.1.0" || d.Minimum != "1.0.0" || d.Maximum != "2.0.0" {
		t.Fatalf("version range = %+v, want tested 1.1.0 in [1.0.0, 2.0.0)", d)
	}
	if !d.DefaultEnabled {
		t.Fatal("pi must be enabled by default")
	}
}

// TestDescriptorServiceFlagMatchesAdapter proves pi declares no machine-level
// service, and the descriptor agrees with the adapter.
func TestDescriptorServiceFlagMatchesAdapter(t *testing.T) {
	var adapter any = New(Options{})
	_, implements := adapter.(agents.ServiceController)
	if implements || Descriptor.ServiceSupported {
		t.Fatalf("pi must not expose a machine-level service: descriptor=%v implements=%v", Descriptor.ServiceSupported, implements)
	}
}
