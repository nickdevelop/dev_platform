package opencode

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"devplatform/internal/agents"
)

// TestEnsureSharedAdoptsExistingService proves task creation reuses the
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
