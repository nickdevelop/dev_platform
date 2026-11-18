package groups

import (
	"errors"
	"time"

	"devplatform/internal/core/agents"
)

var (
	ErrGroupNotFound             = errors.New("groups: group not found")
	ErrInvalidGroupName          = errors.New("groups: group name cannot be empty")
	ErrInvalidOrchestratorModel  = errors.New("groups: orchestrator model cannot be empty")
	ErrGroupTaskNotFound         = errors.New("groups: task not found")
	ErrInvalidTaskTitle          = errors.New("groups: task title cannot be empty")
	ErrInvalidProject            = errors.New("groups: project id is required")
	ErrInvalidPrompt             = errors.New("groups: prompt cannot be empty")
	ErrNoMembersConfigured       = errors.New("groups: at least one member role is required")
	ErrTaskAlreadyRunning        = errors.New("groups: task is currently running")
)

// Group defines a collaborative team of agents with an orchestrator brain.
type Group struct {
	ID                 uint      `json:"id"`
	Name               string    `json:"name"`
	Description        string    `json:"description"`
	OrchestratorModel  string    `json:"orchestratorModel"`
	OrchestratorPrompt string    `json:"orchestratorPrompt"`
	Icon               string    `json:"icon,omitempty"`
	Color              string    `json:"color,omitempty"`
	MemberCount        int       `json:"memberCount,omitempty"`
	TaskCount          int       `json:"taskCount,omitempty"`
	CreatedAt          time.Time `json:"createdAt"`
	UpdatedAt          time.Time `json:"updatedAt"`
}

// GroupMemberDetail represents a member in a group, populated with role library details.
type GroupMemberDetail struct {
	ID           uint             `json:"id"`
	GroupID      uint             `json:"groupId"`
	RoleID       uint             `json:"roleId"`
	SortOrder    int              `json:"sortOrder"`
	Name         string           `json:"name"`
	Description  string           `json:"description"`
	Backend      agents.BackendID `json:"backend"`
	Model        string           `json:"model"`
	PromptID     uint             `json:"promptId"`
	SystemPrompt string           `json:"systemPrompt"`
	Icon         string           `json:"icon,omitempty"`
	Color        string           `json:"color,omitempty"`
	CreatedAt    time.Time        `json:"createdAt"`
}

// GroupDetail is the full aggregate view of a group including its member roles.
type GroupDetail struct {
	Group
	Members []GroupMemberDetail `json:"members"`
}

// CreateGroupRequest payload for creating a team group.
type CreateGroupRequest struct {
	Name               string   `json:"name"`
	Description        string   `json:"description"`
	OrchestratorModel  string   `json:"orchestratorModel"`
	OrchestratorPrompt string   `json:"orchestratorPrompt"`
	Icon               string   `json:"icon,omitempty"`
	Color              string   `json:"color,omitempty"`
	RoleIDs            []uint   `json:"roleIds,omitempty"`
}

// UpdateGroupRequest payload for updating a team group.
type UpdateGroupRequest struct {
	Name               *string `json:"name"`
	Description        *string `json:"description"`
	OrchestratorModel  *string `json:"orchestratorModel"`
	OrchestratorPrompt *string `json:"orchestratorPrompt"`
	Icon               *string `json:"icon,omitempty"`
	Color              *string `json:"color,omitempty"`
}

// GroupTask represents a task assigned to a team group.
type GroupTask struct {
	ID              uint       `json:"id"`
	GroupID         uint       `json:"groupId"`
	Title           string     `json:"title"`
	Description     string     `json:"description"`
	ProjectID       uint       `json:"projectId"`
	WorktreeID      uint       `json:"worktreeId"`
	Prompt          string     `json:"prompt"`
	ScheduleType    string     `json:"scheduleType"` // "manual" | "once" | "cron"
	ScheduledAt     *time.Time `json:"scheduledAt,omitempty"`
	CronExpr        string     `json:"cronExpr,omitempty"`
	ScheduleEnabled bool       `json:"scheduleEnabled"`
	LastRunAt       *time.Time `json:"lastRunAt,omitempty"`
	NextRunAt       *time.Time `json:"nextRunAt,omitempty"`
	LastRunStatus   string     `json:"lastRunStatus,omitempty"`
	Status          string     `json:"status"` // "active" | "disabled"
	CreatedAt       time.Time  `json:"createdAt"`
	UpdatedAt       time.Time  `json:"updatedAt"`
}

// CreateGroupTaskRequest carries arguments to create a group task.
type CreateGroupTaskRequest struct {
	Title           string     `json:"title"`
	Description     string     `json:"description"`
	ProjectID       uint       `json:"projectId"`
	WorktreeID      uint       `json:"worktreeId"`
	Prompt          string     `json:"prompt"`
	ScheduleType    string     `json:"scheduleType"`
	ScheduledAt     *time.Time `json:"scheduledAt,omitempty"`
	CronExpr        string     `json:"cronExpr,omitempty"`
	ScheduleEnabled bool       `json:"scheduleEnabled"`
}

// UpdateGroupTaskRequest carries arguments to update a group task.
type UpdateGroupTaskRequest struct {
	Title           *string    `json:"title"`
	Description     *string    `json:"description"`
	ProjectID       *uint      `json:"projectId"`
	WorktreeID      *uint      `json:"worktreeId"`
	Prompt          *string    `json:"prompt"`
	ScheduleType    *string    `json:"scheduleType"`
	ScheduledAt     *time.Time `json:"scheduledAt"`
	CronExpr        *string    `json:"cronExpr"`
	ScheduleEnabled *bool      `json:"scheduleEnabled"`
	Status          *string    `json:"status"`
}
