//go:build windows

package windows_test

import (
	"errors"
	"testing"

	"devplatform/internal/core/platform"
)

// TestWindowsTrashUnavailable pins the accepted gap: moving to the trash
// always reports unavailable, so deletion falls back to permanent removal
// with confirmation.
func TestWindowsTrashUnavailable(t *testing.T) {
	err := platform.New().Trash().Move(`C:\work\proj`)
	if !errors.Is(err, platform.ErrTrashUnavailable) {
		t.Fatalf("Move() = %v, want ErrTrashUnavailable", err)
	}
}
