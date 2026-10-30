package auth

import (
	"errors"
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
	repo           Repository
	sessions       *SessionStore
	throttle       *LoginThrottle
	sessionTTL     time.Duration
	throttleMax    int
	throttleWindow time.Duration
	now            func() time.Time
}

// NewService builds the auth service over db.
func NewService(db *gorm.DB, opts ...Option) *Service {
	return NewServiceWithRepository(NewGORMRepository(db), opts...)
}

// NewServiceWithRepository builds the auth service over a custom repository.
func NewServiceWithRepository(repo Repository, opts ...Option) *Service {
	s := &Service{
		repo:       repo,
		sessionTTL: DefaultSessionTTL,
		now:        time.Now,
	}
	for _, opt := range opts {
		opt(s)
	}
	s.sessions = NewSessionStoreWithRepository(repo, s.sessionTTL, s.now)
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
	return SetPasswordWithRepository(s.repo, password)
}

// PasswordConfigured reports whether a platform password is set.
func (s *Service) PasswordConfigured() (bool, error) {
	return PasswordConfiguredWithRepository(s.repo)
}

// RegisterRoutes registers the auth endpoints under r. Mutating routes rely
// on RequireCSRF being installed (the platform router installs it globally).
func (s *Service) RegisterRoutes(r gin.IRoutes) {
	r.POST("/auth/login", s.handleLogin)
	r.POST("/auth/logout", RequireSession(s), s.handleLogout)
	r.POST("/auth/setup", s.handleSetupPassword)
	r.GET("/auth/session", s.handleSession)
}

// sessionResponse is the JSON body shared by the auth endpoints. It exposes
// only session metadata and the CSRF token, never the session ID.
type sessionResponse struct {
	Authenticated bool       `json:"authenticated"`
	ExpiresAt     *time.Time `json:"expiresAt,omitempty"`
	CSRFToken     string     `json:"csrfToken"`
	NeedsSetup    bool       `json:"needsSetup,omitempty"`
}

// errorResponse is the JSON error envelope for auth failures.
type errorResponse struct {
	Error string `json:"error"`
}

// abortWithError stops the chain and writes a JSON error envelope.
func abortWithError(c *gin.Context, status int, code string) {
	c.AbortWithStatusJSON(status, errorResponse{Error: code})
}

// setNoStore marks an auth response as non-cacheable. Session metadata and
// the CSRF token must never be written to a browser or intermediary cache.
func setNoStore(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
}

// loginRequest is the JSON body of POST /api/auth/login.
type loginRequest struct {
	Password string `json:"password"`
}

// handleLogin verifies the shared password and issues a session. Failures
// return a generic error and never reveal whether a password is configured.
func (s *Service) handleLogin(c *gin.Context) {
	setNoStore(c)
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		abortWithError(c, http.StatusBadRequest, "invalid_request")
		return
	}

	key := clientKey(c)
	// Reserve the attempt before doing any password work so that the limit
	// check and the record are atomic: a concurrent burst can never admit
	// more than max attempts per window.
	reservation, ok := s.throttle.Reserve(key)
	if !ok {
		abortWithError(c, http.StatusTooManyRequests, "too_many_attempts")
		return
	}

	hash, configured, err := passwordHashWithRepository(s.repo)
	if err != nil {
		abortWithError(c, http.StatusInternalServerError, "server_error")
		return
	}
	if !configured {
		// The reserved slot stands; the response is identical to a wrong
		// password so it cannot reveal whether a password is configured.
		abortWithError(c, http.StatusUnauthorized, "invalid_credentials")
		return
	}

	verified, err := VerifyPassword(hash, req.Password)
	if err != nil {
		abortWithError(c, http.StatusInternalServerError, "server_error")
		return
	}
	if !verified {
		abortWithError(c, http.StatusUnauthorized, "invalid_credentials")
		return
	}

	// Successful login releases ONLY this request's own reserved slot; other
	// in-flight or failed attempts stay counted in the window.
	s.throttle.Release(key, reservation)
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
		// Rotate: the pre-login token was handed to an unauthenticated caller,
		// so the authenticated session gets a token of its own.
		CSRFToken: s.issueCSRFToken(c),
	})
}

// handleSession reports the current authentication state and always provides
// a CSRF token so the browser can drive mutating requests.
func (s *Service) handleSession(c *gin.Context) {
	setNoStore(c)
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

	if !resp.Authenticated && isLocalClient(c) {
		configured, err := s.PasswordConfigured()
		if err != nil {
			abortWithError(c, http.StatusInternalServerError, "server_error")
			return
		}
		if !configured {
			resp.NeedsSetup = true
		}
	}

	c.JSON(http.StatusOK, resp)
}

// setupPasswordRequest is the JSON body of POST /api/auth/setup.
type setupPasswordRequest struct {
	Password string `json:"password"`
}

