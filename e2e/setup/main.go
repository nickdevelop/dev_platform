// Command setup bootstraps an isolated platform data directory for the
// end-to-end test run: it opens and migrates the SQLite database at -data-dir
// and configures a throwaway platform password read from stdin. It exists so
// the E2E harness can run the real server without an interactive terminal;
// production password setup is `platform set-password`, which refuses
// non-interactive input. The password is never accepted from arguments, the
// environment, or logs.
package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"devplatform/internal/auth"
	"devplatform/internal/database"
)

func main() {
	dataDir := flag.String("data-dir", "", "platform data directory to initialize")
	flag.Parse()
	if *dataDir == "" {
		log.Fatal("setup: -data-dir is required")
	}

	raw, err := io.ReadAll(os.Stdin)
	if err != nil {
		log.Fatalf("setup: read password from stdin: %v", err)
	}
	password := strings.TrimRight(string(raw), "\r\n")
	if password == "" {
		log.Fatal("setup: empty password")
	}

	db, err := database.Open(*dataDir)
	if err != nil {
		log.Fatalf("setup: open database: %v", err)
	}
	if err := database.Migrate(db); err != nil {
		log.Fatalf("setup: migrate: %v", err)
	}
	if err := auth.SetPassword(db, password); err != nil {
		log.Fatalf("setup: set password: %v", err)
	}
	if sqlDB, err := db.DB(); err == nil {
		_ = sqlDB.Close()
	}
	fmt.Fprintln(os.Stderr, "e2e: platform password configured")
}
