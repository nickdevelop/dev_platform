package http

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// TestSessionMessageRouteAcceptsImageSizedBody pins the session write routes
// to the same image-sized body cap as pure chat. An image turn base64-expands
// past the generic 1 MiB API cap, so the request must reach the session
// handler (which then reports session_not_found) instead of failing in the
// JSON binder with invalid_request.
func TestSessionMessageRouteAcceptsImageSizedBody(t *testing.T) {
	fixture := newRouterFixture(t)

	// 2 MiB of base64 is comfortably above the generic cap but well inside one
	// 5 MiB image's encoded size.
	imageData := strings.Repeat("A", 2<<20)
	resp := postJSON(t, fixture.client, fixture.base,
		"/api/projects/999/sessions/missing/messages", fixture.csrf,
		map[string]any{
			"text": "",
			"images": []map[string]any{{
				"id":        "img-1",
				"name":      "shot.png",
				"mediaType": "image/png",
				"data":      imageData,
				"size":      int64(1500000),
			}},
		})
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want %d (body parsed before session lookup)", resp.StatusCode, http.StatusNotFound)
	}
	var envelope struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode error envelope: %v", err)
	}
	if envelope.Error == "invalid_request" {
		t.Fatalf("image body was rejected by the request-body cap before reaching the handler")
	}
}
