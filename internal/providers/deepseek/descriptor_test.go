package deepseek

import (
	"testing"

	"devplatform/internal/core/agents"
)

// TestDescriptorMatchesCompatibilityMatrix pins the deepseek descriptor to the tested
// range documented in docs/agent-compatibility.md.
func TestDescriptorMatchesCompatibilityMatrix(t *testing.T) {
	d := Descriptor
	if d.ID != agents.BackendDeepSeek || d.Binary != "dsh" || d.Label != "DeepSeek" || d.Tone != "info" {
		t.Fatalf("identity = %+v", d)
	}
	if d.Tested != "0.2.1-rc.2" || d.Minimum != "0.2.0" || d.Maximum != "1.0.0" {
		t.Fatalf("version range = %+v, want tested 0.2.1-rc.2 in [0.2.0, 1.0.0)", d)
	}
	if !d.DefaultEnabled {
		t.Fatal("deepseek must be enabled by default")
	}
}

// TestDescriptorServiceFlagMatchesAdapter proves deepseek declares no machine-level
// service, and the descriptor agrees with the adapter.
func TestDescriptorServiceFlagMatchesAdapter(t *testing.T) {
	var adapter any = New(Options{})
	_, implements := adapter.(agents.ServiceController)
	if implements || Descriptor.ServiceSupported {
		t.Fatalf("deepseek must not expose a machine-level service: descriptor=%v implements=%v", Descriptor.ServiceSupported, implements)
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
