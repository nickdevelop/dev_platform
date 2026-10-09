//go:build unix

package pi

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

// setProcessGroup makes the child the leader of a new process group so a stop
// can signal the whole tree (pi may spawn helper processes).
func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killProcessGroup SIGKILLs the child's process group. A group that is already
// gone is not an error: stopping an exited instance is a successful stop.
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
