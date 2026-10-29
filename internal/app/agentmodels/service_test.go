package agentmodels

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"devplatform/internal/core/agents"
)

// stubAdapter implements agents.AgentAdapter with a scriptable model list. Only
// ListModels has behavior; the rest exist so the registry accepts it.
type stubAdapter struct {
	caps   agents.Capabilities
	models []agents.ModelInfo
	err    error
	calls  int
	dirs   []string
}

func (s *stubAdapter) Capabilities() agents.Capabilities { return s.caps }
func (s *stubAdapter) Probe(context.Context) error       { return nil }
func (s *stubAdapter) StartInstance(context.Context, agents.StartInstanceRequest) (*agents.Instance, error) {
	return nil, errors.New("not implemented")
}
func (s *stubAdapter) StopInstance(context.Context, string) error { return nil }
func (s *stubAdapter) ListSessions(context.Context, string) ([]agents.SessionRef, error) {
	return nil, nil
}
func (s *stubAdapter) CreateSession(context.Context, string, agents.CreateSessionOptions) (*agents.SessionRef, error) {
	return nil, errors.New("not implemented")
}
func (s *stubAdapter) ContinueSession(context.Context, agents.SessionRef) (*agents.SessionRef, error) {
	return nil, errors.New("not implemented")
}
func (s *stubAdapter) SendMessage(context.Context, agents.SessionRef, agents.SendMessageRequest) error {
	return nil
}
func (s *stubAdapter) Subscribe(context.Context, string) (<-chan agents.NormalizedEvent, error) {
	return nil, errors.New("not implemented")
}
func (s *stubAdapter) ListModels(_ context.Context, directory string) ([]agents.ModelInfo, error) {
	s.calls++
	s.dirs = append(s.dirs, directory)
	if s.err != nil {
		return nil, s.err
	}
	return append([]agents.ModelInfo(nil), s.models...), nil
}
func (s *stubAdapter) Interrupt(context.Context, agents.SessionRef) error { return nil }

// fakeRepository is an in-memory snapshots store.
type fakeRepository struct {
	models       []agents.ModelInfo
	updatedAt    *time.Time
	listErr      error
	replaceErr   error
	replaceCalls int
	replaced     []agents.ModelInfo
	replacedAt   time.Time
}

func (f *fakeRepository) List(agents.BackendID) ([]agents.ModelInfo, *time.Time, error) {
	if f.listErr != nil {
		return nil, nil, f.listErr
	}
	return append([]agents.ModelInfo(nil), f.models...), f.updatedAt, nil
}

func (f *fakeRepository) Replace(_ agents.BackendID, models []agents.ModelInfo, syncedAt time.Time) error {
	f.replaceCalls++
	if f.replaceErr != nil {
		return f.replaceErr
	}
	f.replaced = append([]agents.ModelInfo(nil), models...)
	f.replacedAt = syncedAt
	f.models = append([]agents.ModelInfo(nil), models...)
	f.updatedAt = &syncedAt
	return nil
}

func (f *fakeRepository) Update(_ agents.BackendID, modelID string, patch ModelPatch) (agents.ModelInfo, error) {
	for i, m := range f.models {
		if m.ID == modelID {
			if patch.Enabled != nil {
				m.Enabled = *patch.Enabled
			}
			if patch.Name != nil {
				m.Name = *patch.Name
			}
			if patch.ContextLimit != nil {
				m.ContextLimit = patch.ContextLimit
			}
			if patch.SupportsImages != nil {
				m.SupportsImages = *patch.SupportsImages
			}
			f.models[i] = m
			return m, nil
		}
	}
	return agents.ModelInfo{}, ErrModelNotFound
}

func newService(t *testing.T, caps agents.Capabilities, repo Repository) (*Service, *stubAdapter) {
	t.Helper()

	adapter := &stubAdapter{caps: caps}
	registry := agents.NewRegistry()
	if err := registry.Register(agents.BackendPi, adapter); err != nil {
		t.Fatalf("register adapter: %v", err)
	}
	return NewServiceWithRepository(repo, registry), adapter
}

func TestListServesPersistedSnapshotWithoutAskingBackend(t *testing.T) {
	synced := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	repo := &fakeRepository{
		models:    []agents.ModelInfo{{ID: "acme/gpt", Name: "GPT", Provider: "acme"}},
		updatedAt: &synced,
	}
	service, adapter := newService(t, agents.Capabilities{ModelSelection: true}, repo)

	list, err := service.List(agents.BackendPi)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if adapter.calls != 0 {
		t.Fatalf("adapter was asked %d times, want the persisted snapshot only", adapter.calls)
	}
	if len(list.Models) != 1 || list.Models[0].ID != "acme/gpt" {
		t.Fatalf("List models = %+v, want the persisted model", list.Models)
	}
	if list.UpdatedAt == nil || !list.UpdatedAt.Equal(synced) {
		t.Errorf("List updatedAt = %v, want %v", list.UpdatedAt, synced)
	}
}

