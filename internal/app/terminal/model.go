package terminal

import (
	"time"
)

// ServerDTO is the presentation model for a terminal server.
type ServerDTO struct {
	ID        uint      `json:"id"`
	Name      string    `json:"name"`
	Host      string    `json:"host"`
	Port      int       `json:"port"`
	Username  string    `json:"username"`
	AuthType  string    `json:"auth_type"` // "local" | "password" | "key"
	IsLocal   bool      `json:"is_local"`
	HasSecret bool      `json:"has_secret"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// CreateServerRequest is the payload to register a new remote third-party server.
type CreateServerRequest struct {
	Name     string `json:"name" binding:"required"`
	Host     string `json:"host" binding:"required"`
	Port     int    `json:"port"`
	Username string `json:"username" binding:"required"`
	AuthType string `json:"auth_type" binding:"required"` // "password" | "key"
	Secret   string `json:"secret" binding:"required"`    // Password or SSH private key text
}

// UpdateServerRequest is the payload to edit an existing remote server.
type UpdateServerRequest struct {
	Name     string `json:"name" binding:"required"`
	Host     string `json:"host" binding:"required"`
	Port     int    `json:"port"`
	Username string `json:"username" binding:"required"`
	AuthType string `json:"auth_type" binding:"required"` // "password" | "key"
	Secret   string `json:"secret"`                       // If empty, existing secret is retained
}

// TestConnectionRequest is the payload to verify remote SSH connectivity.
type TestConnectionRequest struct {
	Host     string `json:"host" binding:"required"`
	Port     int    `json:"port"`
	Username string `json:"username" binding:"required"`
	AuthType string `json:"auth_type" binding:"required"` // "password" | "key"
	Secret   string `json:"secret"`                       // Required if not testing by existing server ID
	ServerID *uint  `json:"server_id"`                    // If set, uses stored secret
}

// StatusDTO reports the platform terminal engine status.
type StatusDTO struct {
	Supported bool   `json:"supported"`
	OS        string `json:"os"`
	Message   string `json:"message,omitempty"`
}

// WSClientMessage represents an incoming control message from the WebSocket client.
type WSClientMessage struct {
	Type string `json:"type"` // "input" | "resize"
	Data string `json:"data,omitempty"`
	Cols uint16 `json:"cols,omitempty"`
	Rows uint16 `json:"rows,omitempty"`
}
