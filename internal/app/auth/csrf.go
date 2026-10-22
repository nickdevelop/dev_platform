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

// CSRFRejectedKey is the Gin context key RequireCSRF sets on a rejected
// request. Middleware that runs before it (the access log) reads the flag to
// record the rejection, which is how a client systematically dropping the
// header becomes visible in the logs instead of only in a user's browser.
const CSRFRejectedKey = "auth.csrf_rejected"

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
	return s.issueCSRFToken(c)
}

// issueCSRFToken always mints a new token and sets it as the cookie, discarding
// whatever the request presented. Login and logout use it to rotate the token
// at the two points where the session identity changes, so a token observed
// before that change cannot be replayed after it.
func (s *Service) issueCSRFToken(c *gin.Context) string {
	token, err := newCSRFToken()
	if err != nil {
		// Fail closed: without a token the client cannot pass CSRF checks.
		return ""
	}
	s.setCSRFCookie(c, token)
	return token
}

// setCSRFCookie writes the double-submit cookie. It is not HttpOnly by design:
// the same-origin app echoes the value in X-CSRF-Token (through the transport,
// see web/src/api/client.ts).
func (s *Service) setCSRFCookie(c *gin.Context, token string) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     CSRFCookieName,
		Value:    token,
		Path:     "/",
		SameSite: http.SameSiteStrictMode,
		Expires:  s.now().UTC().Add(s.sessionTTL),
		MaxAge:   maxAgeSeconds(s.sessionTTL),
	})
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
			c.Set(CSRFRejectedKey, true)
			abortWithError(c, http.StatusForbidden, "invalid_csrf_token")
			return
		}
		c.Next()
	}
}
