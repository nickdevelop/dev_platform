package proxy

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"devplatform/internal/app/database"
)

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := database.Migrate(db); err != nil {
		t.Fatalf("migrate db: %v", err)
	}
	return db
}

func TestProxyService_ValidationAndCRUD(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)
	svc := NewService(repo)

	// Create a temp directory for directory proxy test
	tempDir := t.TempDir()
	indexFile := filepath.Join(tempDir, "index.html")
	if err := os.WriteFile(indexFile, []byte("<h1>Demo</h1>"), 0o644); err != nil {
		t.Fatalf("write index.html: %v", err)
	}

	// 1. Invalid alias
	_, err := svc.Create(CreateProxyRequest{
		Name:   "Invalid Alias",
		Alias:  "api", // reserved
		Type:   TypeDirectory,
		Target: tempDir,
	})
	if err != ErrInvalidProxyAlias {
		t.Errorf("got error %v, want ErrInvalidProxyAlias", err)
	}

	// 2. Invalid port
	_, err = svc.Create(CreateProxyRequest{
		Name:   "Invalid Port",
		Alias:  "port-bad",
		Type:   TypePort,
		Target: "80", // privileged port < 1024
	})
	if err != ErrInvalidProxyTarget {
		t.Errorf("got error %v, want ErrInvalidProxyTarget", err)
	}

	// 3. Create valid directory proxy
	dirProxy, err := svc.Create(CreateProxyRequest{
		Name:   "My Static Demo",
		Alias:  "static-demo",
		Type:   TypeDirectory,
		Target: tempDir,
	})
	if err != nil {
		t.Fatalf("create directory proxy: %v", err)
	}
	if dirProxy.Alias != "static-demo" || !dirProxy.Enabled {
		t.Errorf("unexpected proxy data: %+v", dirProxy)
	}

	// 4. Duplicate alias
	_, err = svc.Create(CreateProxyRequest{
		Name:   "Duplicate",
		Alias:  "static-demo",
		Type:   TypePort,
		Target: "3000",
	})
	if err != ErrDuplicateProxyAlias {
		t.Errorf("got error %v, want ErrDuplicateProxyAlias", err)
	}

	// 5. Create valid port proxy
	portProxy, err := svc.Create(CreateProxyRequest{
		Name:   "My Port Demo",
		Alias:  "node-demo",
		Type:   TypePort,
		Target: "3000",
	})
	if err != nil {
		t.Fatalf("create port proxy: %v", err)
	}

	// 6. List
	list, err := svc.List()
	if err != nil {
		t.Fatalf("list proxies: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("got %d proxies, want 2", len(list))
	}

	// 7. Toggle
	toggled, err := svc.Toggle(dirProxy.ID)
	if err != nil {
		t.Fatalf("toggle proxy: %v", err)
	}
	if toggled.Enabled {
		t.Errorf("expected proxy to be disabled after toggle")
	}

	// 8. Delete
	if err := svc.Delete(portProxy.ID); err != nil {
		t.Fatalf("delete proxy: %v", err)
	}
	listAfterDelete, err := svc.List()
	if err != nil {
		t.Fatalf("list proxies after delete: %v", err)
	}
	if len(listAfterDelete) != 1 {
		t.Errorf("got %d proxies after delete, want 1", len(listAfterDelete))
	}
}

func TestBrowseDirectories(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)
	svc := NewService(repo)

	// Create a temp folder hierarchy:
	// tempDir/
	//   ├── subA/
	//   ├── subB/
	//   ├── .hiddenDir/
	//   └── file.txt
	tempDir := t.TempDir()
	subA := filepath.Join(tempDir, "subA")
	subB := filepath.Join(tempDir, "subB")
	hidden := filepath.Join(tempDir, ".hiddenDir")
	file := filepath.Join(tempDir, "file.txt")

	_ = os.Mkdir(subA, 0o755)
	_ = os.Mkdir(subB, 0o755)
	_ = os.Mkdir(hidden, 0o755)
	_ = os.WriteFile(file, []byte("content"), 0o644)

	res, err := svc.BrowseDirectories(tempDir)
	if err != nil {
		t.Fatalf("browse directories: %v", err)
	}

	if len(res.Entries) != 2 {
		t.Fatalf("expected 2 directory entries, got %d", len(res.Entries))
	}
	if res.Entries[0].Name != "subA" || res.Entries[1].Name != "subB" {
		t.Errorf("unexpected directory entries: %+v", res.Entries)
	}
	if res.Current == "" || res.Parent == "" {
		t.Errorf("expected non-empty current and parent, got current=%q parent=%q", res.Current, res.Parent)
	}

	// Non-existent directory
	_, err = svc.BrowseDirectories(filepath.Join(tempDir, "does-not-exist"))
	if err == nil {
		t.Errorf("expected error for non-existent path")
	}

	// Empty path defaults to the filesystem root, not an error.
	for _, input := range []string{"", "   "} {
		root, err := svc.BrowseDirectories(input)
		if err != nil {
			t.Fatalf("browse empty path %q: %v", input, err)
		}
		if root.Current != "/" {
			t.Errorf("browse %q: current = %q, want %q", input, root.Current, "/")
		}
		if root.Parent != "" {
			t.Errorf("browse %q: parent = %q, want empty at root", input, root.Parent)
		}
		for _, e := range root.Entries {
			if !filepath.IsAbs(e.Path) || e.Path == "/" {
				t.Errorf("browse %q: unexpected entry path %q", input, e.Path)
			}
		}
	}
}

