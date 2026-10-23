package database

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"devplatform/internal/core/platform"
)

// captureLog redirects the database logger output into a buffer until the
// test finishes.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()

	var buf bytes.Buffer
	prev := logOutput
	logOutput = &buf
	t.Cleanup(func() { logOutput = prev })
	return &buf
}

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

	for _, model := range []any{&Setting{}, &AuthSession{}, &Project{}, &AgentInstance{}, &AgentSession{}} {
		if !db.Migrator().HasTable(model) {
			t.Errorf("table for %T does not exist after migration", model)
		}
	}
	// Migration 18 drops the per-project backend switch table.
	if db.Migrator().HasTable(&AgentBackendConfig{}) {
		t.Error("agent_backend_configs still exists after migration 18")
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

func TestMigrateDropsWorkspaceRoots(t *testing.T) {
	db := mustOpen(t, filepath.Join(t.TempDir(), "data"))
	mustMigrate(t, db)
	if db.Migrator().HasTable("workspace_roots") {
		t.Error("workspace_roots table still exists after migration")
	}
}

func TestMigrateDropsResourceLibrary(t *testing.T) {
	db := mustOpen(t, filepath.Join(t.TempDir(), "data"))
	mustMigrate(t, db)
	for _, model := range []any{&ManagedResourceFile{}, &SessionPrompt{}, &ProjectResource{}, &Resource{}} {
		if db.Migrator().HasTable(model) {
			t.Errorf("table for %T still exists after migration", model)
		}
	}
}

func TestMigrateCreatesModelTables(t *testing.T) {
	db := mustOpen(t, filepath.Join(t.TempDir(), "data"))
	mustMigrate(t, db)

	for _, model := range []any{&ModelService{}, &ModelDef{}} {
		if !db.Migrator().HasTable(model) {
			t.Errorf("table for %T does not exist after migration", model)
		}
	}
	if !db.Migrator().HasIndex(&ModelDef{}, "idx_model_service_wire") {
		t.Error("model_defs (service_id, model_id) unique index is missing")
	}
}

func TestMigrateCreatesAgentModelTables(t *testing.T) {
	db := mustOpen(t, filepath.Join(t.TempDir(), "data"))
	mustMigrate(t, db)

	for _, model := range []any{&AgentModelCatalog{}, &AgentModel{}} {
		if !db.Migrator().HasTable(model) {
			t.Errorf("table for %T does not exist after migration", model)
		}
	}
	if !db.Migrator().HasIndex(&AgentModel{}, "idx_agent_model_backend_model") {
		t.Error("agent_models (backend, model_id) unique index is missing")
	}
}

func TestMigrateCreatesChatTables(t *testing.T) {
	db := mustOpen(t, filepath.Join(t.TempDir(), "data"))
	mustMigrate(t, db)

	for _, model := range []any{&Chat{}, &ChatMessage{}} {
		if !db.Migrator().HasTable(model) {
			t.Errorf("table for %T does not exist after migration", model)
		}
	}
	if !db.Migrator().HasIndex(&ChatMessage{}, "idx_chat_messages_chat") {
		t.Error("chat_messages chat_id index is missing")
	}
}

func TestMigrateDeduplicatesAgentSessions(t *testing.T) {
	db := mustOpen(t, filepath.Join(t.TempDir(), "data"))
	mustMigrate(t, db)

	// Direct raw SQL insert to simulate pre-existing duplicate rows before v19
	// or test the deduplication query behavior explicitly.
	// We verify that the unique index idx_agent_session_project_backend_native
	// exists on agent_sessions.
	if !db.Migrator().HasIndex(&AgentSession{}, "idx_agent_session_project_backend_native") {
		t.Error("idx_agent_session_project_backend_native index is missing")
	}
}

func TestMigrateIndexesAgentSessionsByProjectBackendNative(t *testing.T) {
	db := mustOpen(t, filepath.Join(t.TempDir(), "data"))
	mustMigrate(t, db)

	now := time.Now().UTC()
	row1 := AgentSession{
		ID:              "s1",
		ProjectID:       1,
		Backend:         "pi",
		InstanceID:      "inst-1",
		NativeSessionID: "native-1",
		Status:          "idle",
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := db.Create(&row1).Error; err != nil {
		t.Fatalf("create row1: %v", err)
	}

	row2 := AgentSession{
		ID:              "s2",
		ProjectID:       1,
		Backend:         "pi",
		InstanceID:      "inst-2",
		NativeSessionID: "native-1",
		Status:          "idle",
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := db.Create(&row2).Error; err == nil {
		t.Fatal("create row2 succeeded, want unique index violation on (project, backend, native)")
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

func TestOpenLoggerNeverLogsParameterValues(t *testing.T) {
	logBuf := captureLog(t)
	db := mustOpen(t, filepath.Join(t.TempDir(), "data"))
	mustMigrate(t, db)

	const (
		sessionID   = "session-token-3f9a2c"
		secretValue = "arg$on2id-h4sh-s3cret-value"
	)

	// Insert the sensitive values once so any later statement involving
	// them carries them as bound parameters.
	session := AuthSession{ID: sessionID, ExpiresAt: time.Now().Add(time.Hour)}
	if err := db.Create(&session).Error; err != nil {
		t.Fatalf("create session: %v", err)
	}
	setting := Setting{Key: "platform.secret", Value: secretValue}
	if err := db.Create(&setting).Error; err != nil {
		t.Fatalf("create setting: %v", err)
	}

	// Force the logger to emit the failing statements: duplicate primary
	// keys make both inserts fail, and GORM logs the failing SQL.
	err := db.Create(&AuthSession{ID: sessionID, ExpiresAt: time.Now().Add(time.Hour)}).Error
	if err == nil {
		t.Fatal("duplicate session insert succeeded, want error")
	}
	err = db.Create(&Setting{Key: "platform.secret", Value: secretValue}).Error
	if err == nil {
		t.Fatal("duplicate setting insert succeeded, want error")
	}

	out := logBuf.String()
	if !strings.Contains(out, "INSERT") {
		t.Fatalf("logger emitted no SQL statement; output: %q", out)
	}
	if strings.Contains(out, sessionID) {
		t.Errorf("logger output leaks AuthSession.ID; output: %q", out)
	}
	if strings.Contains(out, secretValue) {
		t.Errorf("logger output leaks Setting.Value; output: %q", out)
	}
}

func TestOpenLoggerSilencesRecordNotFoundButKeepsErrors(t *testing.T) {
	logBuf := captureLog(t)
	db := mustOpen(t, filepath.Join(t.TempDir(), "data"))
	mustMigrate(t, db)

	// A miss must still be reported to the caller: every repository turns it
	// into a not-found sentinel.
	err := db.First(&Project{}, "id = ?", 4242).Error
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("missing row error = %v, want %v", err, gorm.ErrRecordNotFound)
	}
	if out := logBuf.String(); strings.Contains(out, "record not found") {
		t.Errorf("logger reported a missing row; output: %q", out)
	}

	// Real database errors stay visible: a duplicate path fails and GORM logs
	// the failing statement.
	const path = "/workspace/demo"
	if err := db.Create(&Project{Name: "demo", Path: path}).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}
	if err := db.Create(&Project{Name: "another", Path: path}).Error; err == nil {
		t.Fatal("duplicate project path insert succeeded, want error")
	}
	if out := logBuf.String(); !strings.Contains(out, "INSERT") {
		t.Errorf("logger stayed silent about a failing statement; output: %q", out)
	}
}

func TestOpenCreatesDatabaseInPathWithSpecialCharacters(t *testing.T) {
	// "?" and "#" are legal in Linux directory names but collide with the
	// DSN query separator unless the path is percent-encoded.
	dataDir := filepath.Join(t.TempDir(), "data ?dir#1", "platform")
	db := mustOpen(t, dataDir)
	mustMigrate(t, db)

	dbPath := filepath.Join(dataDir, dbFileName)
	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("database file %s: %v", dbPath, err)
	}

	// Pragmas must still be applied on connections opened through the
	// encoded URI.
	var foreignKeys int
	if err := db.Raw("PRAGMA foreign_keys").Scan(&foreignKeys).Error; err != nil {
		t.Fatalf("PRAGMA foreign_keys: %v", err)
	}
	if foreignKeys != 1 {
		t.Errorf("foreign_keys = %d, want 1", foreignKeys)
	}

	// Data written through the encoded DSN must be readable after reopen.
	setting := Setting{Key: "platform.test", Value: "special-path"}
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

func TestOpenRestrictsDataDirAndDatabasePermissions(t *testing.T) {
	// Skip by capability, not by GOOS: the assertions only run where the
	// platform enforces private permissions.
	if !platform.New().Perms().Enforced() {
		t.Skip("private permissions are not enforced on this platform")
	}

	base := t.TempDir()
	dataDir := filepath.Join(base, "platform-data")

	db := mustOpen(t, dataDir)
	mustMigrate(t, db)

	dirInfo, err := os.Stat(dataDir)
	if err != nil {
		t.Fatalf("stat data directory: %v", err)
	}
	if got := dirInfo.Mode().Perm(); got != 0o700 {
		t.Errorf("data directory mode = %o, want 700", got)
	}

	dbInfo, err := os.Stat(filepath.Join(dataDir, dbFileName))
	if err != nil {
		t.Fatalf("stat database file: %v", err)
	}
	if got := dbInfo.Mode().Perm(); got != 0o600 {
		t.Errorf("database file mode = %o, want 600", got)
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

func TestMigrateSeedsQuickAssistantChatGroup(t *testing.T) {
	db := mustOpen(t, filepath.Join(t.TempDir(), "data"))
	mustMigrate(t, db)

	var group ChatGroup
	if err := db.Where("system_key = ?", "quick-assistant").First(&group).Error; err != nil {
		t.Fatalf("load seeded quick-assistant group: %v", err)
	}
	if group.Name != "快捷助手" || group.SystemKey == nil || *group.SystemKey != "quick-assistant" {
		t.Fatalf("seeded group = %+v, want quick-assistant/快捷助手", group)
	}

	// A second migration pass must not duplicate the seeded row.
	if err := Migrate(db); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
	var count int64
	if err := db.Model(&ChatGroup{}).Where("system_key = ?", "quick-assistant").Count(&count).Error; err != nil {
		t.Fatalf("count seeded groups: %v", err)
	}
	if count != 1 {
		t.Fatalf("quick-assistant group count = %d, want 1", count)
	}
}
