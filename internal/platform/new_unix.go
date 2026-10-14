//go:build unix

package platform

import (
	"errors"
	"os/exec"
	"runtime"

	"devplatform/internal/platform/unix"
)

// New returns the unix Platform (shared by Linux and WSL2). Implemented
// ports are wired below; the rest still come from the embedded stub until
// their phase 2/3 tasks land.
func New() Platform {
	return unixPlatform{stubPlatform: stubPlatform{name: runtime.GOOS}}
}

// unixPlatform wires the implemented unix ports.
type unixPlatform struct {
	stubPlatform
}

// Instance returns the unix single-instance lock.
func (p unixPlatform) Instance() Instance { return unixInstance{} }

// Process returns the unix child-process management.
func (p unixPlatform) Process() Process { return unixProcess{} }

// unixInstance adapts the unix implementation to the Instance port.
type unixInstance struct{}

func (unixInstance) Acquire(dataDir string) (Lock, error) {
	l, err := unix.Instance{}.Acquire(dataDir)
	if err != nil {
		// Map the implementation's copy to the canonical port value;
		// callers match with errors.Is against platform.ErrLockHeld.
		if errors.Is(err, unix.ErrLockHeld) {
			return nil, ErrLockHeld
		}
		return nil, err
	}
	return l, nil
}

// unixProcess adapts the unix implementation to the Process port.
type unixProcess struct{}

func (unixProcess) Configure(cmd *exec.Cmd) error {
	return unix.Process{}.Configure(cmd)
}

// Alive and IsZombie are wired in Task 2.3; until then the stub reports
// absence. No caller uses them yet (discovery and the pi tests still use
// their local implementations).
func (unixProcess) Alive(pid int) bool    { return false }
func (unixProcess) IsZombie(pid int) bool { return false }

func (unixProcess) Terminate(pid int) error {
	return unix.Process{}.Terminate(pid)
}
