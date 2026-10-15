//go:build windows

package windows

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"

	"golang.org/x/sys/windows"
)

// Process is the Windows child-process management: a private process group
// per child plus enrollment in the platform Job Object.
type Process struct{}

// Configure gives the child its own process group so Ctrl+C at the platform
// console does not reach it; stopping stays an explicit Terminate.
func (Process) Configure(cmd *exec.Cmd) error {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags = windows.CREATE_NEW_PROCESS_GROUP
	return nil
}

// Attach enrolls an already-started platform child in the platform Job
// Object, so the OS reaps it when the platform exits. Call it right after
// cmd.Start; the process handle only exists then.
func (Process) Attach(proc *os.Process) error {
	if proc == nil {
		return nil
	}
	// Open a fresh handle by pid: os.Process keeps its own handle private,
	// and a self-opened handle needs no lifetime coordination with the
	// *os.Process owner.
	handle, err := windows.OpenProcess(
		windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(proc.Pid))
	if err != nil {
		return fmt.Errorf("platform: open child process %d: %w", proc.Pid, err)
	}
	defer windows.CloseHandle(handle)
	return assignToPlatformJob(handle)
}

// stillActive is the GetExitCodeProcess sentinel for a running process
// (STILL_ACTIVE).
const stillActive = 259

// Alive reports whether pid currently runs: the handle must open and the
// exit code must still be STILL_ACTIVE.
func (Process) Alive(pid int) bool {
	if pid <= 0 {
		return false
	}
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(handle)
	var code uint32
	if err := windows.GetExitCodeProcess(handle, &code); err != nil {
		return false
	}
	return code == stillActive
}

// IsZombie always reports false: Windows has no zombie state, a terminated
// process is reaped with its last handle close.
func (Process) IsZombie(int) bool {
	return false
}

// Terminate ends pid and its child tree via taskkill /T /F. A process that
// is already gone is not an error: the post-kill liveness check keeps this
// locale-independent (taskkill's own "not found" text is localized).
func (Process) Terminate(pid int) error {
	if pid <= 0 {
		return errors.New("platform: no process to terminate")
	}
	out, err := exec.Command(taskkillPath(), "/T", "/F", "/PID", strconv.Itoa(pid)).CombinedOutput()
	if err == nil {
		return nil
	}
	if !(Process{}).Alive(pid) {
		return nil
	}
	return fmt.Errorf("platform: terminate process %d: %v: %s", pid, err, out)
}

// taskkillPath locates the system taskkill; it always ships with Windows.
func taskkillPath() string {
	if systemRoot := os.Getenv("SystemRoot"); systemRoot != "" {
		candidate := filepath.Join(systemRoot, "System32", "taskkill.exe")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return "taskkill"
}
