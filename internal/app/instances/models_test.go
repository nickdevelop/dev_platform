package instances

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"devplatform/internal/core/agents"
)

// newModelService builds a service whose fake backend offers model selection and
// one catalog entry, returning the service, the project, its directory and the
// backend.
func newModelService(t *testing.T, opts ...Option) (*Service, uint, string, *fakeAdapter) {
	t.Helper()

	svc, _, projectID, projectDir, fake := newTestService(t, opts...)
	fake.mu.Lock()
	fake.caps = agents.Capabilities{ModelSelection: true}
	fake.models = []agents.ModelInfo{
		{ID: "acme/gpt", Name: "GPT", Provider: "acme", Variants: []string{"acme/gpt#fast"}},
		{ID: "acme/small", Name: "Small", Provider: "acme"},
	}
	fake.mu.Unlock()
	return svc, projectID, projectDir, fake
}

// waitForModelCalls waits for a background refresh to reach the backend, so a
// stale-while-revalidate assertion does not depend on how the scheduler runs.
func waitForModelCalls(t *testing.T, fake *fakeAdapter, want int) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if fake.modelCallCount() >= want {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("backend was asked for models %d times, want at least %d", fake.modelCallCount(), want)
}

// TestBackendModelsNeedsNoInstance is the point of the route: a project's model
// list comes from the backend and the project's directory, so the workbench can
// offer a model while the session is still being created.
func TestBackendModelsNeedsNoInstance(t *testing.T) {
	svc, projectID, projectDir, fake := newModelService(t)

	list, err := svc.BackendModels(context.Background(), projectID, agents.BackendPi, false)
	if err != nil {
		t.Fatalf("BackendModels: %v", err)
	}
	if len(list.Models) != 2 || list.Models[0].ID != "acme/gpt" || list.Stale {
		t.Fatalf("BackendModels = %+v, want the backend's two fresh models", list)
	}
	if fake.modelCallCount() != 1 {
		t.Fatalf("backend was asked %d times, want once", fake.modelCallCount())
	}
	// The adapter was handed the project's authorized directory, not an instance.
	dirs := fake.askedDirectories()
	if len(dirs) != 1 || dirs[0] != projectDir {
		t.Errorf("adapter was asked about %v, want the project directory %q", dirs, projectDir)
	}
}

// TestBackendModelsCachesPerDirectory checks repeated reads are served from
// cache, a force refresh does reach the backend, and another project's catalog
// is a separate entry.
func TestBackendModelsCachesPerDirectory(t *testing.T) {
	svc, projectID, _, fake := newModelService(t)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		if _, err := svc.BackendModels(ctx, projectID, agents.BackendPi, false); err != nil {
			t.Fatalf("BackendModels: %v", err)
		}
	}
	if got := fake.modelCallCount(); got != 1 {
		t.Fatalf("backend was asked %d times, want the cached list to answer twice", got)
	}
	if _, err := svc.BackendModels(ctx, projectID, agents.BackendPi, true); err != nil {
		t.Fatalf("BackendModels refresh: %v", err)
	}
	if got := fake.modelCallCount(); got != 2 {
		t.Fatalf("backend was asked %d times, want a forced refresh to reach it", got)
	}

	// An unknown project is refused: its directory cannot be authorized, so
	// neither can its catalog.
	if _, err := svc.BackendModels(ctx, 4242, agents.BackendPi, false); !errors.Is(err, ErrProjectNotFound) {
		t.Errorf("BackendModels unknown project error = %v, want ErrProjectNotFound", err)
	}
}

// TestBackendModelsRefusesUnsupportedCapability checks a backend that does not
// declare model selection is refused rather than reported as offering nothing.
func TestBackendModelsRefusesUnsupportedCapability(t *testing.T) {
	svc, projectID, _, fake := newModelService(t)
	fake.mu.Lock()
	fake.caps = agents.Capabilities{Streaming: true}
	fake.mu.Unlock()

	if _, err := svc.BackendModels(context.Background(), projectID, agents.BackendPi, false); !errors.Is(err, ErrUnsupportedCapability) {
		t.Fatalf("BackendModels error = %v, want ErrUnsupportedCapability", err)
	}
	if got := fake.modelCallCount(); got != 0 {
		t.Errorf("backend was asked %d times, want it left alone", got)
	}
}

