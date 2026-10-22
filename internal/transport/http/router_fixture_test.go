package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"devplatform/internal/app/agentservices"
	"devplatform/internal/app/auth"
	"devplatform/internal/app/instances"
	"devplatform/internal/app/modelconfig"
	"devplatform/internal/app/projects"
	"devplatform/internal/app/sessions"
	"devplatform/internal/app/settings"
	"devplatform/internal/app/tasks"
	"devplatform/internal/app/terminal"
	"devplatform/internal/core/agents"
	"devplatform/internal/core/platform/fake"
)

// fixturePassword is the shared password configured by every test that needs a
// logged-in client.
const fixturePassword = "correct horse battery staple"

// newFullDependencies wires every service the production router mounts, so a
// test that walks router.Routes() observes the same route table as production
// instead of a hand-written subset that can drift.
func newFullDependencies(t *testing.T) Dependencies {
	t.Helper()

	db := openTestDB(t)
	registry := agents.NewRegistry()
	projSvc := projects.NewService(db)
	instSvc := instances.NewService(db, registry, projSvc)
	authSvc := auth.NewService(db)

	catalog, err := agents.NewCatalog(
		agents.BackendDescriptor{ID: agents.BackendOpenCodeV2, Binary: "opencode", Order: 1},
		agents.BackendDescriptor{ID: agents.BackendPi, Binary: "pi", Order: 2},
	)
	if err != nil {
		t.Fatalf("agents.NewCatalog: %v", err)
	}
	settingsSvc := settings.NewService(db, catalog)
	tasksSvc := tasks.NewService(tasks.NewRepository(db), nil)

	return Dependencies{
		Auth:          authSvc,
		Projects:      projSvc,
		Settings:      settingsSvc,
		Models:        modelconfig.NewService(db, nil),
		Instances:     instSvc,
		AgentServices: agentservices.NewService(nil),
		Sessions:      sessions.NewService(db, registry, instSvc),
		Tasks:         tasksSvc,
		Terminal:      terminal.NewService(terminal.NewRepository(db), nil, fake.New()),
	}
}

// newFullRouter builds the router with every service mounted.
func newFullRouter(t *testing.T) *gin.Engine {
	t.Helper()
	return NewRouter(newFullDependencies(t))
}

// loginClient performs the session + CSRF handshake against base and returns
// the CSRF token the caller must echo on mutating requests. The client must be
// backed by a cookie jar so the session and CSRF cookies survive the calls.
func loginClient(t *testing.T, client *http.Client, base string) string {
	t.Helper()

	resp, err := client.Get(base + "/api/auth/session")
	if err != nil {
		t.Fatalf("GET session: %v", err)
	}
	var session struct {
		CSRFToken string `json:"csrfToken"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
		t.Fatalf("decode session body: %v", err)
	}
	resp.Body.Close()
	if session.CSRFToken == "" {
		t.Fatal("session endpoint returned an empty CSRF token")
	}

	req, err := http.NewRequest(http.MethodPost, base+"/api/auth/login", strings.NewReader(`{"password":"`+fixturePassword+`"}`))
	if err != nil {
		t.Fatalf("build login request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(auth.CSRFHeaderName, session.CSRFToken)
	loginResp, err := client.Do(req)
	if err != nil {
		t.Fatalf("POST login: %v", err)
	}
	loginResp.Body.Close()
	if loginResp.StatusCode != http.StatusOK {
		t.Fatalf("login status = %d, want %d", loginResp.StatusCode, http.StatusOK)
	}

	// Login rotates the CSRF token, so the pre-login value is dead. Re-read the
	// session the way a browser does and hand back the token the cookie now
	// holds; using the old one would make every later mutation answer 403.
	return fetchSessionToken(t, client, base)
}

// fetchSessionToken reads the CSRF token the client's cookie jar currently
// holds, issuing the session request that keeps it in sync.
func fetchSessionToken(t *testing.T, client *http.Client, base string) string {
	t.Helper()

	resp, err := client.Get(base + "/api/auth/session")
	if err != nil {
		t.Fatalf("GET session: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET session status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	var session struct {
		CSRFToken string `json:"csrfToken"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
		t.Fatalf("decode session body: %v", err)
	}
	if session.CSRFToken == "" {
		t.Fatal("session endpoint returned an empty CSRF token")
	}
	return session.CSRFToken
}

// routerFixture is a logged-in client bound to a router that mounts every
// service, plus the route table that router exposes.
type routerFixture struct {
	client *http.Client
	base   string
	csrf   string
	router *gin.Engine
}

// newRouterFixture returns the fixture, failing the test when the handshake
// cannot complete.
func newRouterFixture(t *testing.T) *routerFixture {
	t.Helper()

	deps := newFullDependencies(t)
	if err := deps.Auth.SetPassword(fixturePassword); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}
	router := NewRouter(deps)

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar.New: %v", err)
	}
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)

	// The timeout keeps a deliberately blocked handler from hanging the matrix
	// walk; a timeout is handled as a non-CSRF outcome by the caller.
	client := &http.Client{Jar: jar, Timeout: 10 * time.Second}
	csrf := loginClient(t, client, server.URL)

	return &routerFixture{client: client, base: server.URL, csrf: csrf, router: router}
}

// assertPathParams fails when a route still carries an unfilled :param, which
// would make the matrix request hit a 404 instead of the handler under test.
func assertPathParams(t *testing.T, path string) {
	t.Helper()
	if strings.Contains(path, ":") {
		t.Fatalf("route path %q still contains a path parameter", path)
	}
}

// describeRoute renders a route for subtest names and failure messages.
func describeRoute(method, path string) string {
	return fmt.Sprintf("%s %s", method, path)
}
