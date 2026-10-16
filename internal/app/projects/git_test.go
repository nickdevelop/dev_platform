package projects

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
)

// gitAvailable reports whether the git CLI can run here. Probe tests that
// need a real repository skip when it cannot.
func gitAvailable() bool {
	return exec.Command("git", "--version").Run() == nil
}

// initRepo creates a git repository with one committed file and returns its
// path. The caller must set a branch name explicitly: the test env's default
// branch is not guaranteed.
func initRepo(t *testing.T, branch string) string {
	t.Helper()

	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	run("init", "-b", branch, "-q")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "test")
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	run("add", "file.txt")
	run("commit", "-qm", "init")
	return dir
}

func TestProbeGitDirNotARepo(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git CLI unavailable")
	}
	info := probeGitDir(context.Background(), t.TempDir())
	if info.IsGit {
		t.Fatal("IsGit = true, want false for a plain directory")
	}
}

func TestProbeGitDirEmpty(t *testing.T) {
	if info := probeGitDir(context.Background(), ""); info.IsGit {
		t.Fatal("IsGit = true, want false for an empty directory")
	}
}

func TestProbeGitDirBranchAndDirty(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git CLI unavailable")
	}
	dir := initRepo(t, "main")
	info := probeGitDir(context.Background(), dir)
	if !info.IsGit {
		t.Fatal("IsGit = false, want true")
	}
	if info.Branch != "main" {
		t.Errorf("Branch = %q, want %q", info.Branch, "main")
	}
	if info.Dirty {
		t.Error("Dirty = true, want false on a clean tree")
	}
	if info.Root == "" {
		t.Error("Root is empty, want the work-tree top level")
	}

	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("modified"), 0o644); err != nil {
		t.Fatalf("modify file: %v", err)
	}
	if info := probeGitDir(context.Background(), dir); !info.Dirty {
		t.Error("Dirty = false, want true after modifying a tracked file")
	}
}

// TestGitInfoRoute pins the HTTP surface: a git work tree reports its branch,
// and an unknown project is a 404.
func TestGitInfoRoute(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git CLI unavailable")
	}
	db := openTestDB(t)
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}
	projectDir := filepath.Join(root, "alpha")
	if err := os.Mkdir(projectDir, 0o755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	svc := NewService(db)
	project, err := svc.Register(projectDir)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	svc.RegisterRoutes(router)
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = projectDir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	git("init", "-b", "trunk", "-q")
	git("config", "user.email", "test@example.com")
	git("config", "user.name", "test")

	path := "/projects/" + strconv.FormatUint(uint64(project.ID), 10) + "/git-info"
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	var body GitInfo
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", rec.Body.String(), err)
	}
	if !body.IsGit || body.Branch != "trunk" {
		t.Fatalf("git info = %+v, want a git tree on trunk", body)
	}

	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/projects/9999/git-info", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown project status = %d, want 404", rec.Code)
	}
}
