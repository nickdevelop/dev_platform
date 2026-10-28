package proxy

import (
	"errors"
	"time"
)

var (
	ErrInvalidProxyName    = errors.New("invalid proxy name")
	ErrInvalidProxyAlias   = errors.New("invalid proxy alias")
	ErrInvalidProxyType    = errors.New("invalid proxy type")
	ErrInvalidProxyTarget  = errors.New("invalid proxy target")
	ErrDuplicateProxyAlias = errors.New("duplicate proxy alias")
	ErrProxyNotFound       = errors.New("proxy not found")
)

const (
	TypeDirectory = "directory"
	TypePort      = "port"
)

// ServiceProxy represents a contextpath proxy mapping.
type ServiceProxy struct {
	ID        uint      `json:"id"`
	Name      string    `json:"name"`
	Alias     string    `json:"alias"`
	Type      string    `json:"type"` // "directory" | "port"
	Target    string    `json:"target"`
	Enabled   bool      `json:"enabled"`
	Online    *bool     `json:"online,omitempty"` // For port type: whether the target port is currently listening
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type CreateProxyRequest struct {
	Name    string `json:"name"`
	Alias   string `json:"alias"`
	Type    string `json:"type"`
	Target  string `json:"target"`
	Enabled *bool  `json:"enabled"`
}

type UpdateProxyRequest struct {
	Name    string `json:"name"`
	Alias   string `json:"alias"`
	Type    string `json:"type"`
	Target  string `json:"target"`
	Enabled *bool  `json:"enabled"`
}

type DirectoryEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

type BrowseDirectoriesResponse struct {
	Current string           `json:"current"`
	Parent  string           `json:"parent"`
	Entries []DirectoryEntry `json:"entries"`
}

