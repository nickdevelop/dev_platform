package projects

import (
	"bytes"
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

func TestGitOpsLifecycle(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git CLI unavailable")
	}

	repoDir := initRepo(t, "main")
	svc, _, _, _ := newTestService(t)
	ctx := context.Background()

	project, err := svc.Register(repoDir)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	// 1. Initially clean
	status, err := svc.GitStatus(ctx, project.ID)
	if err != nil {
		t.Fatalf("GitStatus: %v", err)
	}
	if len(status) != 0 {
		t.Fatalf("want 0 changes, got %d", len(status))
	}

	// 2. Create a new file (untracked)
	newFilePath := filepath.Join(repoDir, "new.txt")
	if err := os.WriteFile(newFilePath, []byte("new file content"), 0o644); err != nil {
		t.Fatalf("write new.txt: %v", err)
	}

	status, err = svc.GitStatus(ctx, project.ID)
	if err != nil {
		t.Fatalf("GitStatus after new file: %v", err)
	}
	if len(status) != 1 || status[0].Path != "new.txt" || status[0].Status != "untracked" || status[0].Staged {
		t.Fatalf("unexpected status: %+v", status)
	}

	// 3. Stage the new file
	if err := svc.StageFiles(ctx, project.ID, []string{"new.txt"}); err != nil {
		t.Fatalf("StageFiles: %v", err)
	}

	status, err = svc.GitStatus(ctx, project.ID)
	if err != nil {
		t.Fatalf("GitStatus after stage: %v", err)
	}
	if len(status) != 1 || status[0].Path != "new.txt" || !status[0].Staged {
		t.Fatalf("unexpected status after stage: %+v", status)
	}

	// 4. Unstage the new file
	if err := svc.UnstageFiles(ctx, project.ID, []string{"new.txt"}); err != nil {
		t.Fatalf("UnstageFiles: %v", err)
	}

	status, err = svc.GitStatus(ctx, project.ID)
	if err != nil {
		t.Fatalf("GitStatus after unstage: %v", err)
	}
	if len(status) != 1 || status[0].Path != "new.txt" || status[0].Staged {
		t.Fatalf("unexpected status after unstage: %+v", status)
	}

	// 5. Discard the untracked file (should be removed from disk)
	if err := svc.DiscardFileChanges(ctx, project.ID, "new.txt"); err != nil {
		t.Fatalf("DiscardFileChanges untracked: %v", err)
	}
	if _, err := os.Stat(newFilePath); !os.IsNotExist(err) {
		t.Fatalf("new.txt still exists after discard")
	}

	// 6. Modify an existing tracked file (file.txt)
	trackedFilePath := filepath.Join(repoDir, "file.txt")
	if err := os.WriteFile(trackedFilePath, []byte("modified hello"), 0o644); err != nil {
		t.Fatalf("write file.txt: %v", err)
	}

	// Discard tracked file changes
	if err := svc.DiscardFileChanges(ctx, project.ID, "file.txt"); err != nil {
		t.Fatalf("DiscardFileChanges tracked: %v", err)
	}
	content, _ := os.ReadFile(trackedFilePath)
	if string(content) != "hello" {
		t.Fatalf("file.txt content after discard = %q, want 'hello'", string(content))
	}

	// 7. Stage all and Commit
	if err := os.WriteFile(trackedFilePath, []byte("hello world v2"), 0o644); err != nil {
		t.Fatalf("write file.txt: %v", err)
	}
	if err := svc.StageFiles(ctx, project.ID, nil); err != nil {
		t.Fatalf("StageFiles all: %v", err)
	}

	// Empty commit message should be rejected
	if _, err := svc.CommitChanges(ctx, project.ID, "   "); err != ErrEmptyCommitMessage {
		t.Fatalf("CommitChanges empty message: got %v, want ErrEmptyCommitMessage", err)
	}

	commitItem, err := svc.CommitChanges(ctx, project.ID, "feat: update hello world")
	if err != nil {
		t.Fatalf("CommitChanges: %v", err)
	}
	if commitItem == nil || commitItem.Message != "feat: update hello world" {
		t.Fatalf("unexpected commitItem: %+v", commitItem)
	}

	// 8. Branches: list, create and checkout
	branchRes, err := svc.ListBranches(ctx, project.ID)
	if err != nil {
		t.Fatalf("ListBranches: %v", err)
	}
	if branchRes.Current != "main" {
		t.Fatalf("current branch = %q, want 'main'", branchRes.Current)
	}

	// Create and switch to new branch
	if err := svc.CheckoutBranch(ctx, project.ID, "feat/git-ops", true); err != nil {
		t.Fatalf("CheckoutBranch create: %v", err)
	}

	branchRes, err = svc.ListBranches(ctx, project.ID)
	if err != nil {
		t.Fatalf("ListBranches: %v", err)
	}
	if branchRes.Current != "feat/git-ops" {
		t.Fatalf("current branch = %q, want 'feat/git-ops'", branchRes.Current)
	}

	// Switch back to main
	if err := svc.CheckoutBranch(ctx, project.ID, "main", false); err != nil {
		t.Fatalf("CheckoutBranch switch back: %v", err)
	}
	branchRes, _ = svc.ListBranches(ctx, project.ID)
	if branchRes.Current != "main" {
		t.Fatalf("current branch = %q, want 'main'", branchRes.Current)
	}

	// 9. Path traversal attempt in stage
	if err := svc.StageFiles(ctx, project.ID, []string{"../../etc/passwd"}); err != ErrPathEscaped {
		t.Fatalf("StageFiles traversal: got %v, want ErrPathEscaped", err)
	}
}

func TestGitOpsRoutes(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git CLI unavailable")
	}

	repoDir := initRepo(t, "main")
	svc, _, _, _ := newTestService(t)
	project, err := svc.Register(repoDir)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	svc.RegisterRoutes(router)

	pID := strconv.FormatUint(uint64(project.ID), 10)

	// 1. GET branches
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/projects/"+pID+"/git/branches", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET branches code = %d, want 200", rec.Code)
	}

	// 2. Modify file and POST stage
	if err := os.WriteFile(filepath.Join(repoDir, "file.txt"), []byte("routes test"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	body, _ := json.Marshal(map[string]any{"paths": []string{"file.txt"}})
	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/projects/"+pID+"/git/stage", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST stage code = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	// 3. POST unstage
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/projects/"+pID+"/git/unstage", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST unstage code = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	// 4. POST stage again & commit
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/projects/"+pID+"/git/stage", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	commitBody, _ := json.Marshal(map[string]any{"message": "feat: test route commit"})
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/projects/"+pID+"/git/commit", bytes.NewReader(commitBody))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST commit code = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	// 5. POST discard for untracked file
	untrackedPath := filepath.Join(repoDir, "discard_me.txt")
	_ = os.WriteFile(untrackedPath, []byte("temp"), 0o644)
	discardBody, _ := json.Marshal(map[string]any{"path": "discard_me.txt"})
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/projects/"+pID+"/git/discard", bytes.NewReader(discardBody))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST discard code = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(untrackedPath); !os.IsNotExist(err) {
		t.Fatalf("discard_me.txt still exists")
	}
}
