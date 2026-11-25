package projects

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

type memoryRepo struct {
	projects      map[uint]Project
	displayConfig DisplayConfig
}

func (m *memoryRepo) List() ([]Project, error) {
	var res []Project
	for _, p := range m.projects {
		res = append(res, p)
	}
	return res, nil
}

func (m *memoryRepo) Get(id uint) (Project, error) {
	p, ok := m.projects[id]
	if !ok {
		return Project{}, ErrProjectNotFound
	}
	return p, nil
}

func (m *memoryRepo) GetByPath(path string) (Project, error) {
	for _, p := range m.projects {
		if p.Path == path {
			return p, nil
		}
	}
	return Project{}, ErrProjectNotFound
}

func (m *memoryRepo) Create(name, path string) (Project, error) {
	id := uint(len(m.projects) + 1)
	p := Project{ID: id, Name: name, Path: path}
	m.projects[id] = p
	return p, nil
}

func (m *memoryRepo) Update(id uint, name, path string) (Project, error) {
	p := Project{ID: id, Name: name, Path: path}
	m.projects[id] = p
	return p, nil
}

func (m *memoryRepo) RemoveProject(id uint) error {
	delete(m.projects, id)
	return nil
}

func (m *memoryRepo) GetDisplayConfig() (DisplayConfig, error) {
	if m.displayConfig.Order == nil {
		m.displayConfig.Order = []uint{}
	}
	if m.displayConfig.Hidden == nil {
		m.displayConfig.Hidden = []uint{}
	}
	return m.displayConfig, nil
}

func (m *memoryRepo) SetDisplayConfig(cfg DisplayConfig) error {
	if cfg.Order == nil {
		cfg.Order = []uint{}
	}
	if cfg.Hidden == nil {
		cfg.Hidden = []uint{}
	}
	m.displayConfig = cfg
	return nil
}

func (m *memoryRepo) ListWorktrees(uint) ([]Worktree, error) { return nil, nil }

func (m *memoryRepo) GetWorktree(uint, uint) (Worktree, error) {
	return Worktree{}, ErrWorktreeNotFound
}

func (m *memoryRepo) CreateWorktree(worktree Worktree) (Worktree, error) { return worktree, nil }

func (m *memoryRepo) UpdateWorktreeBranch(uint, uint, string) error { return nil }

func (m *memoryRepo) DeleteWorktree(uint, uint) error { return nil }

func setupTestProject(t *testing.T) (*Service, uint, string) {
	t.Helper()
	dir := t.TempDir()

	// Non-git test structure:
	// dir/
	//   src/
	//     main.go
	//     utils/
	//       helper.go
	//   README.md
	//   .hidden_dir/
	//     secret.js (should be hidden)
	//   .env (should be hidden)
	if err := os.MkdirAll(filepath.Join(dir, "src", "utils"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".hidden_dir"), 0o755); err != nil {
		t.Fatalf("mkdir .hidden_dir: %v", err)
	}

	_ = os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Hello"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "src", "main.go"), []byte("package main"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "src", "utils", "helper.go"), []byte("package utils"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, ".hidden_dir", "secret.js"), []byte(""), 0o644)
	_ = os.WriteFile(filepath.Join(dir, ".env"), []byte("SECRET=1"), 0o644)

	repo := &memoryRepo{projects: make(map[uint]Project)}
	p, _ := repo.Create("test-project", dir)
	svc := NewServiceWithRepository(repo)

	return svc, p.ID, dir
}

