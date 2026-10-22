package http

import (
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"testing"
)

// TestFetchModelsRouteProbesUpstreamWithCSRF pins the behaviour of the model
// discovery endpoint end to end: it is a POST that carries the service URL and
// key in the body and calls the upstream /models endpoint on the operator's
// behalf, and it is CSRF protected like every other mutation.
//
// This is the regression that started the CSRF hardening: fetchModels reads like
// a query, was written as a POST, and a client forgot the header, so the user
// saw 403 invalid_csrf_token instead of a model list.
func TestFetchModelsRouteProbesUpstreamWithCSRF(t *testing.T) {
	var gotAuth string
	var gotPath string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"m1","name":"Model One"},{"id":"m2"}]}`))
	}))
	defer upstream.Close()

	deps := newFullDependencies(t)
	if err := deps.Auth.SetPassword(fixturePassword); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}
	router := NewRouter(deps)

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar.New: %v", err)
	}
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	client := &http.Client{Jar: jar}
	base := server.URL
	csrf := loginClient(t, client, base)

	request := map[string]string{"baseUrl": upstream.URL + "/v1/", "apiKey": "sk-test"}

	// Without the header the middleware refuses the call before any probe runs.
	rejected := postJSON(t, client, base, "/api/model-services/fetch-models", "", request)
	defer rejected.Body.Close()
	if rejected.StatusCode != http.StatusForbidden {
		t.Fatalf("fetch-models without CSRF status = %d, want %d", rejected.StatusCode, http.StatusForbidden)
	}
	var rejection struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(rejected.Body).Decode(&rejection); err != nil {
		t.Fatalf("decode rejection body: %v", err)
	}
	if rejection.Error != "invalid_csrf_token" {
		t.Fatalf("fetch-models without CSRF error = %q, want invalid_csrf_token", rejection.Error)
	}

	// With the header the server performs the upstream call and normalizes the
	// answer: the trailing slash is trimmed, empty ids are dropped, and the key
	// travels as a bearer token.
	ok := postJSON(t, client, base, "/api/model-services/fetch-models", csrf, request)
	defer ok.Body.Close()
	if ok.StatusCode != http.StatusOK {
		t.Fatalf("fetch-models status = %d, want %d", ok.StatusCode, http.StatusOK)
	}
	var body struct {
		Models []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(ok.Body).Decode(&body); err != nil {
		t.Fatalf("decode fetch-models body: %v", err)
	}
	if len(body.Models) != 2 || body.Models[0].ID != "m1" || body.Models[0].Name != "Model One" || body.Models[1].ID != "m2" {
		t.Fatalf("models = %+v, want m1/Model One and m2", body.Models)
	}
	if gotPath != "/v1/models" {
		t.Errorf("upstream path = %q, want /v1/models", gotPath)
	}
	if gotAuth != "Bearer sk-test" {
		t.Errorf("upstream Authorization = %q, want the probe key", gotAuth)
	}
}

// TestFetchModelsRouteRejectsUnusableServiceURL verifies the probe route keeps
// mapping upstream failures to the documented API error codes rather than
// leaking transport details.
func TestFetchModelsRouteRejectsUnusableServiceURL(t *testing.T) {
	fixture := newRouterFixture(t)

	invalid := postJSON(t, fixture.client, fixture.base, "/api/model-services/fetch-models",
		fixture.csrf, map[string]string{"baseUrl": "not-a-url"})
	defer invalid.Body.Close()
	if invalid.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid base url status = %d, want %d", invalid.StatusCode, http.StatusBadRequest)
	}
	var body struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(invalid.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Error != "invalid_service" {
		t.Errorf("error = %q, want invalid_service", body.Error)
	}
}

// TestTestModelRouteRequiresCSRF mirrors the probe test for the model test
// endpoint, the second call that was missing the header.
func TestTestModelRouteRequiresCSRF(t *testing.T) {
	fixture := newRouterFixture(t)
	request := map[string]string{"baseUrl": "http://127.0.0.1:9/v1", "apiKey": "k", "modelId": "m"}

	rejected := postJSON(t, fixture.client, fixture.base, "/api/models/test", "", request)
	defer rejected.Body.Close()
	if rejected.StatusCode != http.StatusForbidden {
		t.Fatalf("models/test without CSRF status = %d, want %d", rejected.StatusCode, http.StatusForbidden)
	}

	// With the token the call reaches the upstream dial and fails there: a 502
	// upstream_unreachable, never a CSRF rejection.
	upstream := postJSON(t, fixture.client, fixture.base, "/api/models/test", fixture.csrf, request)
	defer upstream.Body.Close()
	if upstream.StatusCode != http.StatusBadGateway {
		t.Fatalf("models/test status = %d, want %d", upstream.StatusCode, http.StatusBadGateway)
	}
	var body struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(upstream.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Error != "upstream_unreachable" {
		t.Errorf("error = %q, want upstream_unreachable", body.Error)
	}
}
