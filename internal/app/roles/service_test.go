package roles

import (
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"devplatform/internal/app/database"
	"devplatform/internal/app/prompts"
	"devplatform/internal/core/agents"
)

type fakePromptReader struct {
	prompts map[uint]prompts.Prompt
}

func (f *fakePromptReader) Get(id uint) (prompts.Prompt, error) {
	if p, ok := f.prompts[id]; ok {
		return p, nil
	}
	return prompts.Prompt{}, prompts.ErrPromptNotFound
}

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&database.AgentRole{}); err != nil {
		t.Fatalf("migrate agent_roles: %v", err)
	}
	return db
}

func TestRoleServiceCRUDAndPromptSync(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)
	fakePrompts := &fakePromptReader{
		prompts: map[uint]prompts.Prompt{
			10: {ID: 10, Name: "Architect Prompt", Type: prompts.PromptTypeRole, Content: "You are a software architect."},
		},
	}
	svc := NewService(repo, fakePrompts)

	// Validation
	_, err := svc.Create(CreateRoleRequest{Name: "", Model: "gpt-4o"})
	if err != ErrInvalidRoleName {
		t.Fatalf("expected ErrInvalidRoleName, got %v", err)
	}
	_, err = svc.Create(CreateRoleRequest{Name: "Architect", Model: ""})
	if err != ErrInvalidRoleModel {
		t.Fatalf("expected ErrInvalidRoleModel, got %v", err)
	}

	// Create with prompt reference
	role, err := svc.Create(CreateRoleRequest{
		Name:        "Architect",
		Description: "Designs system architecture",
		Backend:     agents.BackendID("opencode"),
		Model:       "claude-3-5-sonnet",
		PromptID:    10,
		Color:       "#3b82f6",
	})
	if err != nil {
		t.Fatalf("create role: %v", err)
	}
	if role.ID == 0 || role.SystemPrompt != "You are a software architect." {
		t.Fatalf("expected synced system prompt, got: %+v", role)
	}

	// Get
	got, err := svc.Get(role.ID)
	if err != nil || got.ID != role.ID {
		t.Fatalf("get role: %v", err)
	}

	// List
	list, err := svc.List("")
	if err != nil || len(list) != 1 {
		t.Fatalf("list roles: len=%d, err=%v", len(list), err)
	}

	// Update prompt content in fakePromptReader and verify Get syncs it
	fakePrompts.prompts[10] = prompts.Prompt{ID: 10, Name: "Architect Prompt", Content: "Updated: You are a principal architect."}
	refreshed, err := svc.Get(role.ID)
	if err != nil || refreshed.SystemPrompt != "Updated: You are a principal architect." {
		t.Fatalf("expected refreshed prompt content, got: %s", refreshed.SystemPrompt)
	}

	// Delete
	if err := svc.Delete(role.ID); err != nil {
		t.Fatalf("delete role: %v", err)
	}
	_, err = svc.Get(role.ID)
	if err != ErrRoleNotFound {
		t.Fatalf("expected ErrRoleNotFound, got %v", err)
	}
}
