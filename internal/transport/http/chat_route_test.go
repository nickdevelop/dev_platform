package http

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

// TestChatRouteAcceptsImageSizedBody pins the one deliberate body-limit
// difference: chat messages may carry up to four 5 MiB images, while every
// other API route keeps the 1 MiB cap. The request here is larger than the
// normal cap but reaches the chat handler, which reports a business error
// (model_unavailable) instead of the binder's invalid_request.
func TestChatRouteAcceptsImageSizedBody(t *testing.T) {
	fixture := newRouterFixture(t)

	payload, err := json.Marshal(map[string]any{
		"modelId": 1,
		"text":    strings.Repeat("a", maxRequestBodyBytes+1024),
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, fixture.base+"/api/chats", strings.NewReader(string(payload)))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", fixture.csrf)
	resp, err := fixture.client.Do(req)
	if err != nil {
		t.Fatalf("POST /api/chats: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", resp.StatusCode, body)
	}
	var envelope struct {
		Error string `json:"error"`
	}
	_ = json.Unmarshal(body, &envelope)
	if envelope.Error != "model_unavailable" {
		t.Fatalf("error = %q, want model_unavailable (body was parsed)", envelope.Error)
	}
}