func TestSearchFiles_NonGit(t *testing.T) {
	svc, id, _ := setupTestProject(t)
	ctx := context.Background()

	// 1. Search without query should return all non-hidden items
	items, err := svc.SearchFiles(ctx, id, "", 50)
	if err != nil {
		t.Fatalf("SearchFiles: %v", err)
	}

	paths := make(map[string]bool)
	for _, item := range items {
		paths[item.Path] = true
	}

	if !paths["README.md"] || !paths["src/main.go"] || !paths["src/utils/helper.go"] {
		t.Fatalf("expected workspace files not found: %v", paths)
	}

	// Must include dot files and dot folders (only .git is skipped)
	if !paths[".env"] || !paths[".hidden_dir"] || !paths[".hidden_dir/secret.js"] {
		t.Fatalf("dot items missing from non-git results: %v", paths)
	}

	// 2. Search with query "helper"
	filtered, err := svc.SearchFiles(ctx, id, "helper", 50)
	if err != nil {
		t.Fatalf("SearchFiles filtered: %v", err)
	}
	if len(filtered) != 1 || filtered[0].Path != "src/utils/helper.go" {
		t.Fatalf("expected only helper.go, got: %v", filtered)
	}

	// 3. Search with limit 1
	limited, err := svc.SearchFiles(ctx, id, "", 1)
	if err != nil {
		t.Fatalf("SearchFiles limited: %v", err)
	}
	if len(limited) != 1 {
		t.Fatalf("expected 1 item, got: %d", len(limited))
	}
}

func TestSearchFiles_GitProject(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git CLI unavailable")
	}
	dir := t.TempDir()

	// Initialize git repo
	cmd := exec.Command("git", "init", "-q")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}

	// Create files:
	//   .gitignore
	//   app.go
	//   ignored.log
	//   node_modules/dep.js
	//   src/component.go
	if err := os.MkdirAll(filepath.Join(dir, "src"), 0o755); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "node_modules"), 0o755); err != nil {
		t.Fatalf("mkdir node_modules: %v", err)
	}

	_ = os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("*.log\nnode_modules/\n"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "app.go"), []byte("package app"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "src", "component.go"), []byte("package src"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "debug.log"), []byte("log"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "node_modules", "dep.js"), []byte("console.log(1)"), 0o644)

	repo := &memoryRepo{projects: make(map[uint]Project)}
	p, _ := repo.Create("git-project", dir)
	svc := NewServiceWithRepository(repo)

	items, err := svc.SearchFiles(context.Background(), p.ID, "", 50)
	if err != nil {
		t.Fatalf("SearchFiles: %v", err)
	}

	paths := make(map[string]bool)
	for _, item := range items {
		paths[item.Path] = true
	}

	// app.go and src/component.go must be present
	if !paths["app.go"] || !paths["src/component.go"] {
		t.Fatalf("expected git files not found: %v", paths)
	}

	// .gitignore items (debug.log, node_modules/dep.js) must be filtered out in git repos
	if paths["debug.log"] || paths["node_modules/dep.js"] {
		t.Fatalf("ignored files leaked into results: %v", paths)
	}
	if paths[".git"] || paths[".git/config"] {
		t.Fatalf(".git metadata directory leaked into results: %v", paths)
	}
}

func TestHandleSearchFiles(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc, id, _ := setupTestProject(t)

	r := gin.New()
	svc.RegisterRoutes(r)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/projects/"+strconv.FormatUint(uint64(id), 10)+"/files?q=main", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body: %s", w.Code, w.Body.String())
	}

	var resp filesResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(resp.Files) == 0 || resp.Files[0].Path != "src/main.go" {
		t.Fatalf("expected src/main.go, got: %+v", resp.Files)
	}
}

func TestSearchFileContent_GitAndNonGit(t *testing.T) {
	svc, id, _ := setupTestProject(t)
	ctx := context.Background()

	// Non-git project content search
	matches, err := svc.SearchFileContent(ctx, id, "Hello", 50)
	if err != nil {
		t.Fatalf("SearchFileContent: %v", err)
	}
	if len(matches) == 0 {
		t.Fatalf("expected matches for 'Hello', got none")
	}
	if matches[0].Path != "README.md" || matches[0].Line != 1 {
		t.Fatalf("expected README.md:1, got: %+v", matches[0])
	}

	// Git project content search
	if gitAvailable() {
		dir := t.TempDir()
		cmd := exec.Command("git", "init", "-q")
		cmd.Dir = dir
		if err := cmd.Run(); err == nil {
			_ = os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("ignored.txt\n"), 0o644)
			_ = os.WriteFile(filepath.Join(dir, "code.go"), []byte("package main\nfunc TargetFunc() {}\n"), 0o644)
			_ = os.WriteFile(filepath.Join(dir, "ignored.txt"), []byte("TargetFunc in ignored file\n"), 0o644)

			repo := &memoryRepo{projects: make(map[uint]Project)}
			p, _ := repo.Create("git-content-proj", dir)
			gitSvc := NewServiceWithRepository(repo)

			gitMatches, gitErr := gitSvc.SearchFileContent(ctx, p.ID, "TargetFunc", 50)
			if gitErr != nil {
				t.Fatalf("git SearchFileContent: %v", gitErr)
			}
			if len(gitMatches) != 1 || gitMatches[0].Path != "code.go" || gitMatches[0].Line != 2 {
				t.Fatalf("expected code.go:2, got %+v", gitMatches)
			}
		}
	}
}

