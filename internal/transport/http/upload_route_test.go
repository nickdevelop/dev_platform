package http

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"
)

// TestFileUploadRouteAcceptsLargeBody pins the project file upload route to the
// large file request-body cap (1 GiB) instead of the generic 1 MiB API cap.
// A file payload larger than 1 MiB must reach the project upload handler (which
// then reports project_not_found for an unlisted project) rather than being
// truncated by http.MaxBytesReader and returning 400 invalid_upload_payload.
func TestFileUploadRouteAcceptsLargeBody(t *testing.T) {
	fixture := newRouterFixture(t)

	// Create a multipart form with a 2 MiB file (comfortably above generic 1 MiB cap).
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "large_asset.bin")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}

	largeContent := strings.Repeat("X", 2<<20)
	if _, err := part.Write([]byte(largeContent)); err != nil {
		t.Fatalf("write form file content: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, fixture.base+"/api/projects/999/files/upload", body)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-CSRF-Token", fixture.csrf)

	resp, err := fixture.client.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want %d (body parsed before project lookup)", resp.StatusCode, http.StatusNotFound)
	}

	var envelope struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode error envelope: %v", err)
	}
	if envelope.Error == "invalid_upload_payload" {
		t.Fatalf("file body was rejected by generic request-body cap before reaching project handler")
	}
	if envelope.Error != "project_not_found" {
		t.Fatalf("unexpected error response: %q, want 'project_not_found'", envelope.Error)
	}
}
