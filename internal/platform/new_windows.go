//go:build windows

package platform

import (
	"errors"
	"runtime"

	"devplatform/internal/platform/windows"
)

// New returns the windows Platform. Implemented ports are wired below; the
// rest still come from the embedded stub until their phase 4 tasks land.
func New() Platform {
	return windowsPlatform{stubPlatform: stubPlatform{name: runtime.GOOS}}
}

// windowsPlatform wires the implemented windows ports.
type windowsPlatform struct {
	stubPlatform
}

// Instance returns the Windows single-instance lock.
func (p windowsPlatform) Instance() Instance { return windowsInstance{} }

// windowsInstance adapts the windows implementation to the Instance port.
type windowsInstance struct{}

func (windowsInstance) Acquire(dataDir string) (Lock, error) {
	l, err := windows.Instance{}.Acquire(dataDir)
	if err != nil {
		// Map the implementation's copy to the canonical port value;
		// callers match with errors.Is against platform.ErrLockHeld.
		if errors.Is(err, windows.ErrLockHeld) {
			return nil, ErrLockHeld
		}
		return nil, err
	}
	return l, nil
}
