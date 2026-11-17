package projects

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// initGitRepo creates a git repository with one commit and returns its
// canonical directory.
func initGitRepo(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# demo\n"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-m", "initial")
	return dir
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

func TestProjectIsGitFlag(t *testing.T) {
	svc, _, _, _ := newTestService(t)
	gitDir := initGitRepo(t)
	plainDir := t.TempDir()

	gitProject, err := svc.Register(gitDir)
	if err != nil {
		t.Fatalf("Register git: %v", err)
	}
	if !gitProject.IsGit {
		t.Fatal("git project IsGit = false, want true")
	}
	plainProject, err := svc.Register(plainDir)
	if err != nil {
		t.Fatalf("Register plain: %v", err)
	}
	if plainProject.IsGit {
		t.Fatal("plain project IsGit = true, want false")
	}
	list, err := svc.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, project := range list {
		if project.ID == gitProject.ID && !project.IsGit {
			t.Fatal("List lost the git flag")
		}
		if project.ID == plainProject.ID && project.IsGit {
			t.Fatal("List invented a git flag")
		}
	}
}

func TestCreateWorktreeDefaultPathAndList(t *testing.T) {
	svc, _, _, _ := newTestService(t)
	repo := initGitRepo(t)
	project, err := svc.Register(repo)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	worktree, err := svc.CreateWorktree(context.Background(), project.ID, CreateWorktreeInput{
		Branch: "feature/login",
	})
	if err != nil {
		t.Fatalf("CreateWorktree: %v", err)
	}
	if worktree.Branch != "feature/login" {
		t.Fatalf("branch = %q, want feature/login", worktree.Branch)
	}
	wantParent := filepath.Join(filepath.Dir(repo), filepath.Base(repo)+"-worktrees")
	if !pathWithin(wantParent, worktree.Path) {
		t.Fatalf("path = %q, want under %q", worktree.Path, wantParent)
	}
	if !directoryExists(worktree.Path) {
		t.Fatalf("worktree directory %q missing", worktree.Path)
	}
	if got := runGit(t, worktree.Path, "rev-parse", "--abbrev-ref", "HEAD"); got != "feature/login" {
		t.Fatalf("worktree branch = %q, want feature/login", got)
	}

	list, err := svc.ListWorktrees(context.Background(), project.ID)
	if err != nil {
		t.Fatalf("ListWorktrees: %v", err)
	}
	if len(list) != 1 || list[0].ID != worktree.ID || !list[0].Available {
		t.Fatalf("ListWorktrees = %+v", list)
	}

	resolved, err := svc.ResolveWorktreeDirectory(project.ID, worktree.ID)
	if err != nil {
		t.Fatalf("ResolveWorktreeDirectory: %v", err)
	}
	if resolved != worktree.Path {
		t.Fatalf("resolved = %q, want %q", resolved, worktree.Path)
	}
}

func TestCreateWorktreeCustomPathAndValidation(t *testing.T) {
	svc, _, _, _ := newTestService(t)
	repo := initGitRepo(t)
	project, err := svc.Register(repo)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	custom := filepath.Join(t.TempDir(), "custom-worktree")
	worktree, err := svc.CreateWorktree(context.Background(), project.ID, CreateWorktreeInput{
		Branch: "feature/custom",
		Path:   custom,
	})
	if err != nil {
		t.Fatalf("CreateWorktree custom: %v", err)
	}
	if worktree.Path != custom {
		t.Fatalf("path = %q, want %q", worktree.Path, custom)
	}

	if _, err := svc.CreateWorktree(context.Background(), project.ID, CreateWorktreeInput{
		Branch: "feature/custom",
	}); !errors.Is(err, ErrWorktreeExists) && !errors.Is(err, ErrGitOperationFailed) {
		t.Fatalf("duplicate branch error = %v", err)
	}

	if _, err := svc.CreateWorktree(context.Background(), project.ID, CreateWorktreeInput{
		Branch: "feature/inside",
		Path:   filepath.Join(repo, "nested-worktree"),
	}); !errors.Is(err, ErrInvalidTarget) {
		t.Fatalf("inside-project target error = %v, want ErrInvalidTarget", err)
	}

	if _, err := svc.CreateWorktree(context.Background(), project.ID, CreateWorktreeInput{
		Branch: "feature/x",
		Path:   filepath.Join(repo, "..", filepath.Base(repo), "inside-again"),
	}); !errors.Is(err, ErrInvalidTarget) {
		t.Fatalf("normalized inside target error = %v, want ErrInvalidTarget", err)
	}
}

func TestCreateWorktreeRejectsNonGitProject(t *testing.T) {
	svc, _, root, _ := newTestService(t)
	project, err := svc.Register(root)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if _, err := svc.CreateWorktree(context.Background(), project.ID, CreateWorktreeInput{
		Branch: "feature/x",
	}); !errors.Is(err, ErrNotGitRepo) {
		t.Fatalf("error = %v, want ErrNotGitRepo", err)
	}
}

func TestRemoveWorktreeRefusesDirtyAndKeepsBranch(t *testing.T) {
	svc, _, _, stopper := newTestService(t)
	repo := initGitRepo(t)
	project, err := svc.Register(repo)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	worktree, err := svc.CreateWorktree(context.Background(), project.ID, CreateWorktreeInput{
		Branch: "feature/remove",
	})
	if err != nil {
		t.Fatalf("CreateWorktree: %v", err)
	}

	if err := os.WriteFile(filepath.Join(worktree.Path, "dirty.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write dirty file: %v", err)
	}
	if err := svc.RemoveWorktree(context.Background(), project.ID, worktree.ID); !errors.Is(err, ErrWorktreeDirty) {
		t.Fatalf("dirty remove error = %v, want ErrWorktreeDirty", err)
	}
	if len(stopper.stopCalls()) != 0 {
		t.Fatal("dirty removal stopped instances before refusing")
	}

	if err := os.Remove(filepath.Join(worktree.Path, "dirty.txt")); err != nil {
		t.Fatalf("remove dirty file: %v", err)
	}
	if err := svc.RemoveWorktree(context.Background(), project.ID, worktree.ID); err != nil {
		t.Fatalf("RemoveWorktree: %v", err)
	}
	if directoryExists(worktree.Path) {
		t.Fatal("worktree directory still exists")
	}
	if list, _ := svc.ListWorktrees(context.Background(), project.ID); len(list) != 0 {
		t.Fatalf("worktree record survived removal: %+v", list)
	}
	// The branch is deliberately preserved.
	if got := runGit(t, repo, "branch", "--list", "feature/remove"); got == "" {
		t.Fatal("branch was deleted with the worktree")
	}
}

func TestWorkspaceScopeSeparatesWorktreeFilesAndGit(t *testing.T) {
	svc, _, _, _ := newTestService(t)
	repo := initGitRepo(t)
	project, err := svc.Register(repo)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	worktree, err := svc.CreateWorktree(context.Background(), project.ID, CreateWorktreeInput{
		Branch: "feature/scope",
	})
	if err != nil {
		t.Fatalf("CreateWorktree: %v", err)
	}
	if err := os.WriteFile(filepath.Join(worktree.Path, "only-in-worktree.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write worktree file: %v", err)
	}

	mainFiles, err := svc.SearchFiles(context.Background(), project.ID, "only-in-worktree", 50)
	if err != nil {
		t.Fatalf("SearchFiles main: %v", err)
	}
	if len(mainFiles) != 0 {
		t.Fatalf("main checkout saw worktree file: %+v", mainFiles)
	}
	wtFiles, err := svc.SearchFiles(context.Background(), project.ID, "only-in-worktree", 50, worktree.ID)
	if err != nil {
		t.Fatalf("SearchFiles worktree: %v", err)
	}
	if len(wtFiles) != 1 || wtFiles[0].Path != "only-in-worktree.txt" {
		t.Fatalf("worktree files = %+v", wtFiles)
	}

	mainStatus, err := svc.GitStatus(context.Background(), project.ID)
	if err != nil {
		t.Fatalf("GitStatus main: %v", err)
	}
	for _, item := range mainStatus {
		if item.Path == "only-in-worktree.txt" {
			t.Fatal("main git status saw the worktree's untracked file")
		}
	}
	wtStatus, err := svc.GitStatus(context.Background(), project.ID, worktree.ID)
	if err != nil {
		t.Fatalf("GitStatus worktree: %v", err)
	}
	found := false
	for _, item := range wtStatus {
		if item.Path == "only-in-worktree.txt" && item.Status == "untracked" {
			found = true
		}
	}
	if !found {
		t.Fatalf("worktree git status = %+v, want untracked only-in-worktree.txt", wtStatus)
	}

	info, err := svc.GitInfo(context.Background(), project.ID, worktree.ID)
	if err != nil {
		t.Fatalf("GitInfo worktree: %v", err)
	}
	if !info.IsGit || info.Branch != "feature/scope" {
		t.Fatalf("GitInfo worktree = %+v", info)
	}
}

func TestWorktreeScopeRejectsAnotherProjectsWorktree(t *testing.T) {
	svc, _, _, _ := newTestService(t)
	repoA := initGitRepo(t)
	repoB := initGitRepo(t)
	projectA, err := svc.Register(repoA)
	if err != nil {
		t.Fatalf("Register A: %v", err)
	}
	projectB, err := svc.Register(repoB)
	if err != nil {
		t.Fatalf("Register B: %v", err)
	}
	worktree, err := svc.CreateWorktree(context.Background(), projectA.ID, CreateWorktreeInput{
		Branch: "feature/a",
	})
	if err != nil {
		t.Fatalf("CreateWorktree: %v", err)
	}

	if _, err := svc.ResolveWorktreeDirectory(projectB.ID, worktree.ID); !errors.Is(err, ErrWorktreeNotFound) {
		t.Fatalf("cross-project resolve error = %v, want ErrWorktreeNotFound", err)
	}
	if _, err := svc.SearchFiles(context.Background(), projectB.ID, "", 50, worktree.ID); !errors.Is(err, ErrWorktreeNotFound) {
		t.Fatalf("cross-project scope error = %v, want ErrWorktreeNotFound", err)
	}
}

func TestCreateWorktreeWithCommitID(t *testing.T) {
	svc, _, _, _ := newTestService(t)
	repo := initGitRepo(t)

	// Make a second commit so we have two distinct commits
	file2 := filepath.Join(repo, "second.txt")
	if err := os.WriteFile(file2, []byte("second"), 0o644); err != nil {
		t.Fatalf("write second.txt: %v", err)
	}
	runGit(t, repo, "add", "second.txt")
	runGit(t, repo, "commit", "-m", "second commit")

	firstCommit := runGit(t, repo, "rev-parse", "HEAD~1")
	firstShort := runGit(t, repo, "rev-parse", "--short", "HEAD~1")

	project, err := svc.Register(repo)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	// 1. Create worktree specifying Commit and explicit branch
	wt1, err := svc.CreateWorktree(context.Background(), project.ID, CreateWorktreeInput{
		Branch: "feature/from-commit-1",
		Commit: firstCommit,
	})
	if err != nil {
		t.Fatalf("CreateWorktree with explicit branch and commit: %v", err)
	}
	if wt1.Branch != "feature/from-commit-1" {
		t.Errorf("Branch = %q, want feature/from-commit-1", wt1.Branch)
	}
	if wt1.BaseBranch != firstShort {
		t.Errorf("BaseBranch = %q, want %q", wt1.BaseBranch, firstShort)
	}
	// In wt1, second.txt should NOT exist because it was created from firstCommit
	if _, err := os.Stat(filepath.Join(wt1.Path, "second.txt")); !os.IsNotExist(err) {
		t.Errorf("second.txt unexpectedly exists in worktree created from first commit")
	}

	// 2. Create worktree specifying Commit with empty branch (should auto-derive wt-<shortHash>)
	wt2, err := svc.CreateWorktree(context.Background(), project.ID, CreateWorktreeInput{
		Commit: firstCommit,
	})
	if err != nil {
		t.Fatalf("CreateWorktree with auto-derived branch: %v", err)
	}
	wantDerived := "wt-" + firstShort
	if wt2.Branch != wantDerived {
		t.Errorf("Auto-derived Branch = %q, want %q", wt2.Branch, wantDerived)
	}

	// 3. Reject invalid commit ref
	if _, err := svc.CreateWorktree(context.Background(), project.ID, CreateWorktreeInput{
		Commit: "../illegal-ref",
	}); !errors.Is(err, ErrInvalidCommitRef) {
		t.Errorf("expected ErrInvalidCommitRef, got %v", err)
	}

	// 4. Reject non-existent commit
	if _, err := svc.CreateWorktree(context.Background(), project.ID, CreateWorktreeInput{
		Commit: "0000000000000000000000000000000000000000",
	}); !errors.Is(err, ErrCommitNotFound) {
		t.Errorf("expected ErrCommitNotFound, got %v", err)
	}

	// 5. Compatible: passing commit ID in BaseBranch should also work
	wt3, err := svc.CreateWorktree(context.Background(), project.ID, CreateWorktreeInput{
		Branch:     "feature/from-base-branch-commit",
		BaseBranch: firstCommit,
	})
	if err != nil {
		t.Fatalf("CreateWorktree with commit in BaseBranch: %v", err)
	}
	if wt3.BaseBranch != firstShort {
		t.Errorf("wt3 BaseBranch = %q, want %q", wt3.BaseBranch, firstShort)
	}
}