// handleSetupPassword configures the platform password on first run from localhost.
func (s *Service) handleSetupPassword(c *gin.Context) {
	setNoStore(c)

	if !isLocalClient(c) {
		abortWithError(c, http.StatusForbidden, "local_access_only")
		return
	}

	var req setupPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Password == "" {
		abortWithError(c, http.StatusBadRequest, "invalid_request")
		return
	}

	configured, err := s.PasswordConfigured()
	if err != nil {
		abortWithError(c, http.StatusInternalServerError, "server_error")
		return
	}
	if configured {
		abortWithError(c, http.StatusConflict, "password_already_set")
		return
	}

	if err := s.SetPassword(req.Password); err != nil {
		if errors.Is(err, ErrPasswordAlreadySet) {
			abortWithError(c, http.StatusConflict, "password_already_set")
			return
		}
		abortWithError(c, http.StatusInternalServerError, "server_error")
		return
	}

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
		CSRFToken:     s.issueCSRFToken(c),
	})
}

// handleLogout invalidates the current session and clears the cookie.
func (s *Service) handleLogout(c *gin.Context) {
	setNoStore(c)
	if session, ok := SessionFromContext(c); ok {
		if err := s.sessions.Delete(session.ID); err != nil {
			abortWithError(c, http.StatusInternalServerError, "server_error")
			return
		}
	}
	clearSessionCookie(c)
	// Rotate as well: the token that authorised the logout must not stay valid
	// for the anonymous session that follows.
	c.JSON(http.StatusOK, sessionResponse{Authenticated: false, CSRFToken: s.issueCSRFToken(c)})
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

// isLocalClient reports whether the request originated from a loopback address.
func isLocalClient(c *gin.Context) bool {
	host, _, err := net.SplitHostPort(c.Request.RemoteAddr)
	if err != nil {
		host = c.Request.RemoteAddr
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// reservation identifies one admitted login attempt. It lets a successful
// login release exactly its own slot without disturbing any other attempt
// still counted in the window.
type reservation struct {
	seq uint64
	at  time.Time
}

// Reservation is the caller-held handle for an admitted attempt.
type Reservation struct {
	seq uint64
}

// LoginThrottle is a bounded, in-memory sliding-window attempt limiter keyed
// by client. A slot is reserved atomically with the limit check before
// credential verification, so a concurrent burst cannot admit more than max
// attempts per window. A successful login releases only its own reservation;
// it never clears other in-flight or failed attempts. It does not depend on
// any user identity because the platform has a single shared password.
type LoginThrottle struct {
	mu       sync.Mutex
	max      int
	window   time.Duration
	maxKeys  int
	now      func() time.Time
	nextSeq  uint64
	attempts map[string][]reservation
}

// NewLoginThrottle builds a throttle allowing max attempts per window. A
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
		attempts: make(map[string][]reservation),
	}
}

// Reserve atomically checks the sliding-window limit for key and, when the
// key is under the limit, records one attempt and returns its reservation.
// Because the check and the record happen under a single lock, concurrent
// callers can never together admit more than max attempts in a window.
func (t *LoginThrottle) Reserve(key string) (Reservation, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := t.now()
	recent := pruneReservations(t.attempts[key], now.Add(-t.window))
	if len(recent) >= t.max {
		if len(recent) == 0 {
			delete(t.attempts, key)
		} else {
			t.attempts[key] = recent
		}
		return Reservation{}, false
	}

	if _, tracked := t.attempts[key]; !tracked && len(t.attempts) >= t.maxKeys {
		t.evictOldestLocked()
	}
	t.nextSeq++
	entry := reservation{seq: t.nextSeq, at: now}
	t.attempts[key] = append(recent, entry)
	return Reservation{seq: entry.seq}, true
}

// Release removes exactly the reservation held by r for key, leaving every
// other attempt in the window untouched. Releasing an unknown or already
// released reservation is a no-op.
func (t *LoginThrottle) Release(key string, r Reservation) {
	t.mu.Lock()
	defer t.mu.Unlock()

	entries := t.attempts[key]
	for i := range entries {
		if entries[i].seq == r.seq {
			t.attempts[key] = append(entries[:i], entries[i+1:]...)
			break
		}
	}
	if len(t.attempts[key]) == 0 {
		delete(t.attempts, key)
	}
}

// evictOldestLocked drops the least recently active key to bound memory.
func (t *LoginThrottle) evictOldestLocked() {
	var oldestKey string
	var oldest time.Time
	for key, attempts := range t.attempts {
		if len(attempts) == 0 {
			delete(t.attempts, key)
			continue
		}
		last := attempts[len(attempts)-1].at
		if oldestKey == "" || last.Before(oldest) {
			oldestKey = key
			oldest = last
		}
	}
	if oldestKey != "" {
		delete(t.attempts, oldestKey)
	}
}

// pruneReservations returns the attempts at or after cutoff.
func pruneReservations(attempts []reservation, cutoff time.Time) []reservation {
	kept := attempts[:0]
	for _, attempt := range attempts {
		if !attempt.at.Before(cutoff) {
			kept = append(kept, attempt)
		}
	}
	return kept
}
