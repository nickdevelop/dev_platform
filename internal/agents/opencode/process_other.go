//go:build !unix

package opencode

import (
	"errors"
	"os"
	"os/exec"
)

// setProcessGroup is a no-op on platforms without process groups.
func setProcessGroup(cmd *exec.Cmd) {}

// killRegisteredProcess stops the service process named in the registration
// file where process groups are unavailable.
func killRegisteredProcess(pid int) error {
	if pid <= 0 {
		return errors.New("opencode: no service process to stop")
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return proc.Kill()
}

// processIsZombie cannot tell: these platforms have no zombie pid that keeps a
// listener closed, so a live pid is treated as live.
func processIsZombie(pid int) bool { return false }

// killProcessGroup kills the server directly where process groups are
// unavailable.
func killProcessGroup(proc *os.Process) error {
	if proc == nil {
		return nil
	}
	return proc.Kill()
}
