package roles

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

func TestRoleHandlers(t *testing.T) {
	db := setupTestDB(t)
	svc := NewService(NewRepository(db), nil)
	engine := setupTestEngine(svc)

	// 1. GET /api/roles (empty)
	{
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/roles", nil)
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
		var resp struct {
			Roles []Role `json:"roles"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal list: %v", err)
		}
		if len(resp.Roles) != 0 {
			t.Fatalf("expected empty list, got %d", len(resp.Roles))
		}
	}

	// 2. POST /api/roles (Create)
	var created Role
	{
		body, _ := json.Marshal(CreateRoleRequest{
			Name:        "Code Reviewer",
			Description: "Reviews pull requests",
			Model:       "gpt-4o",
			Color:       "#10b981",
		})
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/roles", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
		}
		if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
			t.Fatalf("unmarshal created: %v", err)
		}
		if created.ID == 0 || created.Name != "Code Reviewer" {
			t.Fatalf("unexpected created role: %+v", created)
		}
	}

	// 3. GET /api/roles/:id
	{
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/roles/1", nil)
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
		var got Role
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if got.ID != created.ID {
			t.Fatalf("expected ID %d, got %d", created.ID, got.ID)
		}
	}

	// 4. PUT /api/roles/:id
	{
		newName := "Senior Reviewer"
		body, _ := json.Marshal(UpdateRoleRequest{
			Name: &newName,
		})
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("PUT", "/api/roles/1", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
		var updated Role
		if err := json.Unmarshal(w.Body.Bytes(), &updated); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if updated.Name != "Senior Reviewer" {
			t.Fatalf("expected name Senior Reviewer, got %s", updated.Name)
		}
	}

	// 5. DELETE /api/roles/:id
	{
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("DELETE", "/api/roles/1", nil)
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
	}

	// 6. GET /api/roles/1 (404)
	{
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/roles/1", nil)
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", w.Code)
		}
	}
}
