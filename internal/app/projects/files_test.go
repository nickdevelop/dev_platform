package projects

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
)

type memoryRepo struct {
	projects map[uint]Project
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

	// Must hide dot files and dot folders
	if paths[".env"] || paths[".hidden_dir"] || paths[".hidden_dir/secret.js"] {
		t.Fatalf("hidden items leaked into non-git results: %v", paths)
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

	// .gitignore excluded items (debug.log, node_modules) and .git must be filtered
	if paths["debug.log"] || paths["node_modules/dep.js"] || paths[".git"] || paths[".git/config"] {
		t.Fatalf("git ignored files leaked into results: %v", paths)
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

func TestGetFileTree_NonGit(t *testing.T) {
	svc, id, _ := setupTestProject(t)
	ctx := context.Background()

	tree, err := svc.GetFileTree(ctx, id)
	if err != nil {
		t.Fatalf("GetFileTree: %v", err)
	}

	// README.md, src (dir) -> utils (dir) -> helper.go
	if len(tree) != 2 {
		t.Fatalf("expected 2 root items (src dir and README.md), got %d: %+v", len(tree), tree)
	}

	// Directories first: src should be first, then README.md
	if tree[0].Name != "src" || !tree[0].IsDir {
		t.Errorf("expected first item to be src directory, got: %+v", tree[0])
	}
	if tree[1].Name != "README.md" || tree[1].IsDir {
		t.Errorf("expected second item to be README.md file, got: %+v", tree[1])
	}

	if len(tree[0].Children) == 0 {
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

