//go:build unix

package platform

import (
	"errors"
	"os/exec"
	"runtime"

	"devplatform/internal/platform/policy"
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

// Dirs returns the unix directory resolution and path policy.
func (p unixPlatform) Dirs() Dirs { return unix.Dirs{} }

// Trash returns the unix trash mover.
func (p unixPlatform) Trash() Trash { return unixTrash{} }

// Shutdown returns the unix termination signals.
func (p unixPlatform) Shutdown() Shutdown { return unix.Shutdown{} }

// Executables returns the unix command lookup.
func (p unixPlatform) Executables() Executables { return unix.Executables{} }

// Perms returns the unix private permissions.
func (p unixPlatform) Perms() Perms { return unix.Perms{} }

// Environment returns the unix startup checks.
func (p unixPlatform) Environment() Environment { return unix.Environment{} }

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

func (unixProcess) Alive(pid int) bool    { return unix.Process{}.Alive(pid) }
func (unixProcess) IsZombie(pid int) bool { return unix.Process{}.IsZombie(pid) }

func (unixProcess) Terminate(pid int) error {
	return unix.Process{}.Terminate(pid)
}

// unixTrash adapts the unix implementation to the Trash port.
type unixTrash struct{}

func (unixTrash) Move(path string) error {
	err := unix.Trash{}.Move(path)
	if err != nil && errors.Is(err, policy.ErrTrashUnavailable) {
		// Map the implementation's copy to the canonical port value;
		// callers match with errors.Is against platform.ErrTrashUnavailable.
		return ErrTrashUnavailable
	}
	return err
}
