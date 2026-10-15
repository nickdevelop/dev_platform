package instances

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"

	"devplatform/internal/database"
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

func TestGitInfoRejectsForeignProject(t *testing.T) {
	svc, db, _, _, _ := newTestService(t)

	inst, err := svc.Start(context.Background(), 1, "pi")
	if err != nil {
		t.Skipf("start instance: %v", err)
	}
	// Move the instance row to another project id directly: the service must
	// still refuse to report it under the original project.
	if err := db.Model(&database.AgentInstance{}).Where("id = ?", inst.ID).Update("project_id", 9999).Error; err != nil {
		t.Fatalf("move instance: %v", err)
	}
	if _, err := svc.GitInfo(context.Background(), 1, inst.ID); !errors.Is(err, ErrInstanceNotFound) {
		t.Fatalf("GitInfo = %v, want ErrInstanceNotFound", err)
	}
}

// TestGitInfoRoute pins the HTTP surface: a git work tree reports its
// branch, a foreign project is a 404, and an unknown instance is a 404.
func TestGitInfoRoute(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git CLI unavailable")
	}
	svc, _, projectID, projectDir, _ := newTestService(t)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	svc.RegisterRoutes(router)

	inst, err := svc.Start(context.Background(), projectID, "pi")
	if err != nil {
		t.Fatalf("start instance: %v", err)
	}
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

	path := "/projects/1/instances/" + inst.ID + "/git-info"
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
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/projects/9999/instances/"+inst.ID+"/git-info", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("foreign project status = %d, want 404", rec.Code)
	}

	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/projects/1/instances/nope/git-info", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown instance status = %d, want 404", rec.Code)
	}
}
