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
}

// GitStatus inspects the working directory with `git status --porcelain`.
// If the directory is not inside a git work tree or git is unavailable,
// it returns an empty slice without an error.
func (s *Service) GitStatus(ctx context.Context, projectID uint) ([]GitStatusItem, error) {
	row, err := s.load(projectID)
	if err != nil {
		return nil, err
	}
	if !s.isGitDir(ctx, row.Path) {
		return []GitStatusItem{}, nil
	}

	// porcelain output format: XY <path> or XY <orig> -> <path>
	out, err := gitOutputLimit(ctx, row.Path, 2<<20, "status", "--porcelain")
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

		status := "modified"
		if x == '?' && y == '?' {
			status = "untracked"
		} else if x == 'A' || y == 'A' {
			status = "added"
		} else if x == 'D' || y == 'D' {
			status = "deleted"
		} else if x == 'R' || y == 'R' {
			status = "renamed"
		}

		items = append(items, GitStatusItem{
			Path:   cleanPath,
			Status: status,
		})
	}

	return items, nil
}
