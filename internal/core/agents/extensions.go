package agents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// ErrReplyExpired reports a permission or form reply whose target is already
// gone: the request id is unknown, or it was already answered or cancelled. A
// backend wraps its own "not pending" answer with it, so the session layer can
// turn it into an expired reply (HTTP 409) without knowing the backend.
var ErrReplyExpired = errors.New("agents: reply target is no longer pending")

// ErrHistoryImageNotFound reports an image id that does not exist in the
// session transcript (or whose body was not retained by the adapter).
var ErrHistoryImageNotFound = errors.New("agents: history image not found")

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

// CommandCategory describes the nature of a platform-designed command.
type CommandCategory string

const (
	// CommandCategoryAction indicates a functional command (e.g. compact) that triggers
	// a dedicated session capability rather than sending a prompt to the LLM.
	CommandCategoryAction CommandCategory = "action"
	// CommandCategoryPrompt indicates a prompt injection command that injects a
	// prompt template into the conversation turn loop.
	CommandCategoryPrompt CommandCategory = "prompt"
)

// PlatformCommand is a slash command designed and managed by the platform.
// Commands are not synchronized from downstream agent backends; the platform owns
// their definition, categorization, and dispatch logic.
type PlatformCommand struct {
	Name         string          `json:"name"`
	Description  string          `json:"description"`
	Category     CommandCategory `json:"category"`
	ArgumentHint string          `json:"argumentHint,omitempty"`
}

// AgentCommand is an alias for PlatformCommand for compatibility across the codebase.
type AgentCommand = PlatformCommand

// CommandInvocation specifies a slash command invocation dispatched to a backend.
type CommandInvocation struct {
	Name      string `json:"name"`
	RawInput  string `json:"rawInput"`
	Arguments string `json:"arguments"`
}

// CommandExecutionResult holds the immediate outcome of a non-streaming command execution.
type CommandExecutionResult struct {
	Handled bool   `json:"handled"`
	Output  string `json:"output,omitempty"`
	Error   string `json:"error,omitempty"`
}

// SessionCompactor is the extension for agent backends that support
// manual context compaction.
type SessionCompactor interface {
	CompactSession(ctx context.Context, ref SessionRef, instructions string) error
}

// SessionTurnDetector is the optional extension for agent backends that can
// actively check whether an agent-native session is running a turn.
type SessionTurnDetector interface {
	DetectTurn(ctx context.Context, ref SessionRef) (bool, error)
}

// HistoryImageRequest names one image inside an agent-native transcript.
type HistoryImageRequest struct {
	// Directory is the working directory the agent operated in.
	Directory string
	// SessionID is the agent-native session identifier.
	SessionID string
	// ImageID is the opaque id carried by ImageAttachment.ID in history.
	ImageID string
}

// Validate reports whether the request can address a transcript image.
func (r HistoryImageRequest) Validate() error {
	if r.Directory == "" || r.SessionID == "" || r.ImageID == "" {
		return fmt.Errorf("%w: incomplete history image request", ErrInvalidHistoryRequest)
	}
	return nil
}

// HistoryImageReader is the optional half of the history contract: an adapter
// implements it when it can serve one image from a transcript on demand, so a
// history response can omit the base64 body and expose a URL instead.
//
// It is separate from HistoryReader so backends whose transcripts carry no
// images (pi, deepseek) do not have to implement it.
type HistoryImageReader interface {
	ReadHistoryImage(ctx context.Context, request HistoryImageRequest) (ImageAttachment, error)
}
