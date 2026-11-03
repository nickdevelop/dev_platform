package roles

import (
	"errors"
	"time"

	"devplatform/internal/core/agents"
)

var (
	ErrRoleNotFound     = errors.New("roles: role not found")
	ErrInvalidRoleName  = errors.New("roles: role name cannot be empty")
	ErrInvalidRoleModel = errors.New("roles: role model cannot be empty")
)

// Role defines a reusable agent role with prompt reference, backend and model bindings.
type Role struct {
	ID           uint             `json:"id"`
	Name         string           `json:"name"`
	Description  string           `json:"description"`
	Backend      agents.BackendID `json:"backend"`
	Model        string           `json:"model"`
	PromptID     uint             `json:"promptId"`
	SystemPrompt string           `json:"systemPrompt"`
	Icon         string           `json:"icon,omitempty"`
	Color        string           `json:"color,omitempty"`
	CreatedAt    time.Time        `json:"createdAt"`
	UpdatedAt    time.Time        `json:"updatedAt"`
}

// CreateRoleRequest defines payload for creating a role.
type CreateRoleRequest struct {
	Name         string           `json:"name"`
	Description  string           `json:"description"`
	Backend      agents.BackendID `json:"backend"`
	Model        string           `json:"model"`
	PromptID     uint             `json:"promptId"`
	SystemPrompt string           `json:"systemPrompt"`
	Icon         string           `json:"icon,omitempty"`
	Color        string           `json:"color,omitempty"`
}

// UpdateRoleRequest defines payload for updating a role.
type UpdateRoleRequest struct {
	Name         *string           `json:"name"`
	Description  *string           `json:"description"`
	Backend      *agents.BackendID `json:"backend"`
	Model        *string           `json:"model"`
	PromptID     *uint             `json:"promptId"`
	SystemPrompt *string           `json:"systemPrompt"`
	Icon         *string           `json:"icon,omitempty"`
	Color        *string           `json:"color,omitempty"`
}
