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
// backend and the agent-native session id — plus the instance currently or
// last hosting it, a display title and a platform status. The conversation
// transcript itself stays in the agent; the platform never persists
// agent-native history.
//
// Uniqueness is scoped to (project_id, backend, native_session_id): a native
// session belongs to its project and backend directly, while the instance is
// only its runtime host. Table: agent_sessions.
type AgentSession struct {
	ID        string `gorm:"primaryKey;column:id;not null"`
	ProjectID uint   `gorm:"column:project_id;not null;index;uniqueIndex:idx_agent_session_project_backend_native"`
	Backend   string `gorm:"column:backend;not null;uniqueIndex:idx_agent_session_project_backend_native"`
	// InstanceID is the platform instance currently or last hosting this native
	// session. It is only the runtime host, not part of the session identity.
	InstanceID      string `gorm:"column:instance_id;not null;index"`
	NativeSessionID string `gorm:"column:native_session_id;not null;uniqueIndex:idx_agent_session_project_backend_native"`
	Title           string `gorm:"column:title"`
	Model           string `gorm:"column:model"`
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

// ChatGroup is one platform-owned pure-chat conversation group.
// It organizes conversations into logical categories, similar to projects for sessions.
// Table: chat_groups.
type ChatGroup struct {
	ID        uint   `gorm:"primaryKey;column:id"`
	Name      string `gorm:"column:name;not null"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

// TableName pins the chat group table name.
func (ChatGroup) TableName() string { return "chat_groups" }

// Chat is one platform-owned pure-chat conversation. It is deliberately
// unrelated to projects, agent instances and agent sessions: a chat belongs
// only to the platform and the model it was created with. ModelName is a
// snapshot so history stays readable after a model definition is removed.
// Table: chats.
type Chat struct {
	ID        uint   `gorm:"primaryKey;column:id"`
	GroupID   uint   `gorm:"column:group_id;not null;default:0;index:idx_chats_group"`
	Title     string `gorm:"column:title;not null"`
	ModelID   uint   `gorm:"column:model_id;not null"`
	ModelName string `gorm:"column:model_name;not null"`
	CreatedAt time.Time
	UpdatedAt time.Time `gorm:"index:idx_chats_updated"`
}

// TableName pins the chat table name.
func (Chat) TableName() string { return "chats" }

// ChatMessage is one persisted message of a pure-chat conversation. Status is
// the generation lifecycle of an assistant message: pending while the model
// streams, then complete, failed or cancelled. Images and token counts are
// stored as stable JSON/scalar columns so the API never depends on GORM
// internals. Table: chat_messages.
type ChatMessage struct {
	ID           uint   `gorm:"primaryKey;column:id"`
	ChatID       uint   `gorm:"column:chat_id;not null;index:idx_chat_messages_chat"`
	Role         string `gorm:"column:role;not null"`
	Content      string `gorm:"column:content"`
	Reasoning    string `gorm:"column:reasoning"`
	Images       string `gorm:"column:images"`
	Status       string `gorm:"column:status;not null"`
	ModelID      uint   `gorm:"column:model_id"`
	ModelName    string `gorm:"column:model_name"`
	ErrorCode    string `gorm:"column:error_code"`
	ErrorMessage string `gorm:"column:error_message"`
	InputTokens  int    `gorm:"column:input_tokens;not null;default:0"`
	OutputTokens int    `gorm:"column:output_tokens;not null;default:0"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// TableName pins the chat message table name.
func (ChatMessage) TableName() string { return "chat_messages" }

// Task is one managed task with template prompt, agent/model binding and scheduling.
// Table: tasks.
type Task struct {
	ID              uint       `gorm:"primaryKey;column:id"`
	Title           string     `gorm:"column:title;not null"`
	Description     string     `gorm:"column:description"`
	TaskType        string     `gorm:"column:task_type;not null;default:'standard'"` // "standard" | "flow"
	ProjectID       uint       `gorm:"column:project_id;not null;index"`
	Backend         string     `gorm:"column:backend;not null"`
	Model           string     `gorm:"column:model;not null"`
	PromptTemplate  string     `gorm:"column:prompt_template;type:text"`
	ScheduleType    string     `gorm:"column:schedule_type;not null;default:'manual'"` // "manual" | "once" | "cron"
	ScheduledAt     *time.Time `gorm:"column:scheduled_at"`
	CronExpr        string     `gorm:"column:cron_expr"`
	ScheduleEnabled bool       `gorm:"column:schedule_enabled;not null;default:false"`
	LastRunAt       *time.Time `gorm:"column:last_run_at"`
	NextRunAt       *time.Time `gorm:"column:next_run_at"`
	LastRunStatus   string     `gorm:"column:last_run_status"` // "idle" | "running" | "success" | "failed"
	Status          string     `gorm:"column:status;not null;default:'active'"` // "active" | "disabled"

	// Deprecated fields retained for historical schema migration compatibility
	FlowType        string     `gorm:"column:flow_type"`
	CurrentStep     int        `gorm:"column:current_step"`
	StepStatus      string     `gorm:"column:step_status"`
	IsConfirmed     bool       `gorm:"column:is_confirmed"`
	StepsData       string     `gorm:"column:steps_data;type:text"`
	ActiveSessionID string     `gorm:"column:active_session_id;index"`
	ExecutionPrompt string     `gorm:"column:execution_prompt;type:text"`

	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// TableName pins the table name for Task.
func (Task) TableName() string { return "tasks" }

// TaskRun records one concrete execution run of a task, especially for transactional tasks
// which spawn a new AgentSession on every run.
// Table: task_runs.
type TaskRun struct {
	ID          uint       `gorm:"primaryKey;column:id"`
	TaskID      uint       `gorm:"column:task_id;not null;index"`
	SessionID   string     `gorm:"column:session_id;not null;index"`
	TriggerType string     `gorm:"column:trigger_type;not null"` // "manual" | "once" | "cron"
	Status      string     `gorm:"column:status;not null"`       // "running" | "success" | "failed"
	Prompt      string     `gorm:"column:prompt;type:text"`
	Error       string     `gorm:"column:error"`
	TriggeredAt time.Time  `gorm:"column:triggered_at;not null"`
	FinishedAt  *time.Time `gorm:"column:finished_at"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// TableName pins the table name for TaskRun.
func (TaskRun) TableName() string { return "task_runs" }

// Prompt is a reusable prompt template with optional {user_prompt} placeholder.
// Table: prompts.
type Prompt struct {
	ID          uint      `gorm:"primaryKey;column:id"`
	Name        string    `gorm:"column:name;not null;uniqueIndex"`
	Description string    `gorm:"column:description"`
	Content     string    `gorm:"column:content;type:text;not null"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// TableName pins the table name for Prompt.
func (Prompt) TableName() string { return "prompts" }

// TerminalServer records a server configuration for terminal connections.
// Table: terminal_servers.
type TerminalServer struct {
	ID        uint      `gorm:"primaryKey;column:id"`
	Name      string    `gorm:"column:name;not null"`
	Host      string    `gorm:"column:host;not null"`
	Port      int       `gorm:"column:port;not null;default:22"`
	Username  string    `gorm:"column:username;not null"`
	AuthType  string    `gorm:"column:auth_type;not null;default:'password'"` // "password" | "key"
	SecretRef string    `gorm:"column:secret_ref"`                           // reference in secrets.Store
	IsLocal   bool      `gorm:"column:is_local;not null;default:false"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

// TableName pins the table name for TerminalServer.
func (TerminalServer) TableName() string { return "terminal_servers" }
