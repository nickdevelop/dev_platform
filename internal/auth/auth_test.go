package auth

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"devplatform/internal/database"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}

const testPassword = "correct horse battery staple"

// testClock is a manually advanced clock shared by the session store and the
// login throttle so expiry and throttling windows are deterministic.
type testClock struct {
	mu sync.Mutex
	t  time.Time
}

func newTestClock() *testClock {
	return &testClock{t: time.Now().UTC()}
}

func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *testClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

// openTestDB opens and migrates a throwaway database under a temp directory.
func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := database.Open(t.TempDir())
	if err != nil {
		t.Fatalf("database.Open: %v", err)
	}
	if err := database.Migrate(db); err != nil {
		t.Fatalf("database.Migrate: %v", err)
	}
	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

// newService builds a service with the shared test password already
// configured, matching a first-run-setup-then-login deployment.
func newService(t *testing.T, opts ...Option) *Service {
	t.Helper()

	db := openTestDB(t)
	svc := NewService(db, opts...)
	if err := svc.SetPassword(testPassword); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}
	return svc
}

// newRouter mirrors production wiring: global CSRF enforcement plus the auth
// routes and a representative protected management route.
func newRouter(s *Service) *gin.Engine {
	router := gin.New()
	router.Use(RequireCSRF())
	api := router.Group("/api")
	s.RegisterRoutes(api)

	protected := api.Group("/protected", RequireSession(s))
	protected.GET("/ping", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
	protected.POST("/mutate", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
	return router
}

// newClient starts a test server and returns a client with a cookie jar.
func newClient(t *testing.T, handler http.Handler) (*http.Client, string) {
	t.Helper()

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar.New: %v", err)
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return &http.Client{Jar: jar}, server.URL
}

// fetchCSRF calls the session endpoint and returns the CSRF token, which also
// installs the matching CSRF cookie in the client's jar.
func fetchCSRF(t *testing.T, client *http.Client, base string) string {
	t.Helper()

	resp, err := client.Get(base + "/api/auth/session")
	if err != nil {
		t.Fatalf("GET session: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET session status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	var body struct {
		CSRFToken string `json:"csrfToken"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode session body: %v", err)
	}
	if body.CSRFToken == "" {
		t.Fatal("session endpoint returned an empty CSRF token")
	}
	return body.CSRFToken
}

// postJSONRaw posts a JSON body with an optional CSRF header and returns the
// response or an error. It is safe to call from multiple goroutines.
func postJSONRaw(client *http.Client, url, csrf, payload string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if csrf != "" {
		req.Header.Set(CSRFHeaderName, csrf)
	}
	return client.Do(req)
}

// postJSON posts a JSON body with an optional CSRF header, failing the test on
// transport errors.
func postJSON(t *testing.T, client *http.Client, url, csrf, payload string) *http.Response {
	t.Helper()

	resp, err := postJSONRaw(client, url, csrf, payload)
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	return resp
}

func loginPayload(password string) string {
	b, _ := json.Marshal(map[string]string{"password": password})
	return string(b)
}

func TestLoginWithValidPasswordCreatesSession(t *testing.T) {
	svc := newService(t)
	client, base := newClient(t, newRouter(svc))
	csrf := fetchCSRF(t, client, base)

	resp := postJSON(t, client, base+"/api/auth/login", csrf, loginPayload(testPassword))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var sessionCookie *http.Cookie
	for _, cookie := range resp.Cookies() {
		if cookie.Name == SessionCookieName {
			sessionCookie = cookie
		}
	}
	if sessionCookie == nil {
		t.Fatal("login did not set a session cookie")
	}
	if sessionCookie.Value == "" {
		t.Error("session cookie value is empty")
	}
	if !sessionCookie.HttpOnly {
		t.Error("session cookie is not HttpOnly")
	}
	if sessionCookie.SameSite != http.SameSiteStrictMode {
		t.Errorf("session cookie SameSite = %v, want Strict", sessionCookie.SameSite)
	}
	if sessionCookie.Expires.IsZero() && sessionCookie.MaxAge <= 0 {
		t.Error("session cookie is not expiring")
	}

	var body struct {
		Authenticated bool       `json:"authenticated"`
		ExpiresAt     *time.Time `json:"expiresAt"`
		CSRFToken     string     `json:"csrfToken"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode login body: %v", err)
	}
	if !body.Authenticated || body.ExpiresAt == nil || body.CSRFToken == "" {
		t.Errorf("login body = %+v, want authenticated with expiry and csrf token", body)
	}

	// The freshly issued session unlocks a protected management route.
	pr, err := client.Get(base + "/api/protected/ping")
	if err != nil {
		t.Fatalf("protected GET: %v", err)
	}
	defer pr.Body.Close()
	if pr.StatusCode != http.StatusOK {
		t.Errorf("protected status = %d, want %d", pr.StatusCode, http.StatusOK)
	}
}

func TestLoginWithInvalidPasswordIsRejectedGenerically(t *testing.T) {
	svc := newService(t)
	client, base := newClient(t, newRouter(svc))
	csrf := fetchCSRF(t, client, base)

	resp := postJSON(t, client, base+"/api/auth/login", csrf, loginPayload("wrong password"))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("login status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
	for _, cookie := range resp.Cookies() {
		if cookie.Name == SessionCookieName {
			t.Error("failed login issued a session cookie")
		}
	}
	var body struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode login body: %v", err)
	}
	if body.Error != "invalid_credentials" {
		t.Errorf("error = %q, want %q", body.Error, "invalid_credentials")
	}

	pr, err := client.Get(base + "/api/protected/ping")
	if err != nil {
		t.Fatalf("protected GET: %v", err)
	}
	defer pr.Body.Close()
	if pr.StatusCode != http.StatusUnauthorized {
		t.Errorf("protected status after failed login = %d, want %d", pr.StatusCode, http.StatusUnauthorized)
	}
}

func TestProtectedRouteRejectsUnauthenticatedRequest(t *testing.T) {
	svc := newService(t)
	client, base := newClient(t, newRouter(svc))

	resp, err := client.Get(base + "/api/protected/ping")
	if err != nil {
		t.Fatalf("protected GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
	var body struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Error != "unauthorized" {
		t.Errorf("error = %q, want %q", body.Error, "unauthorized")
	}
}

func TestMutationWithoutCSRFTokenIsRejected(t *testing.T) {
	svc := newService(t)
	client, base := newClient(t, newRouter(svc))

	// Login itself is a mutation and must be CSRF-protected.
	resp := postJSON(t, client, base+"/api/auth/login", "", loginPayload(testPassword))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("login without CSRF status = %d, want %d", resp.StatusCode, http.StatusForbidden)
	}

	// Authenticate properly, then try a protected mutation with no token.
	csrf := fetchCSRF(t, client, base)
	login := postJSON(t, client, base+"/api/auth/login", csrf, loginPayload(testPassword))
	login.Body.Close()
	if login.StatusCode != http.StatusOK {
		t.Fatalf("login status = %d, want %d", login.StatusCode, http.StatusOK)
	}

	missing := postJSON(t, client, base+"/api/protected/mutate", "", `{}`)
	missing.Body.Close()
	if missing.StatusCode != http.StatusForbidden {
		t.Errorf("mutation without CSRF status = %d, want %d", missing.StatusCode, http.StatusForbidden)
	}

	ok := postJSON(t, client, base+"/api/protected/mutate", csrf, `{}`)
	ok.Body.Close()
	if ok.StatusCode != http.StatusOK {
		t.Errorf("mutation with CSRF status = %d, want %d", ok.StatusCode, http.StatusOK)
	}
}

func TestSessionExpires(t *testing.T) {
	clock := newTestClock()
	svc := newService(t, WithClock(clock.Now), WithSessionTTL(time.Minute))
	client, base := newClient(t, newRouter(svc))
	csrf := fetchCSRF(t, client, base)

	login := postJSON(t, client, base+"/api/auth/login", csrf, loginPayload(testPassword))
	login.Body.Close()
	if login.StatusCode != http.StatusOK {
		t.Fatalf("login status = %d, want %d", login.StatusCode, http.StatusOK)
	}

	before, err := client.Get(base + "/api/protected/ping")
	if err != nil {
		t.Fatalf("protected GET: %v", err)
	}
	before.Body.Close()
	if before.StatusCode != http.StatusOK {
		t.Fatalf("pre-expiry status = %d, want %d", before.StatusCode, http.StatusOK)
	}

	clock.Advance(time.Minute + time.Second)

	after, err := client.Get(base + "/api/protected/ping")
	if err != nil {
		t.Fatalf("protected GET after expiry: %v", err)
	}
	after.Body.Close()
	if after.StatusCode != http.StatusUnauthorized {
		t.Errorf("expired status = %d, want %d", after.StatusCode, http.StatusUnauthorized)
	}

	session, err := client.Get(base + "/api/auth/session")
	if err != nil {
		t.Fatalf("GET session: %v", err)
	}
	defer session.Body.Close()
	var body struct {
		Authenticated bool `json:"authenticated"`
	}
	if err := json.NewDecoder(session.Body).Decode(&body); err != nil {
		t.Fatalf("decode session: %v", err)
	}
	if body.Authenticated {
		t.Error("session endpoint reports authenticated after expiry")
	}
}

func TestLoginThrottleBoundsRepeatedFailures(t *testing.T) {
	clock := newTestClock()
	svc := newService(t, WithClock(clock.Now), WithLoginThrottle(3, time.Minute))
	client, base := newClient(t, newRouter(svc))
	csrf := fetchCSRF(t, client, base)

	for attempt := 1; attempt <= 3; attempt++ {
		resp := postJSON(t, client, base+"/api/auth/login", csrf, loginPayload("wrong"))
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("attempt %d status = %d, want %d", attempt, resp.StatusCode, http.StatusUnauthorized)
		}
	}

	// Even the correct password is refused once the window is exhausted.
	blocked := postJSON(t, client, base+"/api/auth/login", csrf, loginPayload(testPassword))
	blocked.Body.Close()
	if blocked.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("throttled status = %d, want %d", blocked.StatusCode, http.StatusTooManyRequests)
	}

	// Once the window passes, the throttle forgets the failures.
	clock.Advance(time.Minute + time.Second)
	allowed := postJSON(t, client, base+"/api/auth/login", csrf, loginPayload(testPassword))
	allowed.Body.Close()
	if allowed.StatusCode != http.StatusOK {
		t.Errorf("post-window status = %d, want %d", allowed.StatusCode, http.StatusOK)
	}
}

// TestLoginThrottleLimitHoldsUnderConcurrency is the regression test for the
// check-then-record race: many simultaneous wrong-password requests must
// never admit more than the configured number of attempts in one window.
func TestLoginThrottleLimitHoldsUnderConcurrency(t *testing.T) {
	clock := newTestClock()
	const limit = 3
	svc := newService(t, WithClock(clock.Now), WithLoginThrottle(limit, time.Minute))
	client, base := newClient(t, newRouter(svc))
	csrf := fetchCSRF(t, client, base)

	const burst = 40
	var wg sync.WaitGroup
	var mu sync.Mutex
	statuses := make([]int, 0, burst)
	transportErrors := make([]error, 0)

	start := make(chan struct{})
	for i := 0; i < burst; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			resp, err := postJSONRaw(client, base+"/api/auth/login", csrf, loginPayload("wrong"))
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				transportErrors = append(transportErrors, err)
				return
			}
			statuses = append(statuses, resp.StatusCode)
			resp.Body.Close()
		}()
	}
	close(start)
	wg.Wait()

	if len(transportErrors) != 0 {
		t.Fatalf("transport errors: %v", transportErrors)
	}

	admitted, throttled := 0, 0
	for _, status := range statuses {
		switch status {
		case http.StatusUnauthorized:
			admitted++
		case http.StatusTooManyRequests:
			throttled++
		default:
			t.Errorf("unexpected status %d", status)
		}
	}
	if admitted != limit {
		t.Errorf("admitted %d concurrent attempts, want exactly %d", admitted, limit)
	}
	if admitted+throttled != burst {
		t.Errorf("admitted+throttled = %d, want %d", admitted+throttled, burst)
	}

	// The window is not yet elapsed and the failures are the same ones the
	// burst recorded, so even the correct password stays throttled.
	stillBlocked := postJSON(t, client, base+"/api/auth/login", csrf, loginPayload(testPassword))
	stillBlocked.Body.Close()
	if stillBlocked.StatusCode != http.StatusTooManyRequests {
		t.Errorf("post-burst correct login status = %d, want %d", stillBlocked.StatusCode, http.StatusTooManyRequests)
	}
}

