package builtin

import (
	"context"
	"encoding/json"
	"time"
)

// TranscriptKind names one transcript record kind. The kinds mirror the
// normalized events the adapter emits, so a stored transcript can be replayed
// as history and rebuilt into an LLM context.
type TranscriptKind string

const (
	// TranscriptUser is one user turn message.
	TranscriptUser TranscriptKind = "user"
	// TranscriptAssistant is one final assistant message.
	TranscriptAssistant TranscriptKind = "assistant"
	// TranscriptThinking is one reasoning block (its final text).
	TranscriptThinking TranscriptKind = "thinking"
	// TranscriptToolCall is one tool invocation record.
	TranscriptToolCall TranscriptKind = "tool_call"
	// TranscriptToolResult is one tool outcome record.
	TranscriptToolResult TranscriptKind = "tool_result"
	// TranscriptError is one turn failure record.
	TranscriptError TranscriptKind = "error"
)

// TranscriptSession describes the owning record of one stored conversation.
type TranscriptSession struct {
	SessionID string
	Directory string
	Title     string
	CreatedAt time.Time
}

// TranscriptRecord is one stored conversation entry. Payload carries kind
// specific JSON the agent loop needs to rebuild an LLM context (tool call
// arguments and tool outputs are kept unpruned); Content carries the compact
// display text.
type TranscriptRecord struct {
	Seq       int
	Kind      TranscriptKind
	Content   string
	Payload   json.RawMessage
	CreatedAt time.Time
}

// TranscriptStore is the persistence contract the embedded agent needs. The
// implementation lives in the app layer, bound at the composition root; the
// adapter itself never touches the database. All methods honor ctx.
type TranscriptStore interface {
	// CreateSession records the owning session row. An existing row wins.
	CreateSession(ctx context.Context, session TranscriptSession) error
	// SetModel stamps the session's last-used model.
	SetModel(ctx context.Context, sessionID, model string) error
	// Append writes a batch of records atomically.
	Append(ctx context.Context, sessionID string, records []TranscriptRecord) error
	// Load returns all records of one session, oldest first.
	Load(ctx context.Context, sessionID string) ([]TranscriptRecord, error)
	// Exists reports whether the session has a stored transcript.
	Exists(ctx context.Context, sessionID string) (bool, error)
}

// toolCallRecord is the stored payload of one tool invocation: the full
// unpruned arguments, exactly what the LLM context needs to stay consistent.
type toolCallRecord struct {
	CallID        string          `json:"callId"`
	Name          string          `json:"name"`
	Input         json.RawMessage `json:"input"`
	NeedsApproval bool            `json:"needsApproval,omitempty"`
}

// toolResultRecord is the stored payload of one tool outcome: the output the
// turn fed back to the model (bounded to toolOutputContextLimit).
type toolResultRecord struct {
	CallID  string `json:"callId"`
	Output  string `json:"output"`
	IsError bool   `json:"isError,omitempty"`
}

// maxStoredImageBytes bounds one image's stored base64 body. Borderline
// attachments beyond this are kept by identity; a rebuilt history replays
// their text only.
const maxStoredImageBytes = 2 * 1024 * 1024

// userRecord is the stored payload of one user message that carries images.
type userRecord struct {
	Images []agentsImage `json:"images,omitempty"`
}

// agentsImage mirrors agents.ImageAttachment for the stored payload. Data is
// the base64 body so a rebuilt history renders the same image; oversized
// bodies are dropped and only the identity rides along.
type agentsImage struct {
	ID        string `json:"id"`
	Name      string `json:"name,omitempty"`
	MediaType string `json:"mediaType"`
	Data      string `json:"data,omitempty"`
}
