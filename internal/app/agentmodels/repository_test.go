package agentmodels

import (
	"testing"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"devplatform/internal/app/database"
	"devplatform/internal/core/agents"
)

// openTestDB opens and migrates a throwaway platform database.
func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := database.Open(t.TempDir())
	if err != nil {
		t.Fatalf("database.Open: %v", err)
	}
	if err := database.Migrate(db); err != nil {
		t.Fatalf("database.Migrate: %v", err)
	}
	db.Logger = logger.Discard
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func TestRepositoryRoundTripPreservesSnapshot(t *testing.T) {
	repo := NewGORMRepository(openTestDB(t))
	synced := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	limit := 128000
	models := []agents.ModelInfo{
		{
			ID:             "acme/gpt",
			Name:           "GPT",
			Provider:       "acme",
			Variants:       []string{"fast", "thinking"},
			ContextLimit:   &limit,
			SupportsImages: true,
		},
		{ID: "acme/small", Name: "Small", Provider: "acme"},
	}

	if err := repo.Replace(agents.BackendPi, models, synced); err != nil {
		t.Fatalf("Replace: %v", err)
	}
	got, updatedAt, err := repo.List(agents.BackendPi)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if updatedAt == nil || !updatedAt.Equal(synced) {
		t.Fatalf("List updatedAt = %v, want %v", updatedAt, synced)
	}
	if len(got) != 2 || got[0].ID != "acme/gpt" || got[1].ID != "acme/small" {
		t.Fatalf("List models = %+v, want the stored order", got)
	}
	if len(got[0].Variants) != 2 || got[0].Variants[0] != "fast" || got[0].Variants[1] != "thinking" {
		t.Errorf("variants = %v, want the stored variants", got[0].Variants)
	}
	if got[0].ContextLimit == nil || *got[0].ContextLimit != limit || !got[0].SupportsImages {
		t.Errorf("model metadata = %+v, want context limit and image support", got[0])
	}
	if got[1].ContextLimit != nil || got[1].SupportsImages {
		t.Errorf("model metadata = %+v, want empty optional values", got[1])
	}
}

func TestRepositoryReplaceClearsOldRowsAndKeepsEmptySnapshot(t *testing.T) {
	repo := NewGORMRepository(openTestDB(t))
	first := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	if err := repo.Replace(agents.BackendPi, []agents.ModelInfo{{ID: "acme/gpt", Provider: "acme"}}, first); err != nil {
		t.Fatalf("Replace first: %v", err)
	}

	second := first.Add(time.Hour)
	if err := repo.Replace(agents.BackendPi, nil, second); err != nil {
		t.Fatalf("Replace empty: %v", err)
	}
	got, updatedAt, err := repo.List(agents.BackendPi)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("List models = %+v, want an empty snapshot", got)
	}
	if updatedAt == nil || !updatedAt.Equal(second) {
		t.Errorf("List updatedAt = %v, want the empty snapshot's sync time %v", updatedAt, second)
	}
}

func TestRepositoryNeverSynced(t *testing.T) {
	repo := NewGORMRepository(openTestDB(t))

	got, updatedAt, err := repo.List(agents.BackendPi)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("List models = %+v, want empty", got)
	}
	if updatedAt != nil {
		t.Errorf("List updatedAt = %v, want nil", updatedAt)
	}
}

func TestRepositoryUpdateAndPreserveOnRefresh(t *testing.T) {
	repo := NewGORMRepository(openTestDB(t))
	t1 := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	initial := []agents.ModelInfo{
		{ID: "acme/gpt", Name: "GPT", Provider: "acme"},
		{ID: "acme/old", Name: "Old", Provider: "acme"},
	}
	if err := repo.Replace(agents.BackendPi, initial, t1); err != nil {
		t.Fatalf("Replace initial: %v", err)
	}

	// Update acme/gpt: disable it and change custom name and context limit
	disabled := false
	newName := "Custom GPT"
	newLimit := 64000
	hasImg := true
	updated, err := repo.Update(agents.BackendPi, "acme/gpt", ModelPatch{
		Enabled:        &disabled,
		Name:           &newName,
		ContextLimit:   &newLimit,
		SupportsImages: &hasImg,
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Enabled != false || updated.Name != newName || *updated.ContextLimit != newLimit || !updated.SupportsImages {
		t.Errorf("updated model = %+v, want disabled custom name and limit", updated)
	}

	// Now backend reports a new list: acme/gpt still exists, acme/old is gone, acme/new is added
	t2 := t1.Add(time.Hour)
	refreshed := []agents.ModelInfo{
		{ID: "acme/gpt", Name: "GPT Upstream", Provider: "acme-v2", Variants: []string{"pro"}},
		{ID: "acme/new", Name: "New Model", Provider: "acme-v2"},
	}
	if err := repo.Replace(agents.BackendPi, refreshed, t2); err != nil {
		t.Fatalf("Replace refreshed: %v", err)
	}

	got, _, err := repo.List(agents.BackendPi)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("List models length = %d, want 2", len(got))
	}
	// acme/gpt should preserve disabled and custom name, but receive updated variants & provider
	if got[0].ID != "acme/gpt" || got[0].Enabled != false || got[0].Name != newName || *got[0].ContextLimit != newLimit || !got[0].SupportsImages {
		t.Errorf("got[0] = %+v, want preserved customizations", got[0])
	}
	if got[0].Provider != "acme-v2" || len(got[0].Variants) != 1 || got[0].Variants[0] != "pro" {
		t.Errorf("got[0] variants/provider = %+v, want upstream updates", got[0])
	}
	// acme/new should be enabled by default
	if got[1].ID != "acme/new" || got[1].Enabled != true || got[1].Name != "New Model" {
		t.Errorf("got[1] = %+v, want new enabled model", got[1])
	}
}
