package bot

import (
	"errors"
	"time"
)

// Channel statuses.
const (
	StatusUnconfigured = "unconfigured"
	StatusWaitingQR    = "waiting_qr"
	StatusScanned      = "scanned"
	StatusConnected    = "connected"
	StatusExpired      = "expired"
	StatusDisconnected = "disconnected"
	StatusError        = "error"
)

// Session watch statuses.
const (
	WatchWatching = "watching"
	WatchNotified = "notified"
	WatchFailed   = "failed"
)

// Channel target binding modes.
const (
	TargetModeAssistant = "assistant"
	TargetModeAgent     = "agent"
)

// Service errors.
var (
	ErrChannelNotFound    = errors.New("bot: channel not found")
	ErrInvalidChannel     = errors.New("bot: invalid channel request")
	ErrUnknownChannelType = errors.New("bot: unknown channel type")
	ErrMissingCredentials = errors.New("bot: channel has no stored credentials")
	ErrUnsupportedLogin   = errors.New("bot: channel does not support QR login")
)

// Channel is the JSON-stable API representation of one configured message channel.
// Credentials (bot_token) are never exposed here; HasCredentials indicates login state.
type Channel struct {
	ID               uint       `json:"id"`
	Name             string     `json:"name"`
	ChannelType      string     `json:"channelType"`
	Enabled          bool       `json:"enabled"`
	Status           string     `json:"status"`
	LastError        string     `json:"lastError,omitempty"`
	HasCredentials   bool       `json:"hasCredentials"`
	AccountID        string     `json:"accountId,omitempty"`
	TargetMode       string     `json:"targetMode"`
	ModelID          uint       `json:"modelId"`
	PromptID         uint       `json:"promptId"`
	SystemPrompt     string     `json:"systemPrompt,omitempty"`
	DefaultProjectID uint       `json:"defaultProjectId"`
	WorktreeID       uint       `json:"worktreeId"`
	DefaultBackend   string     `json:"defaultBackend,omitempty"`
	AgentModel       string     `json:"agentModel,omitempty"`
	ChatGroupID      uint       `json:"chatGroupId"`
	ChatID           uint       `json:"chatId"`
	ActiveSessionID  string     `json:"activeSessionId,omitempty"`
	TypingIndicator  bool       `json:"typingIndicator"`
	AutoNotify       bool       `json:"autoNotify"`
	MaxReplyLength   int        `json:"maxReplyLength"`
	LastConnectedAt  *time.Time `json:"lastConnectedAt,omitempty"`
	CreatedAt        time.Time  `json:"createdAt"`
	UpdatedAt        time.Time  `json:"updatedAt"`
}

// CreateChannelInput is the payload for POST /api/bot/channels.
type CreateChannelInput struct {
	Name             string `json:"name"`
	ChannelType      string `json:"channelType"`
	TargetMode       string `json:"targetMode"`
	ChatGroupID      uint   `json:"chatGroupId"`
	ModelID          uint   `json:"modelId"`
	DefaultProjectID uint   `json:"defaultProjectId"`
	WorktreeID       uint   `json:"worktreeId"`
	PromptID         uint   `json:"promptId"`
	SystemPrompt     string `json:"systemPrompt"`
	AgentModel       string `json:"agentModel"`
	TypingIndicator  *bool  `json:"typingIndicator"`
	AutoNotify       *bool  `json:"autoNotify"`
	MaxReplyLength   *int   `json:"maxReplyLength"`
}

// UpdateChannelInput is the payload for PUT /api/bot/channels/:id.
type UpdateChannelInput struct {
	Name             *string `json:"name"`
	Enabled          *bool   `json:"enabled"`
	TargetMode       *string `json:"targetMode"`
	ChatGroupID      *uint   `json:"chatGroupId"`
	ModelID          *uint   `json:"modelId"`
	DefaultProjectID *uint   `json:"defaultProjectId"`
	WorktreeID       *uint   `json:"worktreeId"`
	PromptID         *uint   `json:"promptId"`
	SystemPrompt     *string `json:"systemPrompt"`
	AgentModel       *string `json:"agentModel"`
	TypingIndicator  *bool   `json:"typingIndicator"`
	AutoNotify       *bool   `json:"autoNotify"`
	MaxReplyLength   *int    `json:"maxReplyLength"`
}

// QRLoginState captures the live progress of a channel QR login session.
type QRLoginState struct {
	ChannelID uint   `json:"channelId"`
	Status    string `json:"status"`
	QRCode    string `json:"qrCode,omitempty"`
	QRURL     string `json:"qrUrl,omitempty"`
	Error     string `json:"error,omitempty"`
}
