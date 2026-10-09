// Command platform runs the AI development platform service.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/term"
	"gorm.io/gorm"

	"devplatform/internal/agents"
	"devplatform/internal/agents/instances"
	"devplatform/internal/agents/opencode"
	"devplatform/internal/agents/pi"
	"devplatform/internal/auth"
	"devplatform/internal/database"
	platformhttp "devplatform/internal/http"
	"devplatform/internal/projects"
	platformruntime "devplatform/internal/runtime"
	"devplatform/internal/sessions"
	"devplatform/internal/workspaces"
)

// setPasswordCommand is the first-run subcommand that configures the platform
// password from a hidden TTY prompt.
const setPasswordCommand = "set-password"

func main() {
	args := os.Args[1:]
	if len(args) > 0 && args[0] == setPasswordCommand {
		if err := runSetPassword(args[1:]); err != nil {
			log.Fatalf("%s: %v", setPasswordCommand, err)
		}
		return
	}
	if err := runServer(args); err != nil {
		log.Fatalf("platform: %v", err)
	}
}

// resolveDataDir resolves where platform data lives: the -data-dir flag
// wins, then PLATFORM_DATA_DIR, then the user config directory, then a
// directory next to the working directory as a last resort.
func resolveDataDir(flagValue string) string {
	if flagValue != "" {
		return flagValue
	}
	if dir := os.Getenv("PLATFORM_DATA_DIR"); dir != "" {
		return dir
	}
	if base, err := os.UserConfigDir(); err == nil {
		return filepath.Join(base, "devplatform")
	}
	return filepath.Join(".", "devplatform-data")
}

// resolveListenAddr resolves the address the HTTP service binds: the
// -listen flag wins, then PLATFORM_LISTEN_ADDR (used by deployments and the
// E2E harness, e.g. "127.0.0.1:18080" for localhost-only or ":8080" for LAN
// binding), then the default :8080.
func resolveListenAddr(flagValue string) string {
	if flagValue != "" {
		return flagValue
	}
	if addr := os.Getenv("PLATFORM_LISTEN_ADDR"); addr != "" {
		return addr
	}
	return ":8080"
}

// resolveWebDir resolves the directory of built web UI assets to serve:
// the -web-dir flag wins, then PLATFORM_WEB_DIR, then the repository build
// output ./web/dist when it has been built. An empty result disables static
// hosting so the service runs as a pure API.
func resolveWebDir(flagValue string) string {
	if flagValue != "" {
		return flagValue
	}
	if dir := os.Getenv("PLATFORM_WEB_DIR"); dir != "" {
		return dir
	}
	if info, err := os.Stat(filepath.Join("web", "dist", "index.html")); err == nil && !info.IsDir() {
		return filepath.Join("web", "dist")
	}
	return ""
}

// openDatabase opens and migrates the platform database.
func openDatabase(dataDir string) (*gorm.DB, error) {
	db, err := database.Open(dataDir)
	if err != nil {
		return nil, err
	}
	if err := database.Migrate(db); err != nil {
		return nil, err
	}
	return db, nil
}

// closeDatabase closes the underlying SQL pool.
func closeDatabase(db *gorm.DB) {
	if db == nil {
		return
	}
	sqlDB, err := db.DB()
	if err == nil {
		_ = sqlDB.Close()
	}
}

// runSetPassword configures the platform password. The password is read from
// the terminal without echo and is never accepted from arguments, the
// environment or logs; an already-configured password is never overwritten.
func runSetPassword(args []string) error {
	fs := flag.NewFlagSet(setPasswordCommand, flag.ContinueOnError)
	dataDir := fs.String("data-dir", "", "platform data directory (default: $PLATFORM_DATA_DIR, then the user config directory)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return errors.New("refusing to read a password from arguments; run interactively")
	}

	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return errors.New("requires an interactive terminal")
	}

	fmt.Fprint(os.Stderr, "New platform password: ")
	first, err := term.ReadPassword(fd)
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return fmt.Errorf("read password: %w", err)
	}
	defer clear(first)

	if len(first) == 0 {
		return errors.New("password must not be empty")
	}

	fmt.Fprint(os.Stderr, "Confirm platform password: ")
	second, err := term.ReadPassword(fd)
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return fmt.Errorf("read password confirmation: %w", err)
	}
	defer clear(second)

	if string(first) != string(second) {
		return errors.New("passwords do not match")
	}

	db, err := openDatabase(resolveDataDir(*dataDir))
	if err != nil {
		return err
	}
	defer closeDatabase(db)

	if err := auth.SetPassword(db, string(first)); err != nil {
		return err
	}
	log.Print("platform password configured")
	return nil
}

