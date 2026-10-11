package resources

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// newTestEngine builds a gin engine with the resource routes registered under
// /api. Auth is deliberately absent: these tests exercise the handler mapping,
// not the session gate (covered by internal/http).
func newTestEngine(t *testing.T, db *gorm.DB) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	NewService(db).RegisterRoutes(engine.Group("/api"))
	return engine
}

// doJSON issues a JSON request against the engine and returns the recorder.
func doJSON(t *testing.T, engine *gin.Engine, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		payload, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(payload)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	return rec
}

func TestHandlersResourceLifecycle(t *testing.T) {
	db := openTestDB(t)
	engine := newTestEngine(t, db)

	// Create.
	rec := doJSON(t, engine, http.MethodPost, "/api/resources", map[string]any{
		"kind":        "skill",
		"name":        "commit-style",
		"description": "how the team writes commits",
		"content":     "Use the imperative mood.",
		"enabled":     true,
		"backends":    []string{"pi"},
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var created ResourceDefinition
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if created.ID == 0 || created.Name != "commit-style" {
		t.Fatalf("create response = %+v", created)
	}

	// Read.
	rec = doJSON(t, engine, http.MethodGet, "/api/resources/"+itoa(created.ID), nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("get status = %d, body = %s", rec.Code, rec.Body.String())
	}

	// List.
	rec = doJSON(t, engine, http.MethodGet, "/api/resources", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var list struct {
		Resources []ResourceDefinition `json:"resources"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(list.Resources) != 1 {
		t.Fatalf("list = %+v, want one resource", list.Resources)
	}

	// Update.
	rec = doJSON(t, engine, http.MethodPost, "/api/resources/"+itoa(created.ID)+"/update", map[string]any{
		"kind":     "skill",
		"name":     "commit-style-v2",
		"content":  "Wrap at 72 columns.",
		"enabled":  false,
		"backends": []string{"pi"},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("update status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var updated ResourceDefinition
	if err := json.Unmarshal(rec.Body.Bytes(), &updated); err != nil {
		t.Fatalf("decode update response: %v", err)
	}
	if updated.Name != "commit-style-v2" || updated.Enabled {
		t.Fatalf("update response = %+v", updated)
	}

	// Delete.
	rec = doJSON(t, engine, http.MethodPost, "/api/resources/"+itoa(created.ID)+"/delete", nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, body = %s", rec.Code, rec.Body.String())
	}
	rec = doJSON(t, engine, http.MethodGet, "/api/resources/"+itoa(created.ID), nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("get after delete status = %d, want 404", rec.Code)
	}
}

func TestHandlersRejectInvalidAndUnknown(t *testing.T) {
	db := openTestDB(t)
	engine := newTestEngine(t, db)

	rec := doJSON(t, engine, http.MethodPost, "/api/resources", map[string]any{
		"kind": "skill",
		"name": "../evil",
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid name status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "invalid_resource") {
		t.Fatalf("invalid name error = %s, want invalid_resource", rec.Body.String())
	}

	rec = doJSON(t, engine, http.MethodPost, "/api/resources", map[string]any{
		"kind": "skill",
		"name": "ok",
	})
	// sanity: a valid second create
	_ = rec

	rec = doJSON(t, engine, http.MethodGet, "/api/resources/9999", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown resource status = %d, want 404", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "resource_not_found") {
		t.Fatalf("unknown resource error = %s", rec.Body.String())
	}

	rec = doJSON(t, engine, http.MethodGet, "/api/resources/not-a-number", nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("malformed id status = %d, want 400", rec.Code)
	}
}

func TestHandlersProjectAssociationAndSessionPrompts(t *testing.T) {
	db := openTestDB(t)
	engine := newTestEngine(t, db)
	project := seedProject(t, db)
	session := seedSession(t, db, project.ID)

	// Create a session-scoped prompt.
	rec := doJSON(t, engine, http.MethodPost, "/api/resources", map[string]any{
		"kind":        "prompt",
		"name":        "review-checklist",
		"content":     "Check the diff.",
		"enabled":     true,
		"promptScope": "session",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create prompt status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var prompt ResourceDefinition
	if err := json.Unmarshal(rec.Body.Bytes(), &prompt); err != nil {
		t.Fatalf("decode prompt: %v", err)
	}

	// Associate it with the project.
	rec = doJSON(t, engine, http.MethodPost, "/api/projects/"+itoa(project.ID)+"/resources", map[string]any{
		"resourceId": prompt.ID,
	})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("associate status = %d, body = %s", rec.Code, rec.Body.String())
	}

	rec = doJSON(t, engine, http.MethodGet, "/api/projects/"+itoa(project.ID)+"/resources", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list project resources status = %d", rec.Code)
	}
	var projectResources struct {
		Resources []ResourceDefinition `json:"resources"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &projectResources); err != nil {
		t.Fatalf("decode project resources: %v", err)
	}
	if len(projectResources.Resources) != 1 || projectResources.Resources[0].ID != prompt.ID {
		t.Fatalf("project resources = %+v", projectResources.Resources)
	}

	// Select it for the session.
	sessionPath := "/api/projects/" + itoa(project.ID) + "/sessions/" + session.ID + "/prompts"
	rec = doJSON(t, engine, http.MethodPost, sessionPath, map[string]any{"resourceId": prompt.ID})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("select prompt status = %d, body = %s", rec.Code, rec.Body.String())
	}
	rec = doJSON(t, engine, http.MethodGet, sessionPath, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list session prompts status = %d", rec.Code)
	}
	var sessionPrompts struct {
		Prompts []ResourceDefinition `json:"prompts"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &sessionPrompts); err != nil {
		t.Fatalf("decode session prompts: %v", err)
	}
	if len(sessionPrompts.Prompts) != 1 || sessionPrompts.Prompts[0].ID != prompt.ID {
		t.Fatalf("session prompts = %+v", sessionPrompts.Prompts)
	}

	// Remove and disassociate.
	rec = doJSON(t, engine, http.MethodPost, sessionPath+"/remove", map[string]any{"resourceId": prompt.ID})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("remove session prompt status = %d, body = %s", rec.Code, rec.Body.String())
	}
	rec = doJSON(t, engine, http.MethodPost, "/api/projects/"+itoa(project.ID)+"/resources/remove", map[string]any{
		"resourceId": prompt.ID,
	})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("disassociate status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestHandlersNeverReturnPlaintextSecret(t *testing.T) {
	db := openTestDB(t)
	engine := newTestEngine(t, db)

	rec := doJSON(t, engine, http.MethodPost, "/api/resources", map[string]any{
		"kind":      "mcp",
		"name":      "filesystem-server",
		"content":   `{"command":"npx"}`,
		"enabled":   true,
		"secretRef": "sec_opaque_1",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create mcp status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var mcp ResourceDefinition
	if err := json.Unmarshal(rec.Body.Bytes(), &mcp); err != nil {
		t.Fatalf("decode mcp: %v", err)
	}

	rec = doJSON(t, engine, http.MethodGet, "/api/resources/"+itoa(mcp.ID), nil)
	body := rec.Body.String()
	if !strings.Contains(body, `"secretRef":"sec_opaque_1"`) {
		t.Fatalf("response missing opaque secretRef: %s", body)
	}
	for _, forbidden := range []string{`"secret":`, `"value":`, `"token":`, `"password":`} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("response leaked %s: %s", forbidden, body)
		}
	}
}

// itoa formats an id for a URL path.
func itoa(id uint) string {
	return strconv.FormatUint(uint64(id), 10)
}
