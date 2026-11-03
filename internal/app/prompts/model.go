package prompts

import (
	"errors"
	"time"
)

var (
	ErrPromptNotFound       = errors.New("prompt not found")
	ErrInvalidPromptName    = errors.New("prompt name is required")
	ErrInvalidPromptContent = errors.New("prompt content is required")
	ErrDuplicatePromptName  = errors.New("prompt name already exists")
)

// PromptType indicates the category of prompt template.
type PromptType string

const (
	PromptTypeRole PromptType = "role"
	PromptTypeFlow PromptType = "flow"
)

// NormalizePromptType standardizes the prompt type, defaulting to PromptTypeRole.
func NormalizePromptType(t PromptType) PromptType {
	if t == PromptTypeFlow {
		return PromptTypeFlow
	}
	return PromptTypeRole
}

// Prompt represents a reusable prompt template.
type Prompt struct {
	ID          uint       `json:"id"`
	Name        string     `json:"name"`
	Type        PromptType `json:"type"`
	Description string     `json:"description"`
	Content     string     `json:"content"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
}

// CreatePromptRequest is the payload for creating a prompt.
type CreatePromptRequest struct {
	Name        string     `json:"name"`
	Type        PromptType `json:"type"`
	Description string     `json:"description"`
	Content     string     `json:"content"`
}

// UpdatePromptRequest is the payload for updating a prompt.
type UpdatePromptRequest struct {
	Name        *string     `json:"name"`
	Type        *PromptType `json:"type"`
	Description *string     `json:"description"`
	Content     *string     `json:"content"`
}
