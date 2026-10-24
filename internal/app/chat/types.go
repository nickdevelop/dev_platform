// Package chat owns the platform's non-agent conversation feature: global
// chats that call an OpenAI-compatible model directly. It deliberately has no
// relationship to projects, agent instances or agent sessions: a chat only
// knows a model configured in Platform Settings and the messages the platform
// persisted for it.
package chat

import (
	"errors"
	"time"
)

// Service errors, mapped to HTTP statuses by the handlers.
var (
	ErrChatNotFound       = errors.New("chat: chat not found")
	ErrMessageNotFound    = errors.New("chat: message not found")
	ErrMessageNotEditable = errors.New("chat: message is not editable")
	ErrChatBusy           = errors.New("chat: generation is already running")
	ErrInvalidMessage     = errors.New("chat: invalid message")
	ErrInvalidTitle       = errors.New("chat: invalid title")
	ErrInvalidImage       = errors.New("chat: invalid image")
	ErrUnsupportedImage   = errors.New("chat: model does not support images")
	ErrModelUnavailable   = errors.New("chat: model unavailable")
	ErrGroupNotFound      = errors.New("chat: group not found")
	ErrInvalidGroupName   = errors.New("chat: invalid group name")
	ErrGroupLocked        = errors.New("chat: group is reserved and cannot be changed")
)

// SystemKeyQuickAssistant identifies the platform-owned quick-assistant group.
// It is the only group the quick-assistant surface may create conversations in,
// and the chat service rejects any attempt to move conversations into or out of
// it or to rename/delete the group itself.
const (
	SystemKeyQuickAssistant = "quick-assistant"
	QuickAssistantGroupName = "快捷助手"
)

// Message roles and statuses.
const (
	RoleUser      = "user"
	RoleAssistant = "assistant"

	StatusPending   = "pending"
	StatusComplete  = "complete"
	StatusFailed    = "failed"
	StatusCancelled = "cancelled"
)

// Turn phases and outcomes.
const (
	TurnStarted = "started"
	TurnEnded   = "ended"

	OutcomeCompleted = "completed"
	OutcomeCancelled = "cancelled"
	OutcomeFailed    = "failed"
)

// Event types carried by the chat SSE stream.
const (
	EventSnapshot      = "snapshot"
	EventDelta         = "delta"
	EventMessageUpdate = "message_update"
	EventConversation  = "conversation"
	EventTurn          = "turn"
	EventClosed        = "closed"
)

// Limits shared by validation and the SSE snapshot.
const (
	// MaxImages is the number of images one user message may carry.
	MaxImages = 4
	// MaxImageBytes is the decoded size limit for one image.
	MaxImageBytes = 5 << 20
	// MaxSnapshotMessages bounds one snapshot response. Older messages stay in
	// the database and remain part of the model context.
	MaxSnapshotMessages = 2000
	// DefaultListLimit is the conversation list page size.
	DefaultListLimit = 200
	// MaxListLimit is the largest accepted conversation list page size.
	MaxListLimit = 500
	// MaxTitleRunes bounds a stored title.
	MaxTitleRunes = 120
	// MaxGroupNameRunes bounds a stored group name.
	MaxGroupNameRunes = 60
)

