package agentservices

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"

	"devplatform/internal/core/agents"
)

// fakeServiceAdapter is an adapter that does have a machine-level service, so
// the platform's service façade can be exercised without a real opencode
// process.
type fakeServiceAdapter struct {
	// agents.AgentAdapter is embedded unset: only the service-control surface
	// is exercised here, and the registry needs a non-nil adapter to hold it.
	agents.AgentAdapter

	mu           sync.Mutex
	status       agents.ServiceStatus
	detectErr    error
	availableErr error
	startErr     error
	stopErr      error
	starts       int
	stops        int
}

func (f *fakeServiceAdapter) DetectService(context.Context) (agents.ServiceStatus, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.detectErr != nil {
		return agents.ServiceStatus{}, f.detectErr
	}
	return f.status, nil
}

func (f *fakeServiceAdapter) ServiceAvailable() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.availableErr
}

func (f *fakeServiceAdapter) StartService(context.Context) (agents.ServiceStatus, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.starts++
	if f.startErr != nil {
		return agents.ServiceStatus{}, f.startErr
	}
	// Like the real adapter: an operator's start never replaces a serving
	// process.
	if f.status.State == agents.ServiceRunning {
		return f.status, fmt.Errorf("%w at %s", agents.ErrServiceAlreadyRunning, f.status.Endpoint)
	}
	return f.status, nil
}

func (f *fakeServiceAdapter) StopService(context.Context) (agents.ServiceStatus, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stops++
	if f.stopErr != nil {
		return agents.ServiceStatus{}, f.stopErr
	}
	return agents.ServiceStatus{Backend: agents.BackendOpenCodeV2, Supported: true, State: agents.ServiceStopped}, nil
}

// plainAdapter is an adapter with no machine-level service.
type plainAdapter struct{ agents.AgentAdapter }

// newServiceControlTest wires a service whose opencode backend has a machine
// service and whose pi backend does not.
func newServiceControlTest(t *testing.T) (*Service, *fakeServiceAdapter) {
	t.Helper()

	registry := agents.NewRegistry()
	// A CLI backend: an adapter without the service-control capability.
	if err := registry.Register(agents.BackendPi, plainAdapter{}); err != nil {
		t.Fatalf("register pi fake: %v", err)
	}
	controller := &fakeServiceAdapter{
		status: agents.ServiceStatus{
			Backend:   agents.BackendOpenCodeV2,
			Supported: true,
			State:     agents.ServiceRunning,
			Endpoint:  "http://127.0.0.1:4096",
			PID:       4242,
			Version:   "2.0.24",
		},
	}
	if err := registry.Register(agents.BackendOpenCodeV2, controller); err != nil {
		t.Fatalf("register opencode fake: %v", err)
	}
	return NewService(registry), controller
}

func TestDetectServiceUnsupportedBackend(t *testing.T) {
	svc, _ := newServiceControlTest(t)

	status, err := svc.Detect(context.Background(), agents.BackendPi)
	if err != nil {
		t.Fatalf("DetectService: %v", err)
	}
	if status.Supported {
		t.Fatalf("status = %+v, want supported false for a CLI backend", status)
	}
	if status.Backend != agents.BackendPi {
		t.Fatalf("backend = %q, want %q", status.Backend, agents.BackendPi)
	}

	if _, err := svc.Detect(context.Background(), agents.BackendID("nope")); !errors.Is(err, ErrUnknownBackend) {
		t.Fatalf("unknown backend error = %v, want %v", err, ErrUnknownBackend)
	}
}

func TestStartServiceUnsupported(t *testing.T) {
	svc, _ := newServiceControlTest(t)

	if _, err := svc.Start(context.Background(), agents.BackendPi); !errors.Is(err, agents.ErrServiceUnsupported) {
		t.Fatalf("start error = %v, want %v", err, agents.ErrServiceUnsupported)
	}
	if _, err := svc.Stop(context.Background(), agents.BackendPi); !errors.Is(err, agents.ErrServiceUnsupported) {
		t.Fatalf("stop error = %v, want %v", err, agents.ErrServiceUnsupported)
	}
}

func TestServiceAvailableGatePassesForNonServiceBackend(t *testing.T) {
	svc, controller := newServiceControlTest(t)

	if err := svc.Available(agents.BackendPi); err != nil {
		t.Fatalf("ServiceAvailable(pi) = %v, want nil: a CLI backend has no service to wait for", err)
	}

	controller.availableErr = fmt.Errorf("%w: nothing registered", agents.ErrServiceNotRunning)
	if err := svc.Available(agents.BackendOpenCodeV2); !errors.Is(err, agents.ErrServiceNotRunning) {
		t.Fatalf("ServiceAvailable(opencode) = %v, want it to wrap %v", err, agents.ErrServiceNotRunning)
	}
}

func TestAgentServiceRoutes(t *testing.T) {
	svc, controller := newServiceControlTest(t)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	svc.RegisterRoutes(router)

	t.Run("get reports the observed status", func(t *testing.T) {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/agent-services/opencode-v2", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
		}
		var body struct {
			Status agents.ServiceStatus `json:"status"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode %s: %v", rec.Body.String(), err)
		}
		if body.Status.State != agents.ServiceRunning || body.Status.PID != 4242 {
			t.Fatalf("status = %+v, want the running service", body.Status)
		}
	})

	t.Run("start refuses a running service", func(t *testing.T) {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/agent-services/opencode-v2/start", nil))
		if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "service_already_running") {
			t.Fatalf("start a running service = %d %s, want 409 service_already_running", rec.Code, rec.Body.String())
		}
	})

	t.Run("stop stops and reports stopped", func(t *testing.T) {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/agent-services/opencode-v2/stop", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("stop = %d %s, want 200", rec.Code, rec.Body.String())
		}
		if controller.stops != 1 {
			t.Fatalf("stops = %d, want 1", controller.stops)
		}
	})

	t.Run("an unknown backend is a bad request", func(t *testing.T) {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/agent-services/nope", nil))
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "unknown_backend") {
			t.Fatalf("unknown backend = %d %s, want 400 unknown_backend", rec.Code, rec.Body.String())
		}
	})

	t.Run("starting an unsupported backend is refused", func(t *testing.T) {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/agent-services/pi/start", nil))
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "service_unsupported") {
			t.Fatalf("unsupported start = %d %s, want 400 service_unsupported", rec.Code, rec.Body.String())
		}
	})
}
