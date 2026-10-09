package http

import (
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"devplatform/internal/auth"
	"devplatform/internal/database"
	"devplatform/internal/projects"
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
