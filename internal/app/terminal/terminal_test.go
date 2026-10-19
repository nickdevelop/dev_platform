package terminal

import (
	"context"
	"path/filepath"
	"testing"

	"devplatform/internal/app/database"
	"devplatform/internal/app/secrets"
	"devplatform/internal/core/platform/fake"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&database.TerminalServer{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func setupTestService(t *testing.T, ptySupported bool) (*Service, *secrets.Store) {
	t.Helper()
	db := setupTestDB(t)
	repo := NewRepository(db)
	tempDir := t.TempDir()
	secretStore, err := secrets.NewStore(filepath.Join(tempDir, "data"))
	if err != nil {
		t.Fatalf("create secret store: %v", err)
	}
	p := fake.New(fake.WithPTYSupported(ptySupported))
	service := NewService(repo, secretStore, p)
	return service, secretStore
}

func TestTerminalStatus(t *testing.T) {
	srvSupported, _ := setupTestService(t, true)
	status := srvSupported.Status()
	if !status.Supported {
		t.Errorf("expected supported to be true")
	}

	srvUnsupported, _ := setupTestService(t, false)
	statusUnsup := srvUnsupported.Status()
	if statusUnsup.Supported {
		t.Errorf("expected supported to be false")
	}
}

func TestLocalServerIncludedByDefault(t *testing.T) {
	srv, _ := setupTestService(t, true)
	ctx := context.Background()

	servers, err := srv.ListServers(ctx)
	if err != nil {
		t.Fatalf("list servers: %v", err)
	}
	if len(servers) != 1 {
		t.Fatalf("expected 1 server (local), got %d", len(servers))
	}
	if servers[0].ID != 0 || !servers[0].IsLocal {
		t.Errorf("expected local server with ID 0, got %+v", servers[0])
	}
}

func TestServerCRUD(t *testing.T) {
	srv, secretStore := setupTestService(t, true)
	ctx := context.Background()

	// 1. Create a third-party server
	created, err := srv.CreateServer(ctx, CreateServerRequest{
		Name:     "Test Server",
		Host:     "192.168.1.100",
		Port:     2222,
		Username: "ubuntu",
		AuthType: "password",
		Secret:   "secret-pass-123",
	})
	if err != nil {
		t.Fatalf("create server: %v", err)
	}
	if created.ID == 0 {
		t.Errorf("expected positive ID, got %d", created.ID)
	}
	if created.Name != "Test Server" || created.Host != "192.168.1.100" || created.Port != 2222 {
		t.Errorf("unexpected created server fields: %+v", created)
	}
	if !created.HasSecret {
		t.Errorf("expected has_secret to be true")
	}

	// 2. List servers should contain local and the created server
	list, err := srv.ListServers(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 servers, got %d", len(list))
	}

	// 3. Update server
	updated, err := srv.UpdateServer(ctx, created.ID, UpdateServerRequest{
		Name:     "Updated Server",
		Host:     "192.168.1.101",
		Port:     22,
		Username: "root",
		AuthType: "key",
		Secret:   "new-key-content",
	})
	if err != nil {
		t.Fatalf("update server: %v", err)
	}
	if updated.Name != "Updated Server" || updated.Host != "192.168.1.101" {
		t.Errorf("updated fields mismatch: %+v", updated)
	}

	// Verify secret update in secrets store
	srvRecord, err := srv.repo.Get(ctx, created.ID)
	if err != nil || srvRecord == nil {
		t.Fatalf("get record: %v", err)
	}
	secretVal, err := secretStore.Get(ctx, srvRecord.SecretRef)
	if err != nil {
		t.Fatalf("get secret: %v", err)
	}
	if secretVal != "new-key-content" {
		t.Errorf("expected secret 'new-key-content', got %q", secretVal)
	}

	// 4. Local server immutability
	_, err = srv.UpdateServer(ctx, 0, UpdateServerRequest{
		Name:     "Hacked Local",
		Host:     "localhost",
		Username: "root",
		AuthType: "password",
	})
	if err == nil {
		t.Errorf("expected error when updating local server")
	}

	err = srv.DeleteServer(ctx, 0)
	if err == nil {
		t.Errorf("expected error when deleting local server")
	}

	// 5. Delete third-party server
	err = srv.DeleteServer(ctx, created.ID)
	if err != nil {
		t.Fatalf("delete server: %v", err)
	}

	afterDelete, err := srv.ListServers(ctx)
	if err != nil {
		t.Fatalf("list after delete: %v", err)
	}
	if len(afterDelete) != 1 {
		t.Fatalf("expected 1 server after delete, got %d", len(afterDelete))
	}
}