// ChatGroup is the JSON-stable API representation of one pure-chat group / assistant.
type ChatGroup struct {
	ID             uint      `json:"id"`
	Name           string    `json:"name"`
	DefaultModelID uint      `json:"defaultModelId"`
	SystemPrompt   string    `json:"systemPrompt"`
	Icon           string    `json:"icon"`
	ShowInFloat    bool      `json:"showInFloat"`
	SystemKey      string    `json:"systemKey,omitempty"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

// ImageAttachment is one user-supplied image in a stable wire shape. Data is
// base64 without the data-URL prefix; media type and size are validated by the
// service before the value is stored or sent upstream.
type ImageAttachment struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	MediaType string `json:"mediaType"`
	Data      string `json:"data"`
	Size      int    `json:"size"`
}

// Usage is exact token usage reported by the upstream service when it provides
// it. It is never estimated.
type Usage struct {
	Input  int `json:"input"`
	Output int `json:"output"`
}

// Chat is the JSON-stable API representation of one conversation. Running is
// derived from the in-process generation manager and is not persisted.
type Chat struct {
	ID           uint      `json:"id"`
	GroupID      uint      `json:"groupId"`
	Title        string    `json:"title"`
	ModelID      uint      `json:"modelId"`
	ModelName    string    `json:"modelName"`
	SystemPrompt string    `json:"-"`
	Running      bool      `json:"running"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

// Message is the JSON-stable API representation of one chat message.
type Message struct {
	ID        uint              `json:"id"`
	ChatID    uint              `json:"chatId"`
	Role      string            `json:"role"`
	Text      string            `json:"text"`
	Reasoning string            `json:"reasoning,omitempty"`
	Images    []ImageAttachment `json:"images,omitempty"`
	Status    string            `json:"status"`
	ModelID   uint              `json:"modelId,omitempty"`
	ModelName string            `json:"modelName,omitempty"`
	Usage     *Usage            `json:"usage,omitempty"`
	ErrorCode string            `json:"errorCode,omitempty"`
	Error     string            `json:"error,omitempty"`
	CreatedAt time.Time         `json:"createdAt"`
	UpdatedAt time.Time         `json:"updatedAt"`
}

// CreateInput is the body of POST /chats: the conversation is created together
// with its first user message.
type CreateInput struct {
	GroupID *uint
	ModelID uint
	Text    string
	Images  []ImageAttachment
}

// SendInput is the body of POST /chats/{id}/messages. ModelID may be zero to
// keep the conversation's current model.
type SendInput struct {
	ModelID uint
	Text    string
	Images  []ImageAttachment
}

// EditInput is the body of POST /chats/{id}/messages/{messageId}/edit.
type EditInput struct {
	Text   string
	Images []ImageAttachment
}

// UpdateInput is the body of PUT /chats/{id}; all fields are optional.
type UpdateInput struct {
	Title   *string
	ModelID *uint
	GroupID *uint
}

// CreateGroupInput is the body of POST /chat-groups.
type CreateGroupInput struct {
	Name           string
	DefaultModelID uint
	SystemPrompt   string
	Icon           string
	ShowInFloat    bool
}

// UpdateGroupInput is the body of PUT /chat-groups/{id}.
type UpdateGroupInput struct {
	Name           string
	DefaultModelID uint
	SystemPrompt   string
	Icon           string
	ShowInFloat    bool
}

// Snapshot is the state a client needs to render a conversation from scratch.
type Snapshot struct {
	Conversation Chat      `json:"conversation"`
	Messages     []Message `json:"messages"`
	Running      bool      `json:"running"`
	Truncated    bool      `json:"truncated"`
}

// Delta is one streaming increment of an assistant message.
type Delta struct {
	MessageID uint   `json:"messageId"`
	Text      string `json:"text,omitempty"`
	Reasoning string `json:"reasoning,omitempty"`
}

// Turn marks a generation boundary.
type Turn struct {
	Phase   string `json:"phase"`
	Outcome string `json:"outcome,omitempty"`
}

// Event is one SSE frame. Exactly one payload field is set for a given Type.
type Event struct {
	ChatID       uint      `json:"chatId"`
	Timestamp    time.Time `json:"timestamp"`
	Type         string    `json:"type"`
	Snapshot     *Snapshot `json:"snapshot,omitempty"`
	Delta        *Delta    `json:"delta,omitempty"`
	Message      *Message  `json:"message,omitempty"`
	Conversation *Chat     `json:"conversation,omitempty"`
	Turn         *Turn     `json:"turn,omitempty"`
	Reason       string    `json:"reason,omitempty"`
}
