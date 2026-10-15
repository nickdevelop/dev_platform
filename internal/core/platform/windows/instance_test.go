//go:build windows

package windows_test

import (
	"errors"
	"testing"

	"devplatform/internal/core/platform"
)

// TestWindowsInstancePortIsWired verifies New().Instance() is a working
// lock, not the nil placeholder: a second acquisition fails with
// ErrLockHeld, and a released lock is reacquirable. Lock release on process
// exit is an OS guarantee (the handle closes with the process).
func TestWindowsInstancePortIsWired(t *testing.T) {
	dataDir := t.TempDir()

	first, err := platform.New().Instance().Acquire(dataDir)
	if err != nil {
		t.Fatalf("Acquire(first): %v", err)
	}
	t.Cleanup(func() { _ = first.Release() })

	if _, err := platform.New().Instance().Acquire(dataDir); !errors.Is(err, platform.ErrLockHeld) {
		t.Fatalf("Acquire(second) error = %v, want ErrLockHeld", err)
	}

	if err := first.Release(); err != nil {
		t.Fatalf("Release: %v", err)
	}
	again, err := platform.New().Instance().Acquire(dataDir)
	if err != nil {
		t.Fatalf("Acquire after release: %v", err)
	}
	_ = again.Release()
}
