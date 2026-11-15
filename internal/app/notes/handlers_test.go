package notes

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

func TestNotesHandlers(t *testing.T) {
	db := setupTestDB(t)
	svc := NewService(NewRepository(db))
	engine := setupTestEngine(svc)

	// 1. GET /api/notes (empty)
	{
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/notes", nil)
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
		var resp struct {
			Notes []Note `json:"notes"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal list: %v", err)
		}
		if len(resp.Notes) != 0 {
			t.Fatalf("expected empty list, got %d", len(resp.Notes))
		}
	}

	// 2. POST /api/notes (Create)
	var created Note
	{
		body, _ := json.Marshal(CreateNoteRequest{
			Title:   "Sprint 1 Note",
			Content: "Tasks to do",
		})
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/notes", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
		}
		var resp struct {
			Note Note `json:"note"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal created: %v", err)
		}
		created = resp.Note
		if created.ID == 0 || created.Title != "Sprint 1 Note" {
			t.Fatalf("unexpected created note: %+v", created)
		}
	}

	// 3. GET /api/notes/:id
	{
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/notes/1", nil)
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
		var resp struct {
			Note Note `json:"note"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal get: %v", err)
		}
		if resp.Note.ID != created.ID || resp.Note.Title != "Sprint 1 Note" {
			t.Fatalf("unexpected note: %+v", resp.Note)
		}
	}

	// 4. PUT /api/notes/:id
	{
		newTitle := "Sprint 1 Note Updated"
		newContent := "Updated details"
		body, _ := json.Marshal(UpdateNoteRequest{
			Title:   &newTitle,
			Content: &newContent,
		})
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("PUT", "/api/notes/1", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		var resp struct {
			Note Note `json:"note"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal update: %v", err)
		}
		if resp.Note.Title != newTitle || resp.Note.Content != newContent {
			t.Fatalf("unexpected updated note: %+v", resp.Note)
		}
	}

	// 5. DELETE /api/notes/:id
	{
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("DELETE", "/api/notes/1", nil)
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
	}

	// 6. GET /api/notes/:id after deletion returns 404
	{
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/notes/1", nil)
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", w.Code)
		}
	}
}
