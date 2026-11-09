package http

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// testJSONBody is large enough to be worth compressing and easy to compare
// byte-for-byte after decompression.
func testJSONBody() string {
	return `{"text":"` + strings.Repeat("压缩测试内容", 300) + `"}`
}

func gzipTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(Compress())
	return router
}

func getWithEncoding(t *testing.T, router *gin.Engine, target, acceptEncoding string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	if acceptEncoding != "" {
		req.Header.Set("Accept-Encoding", acceptEncoding)
	}
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func decodeGzip(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	reader, err := gzip.NewReader(bytes.NewReader(rec.Body.Bytes()))
	if err != nil {
		t.Fatalf("gzip.NewReader: %v (headers: %v)", err, rec.Header())
	}
	defer reader.Close()
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read gzip body: %v", err)
	}
	return string(data)
}

func TestCompressJSONWhenAccepted(t *testing.T) {
	body := testJSONBody()
	router := gzipTestRouter()
	router.GET("/json", func(c *gin.Context) {
		c.Data(http.StatusOK, "application/json; charset=utf-8", []byte(body))
	})

	rec := getWithEncoding(t, router, "/json", "gzip, deflate, br", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("Content-Encoding = %q, want gzip", got)
	}
	if got := strings.Join(rec.Header().Values("Vary"), ","); !strings.Contains(got, "Accept-Encoding") {
		t.Errorf("Vary = %q, want Accept-Encoding", got)
	}
	if got := rec.Header().Get("Content-Length"); got != "" {
		t.Errorf("Content-Length = %q, want it removed for a compressed body", got)
	}
	if got := decodeGzip(t, rec); got != body {
		t.Errorf("decompressed body does not match; got %d bytes, want %d", len(got), len(body))
	}
	if rec.Body.Len() >= len(body) {
		t.Errorf("compressed body = %d bytes, plain = %d; compression did not reduce size", rec.Body.Len(), len(body))
	}
}

func TestCompressNegotiation(t *testing.T) {
	body := testJSONBody()
	router := gzipTestRouter()
	router.GET("/json", func(c *gin.Context) {
		c.Data(http.StatusOK, "application/json", []byte(body))
	})

	cases := []struct {
		name           string
		acceptEncoding string
		wantGzip       bool
	}{
		{name: "absent", acceptEncoding: ""},
		{name: "explicit gzip", acceptEncoding: "gzip", wantGzip: true},
		{name: "uppercase", acceptEncoding: "GZIP", wantGzip: true},
		{name: "weighted", acceptEncoding: "br, gzip;q=0.8", wantGzip: true},
		{name: "rejected", acceptEncoding: "gzip;q=0"},
		{name: "wildcard", acceptEncoding: "*", wantGzip: true},
		{name: "wildcard rejected", acceptEncoding: "*;q=0"},
		{name: "br only", acceptEncoding: "br"},
		{name: "explicit rejection beats wildcard", acceptEncoding: "gzip;q=0, *;q=1"},
		{name: "identity only", acceptEncoding: "identity"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := getWithEncoding(t, router, "/json", tc.acceptEncoding, nil)
			if tc.wantGzip {
				if got := rec.Header().Get("Content-Encoding"); got != "gzip" {
					t.Fatalf("Content-Encoding = %q, want gzip", got)
				}
				if got := decodeGzip(t, rec); got != body {
					t.Errorf("decompressed body = %q, want the plain payload", got)
				}
				return
			}
			if got := rec.Header().Get("Content-Encoding"); got != "" {
				t.Fatalf("Content-Encoding = %q, want identity", got)
			}
			if got := rec.Body.String(); got != body {
				t.Errorf("body = %q, want the plain payload", got)
			}
		})
	}
}

