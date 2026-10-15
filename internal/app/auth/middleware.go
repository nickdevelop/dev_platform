package auth

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// sessionContextKey is the Gin context key under which the validated Session
// is stored by RequireSession.
const sessionContextKey = "auth.session"

// setSessionCookie writes the HttpOnly, SameSite=Strict, expiring session
// cookie. Secure is left off because the platform is served over the LAN on
// plain HTTP; deployments that terminate TLS can enable it in a later task.
func setSessionCookie(c *gin.Context, id string, expires time.Time, maxAge int) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     SessionCookieName,
		Value:    id,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Expires:  expires,
		MaxAge:   maxAge,
	})
}

// clearSessionCookie expires the session cookie immediately.
func clearSessionCookie(c *gin.Context) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Expires:  time.Unix(0, 0).UTC(),
		MaxAge:   -1,
	})
}

// sessionCookieValue returns the session cookie value, or "" when absent.
func sessionCookieValue(c *gin.Context) string {
	value, err := c.Cookie(SessionCookieName)
	if err != nil {
		return ""
	}
	return value
}

// RequireSession is Gin middleware that rejects requests without a valid,
// unexpired session cookie with HTTP 401. Other packages apply it to the
// management API groups. On success the Session is available through
// SessionFromContext.
func RequireSession(s *Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := sessionCookieValue(c)
		if id == "" {
			abortWithError(c, http.StatusUnauthorized, "unauthorized")
			return
		}

		session, ok, err := s.sessions.Validate(id)
		if err != nil {
			abortWithError(c, http.StatusInternalServerError, "server_error")
			return
		}
		if !ok {
			clearSessionCookie(c)
			abortWithError(c, http.StatusUnauthorized, "unauthorized")
			return
		}

		c.Set(sessionContextKey, session)
		c.Next()
	}
}

// SessionFromContext returns the validated session stored by RequireSession.
func SessionFromContext(c *gin.Context) (Session, bool) {
	value, ok := c.Get(sessionContextKey)
	if !ok {
		return Session{}, false
	}
	session, ok := value.(Session)
	return session, ok
}
