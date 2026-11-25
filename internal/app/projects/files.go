package projects

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// FileItem represents a file or folder in the project workspace.
type FileItem struct {
	Path  string `json:"path"`
	IsDir bool   `json:"isDir"`
}

// FileTreeNode represents a node in the project file hierarchy.
type FileTreeNode struct {
	Name     string         `json:"name"`
	Path     string         `json:"path"`
	IsDir    bool           `json:"isDir"`
	Size     int64          `json:"size,omitempty"`
	Children []FileTreeNode `json:"children,omitempty"`
}

// FileContentResponse holds metadata and text content for a read file.
type FileContentResponse struct {
	Path      string `json:"path"`
	Size      int64  `json:"size"`
	Lines     int    `json:"lines"`
	MimeType  string `json:"mimeType"`
	IsBinary  bool   `json:"isBinary"`
	Content   string `json:"content,omitempty"`
	Truncated bool   `json:"truncated,omitempty"`
}

// FileContentMatch represents a single occurrence of query in project files.
type FileContentMatch struct {
	Path    string `json:"path"`
	Line    int    `json:"line"`
	Content string `json:"content"`
}

// SearchFiles scans the project's directory and returns relative paths matching query.
// For Git repositories, it filters files by .gitignore rules (via git ls-files).
// For non-Git workspaces, it performs standard traversal without dotfile filtering (only skipping .git).
func (s *Service) SearchFiles(ctx context.Context, projectID uint, query string, limit int, worktreeID ...uint) ([]FileItem, error) {
	if limit <= 0 {
		limit = 50
	} else if limit > 100 {
		limit = 100
	}

	rootDir, err := s.workspaceRoot(projectID, worktreeID...)
	if err != nil {
		return nil, err
	}

	normQuery := strings.ToLower(strings.TrimSpace(query))

	if s.isGitDir(ctx, rootDir) {
		items, gitErr := s.searchGitFiles(ctx, rootDir, normQuery, limit)
		if gitErr == nil {
			return items, nil
		}
	}

	return s.searchWorkspaceFiles(ctx, rootDir, normQuery, limit)
}

func (s *Service) isGitDir(ctx context.Context, directory string) bool {
	if _, err := os.Stat(filepath.Join(directory, ".git")); err == nil {
		out, err := gitOutputLimit(ctx, directory, 1024, "rev-parse", "--is-inside-work-tree")
		return err == nil && strings.TrimSpace(out) == "true"
	}
	return false
}

// searchGitFiles lists files and folders respecting .gitignore rules via git ls-files.
func (s *Service) searchGitFiles(ctx context.Context, rootDir, normQuery string, limit int) ([]FileItem, error) {
	out, err := gitOutputLimit(ctx, rootDir, 16<<20, "ls-files", "-co", "--exclude-standard")
	if err != nil {
		return nil, err
	}

	lines := strings.Split(out, "\n")
	var items []FileItem
	seenDirs := make(map[string]struct{})
	var dirItems []FileItem
	var fileItems []FileItem

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		slashRel := filepath.ToSlash(trimmed)
		name := filepath.Base(slashRel)

		// Collect and match parent directories
		dir := filepath.ToSlash(filepath.Dir(slashRel))
		for dir != "." && dir != "/" && dir != "" {
			if _, exists := seenDirs[dir]; !exists {
				seenDirs[dir] = struct{}{}
				dirName := filepath.Base(dir)
				if normQuery == "" || strings.Contains(strings.ToLower(dir), normQuery) || strings.Contains(strings.ToLower(dirName), normQuery) {
					dirItems = append(dirItems, FileItem{Path: dir, IsDir: true})
				}
			}
			parent := filepath.ToSlash(filepath.Dir(dir))
			if parent == dir {
				break
			}
			dir = parent
		}

		if normQuery == "" || strings.Contains(strings.ToLower(slashRel), normQuery) || strings.Contains(strings.ToLower(name), normQuery) {
			fileItems = append(fileItems, FileItem{Path: slashRel, IsDir: false})
		}
	}

	sort.Slice(dirItems, func(i, j int) bool { return dirItems[i].Path < dirItems[j].Path })
	sort.Slice(fileItems, func(i, j int) bool { return fileItems[i].Path < fileItems[j].Path })

	// Merge folders then files up to limit
	for _, d := range dirItems {
		if len(items) >= limit {
			break
		}
		items = append(items, d)
	}
	for _, f := range fileItems {
		if len(items) >= limit {
			break
		}
		items = append(items, f)
	}

	return items, nil
}

