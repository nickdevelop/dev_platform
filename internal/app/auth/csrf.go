package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// Cookie and header names used by the browser-facing API.
const (
	// SessionCookieName is the HttpOnly session cookie.
	SessionCookieName = "platform_session"
	// CSRFCookieName is the JS-readable double-submit CSRF cookie.
	CSRFCookieName = "platform_csrf"
	// CSRFHeaderName is the request header that must echo the CSRF cookie.
	CSRFHeaderName = "X-CSRF-Token"
)

// csrfTokenBytes is the amount of entropy in a CSRF token.
const csrfTokenBytes = 32

// newCSRFToken returns a cryptographically random, URL-safe CSRF token.
func newCSRFToken() (string, error) {
	buf := make([]byte, csrfTokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("auth: generate csrf token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// csrfTokensEqual compares tokens in constant time.
func csrfTokensEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// nextCSRFToken returns the current CSRF token from the request cookie, or
// issues and sets a fresh one. The cookie is intentionally readable by the
// same-origin Vue app (not HttpOnly) so it can echo the value in the
// X-CSRF-Token header.
func (s *Service) nextCSRFToken(c *gin.Context) string {
	if token, err := c.Cookie(CSRFCookieName); err == nil && token != "" {
		return token
	}
	token, err := newCSRFToken()
	if err != nil {
		// Fail closed: without a token the client cannot pass CSRF checks.
		return ""
	}
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     CSRFCookieName,
		Value:    token,
		Path:     "/",
		SameSite: http.SameSiteStrictMode,
		Expires:  s.now().UTC().Add(s.sessionTTL),
		MaxAge:   maxAgeSeconds(s.sessionTTL),
	})
	return token
}

// maxAgeSeconds converts a cookie lifetime to a positive Max-Age in seconds.
func maxAgeSeconds(d time.Duration) int {
	seconds := int(d.Seconds())
	if seconds < 1 {
		return 1
	}
	return seconds
}

// RequireCSRF enforces the double-submit CSRF scheme on mutating requests:
// the X-CSRF-Token header must match the CSRF cookie in constant time. Safe
// methods (GET, HEAD, OPTIONS) pass through.
func RequireCSRF() gin.HandlerFunc {
	return func(c *gin.Context) {
		switch c.Request.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			c.Next()
			return
		}

		cookie, err := c.Cookie(CSRFCookieName)
		header := c.GetHeader(CSRFHeaderName)
		if err != nil || cookie == "" || header == "" || !csrfTokensEqual(header, cookie) {
			abortWithError(c, http.StatusForbidden, "invalid_csrf_token")
			return
		}
		c.Next()
	}
}
