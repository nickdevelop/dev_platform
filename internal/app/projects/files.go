package projects

import (
	"bufio"
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// FileItem represents a file or folder in the project workspace.
type FileItem struct {
	Path  string `json:"path"`
	IsDir bool   `json:"isDir"`
}

// SearchFiles scans the project's directory and returns relative paths matching query.
// For git repositories, it filters files adhering to .gitignore rules.
// For non-git directories, it only hides hidden files and folders (names starting with '.').
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

	if s.isGitDir(ctx, rootDir) {
		items, err := s.searchGitFiles(ctx, rootDir, normQuery, limit)
		if err == nil {
			return items, nil
		}
	}
	return s.searchNonGitFiles(ctx, rootDir, normQuery, limit)
}

func (s *Service) isGitDir(ctx context.Context, directory string) bool {
	if _, err := os.Stat(filepath.Join(directory, ".git")); err == nil {
		out, err := gitOutputLimit(ctx, directory, 1024, "rev-parse", "--is-inside-work-tree")
		return err == nil && strings.TrimSpace(out) == "true"
	}
	return false
}

// searchGitFiles uses git ls-files with standard ignore rules (.gitignore).
func (s *Service) searchGitFiles(ctx context.Context, rootDir, normQuery string, limit int) ([]FileItem, error) {
	out, err := gitOutputLimit(ctx, rootDir, 10<<20, "ls-files", "--cached", "--others", "--exclude-standard")
	if err != nil {
		return nil, err
	}

	dirOut, _ := gitOutputLimit(ctx, rootDir, 2<<20, "ls-files", "--others", "--directory", "--exclude-standard")

	seenDirs := make(map[string]struct{})
	var files []string

	scanner := bufio.NewScanner(strings.NewReader(out))
	for scanner.Scan() {
		rel := filepath.ToSlash(strings.TrimSpace(scanner.Text()))
		if rel == "" || rel == ".git" || strings.HasPrefix(rel, ".git/") {
			continue
		}
		files = append(files, rel)
		parts := strings.Split(rel, "/")
		for i := 1; i < len(parts); i++ {
			dirPath := strings.Join(parts[:i], "/")
			if dirPath != "" && dirPath != ".git" && !strings.HasPrefix(dirPath, ".git/") {
				seenDirs[dirPath] = struct{}{}
			}
		}
	}

	if dirOut != "" {
		dirScanner := bufio.NewScanner(strings.NewReader(dirOut))
		for dirScanner.Scan() {
			dirRel := filepath.ToSlash(strings.TrimSpace(dirScanner.Text()))
			dirRel = strings.TrimSuffix(dirRel, "/")
			if dirRel != "" && dirRel != ".git" && !strings.HasPrefix(dirRel, ".git/") {
				seenDirs[dirRel] = struct{}{}
			}
		}
	}

	var matchedDirs []FileItem
	for d := range seenDirs {
		name := filepath.Base(d)
		if normQuery == "" || strings.Contains(strings.ToLower(d), normQuery) || strings.Contains(strings.ToLower(name), normQuery) {
			matchedDirs = append(matchedDirs, FileItem{Path: d, IsDir: true})
		}
	}
	sort.Slice(matchedDirs, func(i, j int) bool {
		return matchedDirs[i].Path < matchedDirs[j].Path
	})

	var matchedFiles []FileItem
	for _, f := range files {
		name := filepath.Base(f)
		if normQuery == "" || strings.Contains(strings.ToLower(f), normQuery) || strings.Contains(strings.ToLower(name), normQuery) {
			matchedFiles = append(matchedFiles, FileItem{Path: f, IsDir: false})
		}
	}
	sort.Slice(matchedFiles, func(i, j int) bool {
		return matchedFiles[i].Path < matchedFiles[j].Path
	})

	items := make([]FileItem, 0, limit)
	for _, d := range matchedDirs {
		items = append(items, d)
		if len(items) >= limit {
			return items, nil
		}
	}
	for _, f := range matchedFiles {
		items = append(items, f)
		if len(items) >= limit {
			break
		}
	}
	return items, nil
}

// searchNonGitFiles scans with standard fs traversal, only skipping items whose name starts with '.'
func (s *Service) searchNonGitFiles(ctx context.Context, rootDir, normQuery string, limit int) ([]FileItem, error) {
	var items []FileItem

	err := filepath.WalkDir(rootDir, func(path string, d fs.DirEntry, walkErr error) error {
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
		// Only hide dot files and folders
		if strings.HasPrefix(name, ".") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
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
