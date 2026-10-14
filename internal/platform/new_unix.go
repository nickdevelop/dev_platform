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

// Dirs returns the unix directory resolution.
func (p unixPlatform) Dirs() Dirs { return unixDirs{} }

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

// unixDirs adapts the unix implementation to the Dirs port. Path
// canonicalization moves to unix/pathpolicy.go in Task 2.6; until then the
// stub fails loudly so no caller can accidentally depend on it.
type unixDirs struct {
	unix.Dirs
}

func (d unixDirs) CanonicalizeUserPath(p string) (string, error) {
	return "", errors.New("platform: Dirs.CanonicalizeUserPath is wired in Task 2.6")
}

// SamePath is case sensitive on unix; Task 2.6 confirms the final form
// alongside the path policy.
func (d unixDirs) SamePath(a, b string) bool { return a == b }
