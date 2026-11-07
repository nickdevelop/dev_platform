package main

import (
	"context"
	"path/filepath"
	"testing"

	"gorm.io/gorm"

	"devplatform/internal/app/database"
	"devplatform/internal/app/modelconfig"
	"devplatform/internal/app/secrets"
)

// openModelConfig builds a real modelconfig service and its raw database
// handle over a temporary database, with one enabled service carrying the
// given models. The raw handle lets a test flip flags directly, because
// GORM's bool-with-default tags ignore a false default on insert.
func openModelConfig(t *testing.T, models ...modelconfig.ModelInput) (*modelconfig.Service, *gorm.DB) {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := database.Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	store, err := secrets.NewStore(filepath.Join(t.TempDir(), "secrets"))
	if err != nil {
		t.Fatalf("SecretStore: %v", err)
	}
	service := modelconfig.NewService(db, store)
	if _, err := service.Create(modelconfig.ServiceInput{Name: "svc", BaseURL: "http://127.0.0.1:9999/v1", Enabled: true}); err != nil {
		t.Fatalf("Create service: %v", err)
	}
	for _, model := range models {
		if _, err := service.AddModel(1, model); err != nil {
			t.Fatalf("AddModel %s: %v", model.ModelID, err)
		}
	}
	return service, db
}

// TestBuiltinModelResolverListsToolAndFallbackTextModels covers the resolver's
// contract: tool-capable models surface first, and when no model carries the
// supports-tools flag (the platform default for new models!) the catalog still
// falls back to all usable text models instead of going empty. A regression
// here is why “刷新模型” on the builtin tab returned nothing.
func TestBuiltinModelResolverListsToolAndFallbackTextModels(t *testing.T) {
	toolModel := modelconfig.ModelInput{ModelID: "tools-on", Name: "工具模型", SupportsText: true, SupportsTools: true, Enabled: true}
	textModel := modelconfig.ModelInput{ModelID: "tools-off", Name: "文本模型", SupportsText: true, Enabled: true}
	service, _ := openModelConfig(t, toolModel, textModel)
	resolver := newBuiltinModelResolver(service)

	models, err := resolver.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("ListModels = %d models, want 2", len(models))
	}
	if models[0].Name != "工具模型" {
		t.Fatalf("tool-capable model must surface first, got %+v", models[0])
	}

	// Without any tool-capable model, the fallback keeps the agent usable.
	fallbackService, _ := openModelConfig(t, textModel)
	fallback, err := newBuiltinModelResolver(fallbackService).ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels fallback: %v", err)
	}
	if len(fallback) != 1 || fallback[0].Name != "文本模型" {
		t.Fatalf("fallback list = %+v, want the text model", fallback)
	}

	// Catalog ids are wire model ids, not database numbers.
	if fallback[0].ID != "tools-off" {
		t.Fatalf("catalog id = %q, want the wire id", fallback[0].ID)
	}

	// The catalog id is exact: resolution goes straight to the flagged row.
	endpoint, err := newBuiltinModelResolver(fallbackService).ResolveModel(context.Background(), fallback[0].ID)
	if err != nil {
		t.Fatalf("ResolveModel(%q): %v", fallback[0].ID, err)
	}
	if endpoint.ModelID != "tools-off" || endpoint.BaseURL != "http://127.0.0.1:9999/v1" {
		t.Fatalf("ResolveModel = %+v", endpoint)
	}

	// Sessions created before the id switch carry numeric refs; they still
	// resolve (the fallback database's model 1 is tools-off).
	legacy, err := newBuiltinModelResolver(fallbackService).ResolveModel(context.Background(), "1")
	if err != nil {
		t.Fatalf("ResolveModel legacy numeric: %v", err)
	}
	if legacy.ModelID != "tools-off" {
		t.Fatalf("legacy resolve = %+v", legacy)
	}
}

// TestBuiltinModelResolverQualifiesDuplicateWireIds covers two services that
// expose the same wire model id: the catalog qualifies them with the provider,
// a bare ref becomes ambiguous, and the composite ref resolves exactly.
func TestBuiltinModelResolverQualifiesDuplicateWireIds(t *testing.T) {
	service, db := openModelConfig(t,
		modelconfig.ModelInput{ModelID: "shared", Name: "首选共享模型", SupportsText: true, SupportsTools: true, Enabled: true},
	)
	if _, err := service.Create(modelconfig.ServiceInput{Name: "第二服务", BaseURL: "http://127.0.0.1:8888/v1", Enabled: true}); err != nil {
		t.Fatalf("Create second service: %v", err)
	}
	if _, err := service.AddModel(2, modelconfig.ModelInput{ModelID: "shared", Name: "第二共享模型", SupportsText: true, Enabled: true}); err != nil {
		t.Fatalf("AddModel second: %v", err)
	}
	_ = db
	resolver := newBuiltinModelResolver(service)

	models, err := resolver.ListModels(context.Background())
	if err != nil || len(models) != 2 {
		t.Fatalf("ListModels = (%d entries, %v)", len(models), err)
	}
	if models[0].ID != "svc/shared" || models[1].ID != "第二服务/shared" {
		t.Fatalf("catalog ids = %q, %q, want provider-qualified", models[0].ID, models[1].ID)
	}

	if _, err := resolver.ResolveModel(context.Background(), "shared"); err == nil {
		t.Fatal("a bare duplicated wire id must be refused as ambiguous")
	}
	endpoint, err := resolver.ResolveModel(context.Background(), "svc/shared")
	if err != nil || endpoint.ModelID != "shared" {
		t.Fatalf("ResolveModel(svc/shared) = (%+v, %v)", endpoint, err)
	}
}

// TestBuiltinModelResolverSkipsUnusable verifies disabled services and models
// never surface.
func TestBuiltinModelResolverSkipsUnusable(t *testing.T) {
	service, db := openModelConfig(t,
		modelconfig.ModelInput{ModelID: "off-model", Name: "停用模型", SupportsText: true, SupportsTools: true},
	)
	// GORM's bool-with-default tags make an inserted false read back true, so
	// disable the row directly.
	if err := db.Model(&database.ModelDef{}).Where("model_id = ?", "off-model").Update("enabled", false).Error; err != nil {
		t.Fatalf("disable model: %v", err)
	}
	resolver := newBuiltinModelResolver(service)
	models, err := resolver.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(models) != 0 {
		t.Fatalf("ListModels = %+v, want no entries", models)
	}
}
