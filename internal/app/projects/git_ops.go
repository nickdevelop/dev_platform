package projects

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

var safeBranchRegex = regexp.MustCompile(`^[a-zA-Z0-9_\-\./]+$`)

// validateBranchName checks if a branch name is well-formed and safe.
func validateBranchName(branch string) bool {
	clean := strings.TrimSpace(branch)
	if clean == "" || len(clean) > 200 {
		return false
	}
	if strings.HasPrefix(clean, "-") || strings.HasPrefix(clean, ".") || strings.HasSuffix(clean, ".") || strings.HasSuffix(clean, ".lock") {
		return false
	}
	if strings.Contains(clean, "..") || strings.Contains(clean, "@{") || strings.Contains(clean, "//") {
		return false
	}
	return safeBranchRegex.MatchString(clean)
}

// validateProjectRelPath validates that relPath is within the project root.
func (s *Service) validateProjectRelPath(projectRoot, relPath string) (string, error) {
	cleanRel := filepath.ToSlash(filepath.Clean(strings.TrimSpace(relPath)))
	if cleanRel == "" || cleanRel == "." {
		return "", ErrInvalidTarget
	}
	if strings.HasPrefix(cleanRel, "../") || cleanRel == ".." {
		return "", ErrPathEscaped
	}
	full := filepath.Join(projectRoot, filepath.FromSlash(cleanRel))
	rel, err := filepath.Rel(projectRoot, full)
	if err != nil || strings.HasPrefix(rel, "..") || rel == ".." {
		return "", ErrPathEscaped
	}
	return cleanRel, nil
}

// gitRun executes a git command in the directory, capturing stdout and stderr.
func gitRun(ctx context.Context, directory string, args ...string) (string, error) {
	full := append([]string{"-C", directory}, args...)
	cmd := exec.CommandContext(ctx, "git", full...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		errText := strings.TrimSpace(stderr.String())
		if errText == "" {
			errText = strings.TrimSpace(stdout.String())
		}
		if errText == "" {
			errText = err.Error()
		}
		return "", fmt.Errorf("%w: %s", ErrGitOperationFailed, errText)
	}
	return strings.TrimSpace(stdout.String()), nil
}

// StageFiles stages files in the git workspace. If paths is empty, it stages all changes (git add -A).
func (s *Service) StageFiles(ctx context.Context, projectID uint, paths []string) error {
	row, err := s.load(projectID)
	if err != nil {
		return err
	}
	if !s.isGitDir(ctx, row.Path) {
		return ErrNotGitRepo
	}

	if len(paths) == 0 {
		_, err := gitRun(ctx, row.Path, "add", "-A")
		return err
	}

	args := []string{"add", "-A", "--"}
	for _, p := range paths {
		valid, err := s.validateProjectRelPath(row.Path, p)
		if err != nil {
			return err
		}
		args = append(args, valid)
	}

	_, err = gitRun(ctx, row.Path, args...)
	return err
}

// UnstageFiles removes files from the git staging area. If paths is empty, it unstages all files.
func (s *Service) UnstageFiles(ctx context.Context, projectID uint, paths []string) error {
	row, err := s.load(projectID)
	if err != nil {
		return err
	}
	if !s.isGitDir(ctx, row.Path) {
		return ErrNotGitRepo
	}

	// Check if repository has commits
	_, hasCommitsErr := gitOutput(ctx, row.Path, "rev-parse", "--verify", "HEAD")
	if hasCommitsErr != nil {
		// New repository with no commits yet: use git rm --cached
		if len(paths) == 0 {
			_, err := gitRun(ctx, row.Path, "rm", "-r", "--cached", "--ignore-unmatch", ".")
			return err
		}
		args := []string{"rm", "-r", "--cached", "--ignore-unmatch", "--"}
		for _, p := range paths {
			valid, err := s.validateProjectRelPath(row.Path, p)
			if err != nil {
				return err
			}
			args = append(args, valid)
		}
		_, err := gitRun(ctx, row.Path, args...)
		return err
	}

	if len(paths) == 0 {
		_, err := gitRun(ctx, row.Path, "reset", "HEAD", "--", ".")
		return err
	}

	args := []string{"reset", "HEAD", "--"}
	for _, p := range paths {
		valid, err := s.validateProjectRelPath(row.Path, p)
		if err != nil {
			return err
		}
		args = append(args, valid)
	}

	_, err = gitRun(ctx, row.Path, args...)
	return err
}

