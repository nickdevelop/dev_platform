// Package resources defines the platform-neutral contracts for reusable
// resource definitions — Skills, MCP servers and prompts — that the platform
// stores canonically and later synchronizes into project or agent
// configuration files.
//
// The package is deliberately implementation-free: it carries the vocabulary
// (kinds, prompt scopes, the definition record and its validation) shared by
// the persistence layer, the HTTP handlers and the resource-sync adapters.
// Nothing here talks to a database or the filesystem.
//
// Secret handling: an MCP definition persists only an opaque SecretRef. The
// raw credential value never appears on this type, so serializing a
// ResourceDefinition cannot leak it; the value lives in the platform-private
// secret store.
package resources

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"devplatform/internal/core/agents"
)

// ErrInvalidResource reports a resource definition that violates the
// contract (unknown kind, unsafe name, misplaced prompt scope or secret
// reference, unknown backend, or missing timestamps).
var ErrInvalidResource = errors.New("resources: invalid resource")

// ResourceKind identifies the class of a reusable resource. The constants are
// the stable wire values shared by the API, the UI and the sync adapters.
type ResourceKind string

const (
	// ResourceSkill is a reusable skill definition.
	ResourceSkill ResourceKind = "skill"
	// ResourceMCP is an MCP server definition; its credential reference is
	// opaque.
	ResourceMCP ResourceKind = "mcp"
	// ResourcePrompt is an instruction or prompt definition.
	ResourcePrompt ResourceKind = "prompt"
)

// Valid reports whether the kind is one the platform knows.
func (k ResourceKind) Valid() bool {
	switch k {
	case ResourceSkill, ResourceMCP, ResourcePrompt:
		return true
	default:
		return false
	}
}

// PromptScope distinguishes a project-level instruction from a prompt that is
// selected per session. It applies only to ResourcePrompt definitions.
type PromptScope string

const (
	// PromptScopeProject marks a project-level instruction.
	PromptScopeProject PromptScope = "project"
	// PromptScopeSession marks a prompt selected within a session.
	PromptScopeSession PromptScope = "session"
)

// Valid reports whether the scope is one the platform knows.
func (s PromptScope) Valid() bool {
	switch s {
	case PromptScopeProject, PromptScopeSession:
		return true
	default:
		return false
	}
}

// ResourceDefinition is one canonical, reusable resource. It is the
// persisted record plus the fields a client may set. Credentials are
// referenced through the opaque SecretRef only.
type ResourceDefinition struct {
	// ID is the platform-wide unique resource identifier.
	ID uint `json:"id"`
	// Kind names the resource class.
	Kind ResourceKind `json:"kind"`
	// Name is the human-facing name, also used to derive a target filename.
	// It is a single path segment: no separators, no traversal.
	Name string `json:"name"`
	// Description is an optional human-facing note.
	Description string `json:"description,omitempty"`
	// Content is the definition body: instruction text for prompts, or the
	// backend-neutral definition for a skill or MCP server.
	Content string `json:"content"`
	// Enabled reports whether the resource is active. Disabled resources are
	// not synchronized.
	Enabled bool `json:"enabled"`
	// PromptScope applies to ResourcePrompt definitions and must be unset for
	// other kinds.
	PromptScope PromptScope `json:"promptScope,omitempty"`
	// SecretRef is the opaque reference to a credential held in the
	// platform-private secret store. It is valid only for ResourceMCP and is
	// never the credential itself.
	SecretRef string `json:"secretRef,omitempty"`
	// Backends lists the agent backends this resource applies to. An empty
	// list means the resource is not restricted to a specific backend.
	Backends []agents.BackendID `json:"backends,omitempty"`
	// CreatedAt and UpdatedAt are the record's lifecycle timestamps.
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Validate reports whether the definition satisfies the contract.
func (d ResourceDefinition) Validate() error {
	if d.ID == 0 {
		return fmt.Errorf("%w: id is zero", ErrInvalidResource)
	}
	if err := validateFields(d.Kind, d.Name, d.PromptScope, d.SecretRef, d.Backends); err != nil {
		return err
	}
	if d.CreatedAt.IsZero() {
		return fmt.Errorf("%w: created timestamp is zero", ErrInvalidResource)
	}
	if d.UpdatedAt.IsZero() {
		return fmt.Errorf("%w: updated timestamp is zero", ErrInvalidResource)
	}
	return nil
}

// validateFields checks the client-settable fields shared by a persisted
// definition and a create/update input. Identity and timestamps are not part
// of it: a caller validates input before the platform assigns them.
func validateFields(kind ResourceKind, name string, scope PromptScope, secretRef string, backends []agents.BackendID) error {
	if !kind.Valid() {
		return fmt.Errorf("%w: unknown kind %q", ErrInvalidResource, kind)
	}
	if !validName(name) {
		return fmt.Errorf("%w: unsafe name %q", ErrInvalidResource, name)
	}
	switch kind {
	case ResourcePrompt:
		if !scope.Valid() {
			return fmt.Errorf("%w: prompt scope %q is not valid", ErrInvalidResource, scope)
		}
	default:
		if scope != "" {
			return fmt.Errorf("%w: prompt scope is only valid for prompts", ErrInvalidResource)
		}
	}
	if secretRef != "" && kind != ResourceMCP {
		return fmt.Errorf("%w: secret reference is only valid for mcp", ErrInvalidResource)
	}
	for _, backend := range backends {
		if !backend.Valid() {
			return fmt.Errorf("%w: unknown backend %q", ErrInvalidResource, backend)
		}
	}
	return nil
}

// validName reports whether name is a safe single path segment for a
// resource. Unicode is allowed; separators, traversal, control characters
// and blank names are not.
func validName(name string) bool {
	if name == "" || !utf8.ValidString(name) {
		return false
	}
	if strings.TrimSpace(name) == "" {
		return false
	}
	if name == "." || name == ".." {
		return false
	}
	if utf8.RuneCountInString(name) > 128 {
		return false
	}
	for _, r := range name {
		switch {
		case r == '/' || r == '\\':
			return false
		case r == 0:
			return false
		case unicode.IsControl(r):
			return false
		}
	}
	return true
}
