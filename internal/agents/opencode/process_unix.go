//go:build unix

package opencode

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
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
//
// The group signal is sent only when the target leads its own group. A
// registration can name a process that shares a group with unrelated work — a
// service started from a shell, or by a harness that runs the suite in the same
// group — and signalling that group would stop work the operator never chose to
// stop.
func killRegisteredProcess(pid int) error {
	if pid <= 0 {
		return errors.New("opencode: no service process to stop")
	}
	if pgid, err := syscall.Getpgid(pid); err == nil && pgid == pid {
		if err := syscall.Kill(-pid, syscall.SIGKILL); err == nil {
			return nil
		} else if !errors.Is(err, syscall.ESRCH) {
			return fmt.Errorf("opencode: stop service process group %d: %w", pid, err)
		}
	}
	err := syscall.Kill(pid, syscall.SIGKILL)
	if err == nil || errors.Is(err, syscall.ESRCH) {
		return nil
	}
	return fmt.Errorf("opencode: stop service pid %d: %w", pid, err)
}

// processIsZombie reports whether the pid names a process that has exited but
// was never reaped by its parent. Such a pid answers signal(0) — so the
// registration looks alive — while its listener is closed and nothing will ever
// answer on it again. A stopped service whose parent is a shell that moved on is
// exactly this, and reporting it as "running but unreachable" would tell the
// operator to debug a service that is simply gone.
func processIsZombie(pid int) bool {
	if pid <= 0 {
		return false
	}
	raw, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return false
	}
	// The comm field is parenthesised and may itself contain spaces or ')';
	// the state field follows the last ')'.
	stat := string(raw)
	end := strings.LastIndexByte(stat, ')')
	if end < 0 || end+2 >= len(stat) {
		return false
	}
	return stat[end+2] == 'Z'
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
