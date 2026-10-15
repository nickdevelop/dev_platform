//go:build windows

package windows_test

import (
	"os"
	"testing"

	"devplatform/internal/core/platform"
)

// TestWindowsShutdownSignals pins the termination contract: Ctrl+C and the
// runtime-mapped close-event signal both feed graceful shutdown.
func TestWindowsShutdownSignals(t *testing.T) {
	signals := platform.New().Shutdown().Signals()
	if len(signals) == 0 {
		t.Fatal("Signals() is empty")
	}
	var interrupt, sigterm bool
	for _, sig := range signals {
		if sig == os.Interrupt {
			interrupt = true
		}
		if sig.String() == "terminated" {
			sigterm = true
		}
	}
	if !interrupt || !sigterm {
		t.Fatalf("Signals() = %v, want Ctrl+C and SIGTERM", signals)
	}
}
