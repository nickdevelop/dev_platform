//go:build unix

package opencode

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

// startDetached starts a command without waiting for it, so the caller can
// observe the process the way the registration file sees it.
func startDetached(t *testing.T, name string, args ...string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(name, args...)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start %s: %v", name, err)
	}
	t.Cleanup(func() { _, _ = cmd.Process.Wait() })
	return cmd
}

// processState reads the state field of a /proc/<pid>/stat contents string. The
// comm field is parenthesised and may itself contain spaces or ')', so the state
// is the field after the last ')'.
func processState(raw []byte) string {
	stat := string(raw)
	end := strings.LastIndexByte(stat, ')')
	if end < 0 || end+2 >= len(stat) {
		return ""
	}
	fields := strings.Fields(stat[end+2:])
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

// TestProcessAliveIgnoresZombie is the stopped-service case the settings page
// gets wrong without it: a service the platform stopped whose parent never
// reaped it leaves a zombie pid that still answers a signal, while its listener
// is long closed. The platform must call that service gone.
//
// Task 2.3 moves the /proc probe behind the Process port; this test moves to
// internal/platform/unix/process_test.go with it.
func TestProcessAliveIgnoresZombie(t *testing.T) {
	if _, err := os.Stat("/proc/self/stat"); err != nil {
		t.Skip("/proc is not available")
	}
	child := startDetached(t, "sh", "-c", "exit 0")
	statPath := "/proc/" + strconv.Itoa(child.Process.Pid) + "/stat"
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		raw, err := os.ReadFile(statPath)
		if err != nil {
			t.Skip("the child was reaped before it could be observed as a zombie")
		}
		if processState(raw) != "Z" {
			time.Sleep(10 * time.Millisecond)
			continue
		}
		if !processIsZombie(child.Process.Pid) {
			t.Fatalf("pid %d is a zombie but processIsZombie said otherwise", child.Process.Pid)
		}
		if processAlive(child.Process.Pid) {
			t.Fatalf("pid %d is a zombie but still reported alive", child.Process.Pid)
		}
		return
	}
	t.Skip("the child never entered the zombie state in time")
}
