//go:build unix

package unix_test

import (
	"os/exec"
	"syscall"
	"testing"
	"time"

	"devplatform/internal/platform"
)

// startDetached starts a command without waiting for it, so the caller can
// observe the process the way a registration file sees it.
func startDetached(t *testing.T, name string, args ...string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(name, args...)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start %s: %v", name, err)
	}
	t.Cleanup(func() { _, _ = cmd.Process.Wait() })
	return cmd
}

// TestTerminateSparesForeignGroup is the harness case: a pid from a
// registration file can share a process group with unrelated work — the
// runner that started it, say. Signalling that group stops work nobody chose
// to stop, so only the named process may die.
func TestTerminateSparesForeignGroup(t *testing.T) {
	child := startDetached(t, "sh", "-c", "sleep 30")
	if err := platform.New().Process().Terminate(child.Process.Pid); err != nil {
		t.Fatalf("terminate: %v", err)
	}
	// This process is still here to assert it: a group signal would have been
	// delivered to its own group and taken it down with the child.
	done := make(chan error, 1)
	go func() { done <- child.Wait() }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("the child exited cleanly; it should have been killed")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the child survived Terminate")
	}
}

// TestTerminateStopsGroupLeader covers the process that leads its own group
// — how the platform starts its children — whose helper processes must go
// with it.
func TestTerminateStopsGroupLeader(t *testing.T) {
	cmd := exec.Command("sh", "-c", "sleep 30")
	if err := platform.New().Process().Configure(cmd); err != nil {
		t.Fatalf("configure process group: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() { _, _ = cmd.Process.Wait() })
	// Configure must have made the child a group leader for the group
	// signal to apply.
	if pgid, err := syscall.Getpgid(cmd.Process.Pid); err != nil || pgid != cmd.Process.Pid {
		t.Fatalf("Getpgid = %d, %v; want leader of its own group", pgid, err)
	}
	if err := platform.New().Process().Terminate(cmd.Process.Pid); err != nil {
		t.Fatalf("terminate: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the group leader survived Terminate")
	}
}

// TestTerminateMissingProcessIsSuccess keeps a repeated stop harmless:
// stopping an exited process is a successful stop. A very high pid that the
// kernel has no record of exercises the ESRCH path deterministically.
func TestTerminateMissingProcessIsSuccess(t *testing.T) {
	const missingPid = 1 << 22
	if _, err := syscall.Getpgid(missingPid); err == nil {
		t.Skip("the probe pid unexpectedly exists")
	}
	if err := platform.New().Process().Terminate(missingPid); err != nil {
		t.Fatalf("terminate a missing process: %v", err)
	}
	if err := platform.New().Process().Terminate(-1); err == nil {
		t.Fatal("terminate an invalid pid succeeded, want an error")
	}
}
