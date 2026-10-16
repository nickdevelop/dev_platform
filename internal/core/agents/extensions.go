package agents

import (
	"context"
	"encoding/json"
	"errors"
	"io"
)

// ErrReplyExpired reports a permission or form reply whose target is already
// gone: the request id is unknown, or it was already answered or cancelled. A
// backend wraps its own "not pending" answer with it, so the session layer can
// turn it into an expired reply (HTTP 409) without knowing the backend.
var ErrReplyExpired = errors.New("agents: reply target is no longer pending")

// PermissionRequest is a pending tool approval the backend is waiting on. It
// carries identity and human-readable context only.
type PermissionRequest struct {
	ID        string   `json:"id"`
	SessionID string   `json:"sessionID"`
	Action    string   `json:"action"`
	Resources []string `json:"resources"`
	Message   string   `json:"message,omitempty"`
}

// FormInfo is a pending question form the backend is waiting on. Its fields
// stay opaque to the platform, which only relays them to the workbench.
type FormInfo struct {
	ID        string          `json:"id"`
	SessionID string          `json:"sessionID"`
	Title     string          `json:"title"`
	Fields    json.RawMessage `json:"fields,omitempty"`
}

// PermissionReplier is the optional extension for backends that can pause a
// tool call for approval and accept form answers. It is separate from
// AgentAdapter so a backend without the feature does not implement it.
// Callers assert it and report ErrUnsupportedCapability when it is absent.
type PermissionReplier interface {
	ListPermissions(ctx context.Context, ref SessionRef) ([]PermissionRequest, error)
	ReplyPermission(ctx context.Context, ref SessionRef, requestID, decision, message string) error
	ListForms(ctx context.Context, ref SessionRef) ([]FormInfo, error)
	ReplyForm(ctx context.Context, ref SessionRef, formID string, answer map[string]any) error
}

// RawEventOpener is the optional extension that exposes a backend's untouched
// event byte stream. The platform proxies bytes and the frontend projects
// events; the platform itself never interprets them.
type RawEventOpener interface {
	OpenRawEvents(ctx context.Context) (io.ReadCloser, error)
}

// CommandKind describes the nature of an agent-provided command.
type CommandKind string

const (
	CommandKindSlash  CommandKind = "command"
	CommandKindSkill  CommandKind = "skill"
	CommandKindPrompt CommandKind = "prompt"
)

// AgentCommand is a slash command, skill, or prompt template exposed by an agent backend.
type AgentCommand struct {
	Name         string      `json:"name"`
	Description  string      `json:"description"`
	Kind         CommandKind `json:"kind"`
	ArgumentHint string      `json:"argumentHint,omitempty"`
}

// CommandDiscoverer is the optional extension for agent backends that expose
// slash commands, skills, or prompt templates to the platform.
type CommandDiscoverer interface {
	ListCommands(ctx context.Context, ref SessionRef) ([]AgentCommand, error)
}
