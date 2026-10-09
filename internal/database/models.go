package database

import "time"

// Setting keys owned by the platform.
const (
	// SettingKeyPasswordHash holds the Argon2id-encoded platform access
	// password hash. It is never a plaintext password.
	SettingKeyPasswordHash = "auth.password_hash"
)

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

// AgentBackendConfig records whether one agent backend is enabled for one
// project. Backend holds a platform backend id (for example "opencode-v2" or
// "pi"); the pair (project_id, backend) is unique so a project can enable each
// backend at most once. Table: agent_backend_configs.
type AgentBackendConfig struct {
	ID        uint   `gorm:"primaryKey;column:id"`
	ProjectID uint   `gorm:"column:project_id;not null;uniqueIndex:idx_agent_backend_project"`
	Backend   string `gorm:"column:backend;not null;uniqueIndex:idx_agent_backend_project"`
	Enabled   bool   `gorm:"column:enabled;not null;default:true"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

// AgentInstance is one platform-managed agent occurrence scoped to a project.
// The ID is the durable platform instance identifier (assigned by the adapter
// that started the instance); Directory is the canonical, already-authorized
// project working directory. Status and Error are copied from the adapter's
// lifecycle so a stopped or failed instance stays diagnosable after the
// backend is gone. Table: agent_instances.
type AgentInstance struct {
	ID        string `gorm:"primaryKey;column:id;not null"`
	ProjectID uint   `gorm:"column:project_id;not null;index"`
	Backend   string `gorm:"column:backend;not null"`
	Directory string `gorm:"column:directory;not null"`
	Status    string `gorm:"column:status;not null"`
	Endpoint  string `gorm:"column:endpoint"`
	Error     string `gorm:"column:error"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

// AgentSession is the platform's index entry for one agent-native session. It
// stores only the mapping the platform owns — its own id, the owning project,
// backend and instance, and the agent-native session id — plus a display title
// and a platform status. The conversation transcript itself stays in the agent;
// the platform never persists agent-native history. Table: agent_sessions.
type AgentSession struct {
	ID        string `gorm:"primaryKey;column:id;not null"`
	ProjectID uint   `gorm:"column:project_id;not null;index"`
	Backend   string `gorm:"column:backend;not null"`
	// InstanceID is the platform instance that owns this native session.
	InstanceID string `gorm:"column:instance_id;not null;index;uniqueIndex:idx_agent_session_native"`
	// NativeSessionID is the agent's own session identifier. The pair
	// (instance_id, native_session_id) is unique so a native session can be
	// discovered at most once and keeps one stable platform id.
	NativeSessionID string `gorm:"column:native_session_id;not null;uniqueIndex:idx_agent_session_native"`
	Title           string `gorm:"column:title"`
	Status          string `gorm:"column:status;not null"`
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// TableName pins the session index table name (GORM would otherwise pluralize
// AgentSession to agent_sessions already; pinning keeps it explicit).
func (AgentSession) TableName() string { return "agent_sessions" }
