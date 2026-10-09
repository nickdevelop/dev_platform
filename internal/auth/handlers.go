package auth

import (
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Login throttle defaults: at most maxLoginFailures failures per client
// within loginFailureWindow before the client is temporarily refused.
const (
	defaultMaxLoginFailures = 5
	defaultThrottleWindow   = time.Minute
	defaultMaxThrottleKeys  = 4096
)

// Option configures a Service.
type Option func(*Service)

// WithSessionTTL overrides how long issued sessions remain valid.
func WithSessionTTL(ttl time.Duration) Option {
	return func(s *Service) { s.sessionTTL = ttl }
}

// WithClock overrides the clock used for session expiry and throttling. It is
// primarily a test seam.
func WithClock(now func() time.Time) Option {
	return func(s *Service) { s.now = now }
}

// WithLoginThrottle overrides the bounded login-failure throttle.
func WithLoginThrottle(max int, window time.Duration) Option {
	return func(s *Service) {
		s.throttleMax = max
		s.throttleWindow = window
	}
}

// Service bundles the platform's authentication state and handlers.
type Service struct {
	db             *gorm.DB
	sessions       *SessionStore
	throttle       *LoginThrottle
	sessionTTL     time.Duration
	throttleMax    int
	throttleWindow time.Duration
	now            func() time.Time
}

// NewService builds the auth service over db.
func NewService(db *gorm.DB, opts ...Option) *Service {
	s := &Service{
		db:         db,
		sessionTTL: DefaultSessionTTL,
		now:        time.Now,
	}
	for _, opt := range opts {
		opt(s)
	}
	s.sessions = NewSessionStore(db, s.sessionTTL, s.now)
	if s.throttleMax < 1 {
		s.throttleMax = defaultMaxLoginFailures
	}
	if s.throttleWindow <= 0 {
		s.throttleWindow = defaultThrottleWindow
	}
	s.throttle = NewLoginThrottle(s.throttleMax, s.throttleWindow, s.now)
	return s
}

// SetPassword configures the platform password, refusing to overwrite one.
func (s *Service) SetPassword(password string) error {
	return SetPassword(s.db, password)
}

// PasswordConfigured reports whether a platform password is set.
func (s *Service) PasswordConfigured() (bool, error) {
	return PasswordConfigured(s.db)
}

// RegisterRoutes registers the auth endpoints under r. Mutating routes rely
// on RequireCSRF being installed (the platform router installs it globally).
func (s *Service) RegisterRoutes(r gin.IRoutes) {
	r.POST("/auth/login", s.handleLogin)
	r.POST("/auth/logout", RequireSession(s), s.handleLogout)
	r.GET("/auth/session", s.handleSession)
}

// sessionResponse is the JSON body shared by the auth endpoints. It exposes
// only session metadata and the CSRF token, never the session ID.
type sessionResponse struct {
	Authenticated bool       `json:"authenticated"`
	ExpiresAt     *time.Time `json:"expiresAt,omitempty"`
	CSRFToken     string     `json:"csrfToken"`
}

// errorResponse is the JSON error envelope for auth failures.
type errorResponse struct {
	Error string `json:"error"`
}

// abortWithError stops the chain and writes a JSON error envelope.
func abortWithError(c *gin.Context, status int, code string) {
	c.AbortWithStatusJSON(status, errorResponse{Error: code})
}

// loginRequest is the JSON body of POST /api/auth/login.
type loginRequest struct {
	Password string `json:"password"`
}

// handleLogin verifies the shared password and issues a session. Failures
// return a generic error and never reveal whether a password is configured.
func (s *Service) handleLogin(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		abortWithError(c, http.StatusBadRequest, "invalid_request")
		return
	}

	key := clientKey(c)
	if !s.throttle.Allow(key) {
		abortWithError(c, http.StatusTooManyRequests, "too_many_attempts")
		return
	}

	hash, configured, err := passwordHash(s.db)
	if err != nil {
		abortWithError(c, http.StatusInternalServerError, "server_error")
		return
	}
	if !configured {
		s.throttle.RecordFailure(key)
		abortWithError(c, http.StatusUnauthorized, "invalid_credentials")
		return
	}

	ok, err := VerifyPassword(hash, req.Password)
	if err != nil {
		abortWithError(c, http.StatusInternalServerError, "server_error")
		return
	}
	if !ok {
		s.throttle.RecordFailure(key)
		abortWithError(c, http.StatusUnauthorized, "invalid_credentials")
		return
	}

	s.throttle.Reset(key)
	session, err := s.sessions.Create()
	if err != nil {
		abortWithError(c, http.StatusInternalServerError, "server_error")
		return
	}

	setSessionCookie(c, session.ID, session.ExpiresAt, maxAgeSeconds(s.sessionTTL))
	expires := session.ExpiresAt
	c.JSON(http.StatusOK, sessionResponse{
		Authenticated: true,
		ExpiresAt:     &expires,
		CSRFToken:     s.nextCSRFToken(c),
	})
}

