package workspaces

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"devplatform/internal/auth"
	"devplatform/internal/database"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}

// newRoot creates a real directory and returns its canonical path, so tests
// compare against what EvalSymlinks would report even under a symlinked
// temporary directory.
func newRoot(t *testing.T) string {
	t.Helper()

	canonical, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}
	return canonical
}

// mustResolve asserts that ResolveAuthorizedPath accepts input.
func mustResolve(t *testing.T, input string, roots ...string) ResolvedPath {
	t.Helper()

	resolved, err := ResolveAuthorizedPath(input, roots)
	if err != nil {
		t.Fatalf("ResolveAuthorizedPath(%q) failed: %v", input, err)
	}
	return resolved
}

// mustReject asserts that ResolveAuthorizedPath refuses input with an error
// matching target.
func mustReject(t *testing.T, input string, target error, roots ...string) {
	t.Helper()

	resolved, err := ResolveAuthorizedPath(input, roots)
	if err == nil {
		t.Fatalf("ResolveAuthorizedPath(%q) = %+v, want rejection", input, resolved)
	}
	if !errors.Is(err, target) {
		t.Fatalf("ResolveAuthorizedPath(%q) error = %v, want %v", input, err, target)
	}
}

func TestResolveAuthorizedPathAllowsValidDescendants(t *testing.T) {
	root := newRoot(t)
	sub := filepath.Join(root, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("mkdir sub: %v", err)
	}
	other := filepath.Join(root, "other")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatalf("mkdir other: %v", err)
	}

	if resolved := mustResolve(t, root, root); resolved.Path != root || resolved.Root != root {
		t.Errorf("root itself = %+v, want Path and Root = %q", resolved, root)
	}
	if resolved := mustResolve(t, sub, root); resolved.Path != sub || resolved.Root != root {
		t.Errorf("existing descendant = %+v, want Path = %q Root = %q", resolved, sub, root)
	}
	// Trailing separators and interior dot segments are normalized away.
	if resolved := mustResolve(t, sub+"/", root); resolved.Path != sub {
		t.Errorf("trailing slash = %+v, want %q", resolved, sub)
	}
	if resolved := mustResolve(t, filepath.Join(root, "sub", "..", "other"), root); resolved.Path != other {
		t.Errorf("dot-dot inside the root = %+v, want %q", resolved, other)
	}

	// A symlink inside the root is resolved to its real target, which stays
	// authorized.
	real := filepath.Join(root, "real")
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatalf("mkdir real: %v", err)
	}
	alias := filepath.Join(root, "alias")
	if err := os.Symlink(real, alias); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	if resolved := mustResolve(t, alias, root); resolved.Path != real {
		t.Errorf("in-root symlink = %+v, want canonical %q", resolved, real)
	}
}

func TestResolveAuthorizedPathRejectsDotDotTraversal(t *testing.T) {
	root := newRoot(t)
	sibling := newRoot(t)

	mustReject(t, filepath.Join(root, "..", filepath.Base(sibling)), ErrOutsideRoots, root)
	mustReject(t, filepath.Join(root, "..", "..", "etc"), ErrOutsideRoots, root)
	mustReject(t, filepath.Join(root, "sub", "..", "..", "escape"), ErrOutsideRoots, root)
}

func TestResolveAuthorizedPathRejectsOutsidePaths(t *testing.T) {
	root := newRoot(t)

	for _, outside := range []string{"/etc", "/etc/passwd", "/usr/share"} {
		mustReject(t, outside, ErrOutsideRoots, root)
	}

	// An existing directory that merely sits next to the root.
	sibling := filepath.Join(filepath.Dir(root), filepath.Base(root)+"-sibling")
	if err := os.MkdirAll(sibling, 0o755); err != nil {
		t.Fatalf("mkdir sibling: %v", err)
	}
	mustReject(t, sibling, ErrOutsideRoots, root)

	// Malformed and ambiguous inputs are invalid, not merely outside.
	mustReject(t, "", ErrInvalidPath, root)
	mustReject(t, "relative/path", ErrInvalidPath, root)
	mustReject(t, `C:relative`, ErrInvalidPath, root)
	mustReject(t, `/tmp/mixed\separator`, ErrInvalidPath, root)
	mustReject(t, `\\server\share\proj`, ErrInvalidPath, root)

	// Without registered roots nothing is authorized.
	mustReject(t, root, ErrOutsideRoots)
}