// SearchFileContent searches text content inside project workspace files.
func (s *Service) SearchFileContent(ctx context.Context, projectID uint, query string, limit int, worktreeID ...uint) ([]FileContentMatch, error) {
	trimmedQuery := strings.TrimSpace(query)
	if trimmedQuery == "" {
		return []FileContentMatch{}, nil
	}
	if limit <= 0 {
		limit = 50
	} else if limit > 100 {
		limit = 100
	}

	rootDir, err := s.workspaceRoot(projectID, worktreeID...)
	if err != nil {
		return nil, err
	}

	if s.isGitDir(ctx, rootDir) {
		return s.searchGitFileContent(ctx, rootDir, trimmedQuery, limit)
	}
	return s.searchWorkspaceFileContent(ctx, rootDir, trimmedQuery, limit)
}

func (s *Service) searchGitFileContent(ctx context.Context, rootDir, query string, limit int) ([]FileContentMatch, error) {
	out, err := gitOutputLimit(ctx, rootDir, 8<<20, "grep", "-n", "-I", "-i", "--untracked", "-e", query)
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			// git grep exit code 1 means no matches found
			return []FileContentMatch{}, nil
		}
		// If git grep fails for other reasons, fallback to standard workspace scan
		return s.searchWorkspaceFileContent(ctx, rootDir, query, limit)
	}

	var matches []FileContentMatch
	lines := strings.Split(out, "\n")
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 3)
		if len(parts) < 3 {
			continue
		}
		lineNum, parseErr := strconv.Atoi(parts[1])
		if parseErr != nil {
			continue
		}
		snippet := strings.TrimRight(parts[2], "\r\n")
		if len(snippet) > 300 {
			snippet = snippet[:300] + "..."
		}
		matches = append(matches, FileContentMatch{
			Path:    filepath.ToSlash(parts[0]),
			Line:    lineNum,
			Content: snippet,
		})
		if len(matches) >= limit {
			break
		}
	}
	return matches, nil
}

func (s *Service) searchWorkspaceFileContent(ctx context.Context, rootDir, query string, limit int) ([]FileContentMatch, error) {
	var matches []FileContentMatch
	normQuery := strings.ToLower(query)

	err := filepath.WalkDir(rootDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil || ctx.Err() != nil {
			return ctx.Err()
		}
		if path == rootDir {
			return nil
		}

		// Only skip .git metadata directory
		if d.Name() == ".git" {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}

		rel, relErr := filepath.Rel(rootDir, path)
		if relErr != nil {
			return nil
		}
		slashRel := filepath.ToSlash(rel)

		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer f.Close()

		header := make([]byte, 512)
		n, _ := io.ReadFull(f, header)
		header = header[:n]
		mimeType := http.DetectContentType(header)
		if isBinaryContent(header, mimeType, d.Name()) {
			return nil
		}

		if _, err := f.Seek(0, io.SeekStart); err != nil {
			return nil
		}

		scanner := bufio.NewScanner(f)
		lineNum := 0
		for scanner.Scan() {
			lineNum++
			lineText := scanner.Text()
			if strings.Contains(strings.ToLower(lineText), normQuery) {
				snippet := strings.TrimRight(lineText, "\r\n")
				if len(snippet) > 300 {
					snippet = snippet[:300] + "..."
				}
				matches = append(matches, FileContentMatch{
					Path:    slashRel,
					Line:    lineNum,
					Content: snippet,
				})
				if len(matches) >= limit {
					return fs.SkipAll
				}
			}
		}
		return nil
	})

	if err != nil && err != fs.SkipAll {
		return nil, err
	}
	return matches, nil
}

