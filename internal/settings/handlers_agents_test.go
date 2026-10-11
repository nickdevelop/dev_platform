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
)

func agentReq(t *testing.T, router *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	return rec
}

func TestAgentHandlerListAndToggle(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // no agent binaries on PATH
	router, _ := newAppearanceRouter(t)

	rec := agentReq(t, router, http.MethodGet, "/api/agents", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET status = %d, body %s", rec.Code, rec.Body.String())
	}
	var list struct {
		Agents []struct {
			Backend    string `json:"backend"`
			Enabled    bool   `json:"enabled"`
			Path       string `json:"path"`
			PathSource string `json:"pathSource"`
			Available  bool   `json:"available"`
		} `json:"agents"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(list.Agents) != 2 {
		t.Fatalf("agents = %+v, want both backends", list.Agents)
	}
	for _, agent := range list.Agents {
		if !agent.Enabled || agent.PathSource != "none" || agent.Available {
			t.Errorf("agent = %+v, want enabled and unavailable", agent)
		}
	}

	disable := agentReq(t, router, http.MethodPut, "/api/agents/pi", `{"enabled":false}`)
	if disable.Code != http.StatusOK {
		t.Fatalf("disable status = %d, body %s", disable.Code, disable.Body.String())
	}
	var updated struct {
		Enabled bool `json:"enabled"`
	}
	_ = json.Unmarshal(disable.Body.Bytes(), &updated)
	if updated.Enabled {
		t.Error("pi should be disabled after PUT")
	}
}

func TestAgentHandlerRejectsUnknownBackend(t *testing.T) {
	router, _ := newAppearanceRouter(t)
	rec := agentReq(t, router, http.MethodPut, "/api/agents/nope", `{"enabled":false}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("unknown backend status = %d, want 400", rec.Code)
	}
}

func TestAgentHandlerManualPath(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	router, _ := newAppearanceRouter(t)

	dir := t.TempDir()
	binary := filepath.Join(dir, "my-pi")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write binary: %v", err)
	}

	ok := agentReq(t, router, http.MethodPut, "/api/agents/pi/path", `{"path":"`+binary+`"}`)
	if ok.Code != http.StatusOK {
		t.Fatalf("set path status = %d, body %s", ok.Code, ok.Body.String())
	}
	var dto struct {
		Path       string `json:"path"`
		PathSource string `json:"pathSource"`
		Available  bool   `json:"available"`
	}
	_ = json.Unmarshal(ok.Body.Bytes(), &dto)
	if dto.Path != binary || dto.PathSource != "manual" || !dto.Available {
		t.Errorf("dto = %+v, want manual available path %q", dto, binary)
	}

	missing := agentReq(t, router, http.MethodPut, "/api/agents/pi/path", `{"path":"/definitely/not/here"}`)
	if missing.Code != http.StatusBadRequest {
		t.Errorf("missing path status = %d, want 400", missing.Code)
	}
}

func TestAgentHandlerDetectReportsVersion(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	router, _ := newAppearanceRouter(t)

	dir := t.TempDir()
	binary := filepath.Join(dir, "my-pi")
	script := "#!/bin/sh\necho \"pi 1.2.3\"\n"
	if err := os.WriteFile(binary, []byte(script), 0o755); err != nil {
		t.Fatalf("write binary: %v", err)
	}
	_ = agentReq(t, router, http.MethodPut, "/api/agents/pi/path", `{"path":"`+binary+`"}`)

	rec := agentReq(t, router, http.MethodPost, "/api/agents/pi/detect", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("detect status = %d, body %s", rec.Code, rec.Body.String())
	}
	var result struct {
		Status  string `json:"status"`
		Path    string `json:"path"`
		Version string `json:"version"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode detect: %v", err)
	}
	if result.Status != "ok" || result.Version != "1.2.3" || result.Path != binary {
		t.Errorf("detect result = %+v, want ok 1.2.3 at %q", result, binary)
	}

	// The detected version is persisted and shown by GET.
	get := agentReq(t, router, http.MethodGet, "/api/agents", "")
	if !strings.Contains(get.Body.String(), "1.2.3") {
		t.Errorf("GET body %s does not include the detected version", get.Body.String())
	}
}
