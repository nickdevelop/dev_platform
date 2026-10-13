package sessions

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"devplatform/internal/agents"
	"devplatform/internal/agents/instances"
	"devplatform/internal/database"
	"devplatform/internal/projects"
)

// gatedAdapter is an opencode-shaped adapter with a machine-level service the
// test can turn on and off, so the session layer's gate is exercised through the
// same facade the platform wires in production.
type gatedAdapter struct {
	*fakeAdapter

	availableErr error
}

func (g *gatedAdapter) DetectService(context.Context) (agents.ServiceStatus, error) {
	return agents.ServiceStatus{Backend: agents.BackendOpenCodeV2, Supported: true}, nil
}

func (g *gatedAdapter) ServiceAvailable() error { return g.availableErr }

func (g *gatedAdapter) StartService(context.Context) (agents.ServiceStatus, error) {
	return agents.ServiceStatus{}, agents.ErrServiceUnsupported
}

func (g *gatedAdapter) StopService(context.Context) (agents.ServiceStatus, error) {
	return agents.ServiceStatus{}, agents.ErrServiceUnsupported
}

// newGatedEnv is a project with an opencode backend whose service state the test
// controls, and with no instance yet: the state a user meets when the machine
// service has never been started.
func newGatedEnv(t *testing.T) (*testEnv, *gatedAdapter) {
	t.Helper()

	db := openTestDB(t)
	root := t.TempDir()
	projectDir := root + "/alpha"
	mustMkdir(t, projectDir)
	projectService := projects.NewService(db)
	project, err := projectService.Register(projectDir)
	if err != nil {
		t.Fatalf("register project: %v", err)
	}
	gate := &gatedAdapter{fakeAdapter: newFakeAdapter()}
	gate.caps.ToolConfirmation = true
	registry := agents.NewRegistry()
	if err := registry.Register(agents.BackendOpenCodeV2, gate); err != nil {
		t.Fatalf("register gated adapter: %v", err)
	}
	instanceService := instances.NewService(db, registry, projectService)
	return &testEnv{
		svc:        NewService(db, registry, instanceService),
		db:         db,
		instances:  instanceService,
		registry:   registry,
		fake:       gate.fakeAdapter,
		root:       root,
		projectID:  project.ID,
		projectDir: projectDir,
	}, gate
}

// serviceNotRunning is what the adapter's cheap gate reports when no service is
// registered.
func serviceNotRunning() error {
	return fmt.Errorf("%w: no `opencode serve --service` is registered", agents.ErrServiceNotRunning)
}

func instanceRowCount(t *testing.T, env *testEnv) int64 {
	t.Helper()
	var count int64
	if err := env.db.Model(&database.AgentInstance{}).Count(&count).Error; err != nil {
		t.Fatalf("count instances: %v", err)
	}
	return count
}

// TestCreateRefusedWhenServiceNotRunning pins the whole point of the gate: a
// task on a backend whose machine service is down is refused with the reason,
// and nothing is left behind — not a failed instance row the user would have to
// clean up.
func TestCreateRefusedWhenServiceNotRunning(t *testing.T) {
	env, gate := newGatedEnv(t)
	gate.availableErr = serviceNotRunning()

	_, err := env.svc.Create(context.Background(), env.projectID, agents.BackendOpenCodeV2, "first task")
	if !errors.Is(err, ErrServiceNotRunning) {
		t.Fatalf("Create error = %v, want it to wrap %v", err, ErrServiceNotRunning)
	}
	if !errors.Is(err, agents.ErrServiceNotRunning) {
		t.Fatalf("Create error = %v, want it to wrap the backend-agnostic %v", err, agents.ErrServiceNotRunning)
	}
	if count := instanceRowCount(t, env); count != 0 {
		t.Fatalf("agent_instances rows = %d, want 0: a refused task must not leave a failed instance behind", count)
	}
}

// TestDiscoverProjectRefusedWhenServiceNotRunning covers the other path that
// would otherwise start an instance of its own.
func TestDiscoverProjectRefusedWhenServiceNotRunning(t *testing.T) {
	env, gate := newGatedEnv(t)
	gate.availableErr = serviceNotRunning()

	_, err := env.svc.DiscoverProject(context.Background(), env.projectID, agents.BackendOpenCodeV2)
	if !errors.Is(err, ErrServiceNotRunning) {
		t.Fatalf("DiscoverProject error = %v, want it to wrap %v", err, ErrServiceNotRunning)
	}
	if count := instanceRowCount(t, env); count != 0 {
		t.Fatalf("agent_instances rows = %d, want 0", count)
	}
}