func TestCompressSkipsEventStream(t *testing.T) {
	router := gzipTestRouter()
	router.GET("/sse", func(c *gin.Context) {
		c.Header("Content-Type", "text/event-stream")
		c.Status(http.StatusOK)
		c.Writer.Flush()
		_, _ = c.Writer.WriteString("event: message\ndata: one\n\n")
		c.Writer.Flush()
	})

	rec := getWithEncoding(t, router, "/sse", "gzip", nil)
	if got := rec.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("SSE Content-Encoding = %q, want identity", got)
	}
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/event-stream") {
		t.Fatalf("SSE Content-Type = %q", got)
	}
	if got := rec.Body.String(); !strings.Contains(got, "data: one") {
		t.Errorf("SSE body = %q, want the flushed frame", got)
	}
}

func TestCompressSkipsWebSocketUpgrade(t *testing.T) {
	router := gzipTestRouter()
	router.GET("/ws", func(c *gin.Context) {
		c.Data(http.StatusOK, "application/json", []byte(testJSONBody()))
	})

	rec := getWithEncoding(t, router, "/ws", "gzip", map[string]string{
		"Upgrade":    "websocket",
		"Connection": "Upgrade",
	})
	if got := rec.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("upgrade Content-Encoding = %q, want identity", got)
	}
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
		t.Fatalf("upgrade Content-Type = %q", got)
	}
}

func TestCompressSkipsRangeRequests(t *testing.T) {
	router := gzipTestRouter()
	router.GET("/range", func(c *gin.Context) {
		c.Data(http.StatusOK, "application/json", []byte(testJSONBody()))
	})

	rec := getWithEncoding(t, router, "/range", "gzip", map[string]string{"Range": "bytes=0-15"})
	if got := rec.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("range Content-Encoding = %q, want identity", got)
	}
}

func TestCompressSkipsPreEncodedResponses(t *testing.T) {
	router := gzipTestRouter()
	router.GET("/encoded", func(c *gin.Context) {
		c.Header("Content-Encoding", "br")
		c.Data(http.StatusOK, "application/json", []byte(testJSONBody()))
	})

	rec := getWithEncoding(t, router, "/encoded", "gzip", nil)
	if got := rec.Header().Get("Content-Encoding"); got != "br" {
		t.Fatalf("Content-Encoding = %q, want the upstream br", got)
	}
}

func TestCompressSkipsEmptyResponses(t *testing.T) {
	router := gzipTestRouter()
	router.GET("/empty", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	rec := getWithEncoding(t, router, "/empty", "gzip", nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	if got := rec.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("204 Content-Encoding = %q, want identity", got)
	}
	if rec.Body.Len() != 0 {
		t.Errorf("204 body = %q, want empty", rec.Body.String())
	}
}

func TestCompressPreservesExistingVary(t *testing.T) {
	router := gzipTestRouter()
	router.GET("/json", func(c *gin.Context) {
		c.Header("Vary", "Origin")
		c.Data(http.StatusOK, "application/json", []byte(testJSONBody()))
	})

	rec := getWithEncoding(t, router, "/json", "gzip", nil)
	vary := strings.Join(rec.Header().Values("Vary"), ",")
	if !strings.Contains(vary, "Origin") || !strings.Contains(vary, "Accept-Encoding") {
		t.Fatalf("Vary = %q, want Origin and Accept-Encoding", vary)
	}
}

func TestCompressSkipsSmallKnownContentLength(t *testing.T) {
	router := gzipTestRouter()
	router.GET("/small", func(c *gin.Context) {
		c.Header("Content-Length", "12")
		c.Data(http.StatusOK, "application/json", []byte(`{"ok":true}`))
	})

	rec := getWithEncoding(t, router, "/small", "gzip", nil)
	if got := rec.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("small Content-Encoding = %q, want identity", got)
	}
}

func TestCompressHealthThroughProductionRouter(t *testing.T) {
	router := NewRouter(Dependencies{})
	rec := getWithEncoding(t, router, "/api/health", "gzip", nil)
	if got := rec.Header().Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("health Content-Encoding = %q, want gzip", got)
	}
	if got := decodeGzip(t, rec); got != `{"status":"ok"}` {
		t.Errorf("health body = %q", got)
	}
}

