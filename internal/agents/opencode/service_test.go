package opencode

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"devplatform/internal/agents"
)

// TestDetectServiceStates pins what the settings page shows. Every unusable
// registration is a plain "stopped" or "unreachable" observation with an
// operator-readable reason — never an error the UI has to guess at.
func TestDetectServiceStates(t *testing.T) {
	t.Run("missing registration is stopped", func(t *testing.T) {
		adapter, _, _ := newAdapterWithServer(t, infoHandler("2.0.24"))
		adapter.serviceFilePath = filepath.Join(t.TempDir(), "absent.json")

		status, err := adapter.DetectService(context.Background())
		if err != nil {
			t.Fatalf("DetectService: %v", err)
		}
		if status.State != agents.ServiceStopped || !status.Supported {
			t.Fatalf("status = %+v, want supported stopped", status)
		}
	})

	t.Run("dead pid is stopped", func(t *testing.T) {
		adapter, _, _ := newAdapterWithServer(t, infoHandler("2.0.24"))
		adapter.serviceFilePath = writeServiceFile(t, serviceRegistration{
			ID: "svc", Version: "2.0.24", URL: "http://127.0.0.1:1", PID: deadPID(t), Password: "pw",
		})

		status, err := adapter.DetectService(context.Background())
		if err != nil {
			t.Fatalf("DetectService: %v", err)
		}
		if status.State != agents.ServiceStopped {
			t.Fatalf("state = %q, want stopped for a registration naming a dead process", status.State)
		}
	})

	t.Run("live service answers as running", func(t *testing.T) {
		server := infoServer(t, "2.0.31")
		adapter, _, _ := newAdapterWithServer(t, infoHandler("2.0.24"))
		adapter.serviceFilePath = writeServiceFile(t, serviceRegistration{
			ID: "svc", Version: "2.0.31", URL: server.URL, PID: os.Getpid(), Password: "pw",
		})

		status, err := adapter.DetectService(context.Background())
		if err != nil {
			t.Fatalf("DetectService: %v", err)
		}
		if status.State != agents.ServiceRunning {
			t.Fatalf("state = %q, want running (%+v)", status.State, status)
		}
		if status.Endpoint != server.URL || status.PID != os.Getpid() || status.Version != "2.0.31" {
			t.Fatalf("status = %+v, want the registered endpoint, pid and reported version", status)
		}
	})

	t.Run("live pid with a dead api is unreachable", func(t *testing.T) {
		adapter, _, _ := newAdapterWithServer(t, infoHandler("2.0.24"))
		adapter.serviceFilePath = writeServiceFile(t, serviceRegistration{
			ID: "svc", Version: "2.0.24", URL: "http://127.0.0.1:1", PID: os.Getpid(), Password: "sup3rs3cret",
		})

		status, err := adapter.DetectService(context.Background())
		if err != nil {
			t.Fatalf("DetectService: %v", err)
		}
		if status.State != agents.ServiceUnreachable {
			t.Fatalf("state = %q, want unreachable (%+v)", status.State, status)
		}
		if status.Detail == "" {
			t.Fatal("an unreachable service must carry a reason for the operator")
		}
		if strings.Contains(status.Detail, "sup3rs3cret") {
			t.Fatalf("detail leaked the service password: %q", status.Detail)
		}
	})
}

// TestServiceAvailableSkipsNetworkProbe pins that the business-path gate is the
// cheap one: a registered, live process passes the gate even when its API would
// not answer, so creating a task never waits on an HTTP round trip.
func TestServiceAvailableSkipsNetworkProbe(t *testing.T) {
	adapter, _, _ := newAdapterWithServer(t, infoHandler("2.0.24"))
	adapter.serviceFilePath = writeServiceFile(t, serviceRegistration{
		ID: "svc", Version: "2.0.24", URL: "http://127.0.0.1:1", PID: os.Getpid(), Password: "pw",
	})

	if err := adapter.ServiceAvailable(); err != nil {
		t.Fatalf("ServiceAvailable = %v, want nil: the check must not make network calls", err)
	}

	adapter.serviceFilePath = filepath.Join(t.TempDir(), "absent.json")
	if err := adapter.ServiceAvailable(); !errors.Is(err, agents.ErrServiceNotRunning) {
		t.Fatalf("ServiceAvailable = %v, want it to wrap %v", err, agents.ErrServiceNotRunning)
	}
}

