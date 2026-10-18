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

	// 1. GET /api/tasks/flows
	{
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/tasks/flows", nil)
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("flows code = %d, want %d", w.Code, http.StatusOK)
		}
	}

	// 2. POST /api/tasks (Create)
	var created Task
	{
		body, _ := json.Marshal(CreateTaskRequest{
			Title:       "HTTP 任务测试",
			Description: "测试用例",
			ProjectID:   1,
			Backend:     agents.BackendOpenCodeV2,
			Model:       "claude-3-5-sonnet",
			FlowType:    FlowTransactional,
		})
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/tasks", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("create task code = %d, want %d, body: %s", w.Code, http.StatusCreated, w.Body.String())
		}
		_ = json.Unmarshal(w.Body.Bytes(), &created)
	}

	// 3. GET /api/tasks
	{
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/tasks", nil)
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("list tasks code = %d, want %d", w.Code, http.StatusOK)
		}
	}

	// 4. PUT /api/tasks/:id/schedule (Should fail before final step)
	{
		future := time.Now().Add(1 * time.Hour)
		body, _ := json.Marshal(ScheduleConfigRequest{
			ScheduleType:    ScheduleOnce,
			ScheduledAt:     &future,
			ScheduleEnabled: true,
		})
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("PUT", "/api/tasks/1/schedule", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusConflict {
			t.Fatalf("expected 409 conflict, got %d", w.Code)
		}
	}

	// 5. POST /api/tasks/:id/advance (Advance to step 2 with confirmed = true)
	{
		body, _ := json.Marshal(AdvanceStepRequest{
			Confirmed: true,
			Notes:     "确认完成",
		})
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/tasks/1/advance", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("advance task code = %d, want %d", w.Code, http.StatusOK)
		}
	}

	// 6. PUT /api/tasks/:id/execution-prompt
	{
		body, _ := json.Marshal(saveExecutionPromptRequest{
			ExecutionPrompt: "执行方案：直接构建镜像并部署",
		})
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("PUT", "/api/tasks/1/execution-prompt", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("save execution prompt code = %d, want %d", w.Code, http.StatusOK)
		}
	}

	// 7. POST /api/tasks/:id/trigger (Trigger execution)
	{
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/tasks/1/trigger", nil)
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("trigger code = %d, want %d", w.Code, http.StatusOK)
		}
	}

	// 8. GET /api/tasks/:id/runs
	{
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/tasks/1/runs", nil)
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("list runs code = %d, want %d", w.Code, http.StatusOK)
		}
	}

	// 9. GET /api/tasks/session-ids
	{
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/tasks/session-ids?projectId=1", nil)
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("get session ids code = %d, want %d", w.Code, http.StatusOK)
		}
	}
}