func TestLoginThrottleKeyMapIsBounded(t *testing.T) {
	throttle := NewLoginThrottle(1, time.Minute, nil)
	throttle.maxKeys = 16
	for i := 0; i < 100; i++ {
		throttle.Reserve(string(rune('a'+i%26)) + string(rune('0'+i/26)))
	}
	if got := len(throttle.attempts); got > throttle.maxKeys {
		t.Errorf("tracked throttle keys = %d, want <= %d", got, throttle.maxKeys)
	}
}

func TestSetPasswordRefusesToOverwriteConfiguredPassword(t *testing.T) {
	db := openTestDB(t)

	if err := SetPassword(db, "first-password"); err != nil {
		t.Fatalf("first SetPassword: %v", err)
	}
	configured, err := PasswordConfigured(db)
	if err != nil {
		t.Fatalf("PasswordConfigured: %v", err)
	}
	if !configured {
		t.Fatal("PasswordConfigured = false after SetPassword")
	}

	if err := SetPassword(db, "second-password"); !errors.Is(err, ErrPasswordAlreadySet) {
		t.Fatalf("second SetPassword error = %v, want ErrPasswordAlreadySet", err)
	}

	var settings []database.Setting
	if err := db.Where("key = ?", database.SettingKeyPasswordHash).Find(&settings).Error; err != nil {
		t.Fatalf("load password hash: %v", err)
	}
	if len(settings) != 1 {
		t.Fatalf("password settings = %d, want 1", len(settings))
	}
	hash := settings[0].Value

	ok, err := VerifyPassword(hash, "first-password")
	if err != nil || !ok {
		t.Errorf("original password verify = (%v, %v), want (true, nil)", ok, err)
	}
	ok, err = VerifyPassword(hash, "second-password")
	if err != nil || ok {
		t.Errorf("overwrite password verify = (%v, %v), want (false, nil)", ok, err)
	}
}

