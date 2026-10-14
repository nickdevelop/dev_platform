package opencode

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"

	"devplatform/internal/agents"
	"devplatform/internal/platform"
)

// TestStartInstanceDoesNotLaunchServiceWhenNoRegistration pins the boundary
// that makes the service a third party: with no machine-wide service to adopt,
// the platform refuses the work instead of booting one of its own.
func TestStartInstanceDoesNotLaunchServiceWhenNoRegistration(t *testing.T) {
	var launches atomic.Int32
	adapter, launcher, _ := newAdapterWithServer(t, queueHandler(nil))
	base := launcher.launch
	adapter.launch = func(ctx context.Context, dir string) (*launchResult, error) {
		launches.Add(1)
		return base(ctx, dir)
	}
	// No shared server and no registration file: nothing to adopt.
	adapter.shared = nil

	_, err := adapter.StartInstance(context.Background(), agents.StartInstanceRequest{Directory: t.TempDir()})
	if !errors.Is(err, agents.ErrServiceNotRunning) {
		t.Fatalf("error = %v, want it to wrap %v", err, agents.ErrServiceNotRunning)
	}
	if launches.Load() != 0 {
		t.Fatalf("launches = %d, want 0: a business path must never start the service", launches.Load())
	}
}

// TestStartInstanceTreatsBadRegistrationAsStopped pins that every unusable
// registration file is a plain "not running" refusal: no panic, no 500-shaped
// internal error, and above all no launch.
func TestStartInstanceTreatsBadRegistrationAsStopped(t *testing.T) {
	dead := deadPID(t)

	cases := map[string]string{
		"unparseable json": "not json at all",
		"missing url":      `{"id":"svc","version":"2.0.24","pid":` + itoa(os.Getpid()) + `,"password":"pw"}`,
		"url without host": `{"id":"svc","version":"2.0.24","url":"nope","pid":` + itoa(os.Getpid()) + `,"password":"pw"}`,
		"dead pid":         `{"id":"svc","version":"2.0.24","url":"http://127.0.0.1:1","pid":` + itoa(dead) + `,"password":"pw"}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "service.json")
			if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
				t.Fatalf("write registration: %v", err)
			}
			adapter, _, _ := newAdapterWithServer(t, queueHandler(nil))
			adapter.shared = nil
			adapter.serviceFilePath = path

			var launches atomic.Int32
			adapter.launch = func(context.Context, string) (*launchResult, error) {
				launches.Add(1)
				t.Errorf("the platform launched a server for a broken registration")
				return nil, errors.New("must not launch")
			}

			if _, err := adapter.StartInstance(context.Background(), agents.StartInstanceRequest{Directory: t.TempDir()}); !errors.Is(err, agents.ErrServiceNotRunning) {
				t.Fatalf("error = %v, want it to wrap %v", err, agents.ErrServiceNotRunning)
			}
			if launches.Load() != 0 {
				t.Fatalf("launches = %d, want 0", launches.Load())
			}
		})
	}
}

// TestStartInstanceRefusesStaleRegistration proves a live-looking registration
// whose API does not answer is reported as not running rather than replaced:
// the platform does not restart a service it did not start.
func TestStartInstanceRefusesStaleRegistration(t *testing.T) {
	reg := serviceRegistration{
		ID:      "svc-stale",
		Version: "2.0.24",
		// Port 1 answers nothing, and the pid is this test process so the
		// liveness check passes.
		URL:      "http://127.0.0.1:1",
		PID:      os.Getpid(),
		Password: "pw",
	}
	adapter, _, _ := newAdapterWithServer(t, queueHandler(map[string]http.HandlerFunc{
		"GET /api/info": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("content-type", "application/json")
			_, _ = w.Write([]byte(`{"version":"2.0.24","pid":1,"urls":[],"paths":{"tmp":"/tmp"}}`))
		},
	}))
	adapter.shared = nil
	adapter.serviceFilePath = writeServiceFile(t, reg)
	var launches atomic.Int32
	adapter.launch = func(context.Context, string) (*launchResult, error) {
		launches.Add(1)
		return nil, errors.New("must not launch")
	}

	if _, err := adapter.StartInstance(context.Background(), agents.StartInstanceRequest{Directory: t.TempDir()}); !errors.Is(err, agents.ErrServiceNotRunning) {
		t.Fatalf("error = %v, want it to wrap %v", err, agents.ErrServiceNotRunning)
	}
	if launches.Load() != 0 {
		t.Fatalf("launches = %d, want 0", launches.Load())
	}
}

// deadPID starts a process, waits for it to exit and returns its pid, which the
// OS has now reaped.
func deadPID(t *testing.T) int {
	t.Helper()
	// The helper needs a real sleeper binary; skip where the platform has
	// none (Windows CI) instead of failing the gate tests.
	if _, err := platform.New().Executables().Resolve("sleep"); err != nil {
		t.Skipf("sleep not on PATH: %v", err)
	}
	cmd := exec.Command("sleep", "0.01")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start sleep: %v", err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("wait sleep: %v", err)
	}
	return cmd.Process.Pid
}

func itoa(value int) string {
	raw, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return string(raw)
}
