package sessions

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"devplatform/internal/app/instances"
	"devplatform/internal/app/projects"
	"devplatform/internal/core/agents"
)

func runGitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func initWorktreeTestRepo(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	runGitIn(t, dir, "init")
	runGitIn(t, dir, "config", "user.email", "test@example.com")
	runGitIn(t, dir, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# demo\n"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	runGitIn(t, dir, "add", "-A")
	runGitIn(t, dir, "commit", "-m", "initial")
}

// TestCreateInWorktreeUsesASeparateInstance pins the parallel-development
// invariant: sessions in different worktrees never share an agent instance and
// the instance's directory is the worktree, not the project root.
func TestCreateInWorktreeUsesASeparateInstance(t *testing.T) {
	db := openTestDB(t)
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}
	projectDir := filepath.Join(root, "alpha")
	initWorktreeTestRepo(t, projectDir)

	projectService := projects.NewService(db)
	project, err := projectService.Register(projectDir)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	worktree, err := projectService.CreateWorktree(context.Background(), project.ID, projects.CreateWorktreeInput{
		Branch: "feature/parallel",
	})
	if err != nil {
		t.Fatalf("CreateWorktree: %v", err)
	}

	registry := agents.NewRegistry()
	fake := newFakeAdapter()
	if err := registry.Register(agents.BackendPi, fake); err != nil {
		t.Fatalf("register fake: %v", err)
	}
	next := 0
	fake.createFn = func(instanceID string, _ agents.CreateSessionOptions) (*agents.SessionRef, error) {
		next++
		return &agents.SessionRef{
			InstanceID: instanceID,
			Backend:    agents.BackendPi,
			SessionID:  fmt.Sprintf("native-%d", next),
		}, nil
	}

	instanceService := instances.NewService(db, registry, projectService)
	svc := NewService(db, registry, instanceService)

	worktreeOutcome, err := svc.CreateInWorktree(
		context.Background(), project.ID, worktree.ID, agents.BackendPi, "worktree session", "", "",
	)
	if err != nil {
		t.Fatalf("CreateInWorktree: %v", err)
	}
	if worktreeOutcome.Session.WorktreeID != worktree.ID {
		t.Fatalf("session worktreeId = %d, want %d", worktreeOutcome.Session.WorktreeID, worktree.ID)
	}
	worktreeInstance, err := instanceService.Get(worktreeOutcome.Session.InstanceID)
	if err != nil {
		t.Fatalf("Get worktree instance: %v", err)
	}
	if worktreeInstance.Directory != worktree.Path {
		t.Fatalf("instance directory = %q, want %q", worktreeInstance.Directory, worktree.Path)
	}
	if worktreeInstance.WorktreeID != worktree.ID {
		t.Fatalf("instance worktreeId = %d, want %d", worktreeInstance.WorktreeID, worktree.ID)
	}

	mainOutcome, err := svc.Create(context.Background(), project.ID, agents.BackendPi, "main session", "", "")
	if err != nil {
		t.Fatalf("Create main: %v", err)
	}
	if mainOutcome.Session.WorktreeID != 0 {
		t.Fatalf("main session worktreeId = %d, want 0", mainOutcome.Session.WorktreeID)
	}
	if mainOutcome.Session.InstanceID == worktreeOutcome.Session.InstanceID {
		t.Fatal("main and worktree sessions shared one instance")
	}
	mainInstance, err := instanceService.Get(mainOutcome.Session.InstanceID)
	if err != nil {
		t.Fatalf("Get main instance: %v", err)
	}
	if mainInstance.Directory != project.Path {
		t.Fatalf("main instance directory = %q, want %q", mainInstance.Directory, project.Path)
	}
}
