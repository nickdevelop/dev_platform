// Package database manages the platform's own persistence: opening the
// SQLite database and applying versioned schema migrations.
//
// The database lives under the platform data directory, never inside a
// managed project directory.
package database

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// dbFileName is the name of the SQLite database file inside the data
// directory.
const dbFileName = "platform.db"

// busyTimeoutMS is the SQLite busy timeout in milliseconds applied to every
// connection, so short write contention waits instead of failing.
const busyTimeoutMS = 5000

// logOutput is where database log messages are written. Tests redirect it
// to capture what the logger emits.
var logOutput io.Writer = os.Stdout

// newDBLogger builds the GORM logger for the platform database. It mirrors
// the default logger except that parameterized query logging is enabled:
// bound SQL values — which include session tokens, password hashes and
// other sensitive configuration — are never emitted, only statements with
// "?" placeholders.
func newDBLogger() logger.Interface {
	return logger.New(log.New(logOutput, "\r\n", log.LstdFlags), logger.Config{
		SlowThreshold:        200 * time.Millisecond,
		LogLevel:             logger.Warn,
		ParameterizedQueries: true,
		Colorful:             true,
	})
}

// buildDSN builds the SQLite DSN for dbPath. The path is embedded as a
// percent-encoded file: URI, so reserved characters that are legal in Linux
// directory names (such as "?") cannot collide with the DSN query
// separator. The pragmas run on every new pooled connection: foreign key
// enforcement, the busy timeout, and the WAL journal mode.
func buildDSN(dbPath string) string {
	uri := url.URL{Scheme: "file", Path: dbPath}
	params := url.Values{}
	params.Add("_pragma", "foreign_keys(1)")
	params.Add("_pragma", fmt.Sprintf("busy_timeout(%d)", busyTimeoutMS))
	params.Add("_pragma", "journal_mode(WAL)")
	return uri.String() + "?" + params.Encode()
}

// Open opens (creating it when missing) the platform SQLite database under
// dataDir and returns the GORM handle. The database file is placed directly
// under dataDir, which must be outside every managed project directory.
func Open(dataDir string) (*gorm.DB, error) {
	if dataDir == "" {
		return nil, errors.New("database: data directory is required")
	}
	// The platform data directory holds the SQLite database, which stores
	// the password hash and plaintext session identifiers; keep it
	// owner-only so another local user cannot read the database and hijack a
	// session.
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, fmt.Errorf("database: create data directory: %w", err)
	}

	dbPath := filepath.Join(dataDir, dbFileName)
	// Pre-create the database file with owner-only permissions so SQLite
	// never creates it with the process umask (typically 0644, group/world
	// readable). Chmod also tightens a file left by an earlier version.
	if err := createRestrictedFile(dbPath); err != nil {
		return nil, err
	}

	db, err := gorm.Open(sqlite.Open(buildDSN(dbPath)), &gorm.Config{
		Logger: newDBLogger(),
		// Surface driver constraint violations as gorm.ErrDuplicatedKey.
		TranslateError: true,
	})
	if err != nil {
		return nil, fmt.Errorf("database: open sqlite database at %s: %w", dbPath, err)
	}
	// The WAL and shared-memory sidecars carry the same data as the database;
	// restrict them too when SQLite has already created them. They are
	// re-restricted on the next write if SQLite recreates them.
	for _, sidecar := range []string{dbPath + "-wal", dbPath + "-shm"} {
		bestEffortChmod(sidecar, 0o600)
	}
	return db, nil
}

// createRestrictedFile creates path when missing and forces its mode to 0600
// on every open, tightening a pre-existing file with wider permissions.
func createRestrictedFile(path string) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("database: create database file %s: %w", path, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("database: close database file %s: %w", path, err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("database: restrict database file %s: %w", path, err)
	}
	return nil
}

// bestEffortChmod tightens the permissions of a file when it exists; a missing
// sidecar (SQLite has not created it yet) is not an error.
func bestEffortChmod(path string, mode os.FileMode) {
	_ = os.Chmod(path, mode)
}
