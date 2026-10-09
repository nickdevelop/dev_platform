//go:build !unix

package pi

import (
	"os"
	"os/exec"
)

// setProcessGroup is a no-op on platforms without process groups.
func setProcessGroup(cmd *exec.Cmd) {}

// killProcessGroup kills the child directly where process groups are
// unavailable.
func killProcessGroup(proc *os.Process) error {
	if proc == nil {
		return nil
	}
	return proc.Kill()
}
