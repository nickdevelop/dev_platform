package projects

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"devplatform/internal/app/paths"
	"devplatform/internal/core/platform"
)

// Worktree errors. Handlers map them to stable HTTP error codes.
var (
	// ErrWorktreeNotFound reports an operation on a worktree that is not
	// registered for the given project.
	ErrWorktreeNotFound = errors.New("projects: worktree not found")
	// ErrWorktreeExists reports that the requested branch or directory is
	// already registered as a worktree.
	ErrWorktreeExists = errors.New("projects: worktree already exists")
	// ErrWorktreeDirty reports that a worktree still has uncommitted or
	// untracked changes, so removing it would discard work.
	ErrWorktreeDirty = errors.New("projects: worktree has uncommitted changes")
)

// Worktree is the JSON-stable representation of one platform-managed git
// worktree. Available is computed when listing: a worktree whose directory was
// deleted outside the platform stays visible with Available false.
type Worktree struct {
	ID         uint      `json:"id"`
	ProjectID  uint      `json:"projectId"`
	Path       string    `json:"path"`
	Branch     string    `json:"branch"`
	BaseBranch string    `json:"baseBranch,omitempty"`
	Available  bool      `json:"available"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

// CreateWorktreeInput is the validated request for creating one worktree.
type CreateWorktreeInput struct {
	// Branch is the new branch checked out in the worktree.
	Branch string `json:"branch"`
	// BaseBranch is the branch the new branch starts from. Empty means HEAD.
	BaseBranch string `json:"baseBranch,omitempty"`
	// Path optionally overrides the default sibling directory. Empty means
	// <project parent>/<repo>-worktrees/<branch slug>.
	Path string `json:"path,omitempty"`
}

var worktreeSlugUnsafe = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

// ListWorktrees returns the project's registered worktrees in creation order.
// Availability is probed here rather than stored, so a directory removed
// outside the platform is reported honestly.
func (s *Service) ListWorktrees(ctx context.Context, projectID uint) ([]Worktree, error) {
	if _, err := s.load(projectID); err != nil {
		return nil, err
	}
	list, err := s.repo.ListWorktrees(projectID)
	if err != nil {
		return nil, err
	}
	for i := range list {
		list[i].Available = directoryExists(list[i].Path)
	}
	return list, nil
}

// CreateWorktree creates a git worktree with a new branch for the project.
// The project must be the root of a git work tree. The directory is created by
// git; an existing target is never reused or overwritten.
func (s *Service) CreateWorktree(ctx context.Context, projectID uint, input CreateWorktreeInput) (Worktree, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	row, err := s.load(projectID)
	if err != nil {
		return Worktree{}, err
	}
	if err := s.requireGitRoot(ctx, row.Path); err != nil {
		return Worktree{}, err
	}

	branch := strings.TrimSpace(input.Branch)
	if !validateBranchName(branch) {
		return Worktree{}, ErrInvalidBranchName
	}

	baseBranch := strings.TrimSpace(input.BaseBranch)
	if baseBranch == "" {
		baseBranch, err = gitOutput(ctx, row.Path, "rev-parse", "--abbrev-ref", "HEAD")
		if err != nil || baseBranch == "" || baseBranch == "HEAD" {
			baseBranch = "HEAD"
		}
	} else if baseBranch != "HEAD" && !validateBranchName(baseBranch) {
		return Worktree{}, ErrInvalidBranchName
	}

	target, err := s.resolveWorktreeTarget(row, branch, strings.TrimSpace(input.Path))
	if err != nil {
		return Worktree{}, err
	}

	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return Worktree{}, fmt.Errorf("projects: create worktree parent for %q: %w", target, err)
	}
	if _, err := os.Lstat(target); err == nil {
		return Worktree{}, fmt.Errorf("%w: %q", ErrWorktreeExists, target)
	} else if !errors.Is(err, os.ErrNotExist) {
		return Worktree{}, fmt.Errorf("projects: inspect worktree target %q: %w", target, err)
	}

	if _, err := gitRun(ctx, row.Path, "worktree", "add", "-b", branch, target, baseBranch); err != nil {
		// git may have created the target before failing; it did not exist
		// before this call, so removing it cannot discard user data.
		_ = os.RemoveAll(target)
		return Worktree{}, err
	}

	created, err := s.repo.CreateWorktree(Worktree{
		ProjectID:  projectID,
		Path:       target,
		Branch:     branch,
		BaseBranch: baseBranch,
	})
	if err != nil {
		// The record is the platform's source of truth; compensate so a
		// failed insert does not leave an unmanaged worktree behind.
		_, _ = gitRun(ctx, row.Path, "worktree", "remove", "--force", target)
		_, _ = gitRun(ctx, row.Path, "branch", "-D", branch)
		return Worktree{}, err
	}
	created.Available = directoryExists(created.Path)
	return created, nil
}

// RemoveWorktree stops every live instance in the worktree, refuses a dirty
// working tree, removes the directory through git, and finally drops the
// platform record. The branch is deliberately kept.
func (s *Service) RemoveWorktree(ctx context.Context, projectID, worktreeID uint) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	row, err := s.load(projectID)
	if err != nil {
		return err
	}
	worktree, err := s.repo.GetWorktree(projectID, worktreeID)
	if err != nil {
		return err
	}

	pathExists := directoryExists(worktree.Path)
	if pathExists {
		out, statusErr := gitOutputLimit(ctx, worktree.Path, 1<<20, "status", "--porcelain")
		if statusErr != nil {
			return statusErr
		}
		if strings.TrimSpace(out) != "" {
			return fmt.Errorf("%w: %q", ErrWorktreeDirty, worktree.Path)
		}
	}

	if s.stopper == nil {
		return fmt.Errorf("%w: no instance stopper is configured", ErrInstanceStop)
	}
	if err := s.stopper.StopProjectWorktreeInstances(ctx, projectID, worktreeID); err != nil {
		return fmt.Errorf("%w: %v", ErrInstanceStop, err)
	}

	if pathExists {
		if _, err := gitRun(ctx, row.Path, "worktree", "remove", worktree.Path); err != nil {
			return err
		}
	} else {
		// The directory is already gone; prune git's stale admin entry.
		_, _ = gitRun(ctx, row.Path, "worktree", "prune")
	}

	return s.repo.DeleteWorktree(projectID, worktreeID)
}

// ResolveWorktreeDirectory returns the canonical working directory of one
// worktree. It is the gate the instance lifecycle uses for worktree-scoped
// sessions: an unknown project/worktree or a missing directory fails rather
// than yielding a directory the platform must not touch.
func (s *Service) ResolveWorktreeDirectory(projectID, worktreeID uint) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, err := s.load(projectID); err != nil {
		return "", err
	}
	worktree, err := s.repo.GetWorktree(projectID, worktreeID)
	if err != nil {
		return "", err
	}
	if err := s.authorizeStoredPath(worktree.Path); err != nil {
		return "", err
	}
	if err := s.requireDirectory(worktree.Path); err != nil {
		return "", err
	}
	return worktree.Path, nil
}

// workspaceRoot resolves the filesystem root for a project operation. A zero
// worktree id means the project's main working directory. The worktree lookup
// is always scoped to the project, so cross-project ids cannot be used.
func (s *Service) workspaceRoot(projectID uint, worktreeID ...uint) (string, error) {
	row, err := s.load(projectID)
	if err != nil {
		return "", err
	}
	if len(worktreeID) == 0 || worktreeID[0] == 0 {
		return row.Path, nil
	}
	worktree, err := s.repo.GetWorktree(projectID, worktreeID[0])
	if err != nil {
		return "", err
	}
	if err := s.authorizeStoredPath(worktree.Path); err != nil {
		return "", err
	}
	return worktree.Path, nil
}

// requireGitRoot fails unless directory is the root of a git work tree. A
// project that is merely a subdirectory of a repository is refused: creating a
// worktree from it would place the default directory inside the repository.
func (s *Service) requireGitRoot(ctx context.Context, directory string) error {
	if !s.isGitDir(ctx, directory) {
		return ErrNotGitRepo
	}
	root, err := gitOutput(ctx, directory, "rev-parse", "--show-toplevel")
	if err != nil {
		return ErrNotGitRepo
	}
	if !platform.New().Dirs().SamePath(root, directory) {
		return ErrNotGitRepo
	}
	return nil
}

// resolveWorktreeTarget validates a caller-supplied target or derives the
// default sibling directory. The target must not exist and must not be inside
// the project's own working tree.
func (s *Service) resolveWorktreeTarget(project Project, branch, requested string) (string, error) {
	target := strings.TrimSpace(requested)
	if target == "" {
		target = filepath.Join(
			filepath.Dir(project.Path),
			filepath.Base(project.Path)+"-worktrees",
			worktreeSlug(branch),
		)
	}
	resolved, err := paths.Resolve(target)
	if err != nil {
		return "", err
	}
	if platform.New().Dirs().SamePath(resolved, project.Path) || pathWithin(project.Path, resolved) {
		return "", fmt.Errorf("%w: %q is inside the project", ErrInvalidTarget, resolved)
	}
	if _, err := os.Lstat(resolved); err == nil {
		return "", fmt.Errorf("%w: %q", ErrWorktreeExists, resolved)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("projects: inspect worktree target %q: %w", resolved, err)
	}
	return resolved, nil
}

// worktreeSlug turns a branch name into one safe directory name.
func worktreeSlug(branch string) string {
	slug := strings.Trim(worktreeSlugUnsafe.ReplaceAllString(branch, "-"), "-.")
	if slug == "" {
		return "worktree"
	}
	if len(slug) > 120 {
		slug = strings.Trim(slug[:120], "-.")
		if slug == "" {
			return "worktree"
		}
	}
	return slug
}

// directoryExists reports whether path exists and is a directory.
func directoryExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
