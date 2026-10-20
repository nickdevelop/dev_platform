package projects

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
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

// maxFileReadBytes is the upper limit (5MB) for inline text content reads.
const maxFileReadBytes = 5 * 1024 * 1024

// resolveProjectSafePath ensures the requested path is safe, absolute, inside the project directory,
// and does not escape the project boundary via traversal or symlinks.
func (s *Service) resolveProjectSafePath(projectID uint, relPath string) (string, string, error) {
	if strings.ContainsRune(relPath, 0) {
		return "", "", ErrInvalidTarget
	}

	proj, err := s.repo.Get(projectID)
	if err != nil {
		return "", "", err
	}

	rootDir := proj.Path
	cleanRel := filepath.ToSlash(filepath.Clean(relPath))
	cleanRel = strings.TrimPrefix(cleanRel, "/")

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
	if _, err := os.Stat(fullPath); err != nil {
		if os.IsNotExist(err) {
			return "", "", ErrFileNotFound
		}
		return "", "", err
	}

	// Symlink escape check
	canonical, err := filepath.EvalSymlinks(fullPath)
	if err == nil {
		canRel, err := filepath.Rel(rootDir, canonical)
		if err != nil || canRel == ".." || strings.HasPrefix(canRel, ".."+string(filepath.Separator)) {
			return "", "", ErrPathEscaped
		}
	}

	return rootDir, fullPath, nil
}

// GetFileTree returns the multi-level directory tree of the project workspace.
func (s *Service) GetFileTree(ctx context.Context, projectID uint) ([]FileTreeNode, error) {
	proj, err := s.repo.Get(projectID)
	if err != nil {
		return nil, err
	}

	rootDir := proj.Path
	var filePaths []string

	if s.isGitDir(ctx, rootDir) {
		out, err := gitOutputLimit(ctx, rootDir, 10<<20, "ls-files", "--cached", "--others", "--exclude-standard")
		if err == nil {
			scanner := bufio.NewScanner(strings.NewReader(out))
			for scanner.Scan() {
				rel := filepath.ToSlash(strings.TrimSpace(scanner.Text()))
				if rel == "" || rel == ".git" || strings.HasPrefix(rel, ".git/") {
					continue
				}
				filePaths = append(filePaths, rel)
			}
		}
	}

	if len(filePaths) == 0 {
		_ = filepath.WalkDir(rootDir, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil || path == rootDir || ctx.Err() != nil {
				return walkErr
			}
			name := d.Name()
			if strings.HasPrefix(name, ".") {
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
	}

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

// ReadFileContent safely reads a file's content and returns its inspection metadata.
func (s *Service) ReadFileContent(ctx context.Context, projectID uint, relPath string) (*FileContentResponse, error) {
	_, fullPath, err := s.resolveProjectSafePath(projectID, relPath)
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
		Path:     filepath.ToSlash(relPath),
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

// ReadFileRaw opens the file for streaming raw content (e.g. for previewing images/media).
func (s *Service) ReadFileRaw(ctx context.Context, projectID uint, relPath string) (io.ReadCloser, string, int64, error) {
	_, fullPath, err := s.resolveProjectSafePath(projectID, relPath)
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
