package settings

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEnginesSettingPersistence(t *testing.T) {
	svc := NewService(openTestDB(t), testCatalog(t))

	initial, err := svc.Engines()
	if err != nil {
		t.Fatalf("svc.Engines: %v", err)
	}
	if initial.Steward.MaxSteps != 5 {
		t.Errorf("got Steward.MaxSteps = %d, want 5", initial.Steward.MaxSteps)
	}

	initial.Steward.MaxSteps = 12
	initial.Coder.PermissionMode = "trust"
	initial.Orchestrator.MaxSteps = 30

	saved, err := svc.SetEngines(initial)
	if err != nil {
		t.Fatalf("svc.SetEngines: %v", err)
	}
	if saved.Steward.MaxSteps != 12 || saved.Coder.PermissionMode != "trust" {
		t.Errorf("saved setting mismatch: %+v", saved)
	}

	fetched, err := svc.Engines()
	if err != nil {
		t.Fatalf("fetch svc.Engines: %v", err)
	}
	if fetched.Steward.MaxSteps != 12 || fetched.Coder.PermissionMode != "trust" || fetched.Orchestrator.MaxSteps != 30 {
		t.Errorf("fetched setting mismatch: %+v", fetched)
	}
}

func TestEnginesHTTPHandlers(t *testing.T) {
	router, _ := newAppearanceRouter(t)

	// 1. GET /api/settings/engines
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/settings/engines", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/settings/engines returned status %d", w.Code)
	}

	var fetched EnginesSetting
	if err := json.Unmarshal(w.Body.Bytes(), &fetched); err != nil {
		t.Fatalf("decode engines response: %v", err)
	}

	// 2. PUT /api/settings/engines
	fetched.Steward.MaxSteps = 8
	fetched.Core.SearchEngine = "tavily"
	body, _ := json.Marshal(fetched)

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPut, "/api/settings/engines", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("PUT /api/settings/engines returned status %d", w.Code)
	}

	var updated EnginesSetting
	if err := json.Unmarshal(w.Body.Bytes(), &updated); err != nil {
		t.Fatalf("decode update response: %v", err)
	}
	if updated.Steward.MaxSteps != 8 || updated.Core.SearchEngine != "tavily" {
		t.Errorf("updated engines setting mismatch: %+v", updated)
	}
}