// searchWorkspaceFiles scans with standard fs traversal, only skipping .git metadata directory.
func (s *Service) searchWorkspaceFiles(ctx context.Context, rootDir, normQuery string, limit int) ([]FileItem, error) {
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
		// Only skip .git metadata directory
		if name == ".git" {
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

// maxFileReadBytes is the upper limit (5MB) for inline text content reads.
const maxFileReadBytes = 5 * 1024 * 1024

// isAllowedExternalPath checks whether a path outside the project workspace is permitted
// for preview and inspection (e.g. agent temporary demo files, plan markdown files in /tmp or agent scratch dirs).
func isAllowedExternalPath(p string) bool {
	clean := filepath.Clean(p)
	if !filepath.IsAbs(clean) {
		return false
	}

	// Forbid dangerous system directories and files
	forbiddenPrefixes := []string{
		"/etc", "/proc", "/sys", "/dev", "/boot", "/root/.ssh",
	}
	for _, f := range forbiddenPrefixes {
		if clean == f || strings.HasPrefix(clean, f+"/") {
			return false
		}
	}

	// Forbid sensitive credential files and shell histories
	lower := strings.ToLower(filepath.ToSlash(clean))
	if strings.Contains(lower, "/.ssh/") ||
		strings.HasSuffix(lower, "/.bash_history") ||
		strings.HasSuffix(lower, "/.zsh_history") ||
		strings.Contains(lower, "/.gnupg/") {
		return false
	}

	// Allowed directories:
	// 1. Temporary directories
	if strings.HasPrefix(clean, "/tmp/") || strings.HasPrefix(clean, "/var/tmp/") {
		return true
	}

	// 2. User home directories (.gemini, agent directories, etc.)
	home, err := os.UserHomeDir()
	if err == nil && home != "" {
		cleanHome := filepath.Clean(home)
		if strings.HasPrefix(clean, cleanHome+"/") {
			return true
		}
	}

	// 3. /root/.gemini or /data directory (common container agent runtime workspaces)
	if strings.HasPrefix(clean, "/root/.gemini/") || strings.HasPrefix(clean, "/data/") {
		return true
	}

	return false
}

// resolveProjectSafePath ensures the requested path is safe, absolute or project-relative,
// and does not escape allowed boundaries via traversal or symlinks.
// It supports files inside the project workspace as well as permitted external files
// (e.g. /tmp demo files, agent plans/artifacts).
func (s *Service) resolveProjectSafePath(projectID uint, relPath string, worktreeID ...uint) (string, string, error) {
	if strings.ContainsRune(relPath, 0) {
		return "", "", ErrInvalidTarget
	}

	rootDir, err := s.workspaceRoot(projectID, worktreeID...)
	if err != nil {
		return "", "", err
	}

	rawClean := filepath.Clean(relPath)
	var cleanRel string

	// If an absolute path is supplied, check whether it falls inside rootDir.
	if filepath.IsAbs(rawClean) {
		rel, err := filepath.Rel(rootDir, rawClean)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			cleanRel = filepath.ToSlash(rel)
		} else {
			// Path is outside project rootDir. Check if it's an allowed external file.
			if isAllowedExternalPath(rawClean) {
				fi, statErr := os.Stat(rawClean)
				if statErr != nil {
					if os.IsNotExist(statErr) {
						return "", "", ErrFileNotFound
					}
					return "", "", statErr
				}
				if fi.IsDir() {
					return "", "", ErrInvalidTarget
				}
				// Verify symlink target does not escape to forbidden areas
				canonical, cErr := filepath.EvalSymlinks(rawClean)
				if cErr == nil {
					if !isAllowedExternalPath(canonical) {
						return "", "", ErrPathEscaped
					}
				}
				return filepath.ToSlash(rawClean), rawClean, nil
			}
			return "", "", ErrPathEscaped
		}
	} else {
		cleanRel = filepath.ToSlash(rawClean)
		cleanRel = strings.TrimPrefix(cleanRel, "/")

		// Also check if the path starts with rootDir with leading slash stripped (e.g. "data/projects/dev_platform/...")
		cleanRoot := strings.TrimPrefix(filepath.ToSlash(filepath.Clean(rootDir)), "/")
		if cleanRoot != "" && (cleanRel == cleanRoot || strings.HasPrefix(cleanRel, cleanRoot+"/")) {
			cleanRel = strings.TrimPrefix(cleanRel, cleanRoot)
			cleanRel = strings.TrimPrefix(cleanRel, "/")
		}
	}

	if cleanRel == "" || cleanRel == "." {
		return "", "", ErrInvalidTarget
	}

	if cleanRel == ".." || strings.HasPrefix(cleanRel, "../") || strings.Contains(cleanRel, "/../") {
		return "", "", ErrPathEscaped
	}

	fullPath := filepath.Join(rootDir, filepath.FromSlash(cleanRel))

	// Lexical escape check
	relCheck, err := filepath.Rel(rootDir, fullPath)
	if err != nil || relCheck == ".." || strings.HasPrefix(relCheck, ".."+string(filepath.Separator)) {
		return "", "", ErrPathEscaped
	}

	// Existence check
	fi, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", "", ErrFileNotFound
		}
		return "", "", err
	}
	if fi.IsDir() {
		return "", "", ErrInvalidTarget
	}

	// Symlink escape check
	canonical, err := filepath.EvalSymlinks(fullPath)
	if err == nil {
		canRel, err := filepath.Rel(rootDir, canonical)
		if err != nil || canRel == ".." || strings.HasPrefix(canRel, ".."+string(filepath.Separator)) {
			// If symlink escapes rootDir, allow only if target is an allowed external path
			if !isAllowedExternalPath(canonical) {
				return "", "", ErrPathEscaped
			}
		}
	}

	return cleanRel, fullPath, nil
}

