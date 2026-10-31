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

func TestGitCommitsAndDiff(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git CLI unavailable")
	}

	db := openTestDB(t)
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}
	projectDir := filepath.Join(root, "repo")
	if err := os.Mkdir(projectDir, 0o755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	svc := NewService(db)
	project, err := svc.Register(projectDir)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = projectDir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}

	git("init", "-b", "main", "-q")
	git("config", "user.email", "dev@example.com")
	git("config", "user.name", "Developer")

	// Commit 1: Initial commit
	if err := os.WriteFile(filepath.Join(projectDir, "hello.txt"), []byte("line 1\nline 2\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	git("add", "hello.txt")
	git("commit", "-qm", "feat: initial commit")

	// Commit 2: Second commit
	if err := os.WriteFile(filepath.Join(projectDir, "hello.txt"), []byte("line 1\nline 2 modified\nline 3\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "new_file.txt"), []byte("new file content\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	git("add", "hello.txt", "new_file.txt")
	git("commit", "-qm", "feat: update hello and add new_file")

	ctx := context.Background()

	// 1. Verify GitCommits
	commits, err := svc.GitCommits(ctx, project.ID, 10)
	if err != nil {
		t.Fatalf("GitCommits: %v", err)
	}
	if len(commits) != 2 {
		t.Fatalf("expected 2 commits, got %d: %+v", len(commits), commits)
	}
	if commits[0].Message != "feat: update hello and add new_file" {
		t.Errorf("commit[0] message = %q, want 'feat: update hello and add new_file'", commits[0].Message)
	}
	if commits[1].Message != "feat: initial commit" {
		t.Errorf("commit[1] message = %q, want 'feat: initial commit'", commits[1].Message)
	}
	if commits[0].Author != "Developer" || commits[0].ShortHash == "" {
		t.Errorf("commit[0] fields unexpected: %+v", commits[0])
	}

	// 2. Verify GitStatusWithRef for commit 2
	statusItems, err := svc.GitStatusWithRef(ctx, project.ID, commits[0].Hash)
	if err != nil {
		t.Fatalf("GitStatusWithRef(commit2): %v", err)
	}
	if len(statusItems) != 2 {
		t.Fatalf("expected 2 changed files in commit 2, got %d: %+v", len(statusItems), statusItems)
	}

	// 3. Verify GitFileDiff for commit 2
	diffResp, err := svc.GitFileDiff(ctx, project.ID, "hello.txt", commits[0].Hash)
	if err != nil {
		t.Fatalf("GitFileDiff(hello.txt, commit2): %v", err)
	}
	if diffResp.Status != "modified" || diffResp.Additions < 1 || diffResp.Deletions < 1 {
		t.Errorf("unexpected diffResp for hello.txt: %+v", diffResp)
	}

	// 4. Test working tree modifications and untracked file diff
	if err := os.WriteFile(filepath.Join(projectDir, "untracked.txt"), []byte("untracked line 1\nuntracked line 2\n"), 0o644); err != nil {
		t.Fatalf("write untracked: %v", err)
	}

	untrackedDiff, err := svc.GitFileDiff(ctx, project.ID, "untracked.txt", "")
	if err != nil {
		t.Fatalf("GitFileDiff(untracked.txt): %v", err)
	}
	if untrackedDiff.Status != "untracked" || untrackedDiff.Additions != 2 {
		t.Errorf("unexpected untracked diff: %+v", untrackedDiff)
	}

	// 5. Verify HTTP Handlers
	gin.SetMode(gin.TestMode)
	router := gin.New()
	svc.RegisterRoutes(router)

	// Commits route
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/projects/"+strconv.FormatUint(uint64(project.ID), 10)+"/git/commits", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /git/commits status = %d, body = %s", rec.Code, rec.Body.String())
	}

	// Status with commit query
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/projects/"+strconv.FormatUint(uint64(project.ID), 10)+"/git/status?commit="+commits[0].Hash, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /git/status?commit=... status = %d, body = %s", rec.Code, rec.Body.String())
	}

	// Diff route
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/projects/"+strconv.FormatUint(uint64(project.ID), 10)+"/git/diff?path=hello.txt&commit="+commits[0].Hash, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /git/diff status = %d, body = %s", rec.Code, rec.Body.String())
	}
}