func TestHashPasswordUsesArgon2idWithFreshSalt(t *testing.T) {
	first, err := HashPassword("s3cret")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if !strings.HasPrefix(first, "$argon2id$") {
		t.Fatalf("hash = %q, want $argon2id$ prefix", first)
	}

	ok, err := VerifyPassword(first, "s3cret")
	if err != nil || !ok {
		t.Errorf("verify correct = (%v, %v), want (true, nil)", ok, err)
	}
	ok, err = VerifyPassword(first, "nope")
	if err != nil || ok {
		t.Errorf("verify wrong = (%v, %v), want (false, nil)", ok, err)
	}

	second, err := HashPassword("s3cret")
	if err != nil {
		t.Fatalf("second HashPassword: %v", err)
	}
	if first == second {
		t.Error("identical passwords produced identical hashes; salt is not random")
	}
	if _, err := VerifyPassword("not-a-hash", "s3cret"); err == nil {
		t.Error("VerifyPassword accepted a malformed hash")
	}
}

func TestSessionIDsAreRandom(t *testing.T) {
	store := NewSessionStore(openTestDB(t), time.Hour, nil)

	first, err := store.Create()
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	second, err := store.Create()
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if first.ID == second.ID {
		t.Error("two sessions share an ID")
	}
	if len(first.ID) < 32 {
		t.Errorf("session ID length = %d, want >= 32", len(first.ID))
	}
}
