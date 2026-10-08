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
