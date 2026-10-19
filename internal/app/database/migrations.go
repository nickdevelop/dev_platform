package database

import (
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// schemaMigration records one applied migration version; it is the
// bookkeeping table for the versioned migration steps below.
type schemaMigration struct {
	Version   int64     `gorm:"primaryKey;column:version;not null"`
	Name      string    `gorm:"column:name;not null"`
	AppliedAt time.Time `gorm:"column:applied_at"`
}

// TableName pins the bookkeeping table name.
func (schemaMigration) TableName() string { return "schema_migrations" }

// migrationStep is one explicit, versioned schema change.
type migrationStep struct {
	version int64
	name    string
	run     func(tx *gorm.DB) error
}

// migrationSteps lists every schema version in order. Each step runs inside
// a transaction together with its schema_migrations record, so a failed
// migration leaves no half-applied state behind.
var migrationSteps = []migrationStep{
	{version: 1, name: "create_settings", run: func(tx *gorm.DB) error {
		return tx.Migrator().CreateTable(&Setting{})
	}},
	{version: 2, name: "create_auth_sessions", run: func(tx *gorm.DB) error {
		return tx.Migrator().CreateTable(&AuthSession{})
	}},
	{version: 3, name: "create_workspace_roots", run: func(tx *gorm.DB) error {
		return tx.Migrator().CreateTable(&WorkspaceRoot{})
	}},
	{version: 4, name: "create_projects", run: func(tx *gorm.DB) error {
		return tx.Migrator().CreateTable(&Project{})
	}},
	{version: 5, name: "create_agent_backend_configs", run: func(tx *gorm.DB) error {
		return tx.Migrator().CreateTable(&AgentBackendConfig{})
	}},
	{version: 6, name: "create_agent_instances", run: func(tx *gorm.DB) error {
		return tx.Migrator().CreateTable(&AgentInstance{})
	}},
	{version: 7, name: "create_agent_sessions", run: func(tx *gorm.DB) error {
		return tx.Migrator().CreateTable(&AgentSession{})
	}},
	{version: 8, name: "create_resources", run: func(tx *gorm.DB) error {
		return tx.Migrator().CreateTable(&Resource{})
	}},
	{version: 9, name: "create_project_resources", run: func(tx *gorm.DB) error {
		return tx.Migrator().CreateTable(&ProjectResource{})
	}},
	{version: 10, name: "create_session_prompts", run: func(tx *gorm.DB) error {
		return tx.Migrator().CreateTable(&SessionPrompt{})
	}},
	{version: 11, name: "create_managed_resource_files", run: func(tx *gorm.DB) error {
		return tx.Migrator().CreateTable(&ManagedResourceFile{})
	}},
	{version: 12, name: "create_resources_kind_name_index", run: func(tx *gorm.DB) error {
		// A resource's (kind, name) is unique: two resources of the same kind
		// render to the same managed target and would overwrite each other.
		if tx.Migrator().HasIndex(&Resource{}, "idx_resource_kind_name") {
			return nil
		}
		return tx.Migrator().CreateIndex(&Resource{}, "idx_resource_kind_name")
	}},
	{version: 13, name: "drop_workspace_roots", run: func(tx *gorm.DB) error {
		// Workspace-root authorization was removed: projects may live in any
		// directory the process can access, so the authorization list is dead
		// data. Dropping the table never touches the directories themselves.
		if tx.Migrator().HasTable(&WorkspaceRoot{}) {
			return tx.Migrator().DropTable(&WorkspaceRoot{})
		}
		return nil
	}},
	{version: 14, name: "create_model_services", run: func(tx *gorm.DB) error {
		return tx.Migrator().CreateTable(&ModelService{})
	}},
	{version: 15, name: "create_model_defs", run: func(tx *gorm.DB) error {
		return tx.Migrator().CreateTable(&ModelDef{})
	}},
	{version: 16, name: "create_model_defs_service_index", run: func(tx *gorm.DB) error {
		// (service_id, model_id) is unique: the same wire id cannot be listed
		// twice under one service.
		if tx.Migrator().HasIndex(&ModelDef{}, "idx_model_service_wire") {
			return nil
		}
		return tx.Migrator().CreateIndex(&ModelDef{}, "idx_model_service_wire")
	}},
	{version: 17, name: "drop_resource_library", run: func(tx *gorm.DB) error {
		// The resource library and its managed-file synchronization were
		// removed. The tables hold data no code reads any more; dropping them
		// never touches project directories or the secret store. Dependents
		// are dropped before the definitions they reference.
		for _, table := range []any{&ManagedResourceFile{}, &SessionPrompt{}, &ProjectResource{}, &Resource{}} {
			if tx.Migrator().HasTable(table) {
				if err := tx.Migrator().DropTable(table); err != nil {
					return err
				}
			}
		}
		return nil
	}},
	{version: 18, name: "drop_agent_backend_configs", run: func(tx *gorm.DB) error {
		// Per-project backend switches were removed: the platform-level agent
		// setting is the only enable/disable. The table holds data no code
		// reads any more, so it is dropped; project directories are untouched.
		if tx.Migrator().HasTable(&AgentBackendConfig{}) {
			return tx.Migrator().DropTable(&AgentBackendConfig{})
		}
		return nil
	}},
	{version: 19, name: "session_owned_by_project_backend_native", run: func(tx *gorm.DB) error {
		// Session ownership is moved from (instance_id, native_session_id) to
		// (project_id, backend, native_session_id). Before dropping the old
		// index and adding the new unique index, deduplicate any existing
		// records that share the same (project_id, backend, native_session_id):
		// keep the most recently updated row (or highest id on tie) and delete
		// the older duplicates.
		dedupSQL := `
			DELETE FROM agent_sessions
			WHERE id IN (
				SELECT a.id FROM agent_sessions a
				JOIN agent_sessions b ON a.project_id = b.project_id
					AND a.backend = b.backend
					AND a.native_session_id = b.native_session_id
					AND (
						a.updated_at < b.updated_at
						OR (a.updated_at = b.updated_at AND a.created_at < b.created_at)
						OR (a.updated_at = b.updated_at AND a.created_at = b.created_at AND a.id < b.id)
					)
			);
		`
		if err := tx.Exec(dedupSQL).Error; err != nil {
			return fmt.Errorf("deduplicate agent_sessions: %w", err)
		}

		// Drop the legacy index on (instance_id, native_session_id).
		_ = tx.Exec("DROP INDEX IF EXISTS idx_agent_session_native").Error

		// Create the new unique index on (project_id, backend, native_session_id).
		if err := tx.Exec(`
			CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_session_project_backend_native
			ON agent_sessions (project_id, backend, native_session_id);
		`).Error; err != nil {
			return fmt.Errorf("create idx_agent_session_project_backend_native: %w", err)
		}
		return nil
	}},
	{version: 20, name: "create_tasks", run: func(tx *gorm.DB) error {
		return tx.Migrator().CreateTable(&Task{})
	}},
	{version: 21, name: "task_runs_and_execution_prompt", run: func(tx *gorm.DB) error {
		if !tx.Migrator().HasColumn(&Task{}, "execution_prompt") {
			if err := tx.Migrator().AddColumn(&Task{}, "execution_prompt"); err != nil {
				return err
			}
		}
		if !tx.Migrator().HasTable(&TaskRun{}) {
			if err := tx.Migrator().CreateTable(&TaskRun{}); err != nil {
				return err
			}
		}
		return nil
	}},
	{version: 22, name: "add_model_to_agent_sessions", run: func(tx *gorm.DB) error {
		if !tx.Migrator().HasColumn(&AgentSession{}, "model") {
			return tx.Migrator().AddColumn(&AgentSession{}, "model")
		}
		return nil
	}},
	{version: 23, name: "create_prompts", run: func(tx *gorm.DB) error {
		return tx.Migrator().CreateTable(&Prompt{})
	}},
	{version: 24, name: "refactor_tasks_modular", run: func(tx *gorm.DB) error {
		if tx.Migrator().HasTable(&Task{}) {
			if !tx.Migrator().HasColumn(&Task{}, "task_type") {
				if err := tx.Migrator().AddColumn(&Task{}, "task_type"); err != nil {
					return err
				}
				_ = tx.Exec("UPDATE tasks SET task_type = 'standard' WHERE task_type IS NULL OR task_type = ''").Error
			}
			if !tx.Migrator().HasColumn(&Task{}, "prompt_template") {
				if err := tx.Migrator().AddColumn(&Task{}, "prompt_template"); err != nil {
					return err
				}
				if tx.Migrator().HasColumn(&Task{}, "execution_prompt") {
					_ = tx.Exec("UPDATE tasks SET prompt_template = execution_prompt WHERE prompt_template IS NULL OR prompt_template = ''").Error
				}
			}
		}
		return nil
	}},
}

// Migrate brings the schema to the latest version: it creates the
// schema_migrations bookkeeping table, then applies every pending migration
// step in order and records it. Schema changes are explicit, versioned
// steps; AutoMigrate is never used as the migration strategy.
func Migrate(db *gorm.DB) error {
	if db == nil {
		return errors.New("database: nil gorm.DB")
	}

	if !db.Migrator().HasTable(&schemaMigration{}) {
		if err := db.Migrator().CreateTable(&schemaMigration{}); err != nil {
			return fmt.Errorf("database: create schema_migrations table: %w", err)
		}
	}

	var applied []schemaMigration
	if err := db.Find(&applied).Error; err != nil {
		return fmt.Errorf("database: read schema_migrations: %w", err)
	}
	known := make(map[int64]bool, len(migrationSteps))
	for _, step := range migrationSteps {
		known[step.version] = true
	}
	appliedVersions := make(map[int64]bool, len(applied))
	for _, record := range applied {
		if !known[record.Version] {
			return fmt.Errorf("database: schema_migrations records unknown migration version %d (%s)",
				record.Version, record.Name)
		}
		appliedVersions[record.Version] = true
	}

	for _, step := range migrationSteps {
		if appliedVersions[step.version] {
			continue
		}
		if err := db.Transaction(func(tx *gorm.DB) error {
			if err := step.run(tx); err != nil {
				return fmt.Errorf("migration %d_%s: %w", step.version, step.name, err)
			}
			record := schemaMigration{Version: step.version, Name: step.name, AppliedAt: time.Now().UTC()}
			if err := tx.Create(&record).Error; err != nil {
				return fmt.Errorf("record migration %d_%s: %w", step.version, step.name, err)
			}
			return nil
		}); err != nil {
			return fmt.Errorf("database: apply migration: %w", err)
		}
	}
	return nil
}
