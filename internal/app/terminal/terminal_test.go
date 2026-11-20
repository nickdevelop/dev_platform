package terminal

import (
	"context"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"devplatform/internal/app/database"
	"devplatform/internal/app/secrets"
	"devplatform/internal/core/platform"
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

func TestCheckSameOrigin(t *testing.T) {
	cases := []struct {
		name   string
		origin string
		host   string
		want   bool
	}{
		{
			name:   "empty origin allows non-browser clients",
			origin: "",
			host:   "localhost:8080",
			want:   true,
		},
		{
			name:   "matching origin and host",
			origin: "http://localhost:8080",
			host:   "localhost:8080",
			want:   true,
		},
		{
			name:   "matching origin https and host",
			origin: "https://dev.example.com",
			host:   "dev.example.com",
			want:   true,
		},
		{
			name:   "cross-site origin blocked",
			origin: "http://evil.com",
			host:   "localhost:8080",
			want:   false,
		},
		{
			name:   "subdomain cross-origin blocked",
			origin: "http://attacker.example.com",
			host:   "example.com",
			want:   false,
		},
		{
			name:   "port mismatch blocked",
			origin: "http://localhost:9090",
			host:   "localhost:8080",
			want:   false,
		},
		{
			name:   "malformed origin blocked",
			origin: "://invalid-url",
			host:   "localhost:8080",
			want:   false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, "/api/terminal/ws/0", nil)
			if err != nil {
				t.Fatalf("new request: %v", err)
			}
			req.Host = tc.host
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}
			if got := checkSameOrigin(req); got != tc.want {
				t.Errorf("checkSameOrigin(origin=%q, host=%q) = %v; want %v", tc.origin, tc.host, got, tc.want)
			}
		})
	}
}

func TestLocalSessionCwd(t *testing.T) {
	tempDir := t.TempDir()
	validDir := filepath.Join(tempDir, "workspace")
	if err := os.Mkdir(validDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	var capturedCmd *exec.Cmd
	p := fake.New(fake.WithPTYSupported(true))
	p.PTY().(*fake.PTY).StartFunc = func(cmd *exec.Cmd, rows, cols uint16) (platform.PTYSession, error) {
		capturedCmd = cmd
		return nil, nil
	}

	db := setupTestDB(t)
	repo := NewRepository(db)
	secretStore, _ := secrets.NewStore(filepath.Join(tempDir, "data"))
	service := NewService(repo, secretStore, p)
	ctx := context.Background()

	// 1. Connect with valid directory
	_, err := service.Connect(ctx, 0, 24, 80, validDir)
	if err != nil {
		t.Fatalf("connect valid dir: %v", err)
	}
	if capturedCmd == nil || capturedCmd.Dir != validDir {
		t.Errorf("expected cmd.Dir to be %q, got %q", validDir, capturedCmd.Dir)
	}

	// 2. Connect with non-existent directory
	capturedCmd = nil
	nonExistentDir := filepath.Join(tempDir, "not-exist")
	_, err = service.Connect(ctx, 0, 24, 80, nonExistentDir)
	if err != nil {
		t.Fatalf("connect non-existent dir: %v", err)
	}
	if capturedCmd == nil || capturedCmd.Dir != "" {
		t.Errorf("expected cmd.Dir to be empty for non-existent dir, got %q", capturedCmd.Dir)
	}
}
