package opencode

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"devplatform/internal/core/agents"
	"devplatform/internal/core/platform"
)

// TestEnsureSharedAdoptsExistingService proves session creation reuses the
// machine-wide service that is already there instead of booting a second
// one: with a healthy registration file, StartInstance never calls launch
// and binds the registered endpoint.
func TestEnsureSharedAdoptsExistingService(t *testing.T) {
	var launches atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/info", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"version": "2.0.24",
			"pid":     os.Getpid(),
			"urls":    []string{},
			"paths":   map[string]string{"tmp": "/tmp"},
		})
	})
	existing := httptest.NewServer(mux)
	t.Cleanup(existing.Close)

	reg := serviceRegistration{
		ID:       "svc-existing",
		Version:  "2.0.24",
		URL:      existing.URL,
		PID:      os.Getpid(),
		Password: "pw",
	}
	serviceFile := writeServiceFile(t, reg)

	adapter := New(Options{
		Executable:  "opencode",
		ServiceFile: serviceFile,
		NewID:       func() (string, error) { return "inst-adopted", nil },
	})
	base := adapter.launch
	adapter.launch = func(ctx context.Context, dir string) (*launchResult, error) {
		launches.Add(1)
		return base(ctx, dir)
	}

	instance, err := adapter.StartInstance(context.Background(), agents.StartInstanceRequest{Directory: t.TempDir()})
	if err != nil {
		t.Fatalf("StartInstance: %v", err)
	}
	if launches.Load() != 0 {
		t.Fatalf("launches = %d, want 0 (existing service reused)", launches.Load())
	}
	if instance.Endpoint != existing.URL {
		t.Fatalf("endpoint = %q, want existing %q", instance.Endpoint, existing.URL)
	}

	// Stopping the last instance leaves the adopted service alone: the platform
	// keeps its client, and never stops a process it did not start from the
	// settings page.
	if err := adapter.StopInstance(context.Background(), instance.ID); err != nil {
		t.Fatalf("StopInstance: %v", err)
	}
	if launches.Load() != 0 {
		t.Fatalf("launches after stop = %d, want 0", launches.Load())
	}
	if got := adapter.sharedEndpoint(); got != existing.URL {
		t.Fatalf("shared endpoint after last stop = %q, want the reference kept for %q", got, existing.URL)
	}
}

// TestProcessAliveExcludesZombie keeps the effective-liveness composition
// honest: a zombie pid answers a signal but serves nothing, so processAlive
// must report it gone. It skips where /proc or a shell is unavailable.
func TestProcessAliveExcludesZombie(t *testing.T) {
	if _, err := os.Stat("/proc/self/stat"); err != nil {
		t.Skip("/proc is not available")
	}
	cmd := exec.Command("sh", "-c", "exit 0")
	if err := cmd.Start(); err != nil {
		t.Skipf("shell unavailable: %v", err)
	}
	t.Cleanup(func() { _, _ = cmd.Process.Wait() })
	proc := platform.New().Process()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if proc.IsZombie(cmd.Process.Pid) {
			if processAlive(cmd.Process.Pid) {
				t.Fatalf("zombie pid %d reported alive", cmd.Process.Pid)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Skip("the child never entered the zombie state in time")
}

func writeServiceFile(t *testing.T, reg serviceRegistration) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "service.json")
	raw, err := json.Marshal(reg)
	if err != nil {
		t.Fatalf("marshal registration: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("write service file: %v", err)
	}
	return path
}

var _ = net.ParseIP
