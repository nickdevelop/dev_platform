package agy

import (
	"devplatform/internal/core/agents"
	"testing"
)

// TestDescriptorMatchesCompatibilityMatrix pins the agy descriptor to the tested
// range documented in docs/agent-compatibility.md.
func TestDescriptorMatchesCompatibilityMatrix(t *testing.T) {
	d := Descriptor
	if d.ID != agents.BackendAgy || d.Binary != "agy" || d.Label != "Antigravity (AGY)" {
		t.Fatalf("identity = %+v", d)
	}
	if d.ShortName != "AGY" {
		t.Fatalf("short name = %q, want AGY", d.ShortName)
	}
	if d.Color != "#4285F4" {
		t.Fatalf("color = %q, want #4285F4", d.Color)
	}
	if d.Tested != "1.5.6" || d.Minimum != "1.0.0" || d.Maximum != "2.0.0" {
		t.Fatalf("version range = %+v, want tested 1.5.6 in [1.0.0, 2.0.0)", d)
	}
	if !d.DefaultEnabled {
		t.Fatal("agy must be enabled by default")
	}
	if d.ServiceSupported {
		t.Fatal("agy must not declare service support")
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
