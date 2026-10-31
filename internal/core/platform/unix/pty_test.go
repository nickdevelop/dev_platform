//go:build unix

package unix_test

import (
	"os/exec"
	"runtime"
	"testing"
	"time"

	"devplatform/internal/core/platform"
	"devplatform/internal/core/platform/unix"
)

func TestPTYSessionReapsChildProcess(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("pty is only supported on Linux")
	}

	cmd := exec.Command("sh", "-c", "echo hello && exit 0")
	pty := unix.PTY{}
	sess, err := pty.Start(cmd, 24, 80)
	if err != nil {
		t.Fatalf("pty start: %v", err)
	}

	pid := sess.Pid()
	if pid <= 0 {
		t.Fatalf("expected valid pid, got %d", pid)
	}

	// Wait for process to exit and be reaped
	err = sess.Wait()
	if err != nil {
		t.Logf("sess.Wait returned: %v", err)
	}

	proc := platform.New().Process()
	// Allow small window for OS status update
	time.Sleep(50 * time.Millisecond)

	if proc.IsZombie(pid) {
		t.Fatalf("process %d is a zombie, reaper failed to wait on child process", pid)
	}

	// Close should be safe and idempotent
	if err := sess.Close(); err != nil {
		t.Fatalf("sess.Close: %v", err)
	}
}

func TestPTYSessionCloseKillsAndReaps(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("pty is only supported on Linux")
	}

	cmd := exec.Command("sh", "-c", "sleep 60")
	pty := unix.PTY{}
	sess, err := pty.Start(cmd, 24, 80)
	if err != nil {
		t.Fatalf("pty start: %v", err)
	}

	pid := sess.Pid()
	proc := platform.New().Process()
	if !proc.Alive(pid) {
		t.Fatalf("process %d should be alive", pid)
	}

	// Close immediately kills the child and reaps it
	if err := sess.Close(); err != nil {
		t.Fatalf("sess.Close: %v", err)
	}

	time.Sleep(50 * time.Millisecond)
	if proc.IsZombie(pid) {
		t.Fatalf("process %d is a zombie after Close(), reaper failed", pid)
	}
	if proc.Alive(pid) {
		t.Fatalf("process %d should not be alive after Close()", pid)
	}
}