// GetFileTree returns the multi-level directory tree of the project workspace.
// It returns all files without filtering by .gitignore or dotfile prefixes,
// only skipping the .git version control metadata directory.
func (s *Service) GetFileTree(ctx context.Context, projectID uint, worktreeID ...uint) ([]FileTreeNode, error) {
	rootDir, err := s.workspaceRoot(projectID, worktreeID...)
	if err != nil {
		return nil, err
	}

	var filePaths []string

	_ = filepath.WalkDir(rootDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil || path == rootDir || ctx.Err() != nil {
			return walkErr
		}
		name := d.Name()
		if name == ".git" {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.IsDir() {
			rel, err := filepath.Rel(rootDir, path)
			if err == nil {
				filePaths = append(filePaths, filepath.ToSlash(rel))
			}
		}
		return nil
	})

	return buildTreeFromPaths(filePaths), nil
}

type intermediateNode struct {
	name     string
	path     string
	isDir    bool
	size     int64
	children map[string]*intermediateNode
}

func buildTreeFromPaths(paths []string) []FileTreeNode {
	root := &intermediateNode{
		children: make(map[string]*intermediateNode),
	}

	for _, p := range paths {
		parts := strings.Split(p, "/")
		current := root
		for i, part := range parts {
			if part == "" {
				continue
			}
			isLast := i == len(parts)-1
			node, exists := current.children[part]
			if !exists {
				nodePath := strings.Join(parts[:i+1], "/")
				node = &intermediateNode{
					name:     part,
					path:     nodePath,
					isDir:    !isLast,
					children: make(map[string]*intermediateNode),
				}
				current.children[part] = node
			}
			if !isLast {
				node.isDir = true
			}
			current = node
		}
	}

	var convert func(node *intermediateNode) []FileTreeNode
	convert = func(parent *intermediateNode) []FileTreeNode {
		var list []FileTreeNode
		for _, child := range parent.children {
			tn := FileTreeNode{
				Name:  child.name,
				Path:  child.path,
				IsDir: child.isDir,
				Size:  child.size,
			}
			if child.isDir && len(child.children) > 0 {
				tn.Children = convert(child)
			}
			list = append(list, tn)
		}
		sort.Slice(list, func(i, j int) bool {
			if list[i].IsDir != list[j].IsDir {
				return list[i].IsDir // directories first
			}
			return list[i].Name < list[j].Name
		})
		return list
	}

	return convert(root)
}

