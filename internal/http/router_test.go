package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"devplatform/internal/agents"
	"devplatform/internal/agents/instances"
	"devplatform/internal/auth"
	"devplatform/internal/database"
	"devplatform/internal/projects"
	"devplatform/internal/resources"
	"devplatform/internal/secrets"
	"devplatform/internal/sessions"
	"devplatform/internal/workspaces"
)

// openTestDB opens and migrates a throwaway database under a temp directory.
// The GORM logger is discarded because these tests deliberately exercise
// expected-error paths whose warnings would otherwise pollute the output.
func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := database.Open(t.TempDir())
	if err != nil {
		t.Fatalf("database.Open: %v", err)
	}
	if err := database.Migrate(db); err != nil {
		t.Fatalf("database.Migrate: %v", err)
	}
	db.Logger = logger.Discard
	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

// newAuthedClient builds the full platform router with auth, workspace and
// project services wired, logs a client in, and returns the client, base URL
// and the CSRF token to echo on mutating requests.
func newAuthedClient(t *testing.T) (*http.Client, string, string) {
	t.Helper()

	db := openTestDB(t)
	authSvc := auth.NewService(db)
	if err := authSvc.SetPassword("correct horse battery staple"); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}
	router := NewRouter(Dependencies{
		Auth:       authSvc,
		Workspaces: workspaces.NewService(db),
		Projects:   projects.NewService(db),
	})

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar.New: %v", err)
	}
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	client := &http.Client{Jar: jar}
	base := server.URL

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

	req, err := http.NewRequest(http.MethodPost, base+"/api/auth/login", strings.NewReader(`{"password":"correct horse battery staple"}`))
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
	return client, base, session.CSRFToken
}

// postJSON sends an authenticated JSON request with the CSRF token.
func postJSON(t *testing.T, client *http.Client, base, path, csrf string, body any) *http.Response {
	t.Helper()

	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, base+path, strings.NewReader(string(payload)))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if csrf != "" {
		req.Header.Set(auth.CSRFHeaderName, csrf)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	return resp
}

// TestRouterServesProjectsBehindSessionAndCSRF verifies the project routes are
// reachable only with a session and enforce CSRF on mutations, and that the
// full register/list/unregister round trip works over the real router.
func TestRouterServesProjectsBehindSessionAndCSRF(t *testing.T) {
	client, base, csrf := newAuthedClient(t)

	// Unauthenticated requests are rejected.
	unauthed, err := (&http.Client{}).Get(base + "/api/projects")
	if err != nil {
		t.Fatalf("GET projects: %v", err)
	}
	unauthed.Body.Close()
	if unauthed.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated GET status = %d, want %d", unauthed.StatusCode, http.StatusUnauthorized)
	}

	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}
	addRoot := postJSON(t, client, base, "/api/workspaces/roots", csrf, map[string]string{"path": root})
	addRoot.Body.Close()
	if addRoot.StatusCode != http.StatusCreated {
		t.Fatalf("add root status = %d, want %d", addRoot.StatusCode, http.StatusCreated)
	}

	projectDir := filepath.Join(root, "alpha")
	if err := os.Mkdir(projectDir, 0o755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}

	// A mutation without a CSRF token is rejected.
	noCSRF := postJSON(t, client, base, "/api/projects", "", map[string]string{"path": projectDir})
	noCSRF.Body.Close()
	if noCSRF.StatusCode != http.StatusForbidden {
		t.Fatalf("POST without CSRF status = %d, want %d", noCSRF.StatusCode, http.StatusForbidden)
	}

	// With session and CSRF, registration succeeds.
	register := postJSON(t, client, base, "/api/projects", csrf, map[string]string{"path": projectDir})
	if register.StatusCode != http.StatusCreated {
		t.Fatalf("register status = %d, want %d", register.StatusCode, http.StatusCreated)
	}
	var created projects.Project
	if err := json.NewDecoder(register.Body).Decode(&created); err != nil {
		t.Fatalf("decode register body: %v", err)
	}
	register.Body.Close()
	if created.Name != "alpha" || created.Path != projectDir {
		t.Errorf("created project = %+v, want name alpha path %q", created, projectDir)
	}

	list, err := client.Get(base + "/api/projects")
	if err != nil {
		t.Fatalf("GET projects: %v", err)
	}
	var body struct {
		Projects []projects.Project `json:"projects"`
	}
	if err := json.NewDecoder(list.Body).Decode(&body); err != nil {
		t.Fatalf("decode list body: %v", err)
	}
	list.Body.Close()
	if len(body.Projects) != 1 || body.Projects[0].ID != created.ID {
		t.Fatalf("projects = %+v, want the registered project", body.Projects)
	}

	// Unregister removes the record but keeps the directory on disk.
	unregister := postJSON(t, client, base,
		"/api/projects/"+strconv.FormatUint(uint64(created.ID), 10)+"/unregister", csrf, struct{}{})
	unregister.Body.Close()
	if unregister.StatusCode != http.StatusNoContent {
		t.Fatalf("unregister status = %d, want %d", unregister.StatusCode, http.StatusNoContent)
	}
	if _, err := os.Stat(projectDir); err != nil {
		t.Errorf("directory removed by unregister: %v", err)
	}
}

