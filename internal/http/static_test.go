package http

// Tests for production static hosting: the built Vue assets are served from
// the Go service so the UI and the API share one origin, client-side routes
// fall back to the SPA entry, /api routes are never shadowed, and the file
// serving cannot escape the configured web directory.

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"devplatform/internal/auth"
	"devplatform/internal/projects"
)

// writeWebDist creates a minimal built-frontend layout: the SPA entry plus a
// hashed Vite-style asset bundle.
func writeWebDist(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "assets"), 0o755); err != nil {
		t.Fatalf("MkdirAll assets: %v", err)
	}
	files := map[string]string{
		"index.html":           "<!doctype html><html><body><div id=\"app\">SPA-ENTRY-MARKER</div></body></html>",
		"favicon.svg":          "<svg xmlns=\"http://www.w3.org/2000/svg\"></svg>",
		"assets/app-abc123.js": "console.log(\"bundled\");",
		// The built-in font slices land here too.
		"assets/font-abc123.woff2": "woff2-slice",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(name)), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return dir
}

// get performs a GET against a test server built from deps.
func get(t *testing.T, deps Dependencies, target string) *http.Response {
	t.Helper()
	server := httptest.NewServer(NewRouter(deps))
	t.Cleanup(server.Close)

	resp, err := http.Get(server.URL + target)
	if err != nil {
		t.Fatalf("GET %s: %v", target, err)
	}
	return resp
}

// body reads and closes a response, returning its text.
func body(t *testing.T, resp *http.Response) string {
	t.Helper()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	resp.Body.Close()
	return string(data)
}

// TestStaticServesEntryAssetsAndSPAFallback verifies the production serving
// contract: / and unknown client-side routes return the SPA entry, real
// files are served, and a missing hashed asset is a 404 rather than HTML.
func TestStaticServesEntryAssetsAndSPAFallback(t *testing.T) {
	deps := Dependencies{WebDir: writeWebDist(t)}

	// The SPA entry is served at /.
	entry := get(t, deps, "/")
	if entry.StatusCode != http.StatusOK {
		t.Errorf("GET / status = %d, want %d", entry.StatusCode, http.StatusOK)
	}
	if ct := entry.Header.Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Errorf("GET / content type = %q, want text/html", ct)
	}
	// The entry must be revalidated so a redeploy is picked up.
	if cc := entry.Header.Get("Cache-Control"); !strings.Contains(cc, "no-cache") {
		t.Errorf("GET / cache control = %q, want it to contain no-cache", cc)
	}
	if got := body(t, entry); !strings.Contains(got, "SPA-ENTRY-MARKER") {
		t.Errorf("GET / body does not contain the SPA entry marker: %q", got)
	}

	// A client-side route falls back to the SPA entry.
	fallback := get(t, deps, "/login")
	if fallback.StatusCode != http.StatusOK {
		t.Errorf("GET /login status = %d, want %d", fallback.StatusCode, http.StatusOK)
	}
	if got := body(t, fallback); !strings.Contains(got, "SPA-ENTRY-MARKER") {
		t.Errorf("GET /login body is not the SPA entry: %q", got)
	}

	// Real files are served from the web directory.
	asset := get(t, deps, "/assets/app-abc123.js")
	if asset.StatusCode != http.StatusOK {
		t.Errorf("GET asset status = %d, want %d", asset.StatusCode, http.StatusOK)
	}
	if ct := asset.Header.Get("Content-Type"); !strings.Contains(ct, "javascript") {
		t.Errorf("GET asset content type = %q, want javascript", ct)
	}
	if cc := asset.Header.Get("Cache-Control"); !strings.Contains(cc, "immutable") {
		t.Errorf("GET asset cache control = %q, want it to contain immutable", cc)
	}
	if got := body(t, asset); !strings.Contains(got, "bundled") {
		t.Errorf("GET asset body = %q, want the bundle content", got)
	}

	// The favicon is served without a fallback round trip.
	icon := get(t, deps, "/favicon.svg")
	if icon.StatusCode != http.StatusOK {
		t.Errorf("GET /favicon.svg status = %d, want %d", icon.StatusCode, http.StatusOK)
	}
	body(t, icon)

	// A missing hashed asset must 404, not return HTML.
	missing := get(t, deps, "/assets/missing-def456.js")
	if missing.StatusCode != http.StatusNotFound {
		t.Errorf("GET missing asset status = %d, want %d", missing.StatusCode, http.StatusNotFound)
	}
	body(t, missing)

	// The assets directory itself must not expose an HTTP directory listing
	// of the built filenames.
	listing := get(t, deps, "/assets/")
	if listing.StatusCode != http.StatusNotFound {
		t.Errorf("GET /assets/ status = %d, want %d (no directory listing)", listing.StatusCode, http.StatusNotFound)
	}
	if got := body(t, listing); strings.Contains(got, "app-abc123.js") {
		t.Errorf("GET /assets/ leaked build filenames: %q", got)
	}
}

