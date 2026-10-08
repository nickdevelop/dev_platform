package database

import "time"

// Setting stores one platform configuration value addressed by key, so
// platform configuration survives restarts. Table: settings.
type Setting struct {
	Key       string `gorm:"primaryKey;column:key;not null"`
	Value     string `gorm:"column:value"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

// AuthSession is one password login session. Issuing, expiring and
// validating sessions is owned by the auth layer built on this model.
// Table: auth_sessions.
type AuthSession struct {
	ID        string    `gorm:"primaryKey;column:id;not null"`
	ExpiresAt time.Time `gorm:"column:expires_at"`
	CreatedAt time.Time
}

// WorkspaceRoot is an operator-authorized workspace root directory under
// which projects may be registered or created. Table: workspace_roots.
type WorkspaceRoot struct {
	ID        uint   `gorm:"primaryKey;column:id"`
	Path      string `gorm:"column:path;not null;uniqueIndex"`
	CreatedAt time.Time
}

// Project is a managed project directory registered with the platform.
// The ID is the stable platform identifier: it survives renames and moves,
// while Path tracks the directory's current location and is unique.
// Table: projects.
type Project struct {
	ID        uint   `gorm:"primaryKey;column:id"`
	Name      string `gorm:"column:name;not null"`
	Path      string `gorm:"column:path;not null;uniqueIndex"`
	CreatedAt time.Time
	UpdatedAt time.Time
}