// decodeError reads and closes a response, returning its JSON error code.
func decodeError(t *testing.T, resp *http.Response) string {
	t.Helper()

	var envelope struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	resp.Body.Close()
	return envelope.Error
}

// TestRouterDestructiveProjectOpsRequireConfirmationAndCSRF exercises the
// move/rename/delete routes over the real router: each requires the session
// CSRF token and a typed confirmation matching the project's current name,
// and a rename/move keeps the recorded path in step with the directory.
func TestRouterDestructiveProjectOpsRequireConfirmationAndCSRF(t *testing.T) {
	client, base, csrf := newAuthedClient(t)

	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}
	addRoot := postJSON(t, client, base, "/api/workspaces/roots", csrf, map[string]string{"path": root})
	addRoot.Body.Close()
	if addRoot.StatusCode != http.StatusCreated {
		t.Fatalf("add root status = %d, want %d", addRoot.StatusCode, http.StatusCreated)
	}

	alpha := filepath.Join(root, "alpha")
	if err := os.Mkdir(alpha, 0o755); err != nil {
		t.Fatalf("Mkdir alpha: %v", err)
	}
	register := postJSON(t, client, base, "/api/projects", csrf, map[string]string{"path": alpha})
	if register.StatusCode != http.StatusCreated {
		t.Fatalf("register status = %d, want %d", register.StatusCode, http.StatusCreated)
	}
	var created projects.Project
	if err := json.NewDecoder(register.Body).Decode(&created); err != nil {
		t.Fatalf("decode register body: %v", err)
	}
	register.Body.Close()
	id := strconv.FormatUint(uint64(created.ID), 10)

	// A rename without CSRF is rejected before any filesystem change.
	noCSRF := postJSON(t, client, base, "/api/projects/"+id+"/rename", "", map[string]string{
		"name": "beta", "confirmName": "alpha",
	})
	noCSRF.Body.Close()
	if noCSRF.StatusCode != http.StatusForbidden {
		t.Fatalf("rename without CSRF status = %d, want %d", noCSRF.StatusCode, http.StatusForbidden)
	}

	// A rename with a mismatched confirmation is rejected.
	wrongConfirm := postJSON(t, client, base, "/api/projects/"+id+"/rename", csrf, map[string]string{
		"name": "beta", "confirmName": "wrong",
	})
	if code := decodeError(t, wrongConfirm); code != "confirmation_mismatch" {
		t.Errorf("rename wrong-confirm error = %q, want confirmation_mismatch", code)
	}
	if _, err := os.Stat(alpha); err != nil {
		t.Fatalf("directory changed despite rejected rename: %v", err)
	}

	// A valid rename updates both the directory and the record.
	rename := postJSON(t, client, base, "/api/projects/"+id+"/rename", csrf, map[string]string{
		"name": "beta", "confirmName": "alpha",
	})
	if rename.StatusCode != http.StatusOK {
		t.Fatalf("rename status = %d, want %d", rename.StatusCode, http.StatusOK)
	}
	var renamed projects.Project
	if err := json.NewDecoder(rename.Body).Decode(&renamed); err != nil {
		t.Fatalf("decode rename body: %v", err)
	}
	rename.Body.Close()
	beta := filepath.Join(root, "beta")
	if renamed.Name != "beta" || renamed.Path != beta {
		t.Fatalf("renamed project = %+v, want name beta path %q", renamed, beta)
	}
	if _, err := os.Stat(beta); err != nil {
		t.Errorf("renamed directory missing: %v", err)
	}

	// A move with a mismatched confirmation is rejected.
	moveWrong := postJSON(t, client, base, "/api/projects/"+id+"/move", csrf, map[string]string{
		"path": filepath.Join(root, "gamma"), "confirmName": "alpha",
	})
	if code := decodeError(t, moveWrong); code != "confirmation_mismatch" {
		t.Errorf("move wrong-confirm error = %q, want confirmation_mismatch", code)
	}

	// A valid move relocates the directory and record.
	move := postJSON(t, client, base, "/api/projects/"+id+"/move", csrf, map[string]string{
		"path": filepath.Join(root, "gamma"), "confirmName": "beta",
	})
	if move.StatusCode != http.StatusOK {
		t.Fatalf("move status = %d, want %d", move.StatusCode, http.StatusOK)
	}
	var moved projects.Project
	if err := json.NewDecoder(move.Body).Decode(&moved); err != nil {
		t.Fatalf("decode move body: %v", err)
	}
	move.Body.Close()
	gamma := filepath.Join(root, "gamma")
	if moved.Name != "gamma" || moved.Path != gamma {
		t.Fatalf("moved project = %+v, want name gamma path %q", moved, gamma)
	}

	// A delete with a mismatched confirmation is rejected and deletes nothing.
	deleteWrong := postJSON(t, client, base, "/api/projects/"+id+"/delete", csrf, map[string]any{
		"confirmName": "beta", "permanent": true,
	})
	if code := decodeError(t, deleteWrong); code != "confirmation_mismatch" {
		t.Errorf("delete wrong-confirm error = %q, want confirmation_mismatch", code)
	}
	if _, err := os.Stat(gamma); err != nil {
		t.Fatalf("directory removed despite rejected delete: %v", err)
	}

	// A confirmed permanent delete removes the directory and the record.
	deleteOK := postJSON(t, client, base, "/api/projects/"+id+"/delete", csrf, map[string]any{
		"confirmName": "gamma", "permanent": true,
	})
	deleteOK.Body.Close()
	if deleteOK.StatusCode != http.StatusNoContent {
		t.Fatalf("delete status = %d, want %d", deleteOK.StatusCode, http.StatusNoContent)
	}
	if _, err := os.Stat(gamma); !os.IsNotExist(err) {
		t.Errorf("directory still exists after confirmed delete: %v", err)
	}
}

