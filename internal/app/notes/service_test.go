package notes

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
	if err := db.AutoMigrate(&database.Note{}); err != nil {
		t.Fatalf("migrate note: %v", err)
	}
	return db
}

func TestNoteServiceCRUD(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)
	svc := NewService(repo)

	// 1. Create with empty title defaults to "无标题笔记"
	n1, err := svc.Create(CreateNoteRequest{
		Title:   "",
		Content: "Some draft content",
	})
	if err != nil {
		t.Fatalf("create note: %v", err)
	}
	if n1.ID == 0 || n1.Title != "无标题笔记" || n1.Content != "Some draft content" {
		t.Fatalf("unexpected note: %+v", n1)
	}

	// 2. Create note with explicit title and project ID
	n2, err := svc.Create(CreateNoteRequest{
		ProjectID: 10,
		Title:     "Architecture Plan",
		Content:   "# Architecture\nDetails here.",
	})
	if err != nil {
		t.Fatalf("create second note: %v", err)
	}
	if n2.ProjectID != 10 || n2.Title != "Architecture Plan" {
		t.Fatalf("unexpected second note: %+v", n2)
	}

	// 3. Get
	fetched, err := svc.Get(n2.ID)
	if err != nil {
		t.Fatalf("get note: %v", err)
	}
	if fetched.Title != "Architecture Plan" {
		t.Fatalf("expected Architecture Plan, got %s", fetched.Title)
	}

	// 4. Update
	newTitle := "Updated Architecture"
	newContent := "Updated content"
	var newProjID uint = 20
	updated, err := svc.Update(n2.ID, UpdateNoteRequest{
		Title:     &newTitle,
		Content:   &newContent,
		ProjectID: &newProjID,
	})
	if err != nil {
		t.Fatalf("update note: %v", err)
	}
	if updated.Title != newTitle || updated.Content != newContent || updated.ProjectID != 20 {
		t.Fatalf("unexpected updated note: %+v", updated)
	}

	// 5. Update with empty title returns ErrInvalidNoteTitle
	emptyTitle := "   "
	_, err = svc.Update(n2.ID, UpdateNoteRequest{Title: &emptyTitle})
	if err != ErrInvalidNoteTitle {
		t.Fatalf("expected ErrInvalidNoteTitle, got %v", err)
	}

	// 6. Delete
	if err := svc.Delete(n1.ID); err != nil {
		t.Fatalf("delete note: %v", err)
	}
	_, err = svc.Get(n1.ID)
	if err != ErrNoteNotFound {
		t.Fatalf("expected ErrNoteNotFound after delete, got %v", err)
	}

	// 7. Delete non-existent note returns ErrNoteNotFound
	if err := svc.Delete(99999); err != ErrNoteNotFound {
		t.Fatalf("expected ErrNoteNotFound, got %v", err)
	}
}

func TestNoteServiceListAndFilter(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)
	svc := NewService(repo)

	_, _ = svc.Create(CreateNoteRequest{ProjectID: 0, Title: "Global Quick Note", Content: "quick thoughts"})
	_, _ = svc.Create(CreateNoteRequest{ProjectID: 1, Title: "Project Alpha Todo", Content: "build api"})
	_, _ = svc.Create(CreateNoteRequest{ProjectID: 2, Title: "Project Beta Memo", Content: "release notes"})

	// List all
	all, err := svc.List("", nil)
	if err != nil || len(all) != 3 {
		t.Fatalf("expected 3 notes, got %d (err: %v)", len(all), err)
	}

	// Filter by project 1
	var proj1 uint = 1
	p1Notes, err := svc.List("", &proj1)
	if err != nil || len(p1Notes) != 1 || p1Notes[0].Title != "Project Alpha Todo" {
		t.Fatalf("expected 1 note for project 1, got %+v", p1Notes)
	}

	// Filter by global (project 0)
	var proj0 uint = 0
	p0Notes, err := svc.List("", &proj0)
	if err != nil || len(p0Notes) != 1 || p0Notes[0].Title != "Global Quick Note" {
		t.Fatalf("expected 1 global note, got %+v", p0Notes)
	}

	// Search keyword
	searched, err := svc.List("api", nil)
	if err != nil || len(searched) != 1 || searched[0].Title != "Project Alpha Todo" {
		t.Fatalf("expected 1 match for keyword 'api', got %+v", searched)
	}
}
