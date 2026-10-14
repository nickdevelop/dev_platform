package http

import (
	"bytes"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// TestAccessLogRecordsMethodPathStatusAndLatency verifies one log line per
// request carries the response time, strips the query string (secret
// redaction), and reports the handler's status.
func TestAccessLogRecordsMethodPathStatusAndLatency(t *testing.T) {
	var buf bytes.Buffer
	logger := log.New(&buf, "", 0)

	router := gin.New()
	router.Use(AccessLogWithLogger(logger))
	router.GET("/api/projects", func(c *gin.Context) {
		time.Sleep(5 * time.Millisecond)
		c.JSON(http.StatusCreated, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodGet, "/api/projects?token=super-secret", nil)
	req.RemoteAddr = "192.0.2.1:1234"
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusCreated)
	}

	line := buf.String()
	for _, want := range []string{"method=GET", "path=/api/projects", "status=201", "latency=", "ip=192.0.2.1"} {
		if !strings.Contains(line, want) {
			t.Errorf("access log = %q, want it to contain %q", line, want)
		}
	}
	if strings.Contains(line, "super-secret") {
		t.Errorf("access log = %q, must not contain the query string", line)
	}
}

// TestAccessLogStillLogsAfterPanic verifies the middleware emits its line
// even when the handler panics (it runs before gin.Recovery).
func TestAccessLogStillLogsAfterPanic(t *testing.T) {
	var buf bytes.Buffer
	logger := log.New(&buf, "", 0)

	// Discard gin's panic stack dump: the assertion is about our access
	// line, not the recovery output.
	prevWriter := gin.DefaultErrorWriter
	gin.DefaultErrorWriter = &bytes.Buffer{}
	defer func() { gin.DefaultErrorWriter = prevWriter }()

	router := gin.New()
	router.Use(AccessLogWithLogger(logger), gin.Recovery())
	router.GET("/panic", func(_ *gin.Context) { panic("boom") })

	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	if got := buf.String(); !strings.Contains(got, "path=/panic") || !strings.Contains(got, "status=500") {
		t.Errorf("access log after panic = %q, want path and 500 status", got)
	}
}

// TestNewRouterLogsEveryAccess verifies the production router wires the
// middleware, so even the public health endpoint records its response time.
func TestNewRouterLogsEveryAccess(t *testing.T) {
	defaultLogger := log.Default()
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(defaultLogger.Writer())

	router := NewRouter(Dependencies{})
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := buf.String(); !strings.Contains(got, "path=/api/health") || !strings.Contains(got, "latency=") {
		t.Errorf("router access log = %q, want health path with latency", got)
	}
}