// TestRouterRejectsOversizedRequestBody verifies the API body cap: a body
// larger than the limit is refused rather than buffered.
func TestRouterRejectsOversizedRequestBody(t *testing.T) {
	client, base, csrf := newAuthedClient(t)

	huge := strings.Repeat("a", maxRequestBodyBytes+1024)
	resp := postJSON(t, client, base, "/api/projects", csrf, map[string]string{"path": huge})
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("oversized body status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}

// TestAuthSessionResponseIsNotCached verifies session metadata and the CSRF
// token are never stored by a cache.
func TestAuthSessionResponseIsNotCached(t *testing.T) {
	db := openTestDB(t)
	authSvc := auth.NewService(db)

	resp := get(t, Dependencies{Auth: authSvc}, "/api/auth/session")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/auth/session status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if cc := resp.Header.Get("Cache-Control"); cc != "no-store" {
		t.Errorf("session Cache-Control = %q, want no-store", cc)
	}
	body(t, resp)
}

// TestRouterServesWorkspaceRootsBehindSessionAndCSRF verifies the wiring of
// the workspace service into the router: the routes are absent without the
// service, require a session, and enforce CSRF on mutations.
func TestRouterServesWorkspaceRootsBehindSessionAndCSRF(t *testing.T) {
	client, base, csrf := newAuthedClient(t)

	// Unauthenticated requests are rejected before anything else.
	unauthed, err := (&http.Client{}).Get(base + "/api/workspaces/roots")
	if err != nil {
		t.Fatalf("GET roots: %v", err)
	}
	unauthed.Body.Close()
	if unauthed.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated GET status = %d, want %d", unauthed.StatusCode, http.StatusUnauthorized)
	}

	// An authenticated mutation without a CSRF token is rejected.
	root := t.TempDir()
	payload, err := json.Marshal(map[string]string{"path": root})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	noCSRF, err := http.NewRequest(http.MethodPost, base+"/api/workspaces/roots", strings.NewReader(string(payload)))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	noCSRF.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(noCSRF)
	if err != nil {
		t.Fatalf("POST roots without CSRF: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("POST without CSRF status = %d, want %d", resp.StatusCode, http.StatusForbidden)
	}

	// With session and CSRF the add/list/remove round trip works.
	add, err := http.NewRequest(http.MethodPost, base+"/api/workspaces/roots", strings.NewReader(string(payload)))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	add.Header.Set("Content-Type", "application/json")
	add.Header.Set(auth.CSRFHeaderName, csrf)
	resp, err = client.Do(add)
	if err != nil {
		t.Fatalf("POST roots: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("POST roots status = %d, want %d", resp.StatusCode, http.StatusCreated)
	}
	var created workspaces.Root
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode add body: %v", err)
	}
	resp.Body.Close()

	canonical, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}
	if created.Path != canonical {
		t.Errorf("created root path = %q, want %q", created.Path, canonical)
	}

	list, err := client.Get(base + "/api/workspaces/roots")
	if err != nil {
		t.Fatalf("GET roots: %v", err)
	}
	var body struct {
		Roots []workspaces.Root `json:"roots"`
	}
	if err := json.NewDecoder(list.Body).Decode(&body); err != nil {
		t.Fatalf("decode list body: %v", err)
	}
	list.Body.Close()
	if len(body.Roots) != 1 {
		t.Fatalf("roots = %+v, want exactly one", body.Roots)
	}

	remove, err := http.NewRequest(http.MethodDelete, base+"/api/workspaces/roots/"+strconv.FormatUint(uint64(created.ID), 10), nil)
	if err != nil {
		t.Fatalf("build delete request: %v", err)
	}
	remove.Header.Set(auth.CSRFHeaderName, csrf)
	removed, err := client.Do(remove)
	if err != nil {
		t.Fatalf("DELETE root: %v", err)
	}
	removed.Body.Close()
	if removed.StatusCode != http.StatusNoContent {
		t.Errorf("DELETE root status = %d, want %d", removed.StatusCode, http.StatusNoContent)
	}
}

// routerFakeAdapter is the smallest AgentAdapter that can start and stop an
// instance, used to exercise the instance routes over the real router.
type routerFakeAdapter struct {
	mu   sync.Mutex
	next int
}

func (f *routerFakeAdapter) Probe(context.Context) error { return nil }

func (f *routerFakeAdapter) StartInstance(_ context.Context, req agents.StartInstanceRequest) (*agents.Instance, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.next++
	now := time.Now().UTC()
	return &agents.Instance{
		ID:        "router-inst-" + strconv.Itoa(f.next),
		Backend:   agents.BackendPi,
		Directory: req.Directory,
		Status:    agents.InstanceRunning,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

func (f *routerFakeAdapter) StopInstance(context.Context, string) error { return nil }

func (f *routerFakeAdapter) ListSessions(context.Context, string) ([]agents.SessionRef, error) {
	return nil, nil
}

func (f *routerFakeAdapter) CreateSession(context.Context, string, agents.CreateSessionOptions) (*agents.SessionRef, error) {
	return nil, errors.New("not implemented")
}

func (f *routerFakeAdapter) ContinueSession(context.Context, agents.SessionRef) (*agents.SessionRef, error) {
	return nil, errors.New("not implemented")
}

func (f *routerFakeAdapter) SendMessage(context.Context, agents.SessionRef, agents.SendMessageRequest) error {
	return nil
}

func (f *routerFakeAdapter) ListModels(context.Context, string) ([]agents.ModelInfo, error) {
	return nil, nil
}

func (f *routerFakeAdapter) Interrupt(context.Context, agents.SessionRef) error { return nil }

func (f *routerFakeAdapter) Subscribe(context.Context, string) (<-chan agents.NormalizedEvent, error) {
	return make(chan agents.NormalizedEvent), nil
}

// newInstancesClient builds the full router with the instance service wired
// over a fake pi adapter, logs a client in, and registers one project. It
// returns the client, base URL, CSRF token, project id and project directory.
func newInstancesClient(t *testing.T) (*http.Client, string, string, uint, string) {
	t.Helper()

	db := openTestDB(t)
	authSvc := auth.NewService(db)
	if err := authSvc.SetPassword("correct horse battery staple"); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}

	registry := agents.NewRegistry()
	if err := registry.Register(agents.BackendPi, &routerFakeAdapter{}); err != nil {
		t.Fatalf("register fake adapter: %v", err)
	}
	projectService := projects.NewService(db)
	router := NewRouter(Dependencies{
		Auth:       authSvc,
		Workspaces: workspaces.NewService(db),
		Projects:   projectService,
		Instances:  instances.NewService(db, registry, projectService),
	})

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar.New: %v", err)
	}
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	client := &http.Client{Jar: jar}
	base := server.URL

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

	loginReq, err := http.NewRequest(http.MethodPost, base+"/api/auth/login", strings.NewReader(`{"password":"correct horse battery staple"}`))
	if err != nil {
		t.Fatalf("build login request: %v", err)
	}
	loginReq.Header.Set("Content-Type", "application/json")
	loginReq.Header.Set(auth.CSRFHeaderName, session.CSRFToken)
	loginResp, err := client.Do(loginReq)
	if err != nil {
		t.Fatalf("POST login: %v", err)
	}
	loginResp.Body.Close()
	if loginResp.StatusCode != http.StatusOK {
		t.Fatalf("login status = %d, want %d", loginResp.StatusCode, http.StatusOK)
	}

	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}
	addRoot := postJSON(t, client, base, "/api/workspaces/roots", session.CSRFToken, map[string]string{"path": root})
	addRoot.Body.Close()
	if addRoot.StatusCode != http.StatusCreated {
		t.Fatalf("add root status = %d, want %d", addRoot.StatusCode, http.StatusCreated)
	}
	projectDir := filepath.Join(root, "alpha")
	if err := os.Mkdir(projectDir, 0o755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	register := postJSON(t, client, base, "/api/projects", session.CSRFToken, map[string]string{"path": projectDir})
	if register.StatusCode != http.StatusCreated {
		t.Fatalf("register status = %d, want %d", register.StatusCode, http.StatusCreated)
	}
	var created projects.Project
	if err := json.NewDecoder(register.Body).Decode(&created); err != nil {
		t.Fatalf("decode register body: %v", err)
	}
	register.Body.Close()

	return client, base, session.CSRFToken, created.ID, projectDir
}

// TestRouterServesProjectInstancesBehindSessionAndCSRF verifies the instance
// routes are session-protected, enforce CSRF on mutations, and support the
// start/list/stop round trip with isolated per-instance state.
func TestRouterServesProjectInstancesBehindSessionAndCSRF(t *testing.T) {
	client, base, csrf, projectID, projectDir := newInstancesClient(t)
	instancesPath := "/api/projects/" + strconv.FormatUint(uint64(projectID), 10) + "/instances"

	// Unauthenticated requests are rejected.
	unauthed, err := (&http.Client{}).Get(base + instancesPath)
	if err != nil {
		t.Fatalf("GET instances: %v", err)
	}
	unauthed.Body.Close()
	if unauthed.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated GET status = %d, want %d", unauthed.StatusCode, http.StatusUnauthorized)
	}

	// An authenticated mutation without a CSRF token is rejected.
	noCSRF := postJSON(t, client, base, instancesPath, "", map[string]string{"backend": "pi"})
	noCSRF.Body.Close()
	if noCSRF.StatusCode != http.StatusForbidden {
		t.Fatalf("POST without CSRF status = %d, want %d", noCSRF.StatusCode, http.StatusForbidden)
	}

	// Starting an unknown backend is rejected.
	unknown := postJSON(t, client, base, instancesPath, csrf, map[string]string{"backend": "claude-code"})
	if code := decodeError(t, unknown); code != "unknown_backend" {
		t.Errorf("unknown backend error = %q, want unknown_backend", code)
	}

	// With session and CSRF, starting an instance succeeds.
	start := postJSON(t, client, base, instancesPath, csrf, map[string]string{"backend": "pi"})
	if start.StatusCode != http.StatusCreated {
		t.Fatalf("start status = %d, want %d", start.StatusCode, http.StatusCreated)
	}
	var started instances.Instance
	if err := json.NewDecoder(start.Body).Decode(&started); err != nil {
		t.Fatalf("decode start body: %v", err)
	}
	start.Body.Close()
	if started.ID == "" || started.ProjectID != projectID {
		t.Fatalf("started instance = %+v, want id with project %d", started, projectID)
	}
	if started.Directory != projectDir {
		t.Errorf("instance directory = %q, want %q", started.Directory, projectDir)
	}

	// The list route returns the instance.
	list, err := client.Get(base + instancesPath)
	if err != nil {
		t.Fatalf("GET instances: %v", err)
	}
	var listed struct {
		Instances []instances.Instance `json:"instances"`
	}
	if err := json.NewDecoder(list.Body).Decode(&listed); err != nil {
		t.Fatalf("decode list body: %v", err)
	}
	list.Body.Close()
	if len(listed.Instances) != 1 || listed.Instances[0].ID != started.ID {
		t.Fatalf("listed instances = %+v, want the started instance", listed.Instances)
	}

	// Stopping is a JSON mutation and is idempotent.
	stopPath := instancesPath + "/" + started.ID + "/stop"
	stop := postJSON(t, client, base, stopPath, csrf, struct{}{})
	if stop.StatusCode != http.StatusOK {
		t.Fatalf("stop status = %d, want %d", stop.StatusCode, http.StatusOK)
	}
	var stopped instances.Instance
	if err := json.NewDecoder(stop.Body).Decode(&stopped); err != nil {
		t.Fatalf("decode stop body: %v", err)
	}
	stop.Body.Close()
	if stopped.Status != agents.InstanceStopped {
		t.Errorf("stopped status = %q, want %q", stopped.Status, agents.InstanceStopped)
	}
	again := postJSON(t, client, base, stopPath, csrf, struct{}{})
	again.Body.Close()
	if again.StatusCode != http.StatusOK {
		t.Errorf("second stop status = %d, want %d (idempotent)", again.StatusCode, http.StatusOK)
	}
}

