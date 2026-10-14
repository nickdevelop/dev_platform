//go:build unix

package unix

import (
	"errors"
	"fmt"
	"os/exec"
	"syscall"
)

// Process is the unix child-process management: new process groups plus
// group-aware termination.
type Process struct{}

// Configure makes the child the leader of a new process group so a stop can
// signal the whole tree (children may spawn helper processes).
func (Process) Configure(cmd *exec.Cmd) error {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return nil
}

// Terminate ends pid and its children. A process that is already gone is not
// an error.
//
// The group signal goes out only when the target leads its own group. A pid
// from a registration file can share a group with unrelated work — a service
// started from a shell, or by a harness running the suite in the same group —
// and signalling that group would stop work nobody chose to stop. A failed
// group signal falls back to the leader-only kill.
func (Process) Terminate(pid int) error {
	if pid <= 0 {
		return errors.New("platform: no process to terminate")
	}
	if pgid, err := syscall.Getpgid(pid); err == nil && pgid == pid {
		if err := syscall.Kill(-pid, syscall.SIGKILL); err == nil || errors.Is(err, syscall.ESRCH) {
			return nil
		}
		// A failed group signal falls through to the leader-only kill below.
	}
	if err := syscall.Kill(pid, syscall.SIGKILL); err == nil || errors.Is(err, syscall.ESRCH) {
		return nil
	} else {
		return fmt.Errorf("platform: terminate process %d: %w", pid, err)
	}
}
