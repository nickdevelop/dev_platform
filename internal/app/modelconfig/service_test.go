package modelconfig

import (
	"context"
	"errors"
	"testing"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"devplatform/internal/app/database"
)

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

// fakeStore is an in-memory SecretStore so tests can assert exactly which
// references were written and deleted.
type fakeStore struct{ values map[string]string }

func newFakeStore() *fakeStore { return &fakeStore{values: map[string]string{}} }

func (f *fakeStore) Set(_ context.Context, id, value string) error {
	f.values[id] = value
	return nil
}

func (f *fakeStore) Exists(_ context.Context, id string) (bool, error) {
	_, ok := f.values[id]
	return ok, nil
}

func (f *fakeStore) Delete(_ context.Context, id string) error {
	delete(f.values, id)
	return nil
}

func (f *fakeStore) Get(_ context.Context, id string) (string, error) {
	value, ok := f.values[id]
	if !ok {
		return "", errors.New("not found")
	}
	return value, nil
}

func newService(t *testing.T) (*Service, *fakeStore) {
	t.Helper()
	store := newFakeStore()
	return NewService(openTestDB(t), store), store
}

func TestCreateServiceWithAndWithoutKey(t *testing.T) {
	svc, store := newService(t)

	plain, err := svc.Create(ServiceInput{Name: "local", BaseURL: "http://localhost:11434/v1/", Enabled: true})
	if err != nil {
		t.Fatalf("Create without key: %v", err)
	}
	if plain.HasKey || plain.SecretRef != "" {
		t.Errorf("plain service = %+v, want no key", plain)
	}
	if plain.BaseURL != "http://localhost:11434/v1" {
		t.Errorf("BaseURL = %q, want the trailing slash trimmed", plain.BaseURL)
	}

	secured, err := svc.Create(ServiceInput{Name: "openai", BaseURL: "https://api.openai.com/v1", APIKey: "sk-secret", Enabled: true})
	if err != nil {
		t.Fatalf("Create with key: %v", err)
	}
	if !secured.HasKey || secured.SecretRef == "" {
		t.Errorf("secured service = %+v, want a key reference", secured)
	}
	if store.values[secured.SecretRef] != "sk-secret" {
		t.Errorf("stored secret = %q, want sk-secret", store.values[secured.SecretRef])
	}

	list, err := svc.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("List returned %d services, want 2", len(list))
	}
}

func TestUpdateServiceReplacesAndClearsKey(t *testing.T) {
	svc, store := newService(t)

	created, err := svc.Create(ServiceInput{Name: "svc", BaseURL: "https://a.example/v1", APIKey: "old-key", Enabled: true})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	oldRef := created.SecretRef

	replaced, err := svc.Update(created.ID, ServiceInput{Name: "svc", BaseURL: "https://a.example/v1", APIKey: "new-key", Enabled: true})
	if err != nil {
		t.Fatalf("Update replace: %v", err)
	}
	if replaced.SecretRef == "" || replaced.SecretRef == oldRef {
		t.Errorf("replaced secretRef = %q, want a new non-empty reference", replaced.SecretRef)
	}
	if _, ok := store.values[oldRef]; ok {
		t.Errorf("old secret %q was not deleted", oldRef)
	}
	if store.values[replaced.SecretRef] != "new-key" {
		t.Errorf("new secret = %q, want new-key", store.values[replaced.SecretRef])
	}

	cleared, err := svc.Update(created.ID, ServiceInput{Name: "svc", BaseURL: "https://a.example/v1", APIKey: "", Enabled: true})
	if err != nil {
		t.Fatalf("Update clear: %v", err)
	}
	if cleared.HasKey || cleared.SecretRef != "" {
		t.Errorf("cleared service = %+v, want no key", cleared)
	}
	if len(store.values) != 0 {
		t.Errorf("secret store = %v, want empty after clear", store.values)
	}
}

