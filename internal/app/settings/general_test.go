package settings

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"devplatform/internal/core/platform/fake"
)

func TestAgentGeneralDefaults(t *testing.T) {
	dataDir := t.TempDir()
	configDir := t.TempDir()
	svc := NewService(openTestDB(t), testCatalog(t), WithDataDir(dataDir))
	svc.platform = fake.New(fake.WithConfigDir(configDir))

	got, err := svc.AgentGeneral()
	if err != nil {
		t.Fatalf("AgentGeneral: %v", err)
	}
	if got.SystemPrompt != "" {
		t.Errorf("AgentGeneral().SystemPrompt = %q, want empty", got.SystemPrompt)
	}
	wantPath := filepath.Join(dataDir, "AGENTS.md")
	if got.Path != wantPath {
		t.Errorf("AgentGeneral().Path = %q, want %q", got.Path, wantPath)
	}
}

func TestSetAgentGeneralRoundTrip(t *testing.T) {
	dataDir := t.TempDir()
	configDir := t.TempDir()
	db := openTestDB(t)
	svc := NewService(db, testCatalog(t), WithDataDir(dataDir))
	svc.platform = fake.New(fake.WithConfigDir(configDir))

	prompt := "# Project Instructions\nAlways be concise."
	saved, err := svc.SetAgentGeneral(AgentGeneralSetting{SystemPrompt: prompt})
	if err != nil {
		t.Fatalf("SetAgentGeneral: %v", err)
	}
	if !strings.HasPrefix(saved.SystemPrompt, prompt) {
		t.Errorf("saved.SystemPrompt = %q, want prefix %q", saved.SystemPrompt, prompt)
	}

	// Verify dataDir/AGENTS.md was written
	primaryFile := filepath.Join(dataDir, "AGENTS.md")
	data, err := os.ReadFile(primaryFile)
	if err != nil {
		t.Fatalf("read primaryFile: %v", err)
	}
	if string(data) != saved.SystemPrompt {
		t.Errorf("primaryFile content = %q, want %q", string(data), saved.SystemPrompt)
	}

	// Verify configDir/opencode/AGENTS.md was synced
	opencodeFile := filepath.Join(configDir, "opencode", "AGENTS.md")
	ocData, err := os.ReadFile(opencodeFile)
	if err != nil {
		t.Fatalf("read opencodeFile: %v", err)
	}
	if string(ocData) != saved.SystemPrompt {
		t.Errorf("opencodeFile content = %q, want %q", string(ocData), saved.SystemPrompt)
	}

	// Re-reading from a fresh service instance
	freshSvc := NewService(db, testCatalog(t), WithDataDir(dataDir))
	freshSvc.platform = fake.New(fake.WithConfigDir(configDir))
	got, err := freshSvc.AgentGeneral()
	if err != nil {
		t.Fatalf("freshSvc.AgentGeneral: %v", err)
	}
	if got.SystemPrompt != saved.SystemPrompt {
		t.Errorf("freshSvc got = %q, want %q", got.SystemPrompt, saved.SystemPrompt)
	}
}

func TestAgentGeneralHTTPHandlers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dataDir := t.TempDir()
	configDir := t.TempDir()
	svc := NewService(openTestDB(t), testCatalog(t), WithDataDir(dataDir))
	svc.platform = fake.New(fake.WithConfigDir(configDir))

	router := gin.New()
	svc.RegisterRoutes(router.Group("/api"))

	// 1. GET initial state
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/settings/agent-general", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want %d", rec.Code, http.StatusOK)
	}
	var initial AgentGeneralSetting
	if err := json.Unmarshal(rec.Body.Bytes(), &initial); err != nil {
		t.Fatalf("decode GET body: %v", err)
	}
	if initial.SystemPrompt != "" {
		t.Errorf("initial.SystemPrompt = %q, want empty", initial.SystemPrompt)
	}

	// 2. PUT new system prompt
	body := `{"systemPrompt":"# Global Rules\nTest prompt"}`
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPut, "/api/settings/agent-general", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, want %d (body %s)", rec.Code, http.StatusOK, rec.Body.String())
	}
	var updated AgentGeneralSetting
	if err := json.Unmarshal(rec.Body.Bytes(), &updated); err != nil {
		t.Fatalf("decode PUT body: %v", err)
	}
	if !strings.Contains(updated.SystemPrompt, "Global Rules") {
		t.Errorf("updated.SystemPrompt = %q, want containing Global Rules", updated.SystemPrompt)
	}

	// 3. GET after PUT
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/settings/agent-general", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET after PUT status = %d, want %d", rec.Code, http.StatusOK)
	}
	var fetched AgentGeneralSetting
	if err := json.Unmarshal(rec.Body.Bytes(), &fetched); err != nil {
		t.Fatalf("decode GET after PUT body: %v", err)
	}
	if fetched.SystemPrompt != updated.SystemPrompt {
		t.Errorf("fetched.SystemPrompt = %q, want %q", fetched.SystemPrompt, updated.SystemPrompt)
	}
}