// resolveProjectSafeDir ensures the requested directory path is inside the project workspace
// and does not escape via traversal (..) or symlinks. An empty relPath represents the project root.
// Returns (rootDir, targetDir, cleanRel, error).
func (s *Service) resolveProjectSafeDir(projectID uint, relPath string, worktreeID ...uint) (string, string, string, error) {
	if strings.ContainsRune(relPath, 0) {
		return "", "", "", ErrInvalidTarget
	}

	rootDir, err := s.workspaceRoot(projectID, worktreeID...)
	if err != nil {
		return "", "", "", err
	}

	cleanRel := filepath.ToSlash(filepath.Clean(relPath))
	cleanRel = strings.TrimPrefix(cleanRel, "/")

	// Also check if the path starts with rootDir with leading slash stripped (e.g. "data/projects/dev_platform/...")
	cleanRoot := strings.TrimPrefix(filepath.ToSlash(filepath.Clean(rootDir)), "/")
	if cleanRoot != "" && (cleanRel == cleanRoot || strings.HasPrefix(cleanRel, cleanRoot+"/")) {
		cleanRel = strings.TrimPrefix(cleanRel, cleanRoot)
		cleanRel = strings.TrimPrefix(cleanRel, "/")
	}

	if cleanRel == "." || cleanRel == "" {
		return rootDir, rootDir, "", nil
	}

	if cleanRel == ".." || strings.HasPrefix(cleanRel, "../") || strings.Contains(cleanRel, "/../") {
		return "", "", "", ErrPathEscaped
	}

	fullPath := filepath.Join(rootDir, filepath.FromSlash(cleanRel))

	relCheck, err := filepath.Rel(rootDir, fullPath)
	if err != nil || relCheck == ".." || strings.HasPrefix(relCheck, ".."+string(filepath.Separator)) {
		return "", "", "", ErrPathEscaped
	}

	fi, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", "", "", ErrFileNotFound
		}
		return "", "", "", err
	}
	if !fi.IsDir() {
		return "", "", "", ErrInvalidTarget
	}

	canonical, err := filepath.EvalSymlinks(fullPath)
	if err == nil {
		canRel, err := filepath.Rel(rootDir, canonical)
		if err != nil || canRel == ".." || strings.HasPrefix(canRel, ".."+string(filepath.Separator)) {
			return "", "", "", ErrPathEscaped
		}
	}

	return rootDir, fullPath, cleanRel, nil
}