// DiscardFileChanges discards working directory changes for a specific file.
// If the file is untracked, it deletes the file from the filesystem.
// If the file is tracked, it restores it using git checkout HEAD -- <path>.
func (s *Service) DiscardFileChanges(ctx context.Context, projectID uint, relPath string) error {
	row, err := s.load(projectID)
	if err != nil {
		return err
	}
	if !s.isGitDir(ctx, row.Path) {
		return ErrNotGitRepo
	}

	valid, err := s.validateProjectRelPath(row.Path, relPath)
	if err != nil {
		return err
	}

	fullPath := filepath.Join(row.Path, filepath.FromSlash(valid))

	// Check status of this file to determine if untracked
	items, err := s.GitStatus(ctx, projectID)
	if err != nil {
		return err
	}

	isUntracked := false
	for _, it := range items {
		if it.Path == valid && it.Status == "untracked" {
			isUntracked = true
			break
		}
	}

	if isUntracked {
		// Safely delete untracked file or folder
		if err := os.RemoveAll(fullPath); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}

	// First try unstage if it was also staged
	_, _ = gitRun(ctx, row.Path, "reset", "HEAD", "--", valid)

	// Restore from HEAD / worktree
	_, err = gitRun(ctx, row.Path, "checkout", "--", valid)
	return err
}

// CommitChanges commits the staged changes with the provided commit message.
func (s *Service) CommitChanges(ctx context.Context, projectID uint, message string) (*GitCommitItem, error) {
	row, err := s.load(projectID)
	if err != nil {
		return nil, err
	}
	if !s.isGitDir(ctx, row.Path) {
		return nil, ErrNotGitRepo
	}

	cleanMsg := strings.TrimSpace(message)
	if cleanMsg == "" {
		return nil, ErrEmptyCommitMessage
	}

	if _, err := gitRun(ctx, row.Path, "commit", "-m", cleanMsg); err != nil {
		return nil, err
	}

	commits, err := s.GitCommits(ctx, projectID, 1)
	if err != nil || len(commits) == 0 {
		return &GitCommitItem{Message: cleanMsg}, nil
	}

	return &commits[0], nil
}

// GitBranchesResult represents the branch list and current active branch.
type GitBranchesResult struct {
	Branches []string `json:"branches"`
	Current  string   `json:"current"`
}

// ListBranches returns all local branches and the currently checked out branch.
func (s *Service) ListBranches(ctx context.Context, projectID uint) (*GitBranchesResult, error) {
	row, err := s.load(projectID)
	if err != nil {
		return nil, err
	}
	if !s.isGitDir(ctx, row.Path) {
		return nil, ErrNotGitRepo
	}

	out, err := gitRun(ctx, row.Path, "branch", "--list", "--no-color")
	if err != nil {
		return nil, err
	}

	result := &GitBranchesResult{
		Branches: make([]string, 0),
	}

	scanner := bufio.NewScanner(strings.NewReader(out))
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		isCurrent := strings.HasPrefix(line, "* ")
		branchName := trimmed
		if isCurrent {
			branchName = strings.TrimSpace(strings.TrimPrefix(line, "* "))
			result.Current = branchName
		} else if strings.HasPrefix(line, "+ ") {
			branchName = strings.TrimSpace(strings.TrimPrefix(line, "+ "))
		}
		result.Branches = append(result.Branches, branchName)
	}

	return result, nil
}

// CheckoutBranch switches to an existing branch or creates and switches to a new one.
func (s *Service) CheckoutBranch(ctx context.Context, projectID uint, branch string, create bool) error {
	row, err := s.load(projectID)
	if err != nil {
		return err
	}
	if !s.isGitDir(ctx, row.Path) {
		return ErrNotGitRepo
	}

	cleanBranch := strings.TrimSpace(branch)
	if !validateBranchName(cleanBranch) {
		return ErrInvalidBranchName
	}

	if create {
		_, err = gitRun(ctx, row.Path, "checkout", "-b", cleanBranch)
	} else {
		_, err = gitRun(ctx, row.Path, "checkout", cleanBranch)
	}
	return err
}
