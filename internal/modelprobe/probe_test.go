package modelprobe

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchModelsParsesAndSendsAuth(t *testing.T) {
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if r.URL.Path != "/v1/models" {
			t.Errorf("path = %q, want /v1/models", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"m1","name":"Model One"},{"id":"m2"}]}`))
	}))
	defer server.Close()

	models, err := FetchModels(context.Background(), server.URL+"/v1/", "sk-secret")
	if err != nil {
		t.Fatalf("FetchModels: %v", err)
	}
	if len(models) != 2 || models[0].ID != "m1" || models[0].Name != "Model One" || models[1].ID != "m2" {
		t.Fatalf("models = %+v, want m1/Model One and m2", models)
	}
	if gotAuth != "Bearer sk-secret" {
		t.Errorf("Authorization = %q, want Bearer sk-secret", gotAuth)
	}
}

func TestFetchModelsNoKeySendsNoAuth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if auth := r.Header.Get("Authorization"); auth != "" {
			t.Errorf("Authorization = %q, want none", auth)
		}
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer server.Close()

	models, err := FetchModels(context.Background(), server.URL, "")
	if err != nil {
		t.Fatalf("FetchModels: %v", err)
	}
	if len(models) != 0 {
		t.Errorf("models = %+v, want empty", models)
	}
}

func TestFetchModelsRejectsInvalidBaseURL(t *testing.T) {
	for _, base := range []string{"", "ftp://host", "not a url", "relative/path"} {
		if _, err := FetchModels(context.Background(), base, ""); !errors.Is(err, ErrInvalidBaseURL) {
			t.Errorf("FetchModels(%q) error = %v, want ErrInvalidBaseURL", base, err)
		}
	}
}

func TestTestModelSuccess(t *testing.T) {
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("path = %q, want /v1/chat/completions", r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"pong"}}]}`))
	}))
	defer server.Close()

	result, err := TestModel(context.Background(), server.URL+"/v1", "k", "gpt-4o")
	if err != nil {
		t.Fatalf("TestModel: %v", err)
	}
	if result.ModelID != "gpt-4o" {
		t.Errorf("ModelID = %q, want gpt-4o", result.ModelID)
	}
	if body["model"] != "gpt-4o" || body["max_tokens"] != float64(1) {
		t.Errorf("request body = %+v, want model gpt-4o and max_tokens 1", body)
	}
}

func TestTestModelUpstreamErrorDoesNotLeakKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"bad key sk-super-secret"}}`))
	}))
	defer server.Close()

	_, err := TestModel(context.Background(), server.URL, "sk-super-secret", "m")
	if !errors.Is(err, ErrUpstreamError) {
		t.Fatalf("error = %v, want ErrUpstreamError", err)
	}
	if strings.Contains(err.Error(), "sk-super-secret") {
		t.Errorf("error %q leaks the API key", err)
	}
}

func TestTestModelUnreachable(t *testing.T) {
	_, err := TestModel(context.Background(), "http://127.0.0.1:1", "", "m")
	if !errors.Is(err, ErrUpstreamUnreachable) {
		t.Fatalf("error = %v, want ErrUpstreamUnreachable", err)
	}
}
