package sessions

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"devplatform/internal/core/agents"
)

// TestBackendIconRouteAndIconURL proves a backend with an embedded logo is
// advertised with its icon path and served as a sandboxed SVG, while a backend
// without one is advertised without a path and answers 404 so the workbench
// falls back to its short name.
func TestBackendIconRouteAndIconURL(t *testing.T) {
	gin.SetMode(gin.TestMode)
	env := newTestEnv(t)
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 4 4"></svg>`)
	catalog, err := agents.NewCatalog(
		agents.BackendDescriptor{ID: agents.BackendPi, Label: "Pi", Binary: "pi", Icon: svg},
		agents.BackendDescriptor{ID: agents.BackendOpenCodeV2, Label: "OpenCode", Binary: "opencode"},
	)
	if err != nil {
		t.Fatalf("NewCatalog: %v", err)
	}
	WithCatalog(catalog)(env.svc)

	byID := map[agents.BackendID]BackendCapabilities{}
	for _, entry := range env.svc.Backends() {
		byID[entry.Backend] = entry
	}
	if got := byID[agents.BackendPi].IconURL; got != "/api/agent-backends/pi/icon" {
		t.Errorf("pi IconURL = %q, want /api/agent-backends/pi/icon", got)
	}
	if got := byID[agents.BackendOpenCodeV2].IconURL; got != "" {
		t.Errorf("opencode IconURL = %q, want empty", got)
	}

	router := gin.New()
	env.svc.RegisterRoutes(router)
	get := func(path string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		return rec
	}

	rec := get("/agent-backends/pi/icon")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET pi icon = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "image/svg+xml; charset=utf-8" {
		t.Errorf("Content-Type = %q, want image/svg+xml", got)
	}
	if got := rec.Header().Get("Content-Security-Policy"); got != "default-src 'none'; style-src 'unsafe-inline'" {
		t.Errorf("Content-Security-Policy = %q, want the sandbox policy", got)
	}
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q, want nosniff", got)
	}
	if rec.Body.String() != string(svg) {
		t.Errorf("body = %q, want the embedded svg", rec.Body.String())
	}

	for _, path := range []string{"/agent-backends/opencode-v2/icon", "/agent-backends/ghost/icon"} {
		if rec := get(path); rec.Code != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", path, rec.Code)
		}
	}
}
