package platform

import (
	"runtime"
	"testing"
)

// TestNewReportsRuntimeName pins the only promise New makes at this
// stage: a non-nil Platform whose Name is runtime.GOOS for diagnostics.
func TestNewReportsRuntimeName(t *testing.T) {
	p := New()
	if p == nil {
		t.Fatal("New returned nil Platform")
	}
	if got, want := p.Name(), runtime.GOOS; got != want {
		t.Fatalf("Name() = %q, want runtime.GOOS %q", got, want)
	}
}

// TestStubImplementsPlatform keeps the placeholder assignment-compatible
// while the real per-platform constructors are still unwired.
func TestStubImplementsPlatform(t *testing.T) {
	var _ Platform = stubPlatform{}
	var _ Platform = New()
}
