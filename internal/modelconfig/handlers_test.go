package modelconfig

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func newHandlerRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	svc, _ := newService(t)
	router := gin.New()
	svc.RegisterRoutes(router.Group("/api"))
	return router
}

func doJSON(t *testing.T, router *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	return rec
}

func decodeService(t *testing.T, rec *httptest.ResponseRecorder) ServiceDTO {
	t.Helper()
	var dto ServiceDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &dto); err != nil {
		t.Fatalf("decode service: %v (body %s)", err, rec.Body.String())
	}
	return dto
}

func TestHandlerServiceLifecycle(t *testing.T) {
	router := newHandlerRouter(t)

	create := doJSON(t, router, http.MethodPost, "/api/model-services",
		`{"name":"openai","baseUrl":"https://api.openai.com/v1","apiKey":"sk-x","enabled":true}`)
	if create.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body %s", create.Code, create.Body.String())
	}
	created := decodeService(t, create)
	if !created.HasKey || created.SecretRef == "" {
		t.Errorf("created = %+v, want a key reference", created)
	}

	add := doJSON(t, router, http.MethodPost, "/api/model-services/1/models",
		`{"modelId":"gpt-4o","name":"GPT-4o","supportsText":true,"contextTokens":128000,"outputTokens":4096,"enabled":true}`)
	if add.Code != http.StatusCreated {
		t.Fatalf("add model status = %d, body %s", add.Code, add.Body.String())
	}

	list := doJSON(t, router, http.MethodGet, "/api/model-services", "")
	if list.Code != http.StatusOK {
		t.Fatalf("list status = %d", list.Code)
	}
	var envelope struct {
		Services []ServiceDTO `json:"services"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(envelope.Services) != 1 || len(envelope.Services[0].Models) != 1 {
		t.Fatalf("list = %+v, want one service with one model", envelope.Services)
	}
	if !envelope.Services[0].Models[0].Usable {
		t.Error("model should be usable")
	}

	del := doJSON(t, router, http.MethodDelete, "/api/model-services/1", "")
	if del.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d", del.Code)
	}
}

func TestHandlerErrorCodes(t *testing.T) {
	router := newHandlerRouter(t)

	invalid := doJSON(t, router, http.MethodPost, "/api/model-services", `{"name":"","baseUrl":"ftp://x"}`)
	if invalid.Code != http.StatusBadRequest {
		t.Errorf("invalid service status = %d, want 400", invalid.Code)
	}
	missing := doJSON(t, router, http.MethodPut, "/api/model-services/99", `{"name":"x","baseUrl":"https://x.example"}`)
	if missing.Code != http.StatusNotFound {
		t.Errorf("missing service status = %d, want 404", missing.Code)
	}
}

func TestHandlerFetchModelsUsesFormConfig(t *testing.T) {
	router := newHandlerRouter(t)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer form-key" {
			t.Errorf("Authorization = %q, want Bearer form-key", r.Header.Get("Authorization"))
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"a"},{"id":"b"}]}`))
	}))
	defer upstream.Close()

	rec := doJSON(t, router, http.MethodPost, "/api/model-services/fetch-models",
		`{"baseUrl":"`+upstream.URL+`","apiKey":"form-key"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("fetch status = %d, body %s", rec.Code, rec.Body.String())
	}
	var envelope struct {
		Models []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode fetch: %v", err)
	}
	if len(envelope.Models) != 2 {
		t.Errorf("models = %+v, want two", envelope.Models)
	}
}

func TestHandlerTestModel(t *testing.T) {
	router := newHandlerRouter(t)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[]}`))
	}))
	defer upstream.Close()

	rec := doJSON(t, router, http.MethodPost, "/api/models/test",
		`{"baseUrl":"`+upstream.URL+`","apiKey":"k","modelId":"m"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("test status = %d, body %s", rec.Code, rec.Body.String())
	}
	var result struct {
		ModelID   string `json:"modelId"`
		LatencyMs int64  `json:"latencyMs"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if result.ModelID != "m" {
		t.Errorf("result = %+v, want model m", result)
	}
}