// TestStartServiceRefusesWhenRunning pins that the settings page never replaces
// a working service, whoever started it.
func TestStartServiceRefusesWhenRunning(t *testing.T) {
	server := infoServer(t, "2.0.31")
	adapter, launcher, _ := newAdapterWithServer(t, infoHandler("2.0.24"))
	adapter.serviceFilePath = writeServiceFile(t, serviceRegistration{
		ID: "svc", Version: "2.0.31", URL: server.URL, PID: os.Getpid(), Password: "pw",
	})
	launches := 0
	base := launcher.launch
	adapter.launch = func(ctx context.Context, dir string) (*launchResult, error) {
		launches++
		return base(ctx, dir)
	}

	_, err := adapter.StartService(context.Background())
	if !errors.Is(err, agents.ErrServiceAlreadyRunning) {
		t.Fatalf("error = %v, want it to wrap %v", err, agents.ErrServiceAlreadyRunning)
	}
	if launches != 0 {
		t.Fatalf("launches = %d, want 0", launches)
	}
}

// TestStartServicePublishesSharedServer pins the one path that may start the
// service: the operator asked for it, and what comes back is the service every
// later instance binds.
func TestStartServicePublishesSharedServer(t *testing.T) {
	adapter, launcher, _ := newAdapterWithServer(t, infoHandler("2.0.24"))
	adapter.serviceFilePath = filepath.Join(t.TempDir(), "absent.json")
	adapter.shared = nil
	launches := 0
	base := launcher.launch
	adapter.launch = func(ctx context.Context, dir string) (*launchResult, error) {
		launches++
		return base(ctx, dir)
	}

	status, err := adapter.StartService(context.Background())
	if err != nil {
		t.Fatalf("StartService: %v", err)
	}
	if launches != 1 {
		t.Fatalf("launches = %d, want 1", launches)
	}
	if status.State != agents.ServiceRunning {
		t.Fatalf("status = %+v, want running", status)
	}
	endpoint := adapter.sharedEndpoint()
	if endpoint == "" {
		t.Fatal("the started service was not published as the shared one")
	}

	// The instance the platform starts next binds the service the operator
	// started, without starting anything of its own.
	instance := startTestInstance(t, adapter)
	if instance.Endpoint != endpoint {
		t.Fatalf("instance endpoint = %q, want the shared %q", instance.Endpoint, endpoint)
	}
	if launches != 1 {
		t.Fatalf("launches after StartInstance = %d, want still 1", launches)
	}
}

// TestStartServicePropagatesLaunchFailure keeps the operator's failure visible:
// a start that did not come up is reported with its reason.
func TestStartServicePropagatesLaunchFailure(t *testing.T) {
	adapter, launcher, _ := newAdapterWithServer(t, infoHandler("2.0.24"))
	adapter.serviceFilePath = filepath.Join(t.TempDir(), "absent.json")
	adapter.shared = nil
	launcher.launchErr = &APIError{StatusCode: http.StatusUnauthorized, Code: "UnauthorizedError", Message: "Authentication required"}

	if _, err := adapter.StartService(context.Background()); err == nil {
		t.Fatal("expected the start to fail")
	}
}

// TestStopServiceKillsRegisteredPid pins that "stop" means stop: the platform
// signals the process the registration names, even one it did not start. The
// operator owns that decision.
func TestStopServiceKillsRegisteredPid(t *testing.T) {
	exited := make(chan struct{})
	cmd := exec.Command("sleep", "30")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start sleep: %v", err)
	}
	go func() {
		_ = cmd.Wait()
		close(exited)
	}()

	adapter, _, _ := newAdapterWithServer(t, infoHandler("2.0.24"))
	adapter.serviceFilePath = writeServiceFile(t, serviceRegistration{
		ID: "svc", Version: "2.0.24", URL: "http://127.0.0.1:1", PID: cmd.Process.Pid, Password: "pw",
	})
	adapter.shared = &sharedServer{client: nil, endpoint: "http://127.0.0.1:1"}

	if _, err := adapter.StopService(context.Background()); err != nil {
		t.Fatalf("StopService: %v", err)
	}
	select {
	case <-exited:
	case <-time.After(5 * time.Second):
		t.Fatalf("pid %d survived a stop requested from the settings page", cmd.Process.Pid)
	}
	if got := adapter.sharedEndpoint(); got != "" {
		t.Fatalf("shared endpoint after stop = %q, want the reference dropped", got)
	}
}

// TestStopServiceIdempotentWhenStopped keeps a repeated stop harmless: the
// settings page must not turn "already stopped" into a failure.
func TestStopServiceIdempotentWhenStopped(t *testing.T) {
	adapter, _, _ := newAdapterWithServer(t, infoHandler("2.0.24"))
	adapter.serviceFilePath = filepath.Join(t.TempDir(), "absent.json")

	status, err := adapter.StopService(context.Background())
	if err != nil {
		t.Fatalf("StopService of a stopped service: %v", err)
	}
	if status.State != agents.ServiceStopped {
		t.Fatalf("status = %+v, want stopped", status)
	}
}

// infoServer serves a healthy /api/info naming this test process.
func infoServer(t *testing.T, version string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(infoHandler(version))
	t.Cleanup(server.Close)
	return server
}
