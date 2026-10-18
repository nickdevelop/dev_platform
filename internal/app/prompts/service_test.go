package prompts

import (
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"devplatform/internal/app/database"
)

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&database.Prompt{}); err != nil {
		t.Fatalf("migrate prompt: %v", err)
	}
	return db
}

func TestPromptServiceCRUD(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)
	svc := NewService(repo)

	// Validation checks on create
	_, err := svc.Create(CreatePromptRequest{Name: "", Content: "content"})
	if err != ErrInvalidPromptName {
		t.Fatalf("expected ErrInvalidPromptName, got %v", err)
	}

	_, err = svc.Create(CreatePromptRequest{Name: "p1", Content: ""})
	if err != ErrInvalidPromptContent {
		t.Fatalf("expected ErrInvalidPromptContent, got %v", err)
	}

	// Create success
	p1, err := svc.Create(CreatePromptRequest{
		Name:        "Code Review",
		Description: "Review code changes",
		Content:     "Please review this code:\n{user_prompt}",
	})
	if err != nil {
		t.Fatalf("create prompt: %v", err)
	}
	if p1.ID == 0 || p1.Name != "Code Review" || p1.Content != "Please review this code:\n{user_prompt}" {
		t.Fatalf("unexpected prompt: %+v", p1)
	}

	// Duplicate name check
	_, err = svc.Create(CreatePromptRequest{
		Name:        "Code Review",
		Description: "Another",
		Content:     "Hello",
	})
	if err != ErrDuplicatePromptName {
		t.Fatalf("expected ErrDuplicatePromptName, got %v", err)
	}

	// Create second prompt
	p2, err := svc.Create(CreatePromptRequest{
		Name:        "Commit Message",
		Description: "Generate commit msg",
		Content:     "Write git commit message for:\n{user_prompt}",
	})
	if err != nil {
		t.Fatalf("create p2: %v", err)
	}

	// Get
	got, err := svc.Get(p1.ID)
	if err != nil || got.ID != p1.ID {
		t.Fatalf("get prompt: %v", err)
	}

	_, err = svc.Get(99999)
	if err != ErrPromptNotFound {
		t.Fatalf("expected ErrPromptNotFound, got %v", err)
	}

	// List
	list, err := svc.List("")
	if err != nil || len(list) != 2 {
		t.Fatalf("list all: len=%d, err=%v", len(list), err)
	}

	// List with filter
	listFiltered, err := svc.List("Commit")
	if err != nil || len(listFiltered) != 1 || listFiltered[0].ID != p2.ID {
		t.Fatalf("list filtered: %+v, err=%v", listFiltered, err)
	}

	// Update
	newName := "Advanced Code Review"
	newContent := "Perform advanced review:\n{user_prompt}"
	updated, err := svc.Update(p1.ID, UpdatePromptRequest{
		Name:    &newName,
		Content: &newContent,
	})
	if err != nil || updated.Name != newName || updated.Content != newContent {
		t.Fatalf("update p1: %+v, err=%v", updated, err)
	}

	// Update with duplicate name
	_, err = svc.Update(p1.ID, UpdatePromptRequest{Name: &p2.Name})
	if err != ErrDuplicatePromptName {
		t.Fatalf("expected ErrDuplicatePromptName on update, got %v", err)
	}

	// Delete
	if err := svc.Delete(p1.ID); err != nil {
		t.Fatalf("delete p1: %v", err)
	}
	_, err = svc.Get(p1.ID)
	if err != ErrPromptNotFound {
		t.Fatalf("expected ErrPromptNotFound after delete, got %v", err)
	}

	// Delete non-existent
	if err := svc.Delete(99999); err != ErrPromptNotFound {
		t.Fatalf("expected ErrPromptNotFound on deleting missing id, got %v", err)
	}
}