func TestResolveAuthorizedPathRejectsSymlinkEscape(t *testing.T) {
	root := newRoot(t)
	outside := newRoot(t)

	link := filepath.Join(root, "escape")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	// The link itself, an existing directory behind it, and a not-yet-existing
	// descendant behind it must all be refused.
	mustReject(t, link, ErrOutsideRoots, root)
	mustReject(t, filepath.Join(link, "deep"), ErrOutsideRoots, root)
	mustReject(t, filepath.Join(link, "child"), ErrOutsideRoots, root)

	// A dangling symlink pointing outside cannot be canonicalized safely and
	// fails closed.
	dangling := filepath.Join(root, "dangling")
	if err := os.Symlink(filepath.Join(outside, "gone"), dangling); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	mustReject(t, dangling, ErrInvalidPath, root)
}

func TestResolveAuthorizedPathHandlesNonexistentDescendants(t *testing.T) {
	root := newRoot(t)

	// A not-yet-existing target under the root: the nearest existing ancestor
	// (the root) is canonical and the full path stays inside.
	planned := filepath.Join(root, "new-project", "deep", "child")
	if resolved := mustResolve(t, planned, root); resolved.Path != planned {
		t.Errorf("nonexistent descendant = %+v, want %q", resolved, planned)
	}

	// The same shape outside the root is still refused.
	mustReject(t, filepath.Join(filepath.Dir(root), "nowhere", "child"), ErrOutsideRoots, root)

	// A path leading through an existing regular file can never come into
	// existence and is rejected as ambiguous.
	file := filepath.Join(root, "file.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	mustReject(t, filepath.Join(file, "sub"), ErrInvalidPath, root)
}

func TestTranslateWindowsPath(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{`C:\work\project`, "/mnt/c/work/project"},
		{`c:/work/project`, "/mnt/c/work/project"},
		{`C:\`, "/mnt/c"},
		{`Z:\data`, "/mnt/z/data"},
		{"/mnt/c/work/project", "/mnt/c/work/project"},
		{"/opt/linux/path", "/opt/linux/path"},
	}
	for _, tc := range cases {
		got, err := translateWindowsPath(tc.input)
		if err != nil {
			t.Errorf("translateWindowsPath(%q) failed: %v", tc.input, err)
			continue
		}
		if got != tc.want {
			t.Errorf("translateWindowsPath(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}

	for _, ambiguous := range []string{`C:relative`, `C:`, `\\server\share\proj`, `/mixed\separator`} {
		if _, err := translateWindowsPath(ambiguous); !errors.Is(err, ErrInvalidPath) {
			t.Errorf("translateWindowsPath(%q) error = %v, want ErrInvalidPath", ambiguous, err)
		}
	}
}

func TestResolveAuthorizedPathNormalizesWSLDrivePaths(t *testing.T) {
	// Drive paths are translated to their WSL mount form before
	// authorization, so a C:\ input must reach the same decision as its
	// /mnt/<drive> equivalent. With no registered roots both are outside,
	// while a relative input stays invalid.
	mustReject(t, `C:\work\project`, ErrOutsideRoots)
	mustReject(t, "/mnt/c/work/project", ErrOutsideRoots)
	mustReject(t, "relative/project", ErrInvalidPath)
}

func TestResolveAuthorizedPathRejectsCaseVariants(t *testing.T) {
	root := newRoot(t)
	mustResolve(t, root, root)

	// Linux is case-sensitive: a differently-cased root is a different,
	// unauthorized path. On case-insensitive mounts the string containment
	// check still refuses the variant, so this holds on both.
	upper := strings.ToUpper(root)
	if upper != root {
		mustReject(t, filepath.Join(upper, "child"), ErrOutsideRoots, root)
	}
}

// testPassword matches the password the test router configures before login.
const testPassword = "correct horse battery staple"

// openTestDB opens and migrates a throwaway database under a temp directory.
// The GORM logger is discarded because these tests deliberately exercise
// expected-error paths (such as duplicate root registration) whose warnings
// would otherwise pollute the test output.
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

// newWorkspaceRouter wires a production-shaped router: global CSRF
// enforcement, auth routes, and the workspace-root routes behind
// RequireSession.
func newWorkspaceRouter(t *testing.T) (*gin.Engine, *Service) {
	t.Helper()

	db := openTestDB(t)
	authSvc := auth.NewService(db)
	if err := authSvc.SetPassword(testPassword); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}
	svc := NewService(db)

	router := gin.New()
	router.Use(auth.RequireCSRF())
	api := router.Group("/api")
	authSvc.RegisterRoutes(api)
	protected := api.Group("", auth.RequireSession(authSvc))
	svc.RegisterRoutes(protected)
	return router, svc
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

// login authenticates the client against the test router and returns the
// CSRF token to echo in mutating requests.
func login(t *testing.T, client *http.Client, base string) string {
	t.Helper()

	resp, err := client.Get(base + "/api/auth/session")
	if err != nil {
		t.Fatalf("GET session: %v", err)
	}
	defer resp.Body.Close()
	var session struct {
		CSRFToken string `json:"csrfToken"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
		t.Fatalf("decode session body: %v", err)
	}
	if session.CSRFToken == "" {
		t.Fatal("session endpoint returned an empty CSRF token")
	}

	req, err := http.NewRequest(http.MethodPost, base+"/api/auth/login", strings.NewReader(`{"password":"`+testPassword+`"}`))
	if err != nil {
		t.Fatalf("build login request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(auth.CSRFHeaderName, session.CSRFToken)
	loginResp, err := client.Do(req)
	if err != nil {
		t.Fatalf("POST login: %v", err)
	}
	defer loginResp.Body.Close()
	if loginResp.StatusCode != http.StatusOK {
		t.Fatalf("login status = %d, want %d", loginResp.StatusCode, http.StatusOK)
	}
	return session.CSRFToken
}

// postJSON posts a JSON body with an optional CSRF header.
func postJSON(t *testing.T, client *http.Client, url, csrf, payload string) *http.Response {
	t.Helper()

	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(payload))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if csrf != "" {
		req.Header.Set(auth.CSRFHeaderName, csrf)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	return resp
}

func TestWorkspaceRootAPIRequiresSessionAndCSRF(t *testing.T) {
	router, _ := newWorkspaceRouter(t)
	client, base := newClient(t, router)

	// Unauthenticated reads are refused.
	resp, err := client.Get(base + "/api/workspaces/roots")
	if err != nil {
		t.Fatalf("GET roots: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated GET status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}

	// Mutations are refused without a CSRF token even before auth matters.
	csrfless := postJSON(t, client, base+"/api/workspaces/roots", "", `{}`)
	defer csrfless.Body.Close()
	if csrfless.StatusCode != http.StatusForbidden {
		t.Fatalf("POST without CSRF status = %d, want %d", csrfless.StatusCode, http.StatusForbidden)
	}
}

func TestWorkspaceRootAPIAddListRemove(t *testing.T) {
	router, _ := newWorkspaceRouter(t)
	client, base := newClient(t, router)
	csrf := login(t, client, base)

	// The list starts empty.
	resp, err := client.Get(base + "/api/workspaces/roots")
	if err != nil {
		t.Fatalf("GET roots: %v", err)
	}
	var list struct {
		Roots []Root `json:"roots"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatalf("decode list body: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET roots status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if len(list.Roots) != 0 {
		t.Fatalf("fresh platform reports %d roots, want 0", len(list.Roots))
	}

	root := newRoot(t)
	payload, err := json.Marshal(map[string]string{"path": root})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	// Registering an existing directory succeeds and echoes the canonical
	// path back.
	added := postJSON(t, client, base+"/api/workspaces/roots", csrf, string(payload))
	defer added.Body.Close()
	if added.StatusCode != http.StatusCreated {
		t.Fatalf("POST roots status = %d, want %d", added.StatusCode, http.StatusCreated)
	}
	var created Root
	if err := json.NewDecoder(added.Body).Decode(&created); err != nil {
		t.Fatalf("decode add body: %v", err)
	}
	if created.ID == 0 || created.Path != root {
		t.Errorf("created root = %+v, want non-zero id and path %q", created, root)
	}

	// The root shows up in the list.
	resp, err = client.Get(base + "/api/workspaces/roots")
	if err != nil {
		t.Fatalf("GET roots: %v", err)
	}
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatalf("decode list body: %v", err)
	}
	resp.Body.Close()
	if len(list.Roots) != 1 || list.Roots[0].Path != root {
		t.Errorf("roots after add = %+v, want exactly %q", list.Roots, root)
	}

	// Registering the same path again is a conflict.
	duplicate := postJSON(t, client, base+"/api/workspaces/roots", csrf, string(payload))
	defer duplicate.Body.Close()
	if duplicate.StatusCode != http.StatusConflict {
		t.Errorf("duplicate add status = %d, want %d", duplicate.StatusCode, http.StatusConflict)
	}

	// Non-existent and non-directory paths are refused.
	missing := postJSON(t, client, base+"/api/workspaces/roots", csrf, `{"path":"/definitely/not/here"}`)
	defer missing.Body.Close()
	if missing.StatusCode != http.StatusBadRequest {
		t.Errorf("missing root add status = %d, want %d", missing.StatusCode, http.StatusBadRequest)
	}
	file := filepath.Join(newRoot(t), "plain.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	filePayload, err := json.Marshal(map[string]string{"path": file})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	notDir := postJSON(t, client, base+"/api/workspaces/roots", csrf, string(filePayload))
	defer notDir.Body.Close()
	if notDir.StatusCode != http.StatusBadRequest {
		t.Errorf("file root add status = %d, want %d", notDir.StatusCode, http.StatusBadRequest)
	}

	// Removal by id empties the list again.
	req, err := http.NewRequest(http.MethodDelete, base+"/api/workspaces/roots/"+strconv.FormatUint(uint64(created.ID), 10), nil)
	if err != nil {
		t.Fatalf("build delete request: %v", err)
	}
	req.Header.Set(auth.CSRFHeaderName, csrf)
	removed, err := client.Do(req)
	if err != nil {
		t.Fatalf("DELETE root: %v", err)
	}
	removed.Body.Close()
	if removed.StatusCode != http.StatusNoContent {
		t.Errorf("DELETE root status = %d, want %d", removed.StatusCode, http.StatusNoContent)
	}

	resp, err = client.Get(base + "/api/workspaces/roots")
	if err != nil {
		t.Fatalf("GET roots: %v", err)
	}
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatalf("decode list body: %v", err)
	}
	resp.Body.Close()
	if len(list.Roots) != 0 {
		t.Errorf("roots after remove = %+v, want empty", list.Roots)
	}

	// Removing an unknown id is 404.
	req, err = http.NewRequest(http.MethodDelete, base+"/api/workspaces/roots/9999", nil)
	if err != nil {
		t.Fatalf("build delete request: %v", err)
	}
	req.Header.Set(auth.CSRFHeaderName, csrf)
	unknown, err := client.Do(req)
	if err != nil {
		t.Fatalf("DELETE unknown root: %v", err)
	}
	unknown.Body.Close()
	if unknown.StatusCode != http.StatusNotFound {
		t.Errorf("DELETE unknown root status = %d, want %d", unknown.StatusCode, http.StatusNotFound)
	}
}

func TestAddRootStoresCanonicalPath(t *testing.T) {
	target := newRoot(t)
	link := filepath.Join(filepath.Dir(target), filepath.Base(target)+"-link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	svc := NewService(openTestDB(t))
	root, err := svc.AddRoot(link)
	if err != nil {
		t.Fatalf("AddRoot(%q): %v", link, err)
	}
	if root.Path != target {
		t.Errorf("AddRoot stored path = %q, want canonical %q", root.Path, target)
	}

	// Registering the canonical form afterwards is the same root.
	if _, err := svc.AddRoot(target); !errors.Is(err, ErrRootExists) {
		t.Errorf("second AddRoot error = %v, want ErrRootExists", err)
	}

	// Removal frees the unique path for re-registration.
	if err := svc.RemoveRoot(root.ID); err != nil {
		t.Fatalf("RemoveRoot: %v", err)
	}
	if _, err := svc.AddRoot(target); err != nil {
		t.Fatalf("re-add after remove: %v", err)
	}
	if err := svc.RemoveRoot(99999); !errors.Is(err, ErrRootNotFound) {
		t.Errorf("RemoveRoot(unknown) error = %v, want ErrRootNotFound", err)
	}
}