func TestListNeverSyncedIsEmptyWithNilTime(t *testing.T) {
	service, _ := newService(t, agents.Capabilities{ModelSelection: true}, &fakeRepository{})

	list, err := service.List(agents.BackendPi)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if list.Models == nil || len(list.Models) != 0 {
		t.Errorf("List models = %#v, want a non-nil empty slice", list.Models)
	}
	if list.UpdatedAt != nil {
		t.Errorf("List updatedAt = %v, want nil before the first refresh", list.UpdatedAt)
	}
}

func TestListRequiresModelSelection(t *testing.T) {
	service, _ := newService(t, agents.Capabilities{}, &fakeRepository{})

	if _, err := service.List(agents.BackendPi); !errors.Is(err, ErrUnsupportedCapability) {
		t.Fatalf("List error = %v, want ErrUnsupportedCapability", err)
	}
}

func TestListUnknownBackend(t *testing.T) {
	service, _ := newService(t, agents.Capabilities{ModelSelection: true}, &fakeRepository{})

	if _, err := service.List(agents.BackendID("claude-code")); !errors.Is(err, ErrUnknownBackend) {
		t.Fatalf("List error = %v, want ErrUnknownBackend", err)
	}
}

func TestRefreshReadsBackendAndReplacesSnapshot(t *testing.T) {
	repo := &fakeRepository{}
	service, adapter := newService(t, agents.Capabilities{ModelSelection: true}, repo)
	adapter.models = []agents.ModelInfo{
		{ID: "acme/gpt", Name: "GPT", Provider: "acme", Variants: []string{"fast"}},
		{ID: "acme/gpt", Name: "duplicate", Provider: "acme"},
		{ID: "", Name: "empty id"},
		{ID: "acme/small", Name: "Small", Provider: "acme"},
	}
	fixed := time.Date(2026, 10, 23, 12, 30, 0, 0, time.UTC)
	service.now = func() time.Time { return fixed }

	list, err := service.Refresh(context.Background(), agents.BackendPi)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if adapter.calls != 1 || len(adapter.dirs) != 1 || adapter.dirs[0] != "" {
		t.Errorf("adapter calls = %d dirs = %v, want one platform-scope read", adapter.calls, adapter.dirs)
	}
	if repo.replaceCalls != 1 {
		t.Fatalf("Replace calls = %d, want one", repo.replaceCalls)
	}
	if len(repo.replaced) != 2 || repo.replaced[0].ID != "acme/gpt" || repo.replaced[1].ID != "acme/small" {
		t.Errorf("replaced models = %+v, want deduplicated order-preserving list", repo.replaced)
	}
	if !repo.replacedAt.Equal(fixed) {
		t.Errorf("replacedAt = %v, want %v", repo.replacedAt, fixed)
	}
	if list.UpdatedAt == nil || !list.UpdatedAt.Equal(fixed) {
		t.Errorf("Refresh updatedAt = %v, want %v", list.UpdatedAt, fixed)
	}
}

func TestRefreshFailureKeepsSnapshot(t *testing.T) {
	repo := &fakeRepository{}
	service, adapter := newService(t, agents.Capabilities{ModelSelection: true}, repo)
	adapter.err = agents.ErrServiceNotRunning

	_, err := service.Refresh(context.Background(), agents.BackendPi)
	if !errors.Is(err, ErrModelListFailed) || !errors.Is(err, agents.ErrServiceNotRunning) {
		t.Fatalf("Refresh error = %v, want model-list and service-not-running errors", err)
	}
	if repo.replaceCalls != 0 {
		t.Errorf("Replace calls = %d, want the failed refresh to leave the snapshot alone", repo.replaceCalls)
	}
}

func TestRefreshStorageFailureIsReported(t *testing.T) {
	repo := &fakeRepository{replaceErr: errors.New("disk full")}
	service, adapter := newService(t, agents.Capabilities{ModelSelection: true}, repo)
	adapter.models = []agents.ModelInfo{{ID: "acme/gpt", Provider: "acme"}}

	if _, err := service.Refresh(context.Background(), agents.BackendPi); !errors.Is(err, ErrStorage) {
		t.Fatalf("Refresh error = %v, want ErrStorage", err)
	}
}

