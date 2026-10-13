//go:build unix

package opencode

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

// setProcessGroup makes the server the leader of a new process group so a stop
// can signal the whole tree (opencode may spawn helper processes).
func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killRegisteredProcess stops the service process named in the registration
// file — a process the platform did not necessarily start. Stopping it was the
// operator's decision, taken on the platform settings page. A process that is
// already gone is not an error.
func killRegisteredProcess(pid int) error {
	if pid <= 0 {
		return errors.New("opencode: no service process to stop")
	}
	// The service leads its own process group, so the group dies with it; a
	// service that is not a group leader is killed directly.
	if err := syscall.Kill(-pid, syscall.SIGKILL); err == nil {
		return nil
	}
	err := syscall.Kill(pid, syscall.SIGKILL)
	if err == nil || errors.Is(err, syscall.ESRCH) {
		return nil
	}
	return fmt.Errorf("opencode: stop service pid %d: %w", pid, err)
}

// killProcessGroup SIGKILLs the server's process group. A group that is already
// gone is not an error: stopping an exited server is a successful stop.
func killProcessGroup(proc *os.Process) error {
	if proc == nil {
		return nil
	}
	err := syscall.Kill(-proc.Pid, syscall.SIGKILL)
	if err == nil || errors.Is(err, syscall.ESRCH) {
		return nil
	}
	// Fall back to killing just the leader if the group signal failed.
	return proc.Kill()
}
