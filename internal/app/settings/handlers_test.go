package settings

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func newAppearanceRouter(t *testing.T) (*gin.Engine, *Service) {
	t.Helper()

	gin.SetMode(gin.TestMode)
	svc := NewService(openTestDB(t), testCatalog(t))
	router := gin.New()
	svc.RegisterRoutes(router.Group("/api"))
	return router, svc
}

func TestAppearanceHandlerGetReturnsDefault(t *testing.T) {
	router, _ := newAppearanceRouter(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/settings/appearance", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want %d", rec.Code, http.StatusOK)
	}
	var got Appearance
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if got != DefaultAppearance() {
		t.Errorf("GET body = %+v, want %+v", got, DefaultAppearance())
	}
}

func TestAppearanceHandlerPutPersists(t *testing.T) {
	router, svc := newAppearanceRouter(t)

	body := `{"theme":"warm","fontSize":16,"density":"relaxed"}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/settings/appearance", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, want %d (body %s)", rec.Code, http.StatusOK, rec.Body.String())
	}
	stored, err := svc.Appearance()
	if err != nil {
		t.Fatalf("Appearance: %v", err)
	}
	want := Appearance{Theme: "warm", FontSize: 16, Density: "relaxed", PlantumlMode: "local", PlantumlServerURL: "https://www.plantuml.com/plantuml"}
	if stored != want {
		t.Errorf("stored = %+v, want %+v", stored, want)
	}
}

// A page loaded before the font became built-in still PUTs font and
// customFont; the API accepts the request, ignores those keys and answers
// without them.
func TestAppearanceHandlerPutIgnoresFontFields(t *testing.T) {
	router, svc := newAppearanceRouter(t)

	body := `{"theme":"warm","font":"serif","customFont":"Fira Code","fontSize":16,"density":"relaxed"}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/settings/appearance", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, want %d (body %s)", rec.Code, http.StatusOK, rec.Body.String())
	}
	for _, key := range []string{`"font"`, `"customFont"`} {
		if strings.Contains(rec.Body.String(), key) {
			t.Errorf("response body %s contains %s", rec.Body.String(), key)
		}
	}
	stored, err := svc.Appearance()
	if err != nil {
		t.Fatalf("Appearance: %v", err)
	}
	want := Appearance{Theme: "warm", FontSize: 16, Density: "relaxed", PlantumlMode: "local", PlantumlServerURL: "https://www.plantuml.com/plantuml"}
	if stored != want {
		t.Errorf("stored = %+v, want %+v", stored, want)
	}
}

func TestAppearanceHandlerPutRejectsInvalid(t *testing.T) {
	router, _ := newAppearanceRouter(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/settings/appearance", strings.NewReader(`{"theme":"neon","fontSize":14,"density":"standard"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("PUT status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	var envelope struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if envelope.Error != "invalid_appearance" {
		t.Errorf("error code = %q, want invalid_appearance", envelope.Error)
	}
}

func TestQuickAssistantHandlers(t *testing.T) {
	router, _ := newAppearanceRouter(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/settings/quick-assistant", nil)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want 200", rec.Code)
	}
	var got QuickAssistant
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode GET body: %v", err)
	}
	if got.Enabled {
		t.Errorf("GET body = %+v, want disabled default", got)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPut, "/api/settings/quick-assistant", strings.NewReader(`{"enabled":true}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode PUT body: %v", err)
	}
	if !got.Enabled {
		t.Errorf("PUT body = %+v, want enabled", got)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPut, "/api/settings/quick-assistant", strings.NewReader(`{`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid PUT status = %d, want 400", rec.Code)
	}
}
