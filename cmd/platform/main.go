// Command platform runs the AI development platform service.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"golang.org/x/term"
	"gorm.io/gorm"

	"devplatform/internal/auth"
	"devplatform/internal/database"
	platformhttp "devplatform/internal/http"
	"devplatform/internal/workspaces"
)

const listenAddr = ":8080"

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

// defaultDataDir resolves where platform data lives when no -data-dir is
// given. It can be overridden by PLATFORM_DATA_DIR.
func defaultDataDir() string {
	if dir := os.Getenv("PLATFORM_DATA_DIR"); dir != "" {
		return dir
	}
	if base, err := os.UserConfigDir(); err == nil {
		return filepath.Join(base, "devplatform")
	}
	return filepath.Join(".", "devplatform-data")
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
	dataDir := fs.String("data-dir", defaultDataDir(), "platform data directory")
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

	db, err := openDatabase(*dataDir)
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
// platform database.
func runServer(args []string) error {
	fs := flag.NewFlagSet("platform", flag.ContinueOnError)
	dataDir := fs.String("data-dir", defaultDataDir(), "platform data directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return errors.New("unexpected arguments; see -h")
	}

	db, err := openDatabase(*dataDir)
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

	router := platformhttp.NewRouter(platformhttp.Dependencies{
		Auth:       authService,
		Workspaces: workspaces.NewService(db),
	})

	server := &http.Server{
		Addr:              listenAddr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.ListenAndServe()
	}()
	log.Printf("platform listening on %s", listenAddr)

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
