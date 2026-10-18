package prompts

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func setupTestEngine(svc *Service) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("/api")
	svc.RegisterRoutes(api)
	return r
}

func TestPromptHandlers(t *testing.T) {
	db := setupTestDB(t)
	svc := NewService(NewRepository(db))
	engine := setupTestEngine(svc)

	// 1. GET /api/prompts (empty)
	{
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/prompts", nil)
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
		var resp struct {
			Prompts []Prompt `json:"prompts"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal list: %v", err)
		}
		if len(resp.Prompts) != 0 {
			t.Fatalf("expected empty list, got %d", len(resp.Prompts))
		}
	}

	// 2. POST /api/prompts (Create)
	var created Prompt
	{
		body, _ := json.Marshal(CreatePromptRequest{
			Name:        "Test Prompt",
			Description: "For testing API",
			Content:     "Hello {user_prompt}",
		})
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/prompts", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
		}
		if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
			t.Fatalf("unmarshal created: %v", err)
		}
		if created.ID == 0 || created.Name != "Test Prompt" {
			t.Fatalf("unexpected created prompt: %+v", created)
		}
	}

	// 3. POST /api/prompts (Duplicate conflict 409)
	{
		body, _ := json.Marshal(CreatePromptRequest{
			Name:    "Test Prompt",
			Content: "Duplicate",
		})
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/prompts", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusConflict {
			t.Fatalf("expected 409 conflict, got %d: %s", w.Code, w.Body.String())
		}
	}

	// 4. GET /api/prompts/:id
	{
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/prompts/1", nil)
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
		var got Prompt
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatalf("unmarshal got: %v", err)
		}
		if got.ID != created.ID {
			t.Fatalf("got id %d, want %d", got.ID, created.ID)
		}
	}

	// 5. PUT /api/prompts/:id (Update)
	{
		newName := "Updated Prompt"
		body, _ := json.Marshal(UpdatePromptRequest{
			Name: &newName,
		})
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("PUT", "/api/prompts/1", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		var updated Prompt
		if err := json.Unmarshal(w.Body.Bytes(), &updated); err != nil {
			t.Fatalf("unmarshal updated: %v", err)
		}
		if updated.Name != newName {
			t.Fatalf("updated name %q, want %q", updated.Name, newName)
		}
	}

	// 6. DELETE /api/prompts/:id
	{
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("DELETE", "/api/prompts/1", nil)
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
	}

	// 7. GET after delete (404)
	{
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/prompts/1", nil)
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", w.Code)
		}
	}
}