// TestStaticServesFontsAsWOFF2 pins the media type of the bundled font slices:
// Go's built-in table maps .woff2 to font/woff, and a wrong type is the kind of
// thing that makes a browser refuse a face.
func TestStaticServesFontsAsWOFF2(t *testing.T) {
	resp := get(t, Dependencies{WebDir: writeWebDist(t)}, "/assets/font-abc123.woff2")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET font status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "font/woff2" {
		t.Errorf("GET font content type = %q, want font/woff2", ct)
	}
	if got := body(t, resp); got != "woff2-slice" {
		t.Errorf("GET font body = %q, want the slice content", got)
	}
}

// TestStaticDoesNotShadowAPIRoutes verifies that enabling static hosting
// leaves every /api route working and keeps unknown API paths JSON 404s
// instead of the SPA entry.
func TestStaticDoesNotShadowAPIRoutes(t *testing.T) {
	db := openTestDB(t)
	authSvc := auth.NewService(db)
	if err := authSvc.SetPassword("correct horse battery staple"); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}
	deps := Dependencies{
		Auth:     authSvc,
		Projects: projects.NewService(db),
		WebDir:   writeWebDist(t),
	}

	health := get(t, deps, "/api/health")
	if health.StatusCode != http.StatusOK {
		t.Errorf("GET /api/health status = %d, want %d", health.StatusCode, http.StatusOK)
	}
	body(t, health)

	// Management APIs still require a session when static hosting is on.
	unauth := get(t, deps, "/api/projects")
	if unauth.StatusCode != http.StatusUnauthorized {
		t.Errorf("unauthenticated GET /api/projects status = %d, want %d", unauth.StatusCode, http.StatusUnauthorized)
	}
	body(t, unauth)

	// Unknown API paths are JSON 404s, never the SPA entry.
	unknown := get(t, deps, "/api/does-not-exist")
	if unknown.StatusCode != http.StatusNotFound {
		t.Errorf("GET unknown api path status = %d, want %d", unknown.StatusCode, http.StatusNotFound)
	}
	var envelope struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(unknown.Body).Decode(&envelope); err != nil {
		t.Errorf("unknown api path did not return a JSON error envelope: %v", err)
	}
	unknown.Body.Close()
	if envelope.Error == "" {
		t.Errorf("unknown api path JSON error envelope is empty")
	}
}

// TestStaticServingCannotEscapeWebDir verifies that traversal-style requests
// (raw and URL-encoded) never serve files outside the configured web
// directory.
func TestStaticServingCannotEscapeWebDir(t *testing.T) {
	webDir := writeWebDist(t)
	deps := Dependencies{WebDir: webDir}

	// A secret one level above the web directory must be unreachable.
	outside := filepath.Join(webDir, "..", "platform-secret.txt")
	if err := os.WriteFile(outside, []byte("TOP-SECRET-CONTENT"), 0o644); err != nil {
		t.Fatalf("write outside file: %v", err)
	}

	for _, target := range []string{"/../platform-secret.txt", "/%2e%2e/platform-secret.txt", "/../../platform-secret.txt"} {
		resp := get(t, deps, target)
		got := body(t, resp)
		if strings.Contains(got, "TOP-SECRET-CONTENT") {
			t.Errorf("GET %s served a file outside the web directory: %q", target, got)
		}
	}
}
