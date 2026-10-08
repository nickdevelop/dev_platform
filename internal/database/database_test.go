package database

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// mustOpen opens a database under dataDir and registers a cleanup that
// closes it when the test ends.
func mustOpen(t *testing.T, dataDir string) *gorm.DB {
	t.Helper()

	db, err := Open(dataDir)
	if err != nil {
		t.Fatalf("Open(%q) error = %v", dataDir, err)
	}
	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			err = sqlDB.Close()
		}
		if err != nil {
			t.Errorf("close database: %v", err)
		}
	})
	return db
}

// mustMigrate applies the versioned migrations, failing the test on error.
func mustMigrate(t *testing.T, db *gorm.DB) {
	t.Helper()

	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
}

// closeDB closes the database early so a test can reopen it. sql.DB.Close
// is idempotent, so the registered cleanup close is harmless.
func closeDB(t *testing.T, db *gorm.DB) {
	t.Helper()

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get *sql.DB: %v", err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("close database: %v", err)
	}
}

func TestOpenMigratesFreshDatabase(t *testing.T) {
	// The data directory does not exist yet; Open must create it.
	dataDir := filepath.Join(t.TempDir(), "data", "platform")

	db := mustOpen(t, dataDir)
	mustMigrate(t, db)

	for _, model := range []any{&Setting{}, &AuthSession{}, &WorkspaceRoot{}, &Project{}} {
		if !db.Migrator().HasTable(model) {
			t.Errorf("table for %T does not exist after migration", model)
		}
	}

	var applied int64
	if err := db.Model(&schemaMigration{}).Count(&applied).Error; err != nil {
		t.Fatalf("count schema_migrations: %v", err)
	}
	if applied == 0 {
		t.Fatal("schema_migrations records no applied migrations")
	}

	// Re-running migrations on an already-migrated database is a no-op.
	if err := Migrate(db); err != nil {
		t.Errorf("second Migrate() error = %v", err)
	}
	var afterReapply int64
	if err := db.Model(&schemaMigration{}).Count(&afterReapply).Error; err != nil {
		t.Fatalf("recount schema_migrations: %v", err)
	}
	if afterReapply != applied {
		t.Errorf("schema_migrations count after re-run = %d, want %d", afterReapply, applied)
	}
}

func TestOpenEnablesSQLitePragmas(t *testing.T) {
	db := mustOpen(t, filepath.Join(t.TempDir(), "data"))
	mustMigrate(t, db)

	var journalMode string
	if err := db.Raw("PRAGMA journal_mode").Scan(&journalMode).Error; err != nil {
		t.Fatalf("PRAGMA journal_mode: %v", err)
	}
	if !strings.EqualFold(journalMode, "wal") {
		t.Errorf("journal_mode = %q, want wal", journalMode)
	}

	var foreignKeys int
	if err := db.Raw("PRAGMA foreign_keys").Scan(&foreignKeys).Error; err != nil {
		t.Fatalf("PRAGMA foreign_keys: %v", err)
	}
	if foreignKeys != 1 {
		t.Errorf("foreign_keys = %d, want 1", foreignKeys)
	}

	var busyTimeout int
	if err := db.Raw("PRAGMA busy_timeout").Scan(&busyTimeout).Error; err != nil {
		t.Fatalf("PRAGMA busy_timeout: %v", err)
	}
	if busyTimeout != busyTimeoutMS {
		t.Errorf("busy_timeout = %d, want %d", busyTimeout, busyTimeoutMS)
	}
}

func TestSettingPersistsAcrossReopen(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "data")

	db := mustOpen(t, dataDir)
	mustMigrate(t, db)

	setting := Setting{Key: "platform.test", Value: "persistent-value"}
	if err := db.Create(&setting).Error; err != nil {
		t.Fatalf("create setting: %v", err)
	}
	closeDB(t, db)

	reopened := mustOpen(t, dataDir)
	mustMigrate(t, reopened)

	var got Setting
	if err := reopened.First(&got, "key = ?", setting.Key).Error; err != nil {
		t.Fatalf("load setting after reopen: %v", err)
	}
	if got.Value != setting.Value {
		t.Errorf("value after reopen = %q, want %q", got.Value, setting.Value)
	}
}

func TestProjectPathIsUnique(t *testing.T) {
	db := mustOpen(t, t.TempDir())
	mustMigrate(t, db)

	const sharedPath = "/workspace/demo"
	if err := db.Create(&Project{Name: "demo", Path: sharedPath}).Error; err != nil {
		t.Fatalf("create first project: %v", err)
	}

	// This insert is expected to fail; silence GORM's error logger so the
	// intentionally failing query keeps the test output pristine.
	duplicate := db.Session(&gorm.Session{Logger: logger.Discard})
	err := duplicate.Create(&Project{Name: "another", Path: sharedPath}).Error
	if err == nil {
		t.Fatal("inserting a project with an existing path succeeded, want error")
	}
	if !errors.Is(err, gorm.ErrDuplicatedKey) {
		t.Errorf("error = %v, want gorm.ErrDuplicatedKey", err)
	}

	// A distinct path must still insert.
	if err := db.Create(&Project{Name: "other", Path: "/workspace/other"}).Error; err != nil {
		t.Errorf("create project with distinct path: %v", err)
	}

	var count int64
	if err := db.Model(&Project{}).Count(&count).Error; err != nil {
		t.Fatalf("count projects: %v", err)
	}
	if count != 2 {
		t.Errorf("project count = %d, want 2", count)
	}
}

func TestDatabaseFileStaysUnderDataDir(t *testing.T) {
	base := t.TempDir()
	dataDir := filepath.Join(base, "platform-data")
	projectRoot := filepath.Join(base, "my-project")
	if err := os.MkdirAll(projectRoot, 0o755); err != nil {
		t.Fatalf("create project root: %v", err)
	}

	db := mustOpen(t, dataDir)
	mustMigrate(t, db)

	dbPath := filepath.Join(dataDir, dbFileName)
	if _, err := os.Stat(dbPath); err != nil {
		t.Errorf("database file %s: %v", dbPath, err)
	}

	entries, err := os.ReadDir(projectRoot)
	if err != nil {
		t.Fatalf("read project root: %v", err)
	}
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	if len(names) != 0 {
		t.Errorf("project root contains entries after opening database elsewhere: %v", names)
	}
}
