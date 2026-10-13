package opencode

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	"devplatform/internal/agents"
)

// TestSharedEndpointReusedAcrossInstances proves the desktop model: every
// instance of the machine-wide service binds the same adopted endpoint, and
// starting an instance never touches the service process.
func TestSharedEndpointReusedAcrossInstances(t *testing.T) {
	adapter, launcher, _ := newAdapterWithServer(t, infoHandler("2.0.24"))
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
	if launcher.stopCount() != 0 {
		t.Fatalf("stops = %d, want 0 while instances remain", launcher.stopCount())
	}
}

// TestStopLastInstanceLeavesServiceProcessAlone pins the removal of the last
// instance teardown: the opencode service also serves the desktop client and
// manually started servers, so no stop of the platform's own instance may
// reach it.
func TestStopLastInstanceLeavesServiceProcessAlone(t *testing.T) {
	adapter, launcher, _ := newAdapterWithServer(t, infoHandler("2.0.24"))
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

	if err := adapter.StopInstance(ctx, first.ID); err != nil {
		t.Fatalf("StopInstance first: %v", err)
	}
	if err := adapter.StopInstance(ctx, second.ID); err != nil {
		t.Fatalf("StopInstance second: %v", err)
	}
	if launcher.stopCount() != 0 {
		t.Fatalf("stops = %d, want 0: the platform never stops the service outside the settings page", launcher.stopCount())
	}
}

// TestWatchSharedKeepsInstancesThroughTransientProbeFailure pins that one lost
// probe does not sever a working conversation: the watcher confirms with a
// second probe before it reports anything.
func TestWatchSharedKeepsInstancesThroughTransientProbeFailure(t *testing.T) {
	gate := &flakyProbe{}
	adapter, _, _ := newAdapterWithServer(t, gate.handler())
	instance := startTestInstance(t, adapter)
	adapter.watchInterval = 10 * time.Millisecond

	gate.failOnce(true)
	go adapter.watchShared(adapter.shared)

	waitFor(t, 500*time.Millisecond, func() bool { return gate.count() > 3 }, "watcher did not probe")
	if !instanceActive(t, adapter, instance.ID) {
		t.Fatalf("instance %s failed after a single transient probe failure", instance.ID)
	}
}

// TestWatchSharedFailsInstancesAfterSustainedFailure pins the other half: a
// service that really is gone is reported, so its sessions surface as unusable
// instead of hanging on a dead connection. The watcher reports it and drops its
// reference; it does not restart or kill anything.
func TestWatchSharedFailsInstancesAfterSustainedFailure(t *testing.T) {
	adapter, launcher, server := newAdapterWithServer(t, infoHandler("2.0.24"))
	instance := startTestInstance(t, adapter)
	adapter.watchInterval = 10 * time.Millisecond
	watched := adapter.shared
	go adapter.watchShared(watched)

	server.Close() // the service is gone

	waitFor(t, 3*time.Second, func() bool { return !instanceActive(t, adapter, instance.ID) }, "instance never reported the dead service")
	if launcher.stopCount() != 0 {
		t.Fatalf("stops = %d, want 0: reporting a dead service must not stop a process", launcher.stopCount())
	}
	if got := adapter.sharedEndpoint(); got != "" {
		t.Fatalf("shared endpoint after the service went away = %q, want the reference dropped", got)
	}
}

// flakyProbe serves /api/info and can be told to fail exactly the next request,
// standing in for a network blip while the watcher probes.
type flakyProbe struct {
	mu       sync.Mutex
	requests int
	failNext bool
}

func (f *flakyProbe) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/info" {
			http.NotFound(w, r)
			return
		}
		f.mu.Lock()
		f.requests++
		fail := f.failNext
		if fail {
			f.failNext = false
		}
		f.mu.Unlock()
		if fail {
			http.Error(w, "blip", http.StatusServiceUnavailable)
			return
		}
		writeInfo(w, "2.0.24")
	}
}

func (f *flakyProbe) failOnce(value bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failNext = value
}

func (f *flakyProbe) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.requests
}

func waitFor(t *testing.T, timeout time.Duration, cond func() bool, message string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out after %s: %s", timeout, message)
}

func instanceActive(t *testing.T, adapter *Adapter, id string) bool {
	t.Helper()
	adapter.mu.Lock()
	defer adapter.mu.Unlock()
	inst, ok := adapter.instances[id]
	if !ok {
		return false
	}
	return inst.active()
}