// TestBackendModelsUnknownBackend checks an unknown backend id is a client
// error, not a server fault.
func TestBackendModelsUnknownBackend(t *testing.T) {
	svc, projectID, _, _ := newModelService(t)

	if _, err := svc.BackendModels(context.Background(), projectID, "claude-code", false); !errors.Is(err, ErrUnknownBackend) {
		t.Fatalf("BackendModels error = %v, want ErrUnknownBackend", err)
	}
}

// TestBackendModelsRefreshesAfterServiceChange checks the cache is dropped when
// the machine-level service is started or stopped: a catalog learned from a
// service that has since gone down describes a backend that no longer exists.
func TestBackendModelsRefreshesAfterServiceChange(t *testing.T) {
	svc, projectID, _, fake := newModelService(t)
	ctx := context.Background()

	if _, err := svc.BackendModels(ctx, projectID, agents.BackendPi, false); err != nil {
		t.Fatalf("BackendModels: %v", err)
	}
	svc.invalidateModels(agents.BackendPi)
	if _, err := svc.BackendModels(ctx, projectID, agents.BackendPi, false); err != nil {
		t.Fatalf("BackendModels after invalidation: %v", err)
	}
	if got := fake.modelCallCount(); got != 2 {
		t.Fatalf("backend was asked %d times, want the invalidation to force a fresh read", got)
	}
}

// TestBackendModelsServesStaleSnapshotWhileRefreshing checks an aged snapshot
// answers at once and the backend is asked behind the response, which is what
// keeps the workbench instant and eventually correct.
func TestBackendModelsServesStaleSnapshotWhileRefreshing(t *testing.T) {
	svc, projectID, _, fake := newModelService(t, WithModelCatalogOptions(agents.WithModelTTL(time.Nanosecond)))
	ctx := context.Background()

	if _, err := svc.BackendModels(ctx, projectID, agents.BackendPi, false); err != nil {
		t.Fatalf("BackendModels: %v", err)
	}
	list, err := svc.BackendModels(ctx, projectID, agents.BackendPi, false)
	if err != nil {
		t.Fatalf("BackendModels stale: %v", err)
	}
	if !list.Stale {
		t.Fatalf("BackendModels = %+v, want the aged snapshot marked stale", list)
	}
	if len(list.Models) != 2 {
		t.Fatalf("stale answer = %+v, want the models the cache holds", list.Models)
	}
	waitForModelCalls(t, fake, 2)
}

// TestBackendModelRoute covers the HTTP contract: models plus how current they
// are, ?refresh=1, and the refusal codes.
func TestBackendModelRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc, projectID, _, fake := newModelService(t)

	router := gin.New()
	svc.RegisterRoutes(router)
	route := fmt.Sprintf("/projects/%d/agent-backends/pi/models", projectID)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, route, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s = %d %s", route, rec.Code, rec.Body.String())
	}
	var body struct {
		Models []agents.ModelInfo `json:"models"`
		Stale  bool               `json:"stale"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode models: %v (%s)", err, rec.Body.String())
	}
	if len(body.Models) != 2 || body.Models[0].ID != "acme/gpt" || len(body.Models[0].Variants) != 1 {
		t.Fatalf("GET %s body = %s, want the backend's models with variants", route, rec.Body.String())
	}
	if body.Stale {
		t.Errorf("first answer is marked stale: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"updatedAt"`) {
		t.Errorf("response carries no updatedAt: %s", rec.Body.String())
	}

	// The cached answer is served again until the caller asks for a refresh.
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, route, nil))
	if rec.Code != http.StatusOK || fake.modelCallCount() != 1 {
		t.Fatalf("cached GET = %d %s after %d backend calls", rec.Code, rec.Body.String(), fake.modelCallCount())
	}
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, route+"?refresh=1", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("refresh GET = %d %s", rec.Code, rec.Body.String())
	}
	if got := fake.modelCallCount(); got != 2 {
		t.Fatalf("backend was asked %d times, want ?refresh=1 to reach it", got)
	}

	// A backend that does not offer model selection is a 409, and an unknown
	// backend is a 400.
	fake.mu.Lock()
	fake.caps = agents.Capabilities{}
	fake.mu.Unlock()
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, route, nil))
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "unsupported_capability") {
		t.Fatalf("unsupported GET = %d %s, want 409 unsupported_capability", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/projects/%d/agent-backends/claude-code/models", projectID), nil))
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "unknown_backend") {
		t.Fatalf("unknown backend GET = %d %s, want 400 unknown_backend", rec.Code, rec.Body.String())
	}
}