func TestCompressStaticAssets(t *testing.T) {
	webDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(webDir, "assets"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	largeJS := strings.Repeat("console.log('large bundle');\n", 100)
	smallJS := "console.log('small');"
	largePNG := strings.Repeat("\x89PNG\r\n", 500)
	largeFont := strings.Repeat("font-slice-", 300)
	files := map[string]string{
		"assets/large.js":   largeJS,
		"assets/small.js":   smallJS,
		"assets/large.png":  largePNG,
		"assets/font.woff2": largeFont,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(webDir, filepath.FromSlash(name)), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	router := NewRouter(Dependencies{WebDir: webDir})

	rec := getWithEncoding(t, router, "/assets/large.js", "gzip", nil)
	if got := rec.Header().Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("large js Content-Encoding = %q, want gzip", got)
	}
	if got := rec.Header().Get("Content-Length"); got != "" {
		t.Errorf("large js Content-Length = %q, want removed", got)
	}
	if got := rec.Header().Get("Accept-Ranges"); got != "" {
		t.Errorf("large js Accept-Ranges = %q, want removed", got)
	}
	if got := decodeGzip(t, rec); got != largeJS {
		t.Errorf("large js decompressed body mismatch: got %d bytes, want %d", len(got), len(largeJS))
	}

	small := getWithEncoding(t, router, "/assets/small.js", "gzip", nil)
	if got := small.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("small js Content-Encoding = %q, want identity", got)
	}
	if got := small.Body.String(); got != smallJS {
		t.Errorf("small js body = %q", got)
	}

	png := getWithEncoding(t, router, "/assets/large.png", "gzip", nil)
	if got := png.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("png Content-Encoding = %q, want identity", got)
	}
	if got := png.Body.String(); got != largePNG {
		t.Errorf("png body changed")
	}

	font := getWithEncoding(t, router, "/assets/font.woff2", "gzip", nil)
	if got := font.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("font Content-Encoding = %q, want identity", got)
	}

	rangeRec := getWithEncoding(t, router, "/assets/large.js", "gzip", map[string]string{"Range": "bytes=0-9"})
	if rangeRec.Code != http.StatusPartialContent {
		t.Fatalf("range status = %d, want 206", rangeRec.Code)
	}
	if got := rangeRec.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("range Content-Encoding = %q, want identity", got)
	}
	if got := rangeRec.Body.String(); got != largeJS[:10] {
		t.Errorf("range body = %q, want %q", got, largeJS[:10])
	}
	if got := rangeRec.Header().Get("Content-Range"); got == "" {
		t.Errorf("range Content-Range is empty")
	}
}

func TestCompressibleContentType(t *testing.T) {
	cases := []struct {
		value string
		want  bool
	}{
		{"application/json; charset=utf-8", true},
		{"application/problem+json", true},
		{"text/html; charset=utf-8", true},
		{"text/css", true},
		{"text/javascript", true},
		{"application/javascript", true},
		{"image/svg+xml", true},
		{"application/xml", true},
		{"text/event-stream", false},
		{"image/png", false},
		{"font/woff2", false},
		{"application/octet-stream", false},
		{"", false},
		{"not a media type", false},
	}
	for _, tc := range cases {
		if got := compressibleContentType(tc.value); got != tc.want {
			t.Errorf("compressibleContentType(%q) = %v, want %v", tc.value, got, tc.want)
		}
	}
}

func TestAcceptsGzip(t *testing.T) {
	cases := []struct {
		header string
		want   bool
	}{
		{"", false},
		{"gzip", true},
		{"GZip", true},
		{"deflate, gzip", true},
		{"gzip;q=0.5", true},
		{"gzip;q=0", false},
		{"br", false},
		{"*", true},
		{"*;q=0", false},
		{"gzip;q=0, *;q=1", false},
		{"*;q=1, gzip;q=0.2", true},
	}
	for _, tc := range cases {
		if got := acceptsGzip(tc.header); got != tc.want {
			t.Errorf("acceptsGzip(%q) = %v, want %v", tc.header, got, tc.want)
		}
	}
}
