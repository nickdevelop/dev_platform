// Package channels defines provider-neutral contracts for external IM bot
// channels (such as WeChat iLink, Feishu, or WeCom). Concrete protocol adapters
// live under internal/providers/channels/* and register with the composition root.
package channels

import (
	"context"
	"errors"
	"time"
)

// ChannelType identifies a channel protocol implementation (e.g. "wechat").
type ChannelType string

var (
	// ErrUnknownChannel reports a channel type not registered in the registry.
	ErrUnknownChannel = errors.New("channels: unknown channel type")
	// ErrSessionExpired reports an upstream IM credential expiry requiring re-login.
	ErrSessionExpired = errors.New("channels: session expired, re-login required")
	// ErrQRCodeExpired reports an authentication QR code expiring before scan.
	ErrQRCodeExpired = errors.New("channels: qr code expired")
	// ErrQRCodeCancelled reports the user cancelling login on their device.
	ErrQRCodeCancelled = errors.New("channels: login cancelled")
	// ErrLoginTimeout reports a login wait exceeding the deadline or context.
	ErrLoginTimeout = errors.New("channels: login timed out")
)

// ChannelDescriptor describes static facts about a channel adapter.
type ChannelDescriptor struct {
	Type            ChannelType `json:"type"`
	Label           string      `json:"label"`
	Description     string      `json:"description"`
	SupportsQRLogin bool        `json:"supportsQrLogin"`
}

// Credentials stores persisted authentication material for a channel instance.
// It is stored in the platform secret store and never returned verbatim to API callers.
type Credentials struct {
	BotToken  string `json:"bot_token"`
	BaseURL   string `json:"baseurl,omitempty"`
	AccountID string `json:"account_id,omitempty"`
	UserID    string `json:"user_id,omitempty"`
}

// LoginStatus enumerates QR / interactive login states.
type LoginStatus string

const (
	LoginPending   LoginStatus = "pending"
	LoginScanned   LoginStatus = "scanned"
	LoginConfirmed LoginStatus = "confirmed"
	LoginExpired   LoginStatus = "expired"
	LoginCancelled LoginStatus = "cancelled"
	LoginError     LoginStatus = "error"
)

// LoginEvent reports real-time QR login progress.
type LoginEvent struct {
	Status      LoginStatus  `json:"status"`
	QRCode      string       `json:"qrCode,omitempty"`
	QRURL       string       `json:"qrUrl,omitempty"`
	Credentials *Credentials `json:"-"`
	Error       string       `json:"error,omitempty"`
}

// LoginSession represents an active QR login attempt.
type LoginSession struct {
	QRCode string
	QRURL  string
	Wait   func(ctx context.Context, onStatus func(LoginEvent)) (*Credentials, error)
}

// InboundMessage is a normalized message received from an external IM user.
type InboundMessage struct {
	ChannelType  ChannelType
	MessageID    string
	FromUserID   string
	ToUserID     string
	GroupID      string
	Text         string
	ContextToken string
	Timestamp    time.Time
}

// OutboundMessage is a normalized reply or notification delivered to an IM user.
type OutboundMessage struct {
	ToUserID     string
	GroupID      string
	ContextToken string
	Text         string
}

// ListenCallbacks wires lifecycle and message hooks into ChannelAdapter.Listen.
type ListenCallbacks struct {
	OnReady   func()
	OnCursor  func(cursor string)
	OnMessage func(ctx context.Context, msg InboundMessage) error
}

// ChannelAdapter bridges a specific IM protocol to the platform's bot domain.
type ChannelAdapter interface {
	Descriptor() ChannelDescriptor
	StartLogin(ctx context.Context) (*LoginSession, error)
	Listen(ctx context.Context, creds Credentials, cursor string, cb ListenCallbacks) error
	SendMessage(ctx context.Context, creds Credentials, msg OutboundMessage, maxLen int) error
	SendTyping(ctx context.Context, creds Credentials, userID string, typing bool) error
}