func TestHandleSearchFileContent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc, id, _ := setupTestProject(t)

	r := gin.New()
	svc.RegisterRoutes(r)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/projects/"+strconv.FormatUint(uint64(id), 10)+"/files/search-content?q=Hello", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body: %s", w.Code, w.Body.String())
	}

	var resp searchContentResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(resp.Matches) == 0 || resp.Matches[0].Path != "README.md" {
		t.Fatalf("expected README.md match, got: %+v", resp.Matches)
	}
}

func TestGetFileTree_NonGit(t *testing.T) {
	svc, id, _ := setupTestProject(t)
	ctx := context.Background()

	tree, err := svc.GetFileTree(ctx, id)
	if err != nil {
		t.Fatalf("GetFileTree: %v", err)
	}

	// In non-git directory without filtering:
	// dirs: .hidden_dir, src
	// files: .env, README.md
	if len(tree) != 4 {
		t.Fatalf("expected 4 root items (.hidden_dir, src, .env, README.md), got %d: %+v", len(tree), tree)
	}

	// Directories first, sorted by name
	if tree[0].Name != ".hidden_dir" || !tree[0].IsDir {
		t.Errorf("expected first item to be .hidden_dir, got: %+v", tree[0])
	}
	if tree[1].Name != "src" || !tree[1].IsDir {
		t.Errorf("expected second item to be src, got: %+v", tree[1])
	}
	if tree[2].Name != ".env" || tree[2].IsDir {
		t.Errorf("expected third item to be .env, got: %+v", tree[2])
	}
	if tree[3].Name != "README.md" || tree[3].IsDir {
		t.Errorf("expected fourth item to be README.md, got: %+v", tree[3])
	}

	if len(tree[1].Children) == 0 {
		t.Fatalf("expected src to have children, got none")
	}
}

func TestReadFileContent_TextAndBinary(t *testing.T) {
	svc, id, dir := setupTestProject(t)
	ctx := context.Background()

	// Text file
	resp, err := svc.ReadFileContent(ctx, id, "README.md")
	if err != nil {
		t.Fatalf("ReadFileContent(README.md): %v", err)
	}
	if resp.IsBinary || resp.Content != "# Hello" || resp.Lines != 1 {
		t.Errorf("unexpected text content response: %+v", resp)
	}

	// Binary file
	binPath := filepath.Join(dir, "binary.bin")
	_ = os.WriteFile(binPath, []byte{0x00, 0x01, 0x02, 0x03}, 0o644)

	binResp, err := svc.ReadFileContent(ctx, id, "binary.bin")
	if err != nil {
		t.Fatalf("ReadFileContent(binary.bin): %v", err)
	}
	if !binResp.IsBinary || binResp.Content != "" {
		t.Errorf("expected binary file to have IsBinary=true and empty Content, got: %+v", binResp)
	}
}