// GetFileTreeShallow returns only the direct children (files and subdirectories) of the given subPath
// without recursively expanding deeper directories. This powers lazy loading of workspace file trees.
// It returns all items without filtering by .gitignore or dotfile prefixes,
// only skipping the .git version control metadata directory.
func (s *Service) GetFileTreeShallow(ctx context.Context, projectID uint, subPath string, worktreeID ...uint) ([]FileTreeNode, error) {
	_, targetDir, cleanRel, err := s.resolveProjectSafeDir(projectID, subPath, worktreeID...)
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(targetDir)
	if err != nil {
		return nil, err
	}

	var dirs []FileTreeNode
	var files []FileTreeNode

	for _, entry := range entries {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		name := entry.Name()
		// Only skip .git metadata directory
		if name == ".git" {
			continue
		}

		childRel := name
		if cleanRel != "" {
			childRel = filepath.ToSlash(filepath.Join(cleanRel, name))
		}

		isDir := entry.IsDir()
		var size int64
		if !isDir {
			if info, err := entry.Info(); err == nil {
				size = info.Size()
			}
		}

		node := FileTreeNode{
			Name:  name,
			Path:  childRel,
			IsDir: isDir,
			Size:  size,
		}

		if isDir {
			dirs = append(dirs, node)
		} else {
			files = append(files, node)
		}
	}

	sortNodes := func(list []FileTreeNode) {
		sort.Slice(list, func(i, j int) bool {
			lowerI := strings.ToLower(list[i].Name)
			lowerJ := strings.ToLower(list[j].Name)
			if lowerI == lowerJ {
				return list[i].Name < list[j].Name
			}
			return lowerI < lowerJ
		})
	}
	sortNodes(dirs)
	sortNodes(files)

	return append(dirs, files...), nil
}

// ReadFileContent safely reads a file's content and returns its inspection metadata.
func (s *Service) ReadFileContent(ctx context.Context, projectID uint, relPath string, worktreeID ...uint) (*FileContentResponse, error) {
	cleanRel, fullPath, err := s.resolveProjectSafePath(projectID, relPath, worktreeID...)
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(fullPath)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return nil, ErrNotDirectory
	}

	f, err := os.Open(fullPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	size := info.Size()
	header := make([]byte, 512)
	n, _ := io.ReadFull(f, header)
	header = header[:n]

	mimeType := http.DetectContentType(header)
	isBinary := isBinaryContent(header, mimeType, relPath)

	res := &FileContentResponse{
		Path:     filepath.ToSlash(cleanRel),
		Size:     size,
		MimeType: mimeType,
		IsBinary: isBinary,
	}

	if isBinary {
		return res, nil
	}

	// Reset read position to start
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	var truncated bool
	if size > maxFileReadBytes {
		truncated = true
		_, err = io.CopyN(&buf, f, maxFileReadBytes)
	} else {
		_, err = io.Copy(&buf, f)
	}
	if err != nil && err != io.EOF {
		return nil, err
	}

	contentBytes := buf.Bytes()
	res.Content = string(contentBytes)
	res.Truncated = truncated
	res.Lines = bytes.Count(contentBytes, []byte("\n"))
	if len(contentBytes) > 0 && !bytes.HasSuffix(contentBytes, []byte("\n")) {
		res.Lines++
	}

	return res, nil
}

// WriteFileContent safely overwrites an existing text file inside the project workspace
// (or worktree) and returns the updated inspection metadata. External allowlisted paths
// and .git metadata paths are strictly rejected.
func (s *Service) WriteFileContent(ctx context.Context, projectID uint, relPath string, content string, worktreeID ...uint) (*FileContentResponse, error) {
	if strings.ContainsRune(relPath, 0) {
		return nil, ErrInvalidTarget
	}

	rootDir, err := s.workspaceRoot(projectID, worktreeID...)
	if err != nil {
		return nil, err
	}

	cleanRel, fullPath, err := s.resolveProjectSafePath(projectID, relPath, worktreeID...)
	if err != nil {
		return nil, err
	}

	// Reject external paths (resolveProjectSafePath allows reading e.g. /tmp, but writing
	// must stay strictly inside the project/worktree root).
	relCheck, err := filepath.Rel(rootDir, fullPath)
	if err != nil || relCheck == ".." || strings.HasPrefix(relCheck, ".."+string(filepath.Separator)) || filepath.IsAbs(relCheck) {
		return nil, ErrPathEscaped
	}

	// Reject .git metadata directory writes
	slashRel := filepath.ToSlash(relCheck)
	if slashRel == ".git" || strings.HasPrefix(slashRel, ".git/") {
		return nil, ErrPathEscaped
	}

	// Reject symlink targets escaping the project root
	if canonical, cErr := filepath.EvalSymlinks(fullPath); cErr == nil {
		canRel, rErr := filepath.Rel(rootDir, canonical)
		if rErr != nil || canRel == ".." || strings.HasPrefix(canRel, ".."+string(filepath.Separator)) {
			return nil, ErrPathEscaped
		}
	}

	info, err := os.Stat(fullPath)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return nil, ErrInvalidTarget
	}

	perm := info.Mode().Perm()
	if perm == 0 {
		perm = 0o644
	}
	if err := os.WriteFile(fullPath, []byte(content), perm); err != nil {
		return nil, err
	}

	return s.ReadFileContent(ctx, projectID, cleanRel, worktreeID...)
}

