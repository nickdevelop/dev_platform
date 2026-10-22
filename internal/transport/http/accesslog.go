// Access logging for the platform HTTP service.
//
// Every request is logged with its response time so slow or stuck
// operations (large latency values) can be spotted in stdout/stderr or
// journald. Only the URL path is logged, never the query string, request
// body, or headers, so tokens and secrets cannot leak into logs.
package http

import (
	"log"
	"time"

	"github.com/gin-gonic/gin"

	"devplatform/internal/app/auth"
)

// AccessLog returns the production access-log middleware that writes to the
// standard logger. It must run before gin.Recovery so a panicking handler
// still produces one log line with its 500 status.
func AccessLog() gin.HandlerFunc {
	return AccessLogWithLogger(log.Default())
}

// AccessLogWithLogger is the same middleware with an injectable logger so
// tests can capture output without touching global state.
func AccessLogWithLogger(logger *log.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		method := c.Request.Method
		path := "/"
		if c.Request.URL != nil && c.Request.URL.Path != "" {
			path = c.Request.URL.Path
		}
		// No proxy headers are trusted (see NewRouter), so this is the
		// connection's remote address.
		clientIP := c.ClientIP()

		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()
		logger.Printf("http access method=%s path=%s status=%d latency=%s ip=%s%s",
			method, path, status, latency.String(), clientIP, csrfRejectionField(c))
	}
}

// csrfRejectionField appends a marker to the access line when the request was
// refused by the CSRF middleware. A client that drops the double-submit header
// shows up here as a stream of rejections, which is otherwise invisible: the
// browser only reports a 403 to the user.
func csrfRejectionField(c *gin.Context) string {
	if rejected, ok := c.Get(auth.CSRFRejectedKey); ok {
		if flag, isBool := rejected.(bool); isBool && flag {
			return " csrf=rejected"
		}
	}
	return ""
}