// TestContinueRebindRefusedWhenServiceNotRunning covers the reconnect click
// while the service is still down: the user is told to start it, and no
// half-started instance accumulates.
func TestContinueRebindRefusedWhenServiceNotRunning(t *testing.T) {
	env, gate := newGatedEnv(t)

	env.fake.mu.Lock()
	env.fake.createRef = &agents.SessionRef{Backend: agents.BackendOpenCodeV2, SessionID: "native-gated", Title: "gated"}
	env.fake.mu.Unlock()
	session, err := env.svc.Create(context.Background(), env.projectID, agents.BackendOpenCodeV2, "gated")
	if err != nil {
		t.Fatalf("create session while the service is up: %v", err)
	}
	before := instanceRowCount(t, env)

	if _, err := env.instances.Stop(context.Background(), session.InstanceID); err != nil {
		t.Fatalf("stop instance: %v", err)
	}
	gate.availableErr = serviceNotRunning()

	if _, err := env.svc.Continue(context.Background(), env.projectID, session.ID); !errors.Is(err, ErrServiceNotRunning) {
		t.Fatalf("Continue error = %v, want it to wrap %v", err, ErrServiceNotRunning)
	}
	if count := instanceRowCount(t, env); count != before {
		t.Fatalf("agent_instances rows = %d, want the %d from before the reconnect", count, before)
	}
}

// TestContinueSucceedsAfterServiceStarted pins the recovery the spec promises:
// the operator starts the service on the settings page, the user clicks
// reconnect, and the session works again. Nothing is automatic in between.
func TestContinueSucceedsAfterServiceStarted(t *testing.T) {
	env, gate := newGatedEnv(t)

	env.fake.mu.Lock()
	env.fake.createRef = &agents.SessionRef{Backend: agents.BackendOpenCodeV2, SessionID: "native-gated", Title: "gated"}
	env.fake.mu.Unlock()
	session, err := env.svc.Create(context.Background(), env.projectID, agents.BackendOpenCodeV2, "gated")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if _, err := env.instances.Stop(context.Background(), session.InstanceID); err != nil {
		t.Fatalf("stop instance: %v", err)
	}
	gate.availableErr = serviceNotRunning()
	if _, err := env.svc.Continue(context.Background(), env.projectID, session.ID); !errors.Is(err, ErrServiceNotRunning) {
		t.Fatalf("Continue = %v, want the refusal while the service is down", err)
	}

	gate.availableErr = nil
	continued, err := env.svc.Continue(context.Background(), env.projectID, session.ID)
	if err != nil {
		t.Fatalf("Continue after the service was started: %v", err)
	}
	if continued.Status != SessionActive {
		t.Fatalf("status = %q, want %q after reconnecting", continued.Status, SessionActive)
	}
}

// TestSessionCreateServiceNotRunning pins the HTTP contract the workbench
// renders as "the service is not running, start it in Platform Settings".
func TestSessionCreateServiceNotRunning(t *testing.T) {
	env, gate := newGatedEnv(t)
	gate.availableErr = serviceNotRunning()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	env.svc.RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/projects/%d/sessions", env.projectID),
		strings.NewReader(`{"backend":"opencode-v2","title":"first task"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("POST session = %d %s, want 409", rec.Code, rec.Body.String())
	}
	if got := strings.TrimSpace(rec.Body.String()); got != `{"error":"service_not_running"}` {
		t.Fatalf("body = %s, want the service_not_running error envelope", got)
	}
}

// TestInstanceForBackendSkipsGateWhenAnInstanceRuns pins that the gate guards
// only the path that would start something: a project that already has a
// running instance keeps working even if the service registration disappears,
// and the session layer does not spend a probe on it.
func TestInstanceForBackendSkipsGateWhenAnInstanceRuns(t *testing.T) {
	env, gate := newGatedEnv(t)
	gate.availableErr = serviceNotRunning()

	instance, err := env.instances.Start(context.Background(), env.projectID, agents.BackendOpenCodeV2)
	if err != nil {
		t.Fatalf("start instance: %v", err)
	}
	got, err := env.svc.instanceForBackend(context.Background(), env.projectID, agents.BackendOpenCodeV2)
	if err != nil {
		t.Fatalf("instanceForBackend with a running instance: %v", err)
	}
	if got.ID != instance.ID {
		t.Fatalf("instance = %q, want the running %q", got.ID, instance.ID)
	}
}