// ReadFileRaw opens the file for streaming raw content (e.g. for previewing images/media).
func (s *Service) ReadFileRaw(ctx context.Context, projectID uint, relPath string, worktreeID ...uint) (io.ReadCloser, string, int64, error) {
	_, fullPath, err := s.resolveProjectSafePath(projectID, relPath, worktreeID...)
	if err != nil {
		return nil, "", 0, err
	}

	info, err := os.Stat(fullPath)
	if err != nil {
		return nil, "", 0, err
	}
	if info.IsDir() {
		return nil, "", 0, ErrNotDirectory
	}

	f, err := os.Open(fullPath)
	if err != nil {
		return nil, "", 0, err
	}

	header := make([]byte, 512)
	n, _ := io.ReadFull(f, header)
	header = header[:n]
	mimeType := http.DetectContentType(header)

	if _, err := f.Seek(0, io.SeekStart); err != nil {
		_ = f.Close()
		return nil, "", 0, err
	}

	return f, mimeType, info.Size(), nil
}

// UploadFile writes a file to the project's target directory.
// dirRelPath can be empty or "." to address the project root.
func (s *Service) UploadFile(ctx context.Context, projectID uint, dirRelPath string, filename string, r io.Reader, worktreeID ...uint) (string, error) {
	cleanName := filepath.Base(filepath.Clean(filename))
	if cleanName == "" || cleanName == "." || cleanName == "/" || strings.ContainsRune(cleanName, 0) {
		return "", ErrInvalidTarget
	}

	rootDir, targetDir, _, err := s.resolveProjectSafeDir(projectID, dirRelPath, worktreeID...)
	if err != nil {
		return "", err
	}

	targetFile := filepath.Join(targetDir, filepath.FromSlash(cleanName))

	// Verify the destination does not escape project root
	relCheck, err := filepath.Rel(rootDir, targetFile)
	if err != nil || relCheck == ".." || strings.HasPrefix(relCheck, ".."+string(filepath.Separator)) {
		return "", ErrPathEscaped
	}

	dst, err := os.OpenFile(targetFile, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return "", err
	}
	defer dst.Close()

	if _, err := io.Copy(dst, r); err != nil {
		return "", err
	}

	return filepath.ToSlash(relCheck), nil
}

