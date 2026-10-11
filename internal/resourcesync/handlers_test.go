package resourcesync

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"devplatform/internal/agents"
	"devplatform/internal/resources"
)

func newSyncEngine(t *testing.T, c *Coordinator) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	c.RegisterRoutes(engine.Group("/api"))
	return engine
}

func TestCoordinatorHandlersPreviewApplyResolve(t *testing.T) {
	adapter := &fakeAdapter{backend: agents.BackendPi, kinds: allKinds(), dir: "notes"}
	source := &fakeSource{defs: []resources.ResourceDefinition{testDefinition(1, resources.ResourceSkill, "lint", "body")}}
	c, _ := newCoordinator(t, source, adapter)
	engine := newSyncEngine(t, c)

	// Preview.
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/projects/1/resources/sync", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("preview status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var previewBody previewListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &previewBody); err != nil {
		t.Fatalf("decode preview: %v", err)
	}
	if len(previewBody.Previews) != 1 || previewBody.Previews[0].State != SyncStatePending {
		t.Fatalf("previews = %+v", previewBody.Previews)
	}

	// Apply.
	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/projects/1/resources/sync", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("apply status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &previewBody); err != nil {
		t.Fatalf("decode apply: %v", err)
	}
	if len(previewBody.Previews) != 1 || previewBody.Previews[0].State != SyncStateApplied {
		t.Fatalf("apply previews = %+v", previewBody.Previews)
	}

	// Resolve an unknown resource is a 404.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/projects/1/resources/sync/resolve", strings.NewReader(`{"resourceId":99}`))
	req.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("resolve unknown status = %d, body = %s", rec.Code, rec.Body.String())
	}

	// A malformed project id is a 400.
	rec = httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/projects/abc/resources/sync", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("malformed project status = %d, want 400", rec.Code)
	}
}
