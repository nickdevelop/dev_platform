package opencode

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"

	"devplatform/internal/agents"
)

// TestSharedServerReusedAcrossInstances proves the desktop model: two
// StartInstance calls in different directories share one launched server.
func TestSharedServerReusedAcrossInstances(t *testing.T) {
	adapter, launcher, _ := newAdapterWithServer(t, queueHandler(map[string]http.HandlerFunc{
		"GET /api/info": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("content-type", "application/json")
			_, _ = w.Write([]byte(`{"version":"2.0.24","pid":1,"urls":[],"paths":{"tmp":"/tmp"}}`))
		},
	}))
	var launches atomic.Int32
	base := launcher.launch
	adapter.launch = func(ctx context.Context, dir string) (*launchResult, error) {
		launches.Add(1)
		return base(ctx, dir)
	}
	ids := []string{"inst-a", "inst-b"}
	idx := 0
	adapter.newID = func() (string, error) {
		id := ids[idx]
		idx++
		return id, nil
	}

	ctx := context.Background()
	first, err := adapter.StartInstance(ctx, agents.StartInstanceRequest{Directory: t.TempDir()})
	if err != nil {
		t.Fatalf("StartInstance first: %v", err)
	}
	second, err := adapter.StartInstance(ctx, agents.StartInstanceRequest{Directory: t.TempDir()})
	if err != nil {
		t.Fatalf("StartInstance second: %v", err)
	}
	if first.Endpoint != second.Endpoint {
		t.Fatalf("endpoints differ: %q vs %q, want shared", first.Endpoint, second.Endpoint)
	}
	if launches.Load() != 1 {
		t.Fatalf("launches = %d, want 1 shared server", launches.Load())
	}
	if launcher.stopCount() != 0 {
		t.Fatalf("stops = %d, want 0 while instances remain", launcher.stopCount())
	}

	// Stopping one instance keeps the shared server alive.
	if err := adapter.StopInstance(ctx, first.ID); err != nil {
		t.Fatalf("StopInstance first: %v", err)
	}
	if launcher.stopCount() != 0 {
		t.Fatalf("stops after first stop = %d, want 0", launcher.stopCount())
	}
	// Stopping the last instance tears the shared server down.
	if err := adapter.StopInstance(ctx, second.ID); err != nil {
		t.Fatalf("StopInstance second: %v", err)
	}
	if launcher.stopCount() != 1 {
		t.Fatalf("stops after last stop = %d, want 1", launcher.stopCount())
	}
}
