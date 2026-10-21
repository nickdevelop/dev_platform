//go:build unix

package unix

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

// Process is the unix child-process management: new process groups plus
// group-aware termination.
type Process struct{}

// Configure makes the child the leader of a new process group so a stop can
// signal the whole tree (children may spawn helper processes).
func (Process) Configure(cmd *exec.Cmd) error {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
	setPdeathsig(cmd)
	return nil
}

// Attach enrolls an already-started platform child into supervision. Unix
// needs nothing beyond the process group from Configure: a child outlives
// nothing because the platform kills the group on stop.
func (Process) Attach(proc *os.Process) error {
	return nil
}

// Alive reports whether pid answers a signal. A zombie answers too: use
// IsZombie to tell a live process from an unreaped one.
func (Process) Alive(pid int) bool {
	if pid <= 0 {
		return false
	}
	return syscall.Kill(pid, 0) == nil
}

// IsZombie reports whether pid names a process that has exited but was never
// reaped by its parent. Such a pid answers signal(0) while its listener is
// closed and nothing will ever answer on it again.
func (Process) IsZombie(pid int) bool {
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