func TestProxyGateway(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)
	svc := NewService(repo)

	// Create temp directory with html file and a text file
	tempDir := t.TempDir()
	indexFile := filepath.Join(tempDir, "index.html")
	if err := os.WriteFile(indexFile, []byte("<h1>Demo Content</h1>"), 0o644); err != nil {
		t.Fatalf("write index.html: %v", err)
	}
	textFile := filepath.Join(tempDir, "hello.txt")
	if err := os.WriteFile(textFile, []byte("Hello Text"), 0o644); err != nil {
		t.Fatalf("write hello.txt: %v", err)
	}

	// Register directory proxy
	_, err := svc.Create(CreateProxyRequest{
		Name:   "Static Demo",
		Alias:  "demo",
		Type:   TypeDirectory,
		Target: tempDir,
	})
	if err != nil {
		t.Fatalf("create proxy: %v", err)
	}

	// Start a mock HTTP backend on a dynamic port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen tcp: %v", err)
	}
	defer listener.Close()

	port := listener.Addr().(*net.TCPAddr).Port
	mockServer := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			prefix := r.Header.Get("X-Forwarded-Prefix")
			w.Header().Set("Content-Type", "text/plain")
			_, _ = fmt.Fprintf(w, "backend received path: %s, prefix: %s", r.URL.Path, prefix)
		}),
	}
	go func() {
		_ = mockServer.Serve(listener)
	}()
	defer mockServer.Close()

	// Register port proxy
	_, err = svc.Create(CreateProxyRequest{
		Name:   "Port Demo",
		Alias:  "srv",
		Type:   TypePort,
		Target: fmt.Sprintf("%d", port),
	})
	if err != nil {
		t.Fatalf("create port proxy: %v", err)
	}

	// Create test server with gateway
	engine := setupTestRouterWithGateway(svc)
	server := httptest.NewServer(engine)
	defer server.Close()

	client := server.Client()

	// Test 1: Directory serving /proxy/demo/
	res1, err := client.Get(server.URL + "/proxy/demo/")
	if err != nil {
		t.Fatalf("get /proxy/demo/: %v", err)
	}
	defer res1.Body.Close()
	if res1.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", res1.StatusCode)
	}
	b1, _ := io.ReadAll(res1.Body)
	if string(b1) != "<h1>Demo Content</h1>" {
		t.Errorf("got body %q, want %q", string(b1), "<h1>Demo Content</h1>")
	}

	// Test 2: Sub-file /proxy/demo/hello.txt
	res2, err := client.Get(server.URL + "/proxy/demo/hello.txt")
	if err != nil {
		t.Fatalf("get /proxy/demo/hello.txt: %v", err)
	}
	defer res2.Body.Close()
	if res2.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", res2.StatusCode)
	}
	b2, _ := io.ReadAll(res2.Body)
	if string(b2) != "Hello Text" {
		t.Errorf("got body %q, want %q", string(b2), "Hello Text")
	}

	// Test 3: Port reverse proxying
	res3, err := client.Get(server.URL + "/proxy/srv/hello/world")
	if err != nil {
		t.Fatalf("get /proxy/srv/hello/world: %v", err)
	}
	defer res3.Body.Close()
	if res3.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", res3.StatusCode)
	}
	b3, _ := io.ReadAll(res3.Body)
	expected := "backend received path: /hello/world, prefix: /proxy/srv"
	if string(b3) != expected {
		t.Errorf("got body %q, want %q", string(b3), expected)
	}

	// Test 4: Unknown proxy -> 404
	res4, err := client.Get(server.URL + "/proxy/not-found/test")
	if err != nil {
		t.Fatalf("get unknown proxy: %v", err)
	}
	defer res4.Body.Close()
	if res4.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", res4.StatusCode)
	}
}

func setupTestRouterWithGateway(svc *Service) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	RegisterGateway(r, svc)
	return r
}

