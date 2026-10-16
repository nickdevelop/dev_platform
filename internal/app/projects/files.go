package projects

import (
	"context"
	"io/fs"
	"path/filepath"
	"strings"
)

// FileItem represents a file or folder in the project workspace.
type FileItem struct {
	Path  string `json:"path"`
	IsDir bool   `json:"isDir"`
}

// ignoredDirectories lists common dependency, build, or VCS directories to skip during traversal.
var ignoredDirectories = map[string]struct{}{
	".git":          {},
	".svn":          {},
	".hg":           {},
	"node_modules":  {},
	"vendor":        {},
	".vendor":       {},
	"dist":          {},
	"build":         {},
	"target":        {},
	".idea":         {},
	".vscode":       {},
	"__pycache__":   {},
	".pytest_cache": {},
	".next":         {},
	".nuxt":         {},
	".output":       {},
	"bin":           {},
	"obj":           {},
}

func isIgnoredDir(name string) bool {
	_, ok := ignoredDirectories[name]
	return ok
}

// SearchFiles scans the project's directory and returns relative paths matching query.
func (s *Service) SearchFiles(ctx context.Context, projectID uint, query string, limit int) ([]FileItem, error) {
	if limit <= 0 {
		limit = 50
	} else if limit > 100 {
		limit = 100
	}

	proj, err := s.repo.Get(projectID)
	if err != nil {
		return nil, err
	}

	rootDir := proj.Path
	normQuery := strings.ToLower(strings.TrimSpace(query))

	var items []FileItem

	err = filepath.WalkDir(rootDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}

		if path == rootDir {
			return nil
		}

		name := d.Name()
		if d.IsDir() {
			if isIgnoredDir(name) {
				return filepath.SkipDir
			}
		}

		rel, relErr := filepath.Rel(rootDir, path)
		if relErr != nil {
			return nil
		}

		slashRel := filepath.ToSlash(rel)
		if normQuery == "" || strings.Contains(strings.ToLower(slashRel), normQuery) || strings.Contains(strings.ToLower(name), normQuery) {
			items = append(items, FileItem{
				Path:  slashRel,
				IsDir: d.IsDir(),
			})
			if len(items) >= limit {
				return fs.SkipAll
			}
		}

		return nil
	})

	if err != nil && err != fs.SkipAll {
		return nil, err
	}

	return items, nil
}