// TestRouterServesSessionRoutesBehindSessionAndCSRF verifies the session routes
// and the backend-capability route are session-protected and enforce CSRF on
// mutations.
func TestRouterServesSessionRoutesBehindSessionAndCSRF(t *testing.T) {
	db := openTestDB(t)
	authSvc := auth.NewService(db)
	if err := authSvc.SetPassword("correct horse battery staple"); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}

	registry := agents.NewRegistry()
	if err := registry.Register(agents.BackendPi, &routerFakeAdapter{}); err != nil {
		t.Fatalf("register fake adapter: %v", err)
	}
	projectService := projects.NewService(db)
	instanceService := instances.NewService(db, registry, projectService)
	router := NewRouter(Dependencies{
		Auth:       authSvc,
		Workspaces: workspaces.NewService(db),
		Projects:   projectService,
		Instances:  instanceService,
		Sessions:   sessions.NewService(db, registry, instanceService),
	})

	server := httptest.NewServer(router)
	t.Cleanup(server.Close)

	// Unauthenticated management routes are rejected before anything else.
	for _, path := range []string{"/api/agent-backends", "/api/projects/1/sessions"} {
		resp, err := (&http.Client{}).Get(server.URL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("unauthenticated GET %s status = %d, want %d", path, resp.StatusCode, http.StatusUnauthorized)
		}
	}

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar.New: %v", err)
	}
	client := &http.Client{Jar: jar}

	resp, err := client.Get(server.URL + "/api/auth/session")
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

	loginReq, err := http.NewRequest(http.MethodPost, server.URL+"/api/auth/login", strings.NewReader(`{"password":"correct horse battery staple"}`))
	if err != nil {
		t.Fatalf("build login request: %v", err)
	}
	loginReq.Header.Set("Content-Type", "application/json")
	loginReq.Header.Set(auth.CSRFHeaderName, session.CSRFToken)
	loginResp, err := client.Do(loginReq)
	if err != nil {
		t.Fatalf("POST login: %v", err)
	}
	loginResp.Body.Close()
	if loginResp.StatusCode != http.StatusOK {
		t.Fatalf("login status = %d, want %d", loginResp.StatusCode, http.StatusOK)
	}

	// The capability route lists the registered backend.
	caps, err := client.Get(server.URL + "/api/agent-backends")
	if err != nil {
		t.Fatalf("GET agent-backends: %v", err)
	}
	var capsBody struct {
		Backends []struct {
			Backend string `json:"backend"`
		} `json:"backends"`
	}
	if err := json.NewDecoder(caps.Body).Decode(&capsBody); err != nil {
		t.Fatalf("decode capabilities: %v", err)
	}
	caps.Body.Close()
	if len(capsBody.Backends) != 1 || capsBody.Backends[0].Backend != "pi" {
		t.Fatalf("backends = %+v, want only pi", capsBody.Backends)
	}

	// A mutation without a CSRF token is rejected.
	noCSRF := postJSON(t, client, server.URL, "/api/projects/1/sessions", "", map[string]string{"backend": "pi"})
	noCSRF.Body.Close()
	if noCSRF.StatusCode != http.StatusForbidden {
		t.Errorf("POST sessions without CSRF status = %d, want %d", noCSRF.StatusCode, http.StatusForbidden)
	}
}

