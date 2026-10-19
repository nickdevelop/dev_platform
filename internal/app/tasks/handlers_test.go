package tasks

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"devplatform/internal/core/agents"
)

func setupTestEngine(svc *Service) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("/api")
	svc.RegisterRoutes(api)
	return r
}

func TestTaskHandlers(t *testing.T) {
	db := setupTestDB(t)
	launcher := &fakeSessionLauncher{}
	svc := NewService(NewRepository(db), launcher)
	engine := setupTestEngine(svc)

	// 1. POST /api/tasks (Create standard task)
	var created Task
	{
		future := time.Now().Add(2 * time.Hour)
		body, _ := json.Marshal(CreateTaskRequest{
			Title:          "HTTP 任务测试",
			Description:    "测试用例",
			TaskType:       TaskTypeStandard,
			ProjectID:      1,
			Backend:        agents.BackendOpenCodeV2,
			Model:          "claude-3-5-sonnet",
			PromptTemplate: "请检查单元测试覆盖率并输出报告",
			ScheduleType:   ScheduleOnce,
			ScheduledAt:    &future,
			ScheduleEnabled: true,
		})
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/tasks", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("create task code = %d, want %d, body: %s", w.Code, http.StatusCreated, w.Body.String())
		}
		_ = json.Unmarshal(w.Body.Bytes(), &created)
		if created.ID == 0 || created.PromptTemplate != "请检查单元测试覆盖率并输出报告" {
			t.Fatalf("unexpected created task: %+v", created)
		}
	}

	// 2. GET /api/tasks (List)
	{
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/tasks", nil)
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("list tasks code = %d, want %d", w.Code, http.StatusOK)
		}
	}

	// 3. GET /api/tasks/:id
	{
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/tasks/1", nil)
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("get task code = %d, want %d", w.Code, http.StatusOK)
		}
	}

	// 4. PUT /api/tasks/:id (Update)
	{
		newTitle := "更新后的任务标题"
		body, _ := json.Marshal(UpdateTaskRequest{
			Title: &newTitle,
		})
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("PUT", "/api/tasks/1", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("update task code = %d, want %d", w.Code, http.StatusOK)
		}
	}

	// 5. POST /api/tasks/:id/trigger (Trigger manual execution)
	{
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/tasks/1/trigger", nil)
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("trigger execution code = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
		}
	}

	// 6. GET /api/tasks/:id/runs (List runs)
	{
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/tasks/1/runs", nil)
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("list runs code = %d, want %d", w.Code, http.StatusOK)
		}
		var resp struct {
			Runs []TaskRun `json:"runs"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		if len(resp.Runs) != 1 {
			t.Fatalf("expected 1 run, got %d", len(resp.Runs))
		}
	}

	// 7. GET /api/tasks/session-ids
	{
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/tasks/session-ids?projectId=1", nil)
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("get session ids code = %d, want %d", w.Code, http.StatusOK)
		}
	}

	// 8. DELETE /api/tasks/:id
	{
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("DELETE", "/api/tasks/1", nil)
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusNoContent {
			t.Fatalf("delete task code = %d, want %d", w.Code, http.StatusNoContent)
		}
	}
}
