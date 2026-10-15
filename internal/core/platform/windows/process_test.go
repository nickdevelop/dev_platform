//go:build windows

package windows_test

import (
	"os/exec"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"devplatform/internal/core/platform"
)

// TestWindowsConfigureSetsNewProcessGroup pins the pre-Start half: the
// child gets its own process group so console Ctrl+C never reaches it.
func TestWindowsConfigureSetsNewProcessGroup(t *testing.T) {
	cmd := exec.Command("ping", "-n", "1", "127.0.0.1")
	if err := platform.New().Process().Configure(cmd); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	attr := cmd.SysProcAttr
	if attr == nil {
		t.Fatal("Configure did not set SysProcAttr")
	}
	if attr.CreationFlags&windows.CREATE_NEW_PROCESS_GROUP == 0 {
		t.Errorf("CreationFlags = %#x, want CREATE_NEW_PROCESS_GROUP", attr.CreationFlags)
	}
}

// TestWindowsProcessStatusEdges pins the trivial probes without spawning.
func TestWindowsProcessStatusEdges(t *testing.T) {
	proc := platform.New().Process()
	if proc.Alive(-1) {
		t.Error("Alive(-1) = true, want false")
	}
	if proc.Alive(1 << 22) {
		t.Error("Alive(huge pid) = true, want false")
	}
	if proc.IsZombie(1) {
		t.Error("IsZombie = true, want always false on Windows")
	}
	if err := proc.Terminate(-1); err == nil {
		t.Error("Terminate(-1) = nil, want an error")
	}
	// A missing process is a successful stop, like ESRCH on unix.
	if err := proc.Terminate(1 << 22); err != nil {
		t.Errorf("Terminate(missing pid) = %v, want nil", err)
	}
}

// TestWindowsProcessTerminateTree is the stop-semantics core: a supervised
// child tree (cmd waiting on ping) dies as a whole when the platform stops
// the leader.
func TestWindowsProcessTerminateTree(t *testing.T) {
	proc := platform.New().Process()
	cmd := exec.Command("cmd", "/C", "ping -n 300 127.0.0.1")
	if err := proc.Configure(cmd); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		_ = proc.Terminate(cmd.Process.Pid)
		_, _ = cmd.Process.Wait()
	})
	if err := proc.Attach(cmd.Process); err != nil {
		t.Fatalf("Attach: %v", err)
	}

	// The fixture tree is cmd waiting on one ping child; find it by parent.
	var grandchild int
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		for _, pid := range childPids(t, cmd.Process.Pid) {
			grandchild = pid
		}
		if grandchild != 0 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if grandchild == 0 {
		t.Fatal("the ping grandchild never appeared under the cmd child")
	}
	if !proc.Alive(cmd.Process.Pid) || !proc.Alive(grandchild) {
		t.Fatal("the fixture tree is not alive before Terminate")
	}

	if err := proc.Terminate(cmd.Process.Pid); err != nil {
		t.Fatalf("Terminate: %v", err)
	}
	waitGone(t, proc, cmd.Process.Pid)
	waitGone(t, proc, grandchild)
}

// waitGone polls until pid no longer answers, i.e. the whole tree died.
func waitGone(t *testing.T, proc platform.Process, pid int) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if !proc.Alive(pid) {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("process %d still alive after Terminate", pid)
}

// childPids lists the current children of ppid via a Toolhelp32 snapshot.
func childPids(t *testing.T, ppid int) []int {
	t.Helper()
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	defer windows.CloseHandle(snap)
	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	var out []int
	for err := windows.Process32First(snap, &entry); err == nil; err = windows.Process32Next(snap, &entry) {
		if int(entry.ParentProcessID) == ppid {
			out = append(out, int(entry.ProcessID))
		}
	}
	return out
}
