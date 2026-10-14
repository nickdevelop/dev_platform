//go:build unix

package unix_test

import (
	"errors"
	"testing"

	"devplatform/internal/platform"
)

// TestUnixInstancePortIsWired verifies New().Instance() is a working lock,
// not the nil placeholder: a second acquisition fails with ErrLockHeld.
func TestUnixInstancePortIsWired(t *testing.T) {
	dataDir := t.TempDir()

	first, err := platform.New().Instance().Acquire(dataDir)
	if err != nil {
		t.Fatalf("Acquire(first): %v", err)
	}
	t.Cleanup(func() { _ = first.Release() })

	if _, err := platform.New().Instance().Acquire(dataDir); !errors.Is(err, platform.ErrLockHeld) {
		t.Fatalf("Acquire(second) error = %v, want ErrLockHeld", err)
	}
}