// clear zeroes a secret buffer once it is no longer needed.
func clear(buf []byte) {
	for i := range buf {
		buf[i] = 0
	}
}

// runServer starts the HTTP service, wiring the auth service over the
// platform database, holding the data-directory single-instance lock, and
// serving the built web UI from the same origin when available.
func runServer(args []string) error {
	fs := flag.NewFlagSet("platform", flag.ContinueOnError)
	dataDirFlag := fs.String("data-dir", "", "platform data directory (default: $PLATFORM_DATA_DIR, then the user config directory)")
	listenFlag := fs.String("listen", "", "HTTP listen address host:port (default: $PLATFORM_LISTEN_ADDR, then :8080)")
	webDirFlag := fs.String("web-dir", "", "directory of built web UI assets to serve (default: $PLATFORM_WEB_DIR, then ./web/dist when built)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return errors.New("unexpected arguments; see -h")
	}

	dataDir := resolveDataDir(*dataDirFlag)

	// One platform service per data directory: take the single-instance lock
	// before opening the database, so a second process fails fast with a
	// clear error instead of racing on the same SQLite database.
	lock, err := platformruntime.AcquireLock(dataDir)
	if err != nil {
		return err
	}
	// Release runs after closeDatabase (defers unwind LIFO): the lock is
	// dropped only once the database is closed.
	defer lock.Release()

	db, err := openDatabase(dataDir)
	if err != nil {
		return err
	}
	defer closeDatabase(db)

	authService := auth.NewService(db)
	if configured, err := authService.PasswordConfigured(); err != nil {
		return err
	} else if !configured {
		log.Printf("no platform password configured; run %q first", setPasswordCommand)
	}

	// Route dumps and debug warnings are noise in production logs.
	gin.SetMode(gin.ReleaseMode)

	webDir := resolveWebDir(*webDirFlag)
	if webDir != "" {
		log.Printf("serving web UI from %s", webDir)
	}

	projectService := projects.NewService(db)
	// Backend adapters register themselves explicitly here (or through their
	// own Register helper); there is no package-init side effect, so tests can
	// build an isolated registry. Until an adapter is registered, starting an
	// instance of that backend is refused with a clear error.
	registry := agents.NewRegistry()
	// Register the installed backends. Registration only constructs the
	// adapter; it never probes or starts a backend, so a missing binary is
	// reported when an instance is started, not at boot.
	if err := pi.Register(registry); err != nil {
		return fmt.Errorf("register pi adapter: %w", err)
	}
	if err := opencode.Register(registry); err != nil {
		return fmt.Errorf("register opencode adapter: %w", err)
	}
	instanceService := instances.NewService(db, registry, projectService)
	sessionService := sessions.NewService(db, registry, instanceService)

	router := platformhttp.NewRouter(platformhttp.Dependencies{
		Auth:       authService,
		Workspaces: workspaces.NewService(db),
		Projects:   projectService,
		Instances:  instanceService,
		Sessions:   sessionService,
		WebDir:     webDir,
	})

	// Listen explicitly so the log reports the concrete bound address (the
	// port is resolved when the configured port is 0) and startup fails
	// fast on a busy port with a clear error.
	addr := resolveListenAddr(*listenFlag)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", addr, err)
	}

	server := &http.Server{
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		// Bodies are small and bounded (maxRequestBodyBytes); a slow client
		// must not be able to hold a connection open indefinitely.
		ReadTimeout: 30 * time.Second,
		// Responses are small JSON documents today. Revisit when streaming
		// agent output is added, since WriteTimeout caps the whole response.
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Serve(listener)
	}()
	log.Printf("platform listening on %s", listener.Addr())

	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("server error: %w", err)
		}
	case <-ctx.Done():
		log.Printf("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("graceful shutdown failed: %w", err)
		}
	}
	return nil
}
