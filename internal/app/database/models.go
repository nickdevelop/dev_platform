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

// WorkspaceRoot is a legacy table dropped by migration 13. It is retained only
// because the historical migration 3 creates it; no current code reads or
// writes it. Table: workspace_roots.
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

// AgentBackendConfig is a legacy per-project backend switch. Migration 5
// creates its table and migration 18 drops it; no current code reads or writes
// it. The platform-level agent setting is the only enable/disable.
// Table: agent_backend_configs.
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

// Resource is a legacy resource-library row. Migrations 8 and 12 create its
// table and migration 17 drops it; no current code reads or writes it.
// Resource is one canonical, reusable Skill, MCP or prompt definition owned
// by the platform. Secret material is never stored here: an MCP definition
// keeps only an opaque SecretRef, while the value itself lives in the
// platform-private secret store. Backends is a comma-separated list of the
// backend ids the resource applies to (empty means unrestricted).
// Table: resources.
type Resource struct {
	ID          uint   `gorm:"primaryKey;column:id"`
	Kind        string `gorm:"column:kind;not null;index;uniqueIndex:idx_resource_kind_name"`
	Name        string `gorm:"column:name;not null;uniqueIndex:idx_resource_kind_name"`
	Description string `gorm:"column:description"`
	Content     string `gorm:"column:content"`
	Enabled     bool   `gorm:"column:enabled;not null;default:true"`
	PromptScope string `gorm:"column:prompt_scope"`
	SecretRef   string `gorm:"column:secret_ref"`
	Backends    string `gorm:"column:backends"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// ProjectResource is a legacy resource-to-project link. Migration 9 creates its
// table and migration 17 drops it; no current code reads or writes it.
// ProjectResource links one resource to one project. The pair is the primary
// key, so associating twice is idempotent rather than a duplicate row.
// Table: project_resources.
type ProjectResource struct {
	ProjectID  uint `gorm:"primaryKey;column:project_id;not null;index"`
	ResourceID uint `gorm:"primaryKey;column:resource_id;not null;index"`
	CreatedAt  time.Time
}

// SessionPrompt is a legacy session prompt selection. Migration 10 creates its
// table and migration 17 drops it; no current code reads or writes it.
// SessionPrompt links one prompt resource to one platform session, recording
// the prompts selected for that conversation. Table: session_prompts.
type SessionPrompt struct {
	SessionID  string `gorm:"primaryKey;column:session_id;not null;index"`
	ResourceID uint   `gorm:"primaryKey;column:resource_id;not null;index"`
	CreatedAt  time.Time
}

// ManagedResourceFile is a legacy managed-file ownership row. Migration 11
// creates its table and migration 17 drops it; no current code reads or writes it.
// ManagedResourceFile records platform ownership of one managed target file.
// The tuple (project_id, backend, path) identifies the target; ManagedHash is
// the digest of the last content the platform wrote, so an external edit is
// detectable, and BackupPath points at the preserved prior content. Ownership
// is written only after a successful, explicitly confirmed apply.
// Table: managed_resource_files.
type ManagedResourceFile struct {
	ID        uint   `gorm:"primaryKey;column:id"`
	ProjectID uint   `gorm:"column:project_id;not null;uniqueIndex:idx_managed_file_target"`
	Backend   string `gorm:"column:backend;not null;uniqueIndex:idx_managed_file_target"`
	Path      string `gorm:"column:path;not null;uniqueIndex:idx_managed_file_target"`
	// ManagedHash is the hex digest of the last platform-written content.
	ManagedHash string `gorm:"column:managed_hash;not null"`
	// BackupPath is the last preserved prior content, when one exists.
	BackupPath string `gorm:"column:backup_path"`
	// ResourceVersion identifies the resource definition version last applied.
	ResourceVersion string `gorm:"column:resource_version"`
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// ModelService is one OpenAI-compatible model service the operator configured.
// The API key is never stored here: SecretRef is an opaque reference into the
// platform-private secret store, empty when the service needs no auth.
// Table: model_services.
type ModelService struct {
	ID        uint   `gorm:"primaryKey;column:id"`
	Name      string `gorm:"column:name;not null"`
	BaseURL   string `gorm:"column:base_url;not null"`
	SecretRef string `gorm:"column:secret_ref"`
	Enabled   bool   `gorm:"column:enabled;not null;default:true"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

// ModelDef is one model offered by a model service. ModelID is the
// service-native identifier (the value sent as "model"); Name is the display
// name. The pair (service_id, model_id) is unique so a service cannot list the
// same wire id twice. Table: model_defs.
type ModelDef struct {
	ID            uint   `gorm:"primaryKey;column:id"`
	ServiceID     uint   `gorm:"column:service_id;not null;index;uniqueIndex:idx_model_service_wire"`
	ModelID       string `gorm:"column:model_id;not null;uniqueIndex:idx_model_service_wire"`
	Name          string `gorm:"column:name;not null"`
	SupportsText  bool   `gorm:"column:supports_text;not null;default:true"`
	SupportsImage bool   `gorm:"column:supports_image;not null;default:false"`
	SupportsTools bool   `gorm:"column:supports_tools;not null;default:false"`
	ContextTokens int    `gorm:"column:context_tokens;not null;default:0"`
	OutputTokens  int    `gorm:"column:output_tokens;not null;default:0"`
	Enabled       bool   `gorm:"column:enabled;not null;default:true"`
	CreatedAt     time.Time
	UpdatedAt     time.Time
}