func TestDeleteServiceCascadesModelsAndKey(t *testing.T) {
	svc, store := newService(t)

	created, err := svc.Create(ServiceInput{Name: "svc", BaseURL: "https://a.example/v1", APIKey: "k", Enabled: true})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := svc.AddModel(created.ID, ModelInput{ModelID: "m1", Name: "M1", SupportsText: true, Enabled: true}); err != nil {
		t.Fatalf("AddModel: %v", err)
	}

	if err := svc.Delete(created.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := svc.List(); err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(store.values) != 0 {
		t.Errorf("secret store = %v, want empty after service delete", store.values)
	}
	if err := svc.Delete(created.ID); !errors.Is(err, ErrServiceNotFound) {
		t.Errorf("Delete unknown error = %v, want ErrServiceNotFound", err)
	}
}

func TestModelCRUDAndConflict(t *testing.T) {
	svc, _ := newService(t)

	created, err := svc.Create(ServiceInput{Name: "svc", BaseURL: "https://a.example/v1", Enabled: true})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	model, err := svc.AddModel(created.ID, ModelInput{
		ModelID: "gpt-4o", Name: "GPT-4o", SupportsText: true, SupportsImage: true,
		SupportsTools: true, ContextTokens: 128000, OutputTokens: 4096, Enabled: true,
	})
	if err != nil {
		t.Fatalf("AddModel: %v", err)
	}
	if !model.Usable {
		t.Error("model is not usable while enabled under an enabled service")
	}

	if _, err := svc.AddModel(created.ID, ModelInput{ModelID: "gpt-4o", Name: "dup", Enabled: true}); !errors.Is(err, ErrModelConflict) {
		t.Errorf("duplicate AddModel error = %v, want ErrModelConflict", err)
	}

	updated, err := svc.UpdateModel(model.ID, ModelInput{
		ModelID: "gpt-4o-mini", Name: "GPT-4o mini", SupportsText: true, Enabled: false,
	})
	if err != nil {
		t.Fatalf("UpdateModel: %v", err)
	}
	if updated.ModelID != "gpt-4o-mini" || updated.Enabled || updated.Usable {
		t.Errorf("updated model = %+v, want disabled gpt-4o-mini and not usable", updated)
	}

	if err := svc.DeleteModel(model.ID); err != nil {
		t.Fatalf("DeleteModel: %v", err)
	}
	if err := svc.DeleteModel(model.ID); !errors.Is(err, ErrModelNotFound) {
		t.Errorf("DeleteModel unknown error = %v, want ErrModelNotFound", err)
	}
}

func TestUsableReflectsServiceEnablement(t *testing.T) {
	svc, _ := newService(t)

	created, err := svc.Create(ServiceInput{Name: "svc", BaseURL: "https://a.example/v1", Enabled: true})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := svc.AddModel(created.ID, ModelInput{ModelID: "m", Name: "M", Enabled: true}); err != nil {
		t.Fatalf("AddModel: %v", err)
	}

	if _, err := svc.Update(created.ID, ServiceInput{Name: "svc", BaseURL: "https://a.example/v1", APIKey: "", Enabled: false}); err != nil {
		t.Fatalf("disable service: %v", err)
	}
	list, err := svc.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if list[0].Models[0].Usable {
		t.Error("model is usable under a disabled service")
	}
}

func TestInvalidInputsAreRejected(t *testing.T) {
	svc, _ := newService(t)

	for _, input := range []ServiceInput{
		{Name: "", BaseURL: "https://a.example/v1"},
		{Name: "svc", BaseURL: "ftp://a.example"},
		{Name: "svc", BaseURL: "not a url"},
	} {
		if _, err := svc.Create(input); !errors.Is(err, ErrInvalidService) {
			t.Errorf("Create(%+v) error = %v, want ErrInvalidService", input, err)
		}
	}

	created, _ := svc.Create(ServiceInput{Name: "svc", BaseURL: "https://a.example/v1", Enabled: true})
	if _, err := svc.AddModel(created.ID, ModelInput{ModelID: "", Name: "M", Enabled: true}); !errors.Is(err, ErrInvalidModel) {
		t.Errorf("empty model id error = %v, want ErrInvalidModel", err)
	}
	if _, err := svc.AddModel(created.ID, ModelInput{ModelID: "m", Name: "", Enabled: true}); !errors.Is(err, ErrInvalidModel) {
		t.Errorf("empty model name error = %v, want ErrInvalidModel", err)
	}
	if _, err := svc.AddModel(999, ModelInput{ModelID: "m", Name: "M", Enabled: true}); !errors.Is(err, ErrServiceNotFound) {
		t.Errorf("unknown service error = %v, want ErrServiceNotFound", err)
	}
}

func TestResolveKey(t *testing.T) {
	svc, _ := newService(t)

	created, _ := svc.Create(ServiceInput{Name: "svc", BaseURL: "https://a.example/v1", APIKey: "abc", Enabled: true})
	key, err := svc.ResolveKey(created.ID)
	if err != nil || key != "abc" {
		t.Errorf("ResolveKey = %q, %v; want abc", key, err)
	}
	plain, _ := svc.Create(ServiceInput{Name: "plain", BaseURL: "https://b.example/v1", Enabled: true})
	key, err = svc.ResolveKey(plain.ID)
	if err != nil || key != "" {
		t.Errorf("ResolveKey(plain) = %q, %v; want empty", key, err)
	}
}