// DeletePath removes a file or directory inside the project workspace.
// Project root cannot be deleted.
func (s *Service) DeletePath(ctx context.Context, projectID uint, relPath string, worktreeID ...uint) error {
	if strings.ContainsRune(relPath, 0) {
		return ErrInvalidTarget
	}

	rootDir, err := s.workspaceRoot(projectID, worktreeID...)
	if err != nil {
		return err
	}

	cleanRel := filepath.ToSlash(filepath.Clean(relPath))
	cleanRel = strings.TrimPrefix(cleanRel, "/")

	cleanRoot := strings.TrimPrefix(filepath.ToSlash(filepath.Clean(rootDir)), "/")
	if cleanRoot != "" && (cleanRel == cleanRoot || strings.HasPrefix(cleanRel, cleanRoot+"/")) {
		cleanRel = strings.TrimPrefix(cleanRel, cleanRoot)
		cleanRel = strings.TrimPrefix(cleanRel, "/")
	}

	if cleanRel == "" || cleanRel == "." {
		return ErrInvalidTarget
	}

	if cleanRel == ".." || strings.HasPrefix(cleanRel, "../") || strings.Contains(cleanRel, "/../") {
		return ErrPathEscaped
	}

	fullPath := filepath.Join(rootDir, filepath.FromSlash(cleanRel))

	relCheck, err := filepath.Rel(rootDir, fullPath)
	if err != nil || relCheck == ".." || strings.HasPrefix(relCheck, ".."+string(filepath.Separator)) {
		return ErrPathEscaped
	}

	if _, err := os.Lstat(fullPath); err != nil {
		if os.IsNotExist(err) {
			return ErrFileNotFound
		}
		return err
	}

	return os.RemoveAll(fullPath)
}

// trimTrailingIncompleteUTF8 strips at most 3 trailing bytes from b if they
// represent an incomplete leading sequence of a multi-byte UTF-8 rune.
// This prevents false negatives when a fixed-size header buffer truncates a character.
func trimTrailingIncompleteUTF8(b []byte) []byte {
	n := len(b)
	if n == 0 {
		return b
	}

	for i := 1; i <= 3 && i <= n; i++ {
		lead := b[n-i]
		// Check if this byte is the leading byte of a multi-byte sequence (not 10xxxxxx)
		if lead&0xC0 != 0x80 {
			var expectedLen int
			if lead&0xE0 == 0xC0 {
				expectedLen = 2
			} else if lead&0xF0 == 0xE0 {
				expectedLen = 3
			} else if lead&0xF8 == 0xF0 {
				expectedLen = 4
			} else {
				break
			}

			if i < expectedLen {
				// Verify all trailing bytes are valid continuation bytes (10xxxxxx)
				validContinuation := true
				for j := n - i + 1; j < n; j++ {
					if b[j]&0xC0 != 0x80 {
						validContinuation = false
						break
					}
				}
				if validContinuation {
					return b[:n-i]
				}
			}
			break
		}
	}
	return b
}

var textExtensions = map[string]bool{
	".txt":      true,
	".md":       true,
	".markdown": true,
	".json":     true,
	".yaml":     true,
	".yml":      true,
	".toml":     true,
	".xml":      true,
	".html":     true,
	".htm":      true,
	".css":      true,
	".scss":     true,
	".less":     true,
	".js":       true,
	".jsx":      true,
	".ts":       true,
	".tsx":      true,
	".go":       true,
	".py":       true,
	".rs":       true,
	".c":        true,
	".cpp":      true,
	".h":        true,
	".hpp":      true,
	".java":     true,
	".kt":       true,
	".sh":       true,
	".bash":     true,
	".zsh":      true,
	".sql":      true,
	".ini":      true,
	".cfg":      true,
	".conf":     true,
	".env":      true,
	".log":      true,
	".csv":      true,
	".svg":      true,
}

func isBinaryContent(header []byte, mimeType string, filename ...string) bool {
	if bytes.IndexByte(header, 0) != -1 {
		return true
	}
	if strings.HasPrefix(mimeType, "image/") && !strings.Contains(mimeType, "svg") {
		return true
	}
	if strings.HasPrefix(mimeType, "video/") || strings.HasPrefix(mimeType, "audio/") {
		return true
	}
	if mimeType == "application/pdf" || mimeType == "application/zip" {
		return true
	}

	trimmed := trimTrailingIncompleteUTF8(header)
	if utf8.Valid(trimmed) {
		return false
	}

	if len(filename) > 0 && filename[0] != "" {
		ext := strings.ToLower(filepath.Ext(filename[0]))
		if textExtensions[ext] {
			return false
		}
	}

	if strings.HasPrefix(mimeType, "text/") {
		return false
	}

	return true
}
