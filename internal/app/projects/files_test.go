package projects

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
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

	// Create test structure:
	// dir/
	//   src/
	//     main.go
	//     utils/
	//       helper.go
	//   README.md
	//   node_modules/
	//     secret.js (should be ignored)
	//   .git/
	//     config (should be ignored)
	if err := os.MkdirAll(filepath.Join(dir, "src", "utils"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "node_modules"), 0o755); err != nil {
		t.Fatalf("mkdir node_modules: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}

	_ = os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Hello"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "src", "main.go"), []byte("package main"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "src", "utils", "helper.go"), []byte("package utils"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "node_modules", "secret.js"), []byte(""), 0o644)
	_ = os.WriteFile(filepath.Join(dir, ".git", "config"), []byte(""), 0o644)

	repo := &memoryRepo{projects: make(map[uint]Project)}
	p, _ := repo.Create("test-project", dir)
	svc := NewServiceWithRepository(repo)

	return svc, p.ID, dir
}

func TestSearchFiles(t *testing.T) {
	svc, id, _ := setupTestProject(t)
	ctx := context.Background()

	// 1. Search without query should return all non-ignored items
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

	// Must ignore node_modules and .git contents
	if paths["node_modules/secret.js"] || paths[".git/config"] {
		t.Fatalf("ignored directories leaked into results: %v", paths)
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