// TestRouterServesResourcesBehindSessionAndCSRF verifies the resource routes
// register alongside the session routes without a route conflict and enforce
// session authentication and CSRF like every other management route.
func TestRouterServesResourcesBehindSessionAndCSRF(t *testing.T) {
	db := openTestDB(t)
	authSvc := auth.NewService(db)
	if err := authSvc.SetPassword("correct horse battery staple"); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}

	registry := agents.NewRegistry()
	if err := registry.Register(agents.BackendPi, &routerFakeAdapter{}); err != nil {
		t.Fatalf("register fake adapter: %v", err)
	}
	projectService := projects.NewService(db)
	instanceService := instances.NewService(db, registry, projectService)
	secretStore, err := secrets.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("secrets.NewStore: %v", err)
	}
	// Building the router with every service present is the assertion: gin
	// panics on a conflicting route tree before any request is served.
	router := NewRouter(Dependencies{
		Auth:       authSvc,
		Workspaces: workspaces.NewService(db),
		Projects:   projectService,
		Instances:  instanceService,
		Sessions:   sessions.NewService(db, registry, instanceService),
		Resources:  resources.NewService(db, resources.WithSecretStore(secretStore)),
		Secrets:    secrets.NewHandler(secretStore),
	})

	server := httptest.NewServer(router)
	t.Cleanup(server.Close)

	// Unauthenticated resource and secret routes are rejected.
	for _, path := range []string{"/api/resources", "/api/secrets/sec_missing"} {
		resp, err := (&http.Client{}).Get(server.URL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("unauthenticated GET %s status = %d, want %d", path, resp.StatusCode, http.StatusUnauthorized)
		}
	}

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar.New: %v", err)
	}
	client := &http.Client{Jar: jar}

	sessionResp, err := client.Get(server.URL + "/api/auth/session")
	if err != nil {
		t.Fatalf("GET session: %v", err)
	}
	var sessionBody struct {
		CSRFToken string `json:"csrfToken"`
	}
	if err := json.NewDecoder(sessionResp.Body).Decode(&sessionBody); err != nil {
		t.Fatalf("decode session: %v", err)
	}
	sessionResp.Body.Close()

	loginReq, err := http.NewRequest(http.MethodPost, server.URL+"/api/auth/login",
		strings.NewReader(`{"password":"correct horse battery staple"}`))
	if err != nil {
		t.Fatalf("build login: %v", err)
	}
	loginReq.Header.Set("Content-Type", "application/json")
	loginReq.Header.Set(auth.CSRFHeaderName, sessionBody.CSRFToken)
	loginResp, err := client.Do(loginReq)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	loginResp.Body.Close()
	if loginResp.StatusCode != http.StatusOK {
		t.Fatalf("login status = %d, want 200", loginResp.StatusCode)
	}

	// A mutation without a CSRF token is rejected.
	noCSRF := postJSON(t, client, server.URL, "/api/resources", "", map[string]any{"kind": "skill", "name": "x"})
	noCSRF.Body.Close()
	if noCSRF.StatusCode != http.StatusForbidden {
		t.Fatalf("POST resources without CSRF status = %d, want 403", noCSRF.StatusCode)
	}

	created := postJSON(t, client, server.URL, "/api/resources", sessionBody.CSRFToken, map[string]any{
		"kind":    "skill",
		"name":    "router-skill",
		"content": "body",
		"enabled": true,
	})
	created.Body.Close()
	if created.StatusCode != http.StatusCreated {
		t.Fatalf("POST resources status = %d, want 201", created.StatusCode)
	}

	listed, err := client.Get(server.URL + "/api/resources")
	if err != nil {
		t.Fatalf("GET resources: %v", err)
	}
	listed.Body.Close()
	if listed.StatusCode != http.StatusOK {
		t.Fatalf("GET resources status = %d, want 200", listed.StatusCode)
	}
}