// handleSession reports the current authentication state and always provides
// a CSRF token so the browser can drive mutating requests.
func (s *Service) handleSession(c *gin.Context) {
	resp := sessionResponse{CSRFToken: s.nextCSRFToken(c)}

	if id := sessionCookieValue(c); id != "" {
		session, ok, err := s.sessions.Validate(id)
		if err != nil {
			abortWithError(c, http.StatusInternalServerError, "server_error")
			return
		}
		if ok {
			expires := session.ExpiresAt
			resp.Authenticated = true
			resp.ExpiresAt = &expires
		} else {
			clearSessionCookie(c)
		}
	}

	c.JSON(http.StatusOK, resp)
}

// handleLogout invalidates the current session and clears the cookie.
func (s *Service) handleLogout(c *gin.Context) {
	if session, ok := SessionFromContext(c); ok {
		if err := s.sessions.Delete(session.ID); err != nil {
			abortWithError(c, http.StatusInternalServerError, "server_error")
			return
		}
	}
	clearSessionCookie(c)
	c.JSON(http.StatusOK, sessionResponse{Authenticated: false, CSRFToken: s.nextCSRFToken(c)})
}

// clientKey derives the throttle key from the connection's remote address.
// The address is used verbatim rather than trusting forwarding headers, so a
// client cannot forge a header to escape throttling.
func clientKey(c *gin.Context) string {
	host, _, err := net.SplitHostPort(c.Request.RemoteAddr)
	if err != nil {
		return c.Request.RemoteAddr
	}
	return host
}

// LoginThrottle is a bounded, in-memory sliding-window failure counter keyed
// by client. It does not depend on any user identity because the platform has
// a single shared password.
type LoginThrottle struct {
	mu       sync.Mutex
	max      int
	window   time.Duration
	maxKeys  int
	now      func() time.Time
	failures map[string][]time.Time
}

// NewLoginThrottle builds a throttle allowing max failures per window. A
// non-positive max or window falls back to sane defaults.
func NewLoginThrottle(max int, window time.Duration, now func() time.Time) *LoginThrottle {
	if max < 1 {
		max = 1
	}
	if window <= 0 {
		window = defaultThrottleWindow
	}
	if now == nil {
		now = time.Now
	}
	return &LoginThrottle{
		max:      max,
		window:   window,
		maxKeys:  defaultMaxThrottleKeys,
		now:      now,
		failures: make(map[string][]time.Time),
	}
}

// Allow reports whether key may attempt a login right now.
func (t *LoginThrottle) Allow(key string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	recent := pruneFailures(t.failures[key], t.now().Add(-t.window))
	if len(recent) == 0 {
		delete(t.failures, key)
	} else {
		t.failures[key] = recent
	}
	return len(recent) < t.max
}

// RecordFailure records one failed attempt for key.
func (t *LoginThrottle) RecordFailure(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if _, tracked := t.failures[key]; !tracked && len(t.failures) >= t.maxKeys {
		t.evictOldestLocked()
	}
	now := t.now()
	recent := pruneFailures(t.failures[key], now.Add(-t.window))
	t.failures[key] = append(recent, now)
}

// Reset clears the failure history for key after a successful login.
func (t *LoginThrottle) Reset(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.failures, key)
}

// evictOldestLocked drops the least recently active key to bound memory.
func (t *LoginThrottle) evictOldestLocked() {
	var oldestKey string
	var oldest time.Time
	for key, attempts := range t.failures {
		if len(attempts) == 0 {
			delete(t.failures, key)
			continue
		}
		last := attempts[len(attempts)-1]
		if oldestKey == "" || last.Before(oldest) {
			oldestKey = key
			oldest = last
		}
	}
	if oldestKey != "" {
		delete(t.failures, oldestKey)
	}
}

// pruneFailures returns the attempts at or after cutoff.
func pruneFailures(attempts []time.Time, cutoff time.Time) []time.Time {
	kept := attempts[:0]
	for _, at := range attempts {
		if !at.Before(cutoff) {
			kept = append(kept, at)
		}
	}
	return kept
}