func TestReadFileContent_UTF8BoundaryTruncation(t *testing.T) {
	svc, id, dir := setupTestProject(t)
	ctx := context.Background()

	// Construct a file where the 512th byte lands exactly on the first byte of a 3-byte UTF-8 character.
	// 511 ASCII bytes + "测试服务器列表\n"
	prefix := bytes.Repeat([]byte("a"), 511)
	chinese := []byte("测试服务器列表\n")
	fullContent := append(prefix, chinese...)

	testPath := filepath.Join(dir, "truncated.md")
	if err := os.WriteFile(testPath, fullContent, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	resp, err := svc.ReadFileContent(ctx, id, "truncated.md")
	if err != nil {
		t.Fatalf("ReadFileContent(truncated.md): %v", err)
	}
	if resp.IsBinary {
		t.Errorf("expected truncated UTF-8 markdown file to have IsBinary=false, got true")
	}
	if resp.Content != string(fullContent) {
		t.Errorf("expected full content, got len=%d, want len=%d", len(resp.Content), len(fullContent))
	}

	// Test real servers.md content if the file exists on the host
	if realData, err := os.ReadFile("/data/operate/serversoper/servers.md"); err == nil {
		serversPath := filepath.Join(dir, "servers.md")
		if err := os.WriteFile(serversPath, realData, 0o644); err != nil {
			t.Fatalf("WriteFile servers.md: %v", err)
		}

		serversResp, err := svc.ReadFileContent(ctx, id, "servers.md")
		if err != nil {
			t.Fatalf("ReadFileContent(servers.md): %v", err)
		}
		if serversResp.IsBinary {
			t.Errorf("servers.md was incorrectly flagged as binary")
		}
		if len(serversResp.Content) != len(realData) {
			t.Errorf("expected full content for servers.md, got len=%d, want len=%d", len(serversResp.Content), len(realData))
		}
	}
}

func TestTrimTrailingIncompleteUTF8(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected []byte
	}{
		{
			name:     "empty",
			input:    []byte{},
			expected: []byte{},
		},
		{
			name:     "ascii only",
			input:    []byte("hello world"),
			expected: []byte("hello world"),
		},
		{
			name:     "complete utf8 3-byte",
			input:    []byte("hello测试"),
			expected: []byte("hello测试"),
		},
		{
			name:     "truncated 3-byte at 1st byte",
			input:    append([]byte("hello"), []byte("测")[:1]...),
			expected: []byte("hello"),
		},
		{
			name:     "truncated 3-byte at 2nd byte",
			input:    append([]byte("hello"), []byte("测")[:2]...),
			expected: []byte("hello"),
		},
		{
			name:     "truncated 4-byte emoji at 1st byte",
			input:    append([]byte("hello"), []byte("😀")[:1]...),
			expected: []byte("hello"),
		},
		{
			name:     "truncated 4-byte emoji at 3rd byte",
			input:    append([]byte("hello"), []byte("😀")[:3]...),
			expected: []byte("hello"),
		},
		{
			name:     "complete 4-byte emoji",
			input:    []byte("hello😀"),
			expected: []byte("hello😀"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := trimTrailingIncompleteUTF8(tt.input)
			if !bytes.Equal(got, tt.expected) {
				t.Errorf("trimTrailingIncompleteUTF8(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestReadFileContent_PathEscape(t *testing.T) {
	svc, id, _ := setupTestProject(t)
	ctx := context.Background()

	escapes := []string{
		"../outside.txt",
		"../../etc/passwd",
		"src/../../outside.txt",
		"/etc/passwd",
	}

	for _, p := range escapes {
		_, err := svc.ReadFileContent(ctx, id, p)
		if err == nil {
			t.Errorf("expected path escape error for %q, got nil", p)
		}
	}
}

func TestReadFileContent_AbsolutePathSupport(t *testing.T) {
	svc, id, rootDir := setupTestProject(t)
	ctx := context.Background()

	// 1. Full absolute path inside project
	absPath := filepath.Join(rootDir, "README.md")
	resp, err := svc.ReadFileContent(ctx, id, absPath)
	if err != nil {
		t.Fatalf("expected ReadFileContent with absPath to succeed, got %v", err)
	}
	if resp.Path != "README.md" {
		t.Errorf("expected normalized Path 'README.md', got %q", resp.Path)
	}

	// 2. Stripped leading slash absolute path (e.g. "data/projects/dev_platform/README.md")
	strippedAbs := strings.TrimPrefix(filepath.ToSlash(absPath), "/")
	resp2, err := svc.ReadFileContent(ctx, id, strippedAbs)
	if err != nil {
		t.Fatalf("expected ReadFileContent with strippedAbs to succeed, got %v", err)
	}
	if resp2.Path != "README.md" {
		t.Errorf("expected normalized Path 'README.md', got %q", resp2.Path)
	}

	// 3. Allowed external file in /tmp
	tmpFile, err := os.CreateTemp("", "agent_demo_*.py")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	if _, err := tmpFile.WriteString("print('hello external demo')"); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	tmpFile.Close()

	extResp, err := svc.ReadFileContent(ctx, id, tmpFile.Name())
	if err != nil {
		t.Fatalf("expected ReadFileContent for external temp file to succeed, got %v", err)
	}
	if extResp.Path != filepath.ToSlash(tmpFile.Name()) {
		t.Errorf("expected extResp.Path %q, got %q", tmpFile.Name(), extResp.Path)
	}
	if extResp.Content != "print('hello external demo')" {
		t.Errorf("expected external content 'print(\\'hello external demo\\')', got %q", extResp.Content)
	}

	// 4. Forbidden external files (e.g. /etc/hosts or /etc/passwd) must be rejected
	if _, err := svc.ReadFileContent(ctx, id, "/etc/passwd"); err == nil {
		t.Errorf("expected access to /etc/passwd to be rejected, got nil error")
	}
}

func TestHandleFileTreeAndContentRoutes(t *testing.T) {
	svc, id, _ := setupTestProject(t)

	r := gin.New()
	svc.RegisterRoutes(r)

	// Tree route
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/projects/"+strconv.FormatUint(uint64(id), 10)+"/files/tree", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body: %s", w.Code, w.Body.String())
	}

	var treeResp struct {
		Tree []FileTreeNode `json:"tree"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &treeResp); err != nil {
		t.Fatalf("unmarshal tree: %v", err)
	}
	if len(treeResp.Tree) == 0 {
		t.Fatalf("expected non-empty tree")
	}

	// Content route
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/projects/"+strconv.FormatUint(uint64(id), 10)+"/files/content?path=README.md", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body: %s", w.Code, w.Body.String())
	}

	var contentResp FileContentResponse
	if err := json.Unmarshal(w.Body.Bytes(), &contentResp); err != nil {
		t.Fatalf("unmarshal content: %v", err)
	}
	if contentResp.Content != "# Hello" {
		t.Fatalf("expected '# Hello', got %q", contentResp.Content)
	}

	// Content path escape route
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/projects/"+strconv.FormatUint(uint64(id), 10)+"/files/content?path=../outside.txt", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for path escape", w.Code)
	}

	// Raw route
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/projects/"+strconv.FormatUint(uint64(id), 10)+"/files/raw?path=README.md", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("raw route status = %d, want 200", w.Code)
	}
	if w.Body.String() != "# Hello" {
		t.Fatalf("expected raw body '# Hello', got %q", w.Body.String())
	}

	// Shallow tree with path query (level 1)
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/projects/"+strconv.FormatUint(uint64(id), 10)+"/files/tree?path=src", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body: %s", w.Code, w.Body.String())
	}
	var srcTreeResp struct {
		Tree []FileTreeNode `json:"tree"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &srcTreeResp); err != nil {
		t.Fatalf("unmarshal src tree: %v", err)
	}
	if len(srcTreeResp.Tree) == 0 || srcTreeResp.Tree[0].Name != "utils" {
		t.Fatalf("expected direct child 'utils', got %+v", srcTreeResp.Tree)
	}
	if srcTreeResp.Tree[0].Path != "src/utils" {
		t.Fatalf("expected child Path 'src/utils', got %q", srcTreeResp.Tree[0].Path)
	}

	// Shallow tree with nested path query (level 2)
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/projects/"+strconv.FormatUint(uint64(id), 10)+"/files/tree?path=src/utils", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 for level 2 tree, body: %s", w.Code, w.Body.String())
	}
	var utilsTreeResp struct {
		Tree []FileTreeNode `json:"tree"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &utilsTreeResp); err != nil {
		t.Fatalf("unmarshal utils tree: %v", err)
	}
	if len(utilsTreeResp.Tree) == 0 || utilsTreeResp.Tree[0].Path != "src/utils/helper.go" {
		t.Fatalf("expected nested path 'src/utils/helper.go', got %+v", utilsTreeResp.Tree)
	}

	// External temporary file route test
	extTmp, err := os.CreateTemp("", "route_demo_*.txt")
	if err != nil {
		t.Fatalf("create route demo temp: %v", err)
	}
	defer os.Remove(extTmp.Name())
	_, _ = extTmp.WriteString("external route content")
	extTmp.Close()

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/projects/"+strconv.FormatUint(uint64(id), 10)+"/files/content?path="+url.QueryEscape(extTmp.Name()), nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected external file content route status 200, got %d, body: %s", w.Code, w.Body.String())
	}
	var extContentResp FileContentResponse
	if err := json.Unmarshal(w.Body.Bytes(), &extContentResp); err != nil {
		t.Fatalf("unmarshal external content: %v", err)
	}
	if extContentResp.Content != "external route content" {
		t.Fatalf("expected 'external route content', got %q", extContentResp.Content)
	}

	// Path escape on tree route
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/projects/"+strconv.FormatUint(uint64(id), 10)+"/files/tree?path=../outside", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for tree path escape", w.Code)
	}
}

func TestGitStatus(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found in PATH")
	}

	dir := t.TempDir()
	runGit := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v, out: %s", args, err, out)
		}
	}

	runGit("init")
	runGit("config", "user.name", "Test")
	runGit("config", "user.email", "test@test.com")

	f1 := filepath.Join(dir, "committed.txt")
	_ = os.WriteFile(f1, []byte("initial"), 0o644)
	runGit("add", "committed.txt")
	runGit("commit", "-m", "init")

	// Now modify committed.txt and add untracked.txt
	_ = os.WriteFile(f1, []byte("modified"), 0o644)
	f2 := filepath.Join(dir, "untracked.txt")
	_ = os.WriteFile(f2, []byte("new"), 0o644)

	repo := &memoryRepo{projects: make(map[uint]Project)}
	p, _ := repo.Create("git-proj", dir)
	svc := NewServiceWithRepository(repo)

	status, err := svc.GitStatus(context.Background(), p.ID)
	if err != nil {
		t.Fatalf("GitStatus: %v", err)
	}

	if len(status) != 2 {
		t.Fatalf("expected 2 changed files, got %d: %+v", len(status), status)
	}

	var foundModified, foundUntracked bool
	for _, item := range status {
		if item.Path == "committed.txt" && item.Status == "modified" {
			foundModified = true
		}
		if item.Path == "untracked.txt" && item.Status == "untracked" {
			foundUntracked = true
		}
	}

	if !foundModified || !foundUntracked {
		t.Fatalf("status items missing expected changes: %+v", status)
	}
}

func TestPreviewTicketAndFileHandler(t *testing.T) {
	svc, id, dir := setupTestProject(t)

	// Create test html and nested css
	htmlPath := filepath.Join(dir, "index.html")
	if err := os.WriteFile(htmlPath, []byte("<!DOCTYPE html><html><body><h1>Hello</h1></body></html>"), 0o644); err != nil {
		t.Fatalf("write html: %v", err)
	}
	subDir := filepath.Join(dir, "css")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatalf("mkdir css: %v", err)
	}
	cssPath := filepath.Join(subDir, "style.css")
	if err := os.WriteFile(cssPath, []byte("body { color: red; }"), 0o644); err != nil {
		t.Fatalf("write css: %v", err)
	}

	// 1. Test Service CreatePreviewTicket & ValidatePreviewTicket
	ticket, err := svc.CreatePreviewTicket(context.Background(), id)
	if err != nil {
		t.Fatalf("CreatePreviewTicket failed: %v", err)
	}
	if len(ticket) != 64 {
		t.Fatalf("expected 64 hex chars ticket, got %d (%s)", len(ticket), ticket)
	}
	if !svc.ValidatePreviewTicket(ticket, id) {
		t.Fatalf("ValidatePreviewTicket should succeed with valid ticket and matching project")
	}
	if svc.ValidatePreviewTicket(ticket, id+999) {
		t.Fatalf("ValidatePreviewTicket should fail for mismatched project")
	}
	if svc.ValidatePreviewTicket("invalid-ticket", id) {
		t.Fatalf("ValidatePreviewTicket should fail for nonexistent ticket")
	}

	// 2. Test HTTP handler via Gin
	gin.SetMode(gin.TestMode)
	router := gin.New()
	svc.RegisterRoutes(router)
	svc.RegisterPreviewRoutes(router)

	// POST /projects/:id/files/preview-ticket
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/projects/"+strconv.FormatUint(uint64(id), 10)+"/files/preview-ticket", nil)
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("preview-ticket endpoint status = %d, body = %s", w.Code, w.Body.String())
	}
	var res map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil || res["ticket"] == "" {
		t.Fatalf("failed to decode ticket response: %v, body = %s", err, w.Body.String())
	}
	issuedTicket := res["ticket"]

	// GET /projects/:id/files/preview/:ticket/index.html
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/projects/"+strconv.FormatUint(uint64(id), 10)+"/files/preview/"+issuedTicket+"/index.html", nil)
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("preview html status = %d, body = %s", w.Code, w.Body.String())
	}
	if !bytes.Contains(w.Body.Bytes(), []byte("Hello")) {
		t.Fatalf("preview html missing body content: %s", w.Body.String())
	}
	// Check CSP and nosniff headers
	csp := w.Header().Get("Content-Security-Policy")
	if csp != "sandbox allow-scripts allow-forms allow-modals;" {
		t.Errorf("unexpected CSP header: %q", csp)
	}
	nosniff := w.Header().Get("X-Content-Type-Options")
	if nosniff != "nosniff" {
		t.Errorf("unexpected X-Content-Type-Options: %q", nosniff)
	}

	// GET nested relative asset /projects/:id/files/preview/:ticket/css/style.css
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/projects/"+strconv.FormatUint(uint64(id), 10)+"/files/preview/"+issuedTicket+"/css/style.css", nil)
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("preview css status = %d, body = %s", w.Code, w.Body.String())
	}
	if !bytes.Contains(w.Body.Bytes(), []byte("color: red")) {
		t.Fatalf("preview css missing body content: %s", w.Body.String())
	}

	// GET with invalid ticket should return 403
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/projects/"+strconv.FormatUint(uint64(id), 10)+"/files/preview/fake-ticket/index.html", nil)
	router.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("preview with fake ticket status = %d, want 403", w.Code)
	}
}

func TestFileUploadDownloadDelete(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc, _, root, _ := newTestService(t)

	dir := filepath.Join(root, "ops-test")
	mustMkdir(t, dir)
	p, err := svc.Register(dir)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	id := p.ID

	r := gin.New()
	svc.RegisterRoutes(r)

	// 1. Upload a file to project root
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "uploaded.txt")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	_, _ = part.Write([]byte("hello uploaded content"))
	_ = writer.Close()

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/projects/"+strconv.FormatUint(uint64(id), 10)+"/files/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("upload status = %d, body = %s", w.Code, w.Body.String())
	}

	// Verify file content was written on disk
	content, err := os.ReadFile(filepath.Join(dir, "uploaded.txt"))
	if err != nil {
		t.Fatalf("read uploaded file: %v", err)
	}
	if string(content) != "hello uploaded content" {
		t.Fatalf("expected 'hello uploaded content', got %q", string(content))
	}

	// 2. Download the uploaded file
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/projects/"+strconv.FormatUint(uint64(id), 10)+"/files/download?path=uploaded.txt", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("download status = %d, body = %s", w.Code, w.Body.String())
	}
	if disp := w.Header().Get("Content-Disposition"); !strings.Contains(disp, "attachment") || !strings.Contains(disp, "uploaded.txt") {
		t.Fatalf("expected Content-Disposition with attachment and uploaded.txt, got %q", disp)
	}
	if w.Body.String() != "hello uploaded content" {
		t.Fatalf("download content mismatch: %q", w.Body.String())
	}

	// 3. Delete the uploaded file
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodDelete, "/projects/"+strconv.FormatUint(uint64(id), 10)+"/files?path=uploaded.txt", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("delete status = %d, body = %s", w.Code, w.Body.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "uploaded.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected uploaded.txt to be removed, stat err = %v", err)
	}

	// 4. Try to delete root (must be rejected)
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodDelete, "/projects/"+strconv.FormatUint(uint64(id), 10)+"/files?path=.", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("delete root status = %d, want 400", w.Code)
	}

	// 5. Try path escape delete (must be rejected)
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodDelete, "/projects/"+strconv.FormatUint(uint64(id), 10)+"/files?path=../../outside", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("delete path escape status = %d, want 400", w.Code)
	}
}

func TestWriteFileContentAndHandler(t *testing.T) {
	svc, _, root, _ := newTestService(t)
	ctx := context.Background()

	dir := filepath.Join(root, "write-test")
	mustMkdir(t, dir)
	p, err := svc.Register(dir)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	id := p.ID

	// Prepare files
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write main.go: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".git", "config"), []byte("[core]\n"), 0o644); err != nil {
		t.Fatalf("write .git/config: %v", err)
	}
	extFile, err := os.CreateTemp("", "devplatform-ext-*.txt")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	extPath := extFile.Name()
	_, _ = extFile.WriteString("external")
	_ = extFile.Close()
	t.Cleanup(func() { _ = os.Remove(extPath) })

	gin.SetMode(gin.TestMode)
	r := gin.New()
	svc.RegisterRoutes(r)

	pid := strconv.FormatUint(uint64(id), 10)

	// 1. Update existing workspace file via PUT /projects/:id/files/content
	payload := `{"path":"main.go","content":"package main\n\nfunc main() {}\n"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/projects/"+pid+"/files/content", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("update status = %d, body = %s", w.Code, w.Body.String())
	}

	var resp FileContentResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Path != "main.go" || resp.Lines != 3 || resp.Content != "package main\n\nfunc main() {}\n" {
		t.Fatalf("unexpected response: %+v", resp)
	}

	diskBytes, err := os.ReadFile(filepath.Join(dir, "main.go"))
	if err != nil {
		t.Fatalf("read updated file: %v", err)
	}
	if string(diskBytes) != "package main\n\nfunc main() {}\n" {
		t.Fatalf("disk content mismatch: %q", string(diskBytes))
	}

	// 2. Reject .git metadata write
	if _, err := svc.WriteFileContent(ctx, id, ".git/config", "hacked"); !errors.Is(err, ErrPathEscaped) {
		t.Fatalf("expected ErrPathEscaped for .git write, got %v", err)
	}

	// 3. Reject external allowlisted path write (/tmp)
	if _, err := svc.WriteFileContent(ctx, id, extPath, "hacked"); !errors.Is(err, ErrPathEscaped) {
		t.Fatalf("expected ErrPathEscaped for external path write, got %v", err)
	}

	// 4. Reject path traversal
	if _, err := svc.WriteFileContent(ctx, id, "../../outside.txt", "hacked"); !errors.Is(err, ErrPathEscaped) {
		t.Fatalf("expected ErrPathEscaped for traversal, got %v", err)
	}

	// 5. Reject missing path in HTTP handler
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPut, "/projects/"+pid+"/files/content", strings.NewReader(`{"path":"","content":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("missing path status = %d, want 400", w.Code)
	}

	// 6. Reject non-existent file
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPut, "/projects/"+pid+"/files/content", strings.NewReader(`{"path":"no-such-file.go","content":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("non-existent file status = %d, want 404", w.Code)
	}
}