func TestRoutesReadSnapshotAndRefreshOnPost(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &fakeRepository{}
	service, adapter := newService(t, agents.Capabilities{ModelSelection: true}, repo)
	adapter.models = []agents.ModelInfo{{ID: "acme/gpt", Name: "GPT", Provider: "acme"}}

	router := gin.New()
	service.RegisterRoutes(router)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/agent-backends/pi/models", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET models = %d %s", rec.Code, rec.Body.String())
	}
	var body ModelList
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode GET body: %v (%s)", err, rec.Body.String())
	}
	if body.Models == nil || len(body.Models) != 0 || body.UpdatedAt != nil {
		t.Errorf("GET body = %s, want an empty, never-synced snapshot", rec.Body.String())
	}
	if adapter.calls != 0 {
		t.Errorf("GET asked the adapter %d times, want zero", adapter.calls)
	}

	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/agent-backends/pi/models/refresh", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("POST refresh = %d %s", rec.Code, rec.Body.String())
	}
	if adapter.calls != 1 || repo.replaceCalls != 1 {
		t.Errorf("refresh calls = %d replaces = %d, want one each", adapter.calls, repo.replaceCalls)
	}

	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/agent-backends/pi/models", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET after refresh = %d %s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode GET body: %v (%s)", err, rec.Body.String())
	}
	if len(body.Models) != 1 || body.Models[0].ID != "acme/gpt" {
		t.Errorf("GET after refresh models = %+v, want the persisted model", body.Models)
	}
	if adapter.calls != 1 {
		t.Errorf("GET after refresh asked the adapter, want persisted read only")
	}
}

func TestRouteMapsBackendFailures(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service, adapter := newService(t, agents.Capabilities{ModelSelection: true}, &fakeRepository{})
	adapter.err = agents.ErrServiceNotRunning
	router := gin.New()
	service.RegisterRoutes(router)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/agent-backends/pi/models/refresh", nil))
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "service_not_running") {
		t.Fatalf("POST refresh = %d %s, want 409 service_not_running", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/agent-backends/claude-code/models", nil))
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "unknown_backend") {
		t.Fatalf("GET unknown = %d %s, want 400 unknown_backend", rec.Code, rec.Body.String())
	}
}

func TestRouteRefusesUnsupportedCapability(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service, _ := newService(t, agents.Capabilities{}, &fakeRepository{})
	router := gin.New()
	service.RegisterRoutes(router)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/agent-backends/pi/models/refresh", nil))
	if rec.Code != http.StatusConflict {
		t.Fatalf("POST refresh = %d %s, want 409", rec.Code, rec.Body.String())
	}
}

func TestUpdateModelServiceAndRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &fakeRepository{
		models: []agents.ModelInfo{
			{ID: "openai/gpt-4o", Name: "GPT-4o", Provider: "openai", Enabled: true},
		},
	}
	service, _ := newService(t, agents.Capabilities{ModelSelection: true}, repo)
	router := gin.New()
	service.RegisterRoutes(router)

	// 1. Successful PATCH with path param containing slash
	patchPayload := `{"enabled":false,"name":"Custom GPT-4o","contextLimit":128000,"supportsImages":true}`
	req := httptest.NewRequest(http.MethodPatch, "/agent-backends/pi/models/openai/gpt-4o", strings.NewReader(patchPayload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var updated agents.ModelInfo
	if err := json.Unmarshal(rec.Body.Bytes(), &updated); err != nil {
		t.Fatalf("decode updated: %v", err)
	}
	if updated.Enabled != false || updated.Name != "Custom GPT-4o" || updated.ContextLimit == nil || *updated.ContextLimit != 128000 || !updated.SupportsImages {
		t.Errorf("updated model = %+v, want modified fields", updated)
	}

	// 2. Successful PATCH on /models with modelId in payload
	patchPayload2 := `{"modelId":"openai/gpt-4o","enabled":true}`
	req2 := httptest.NewRequest(http.MethodPatch, "/agent-backends/pi/models", strings.NewReader(patchPayload2))
	req2.Header.Set("Content-Type", "application/json")
	rec2 := httptest.NewRecorder()
	router.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("PATCH body modelId status = %d, body = %s", rec2.Code, rec2.Body.String())
	}

	// 3. 404 for non-existent model
	reqNotFound := httptest.NewRequest(http.MethodPatch, "/agent-backends/pi/models/non-existent", strings.NewReader(`{"enabled":false}`))
	reqNotFound.Header.Set("Content-Type", "application/json")
	recNotFound := httptest.NewRecorder()
	router.ServeHTTP(recNotFound, reqNotFound)
	if recNotFound.Code != http.StatusNotFound {
		t.Errorf("PATCH non-existent = %d, want 404", recNotFound.Code)
	}

	// 4. 400 for mismatched modelId
	reqMismatch := httptest.NewRequest(http.MethodPatch, "/agent-backends/pi/models/openai/gpt-4o", strings.NewReader(`{"modelId":"different"}`))
	reqMismatch.Header.Set("Content-Type", "application/json")
	recMismatch := httptest.NewRecorder()
	router.ServeHTTP(recMismatch, reqMismatch)
	if recMismatch.Code != http.StatusBadRequest {
		t.Errorf("PATCH mismatch = %d, want 400", recMismatch.Code)
	}
}
