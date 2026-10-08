// Package database manages the platform's own persistence: opening the
// SQLite database and applying versioned schema migrations.
//
// The database lives under the platform data directory, never inside a
// managed project directory.
package database

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// dbFileName is the name of the SQLite database file inside the data
// directory.
const dbFileName = "platform.db"

// busyTimeoutMS is the SQLite busy timeout in milliseconds applied to every
// connection, so short write contention waits instead of failing.
const busyTimeoutMS = 5000

// Open opens (creating it when missing) the platform SQLite database under
// dataDir and returns the GORM handle. The database file is placed directly
// under dataDir, which must be outside every managed project directory.
func Open(dataDir string) (*gorm.DB, error) {
	if dataDir == "" {
		return nil, errors.New("database: data directory is required")
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("database: create data directory: %w", err)
	}

	dbPath := filepath.Join(dataDir, dbFileName)
	// Pragmas in the DSN run on every new pooled connection: foreign key
	// enforcement, the busy timeout, and the WAL journal mode.
	dsn := fmt.Sprintf("%s?_pragma=foreign_keys(1)&_pragma=busy_timeout(%d)&_pragma=journal_mode(WAL)",
		dbPath, busyTimeoutMS)

	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		// Surface driver constraint violations as gorm.ErrDuplicatedKey.
		TranslateError: true,
	})
	if err != nil {
		return nil, fmt.Errorf("database: open sqlite database at %s: %w", dbPath, err)
	}
	return db, nil
}
