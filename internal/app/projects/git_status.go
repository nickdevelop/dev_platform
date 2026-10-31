package projects

import (
	"bufio"
	"context"
	"path/filepath"
	"strings"
)

// GitStatusItem represents one changed file in a git workspace.
type GitStatusItem struct {
	Path   string `json:"path"`
	Status string `json:"status"` // "modified", "added", "deleted", "untracked", "renamed"
	Staged bool   `json:"staged"`
}

// GitStatus inspects the working directory with `git status --porcelain`.
// If the directory is not inside a git work tree or git is unavailable,
// it returns an empty slice without an error.
func (s *Service) GitStatus(ctx context.Context, projectID uint) ([]GitStatusItem, error) {
	return s.GitStatusWithRef(ctx, projectID, "")
}

// GitStatusWithRef inspects changed files in the working directory (when commitRef is empty)
// or within a specific commit (when commitRef is provided).
func (s *Service) GitStatusWithRef(ctx context.Context, projectID uint, commitRef string) ([]GitStatusItem, error) {
	row, err := s.load(projectID)
	if err != nil {
		return nil, err
	}
	if !s.isGitDir(ctx, row.Path) {
		return []GitStatusItem{}, nil
	}

	cleanRef := strings.TrimSpace(commitRef)
	if cleanRef != "" {
		if !isValidCommitRef(cleanRef) {
			return nil, ErrInvalidTarget
		}
		return s.gitCommitStatus(ctx, row.Path, cleanRef)
	}

	return s.gitWorkingStatus(ctx, row.Path)
}

func (s *Service) gitWorkingStatus(ctx context.Context, rootDir string) ([]GitStatusItem, error) {
	// porcelain output format: XY <path> or XY <orig> -> <path>
	out, err := gitOutputLimit(ctx, rootDir, 2<<20, "status", "--porcelain")
	if err != nil {
		return []GitStatusItem{}, nil
	}

	var items []GitStatusItem
	scanner := bufio.NewScanner(strings.NewReader(out))
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) < 4 {
			continue
		}
		x := line[0]
		y := line[1]
		rawPath := strings.TrimSpace(line[3:])
		if strings.Contains(rawPath, " -> ") {
			parts := strings.Split(rawPath, " -> ")
			rawPath = parts[len(parts)-1]
		}
		cleanPath := filepath.ToSlash(strings.Trim(rawPath, "\""))

		// Check index (staged)
		if x != ' ' && x != '?' && x != '!' {
			status := "modified"
			switch x {
			case 'A':
				status = "added"
			case 'D':
				status = "deleted"
			case 'R':
				status = "renamed"
			case 'C':
				status = "copied"
			case 'M':
				status = "modified"
			}
			items = append(items, GitStatusItem{
				Path:   cleanPath,
				Status: status,
				Staged: true,
			})
		}

		// Check worktree (unstaged)
		if y != ' ' && y != '!' {
			status := "modified"
			if x == '?' && y == '?' {
				status = "untracked"
			} else {
				switch y {
				case 'M':
					status = "modified"
				case 'D':
					status = "deleted"
				case 'A':
					status = "added"
				}
			}
			items = append(items, GitStatusItem{
				Path:   cleanPath,
				Status: status,
				Staged: false,
			})
		}
	}

	return items, nil
}

func (s *Service) gitCommitStatus(ctx context.Context, rootDir, commitRef string) ([]GitStatusItem, error) {
	// diff-tree output: <status-letter><score?>\t<path> or <status-letter><score?>\t<orig>\t<path>
	out, err := gitOutputLimit(ctx, rootDir, 2<<20, "diff-tree", "--no-commit-id", "--name-status", "--root", "-r", commitRef)
	if err != nil {
		return []GitStatusItem{}, nil
	}

	var items []GitStatusItem
	scanner := bufio.NewScanner(strings.NewReader(out))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 2 {
			continue
		}
		code := strings.TrimSpace(parts[0])
		rawPath := strings.TrimSpace(parts[len(parts)-1])
		cleanPath := filepath.ToSlash(strings.Trim(rawPath, "\""))

		status := "modified"
		if len(code) > 0 {
			switch code[0] {
			case 'A':
				status = "added"
			case 'D':
				status = "deleted"
			case 'R':
				status = "renamed"
			case 'M':
				status = "modified"
			}
		}

		items = append(items, GitStatusItem{
			Path:   cleanPath,
			Status: status,
		})
	}

	return items, nil
}
