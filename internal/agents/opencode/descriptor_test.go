package opencode

import (
	"devplatform/internal/agents"
	"testing"
)

// TestDescriptorMatchesCompatibilityMatrix pins the opencode descriptor to the
// tested range documented in docs/agent-compatibility.md.
func TestDescriptorMatchesCompatibilityMatrix(t *testing.T) {
	d := Descriptor
	if d.ID != agents.BackendOpenCodeV2 || d.Binary != "opencode" || d.Label != "OpenCode" {
		t.Fatalf("identity = %+v", d)
	}
	if d.Tested != "2.0.24" || d.Minimum != "2.0.0" || d.Maximum != "3.0.0" {
		t.Fatalf("version range = %+v, want tested 2.0.24 in [2.0.0, 3.0.0)", d)
	}
	if !d.DefaultEnabled {
		t.Fatal("opencode must be enabled by default")
	}
}

// TestDescriptorServiceFlagMatchesAdapter proves the descriptor's service flag
// agrees with whether the adapter really implements the machine-service
// contract.
func TestDescriptorServiceFlagMatchesAdapter(t *testing.T) {
	var adapter any = New(Options{})
	_, implements := adapter.(agents.ServiceController)
	if implements != Descriptor.ServiceSupported {
		t.Fatalf("ServiceSupported = %v, adapter implements ServiceController = %v", Descriptor.ServiceSupported, implements)
	}
}

// TestDescriptorBoundsAreParseable proves every declared bound can classify a
// version; an unparseable bound would make the range match nothing.
func TestDescriptorBoundsAreParseable(t *testing.T) {
	for name, bound := range map[string]string{"tested": Descriptor.Tested, "minimum": Descriptor.Minimum, "maximum": Descriptor.Maximum} {
		if _, ok := agents.ParseVersion(bound); !ok {
			t.Errorf("%s bound %q is not a MAJOR.MINOR.PATCH version", name, bound)
		}
	}
}
