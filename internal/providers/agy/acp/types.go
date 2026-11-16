package acp

import "encoding/json"

// JSON-RPC 2.0 basic frames
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

type RPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *RPCError) Error() string {
	return e.Message
}

type Notification struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// ACP methods
const (
	MethodInitialize        = "initialize"
	MethodSessionNew        = "session/new"
	MethodSessionLoad       = "session/load"
	MethodSessionPrompt     = "session/prompt"
	MethodSessionCancel     = "session/cancel"
	MethodSessionSetModel   = "session/set_model"
	MethodSessionSetMode    = "session/set_mode"
	MethodSessionUpdate     = "session/update"
	MethodRequestPermission = "session/request_permission"
)

// Initialize
type InitializeParams struct {
	ProtocolVersion    int                `json:"protocolVersion"`
	ClientInfo         *Implementation    `json:"clientInfo,omitempty"`
	ClientCapabilities ClientCapabilities `json:"clientCapabilities"`
}

type Implementation struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type ClientCapabilities struct {
	Terminal bool            `json:"terminal,omitempty"`
	FS       *FSCapabilities `json:"fs,omitempty"`
}

type FSCapabilities struct {
	ReadTextFile  bool `json:"readTextFile,omitempty"`
	WriteTextFile bool `json:"writeTextFile,omitempty"`
}

type InitializeResult struct {
	ProtocolVersion   int             `json:"protocolVersion"`
	AgentInfo         *Implementation `json:"agentInfo,omitempty"`
	AgentCapabilities json.RawMessage `json:"agentCapabilities,omitempty"`
}

// Session New / Load
type NewSessionParams struct {
	CWD                   string   `json:"cwd"`
	MCPServers            []any    `json:"mcpServers"`
	AdditionalDirectories []string `json:"additionalDirectories,omitempty"`
}

type LoadSessionParams struct {
	SessionID string `json:"sessionId"`
	CWD       string `json:"cwd"`
}

type SessionResult struct {
	SessionID     string          `json:"sessionId"`
	Models        *SessionModels  `json:"models,omitempty"`
	Modes         *SessionModes   `json:"modes,omitempty"`
	ConfigOptions json.RawMessage `json:"configOptions,omitempty"`
}

type SessionModels struct {
	AvailableModels []ModelDescriptor `json:"availableModels"`
	CurrentModelID  string            `json:"currentModelId"`
}

type ModelDescriptor struct {
	ModelID string `json:"modelId"`
	Name    string `json:"name"`
}

type SessionModes struct {
	CurrentModeID  string           `json:"currentModeId"`
	AvailableModes []ModeDescriptor `json:"availableModes"`
}

type ModeDescriptor struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Session Prompt
type PromptParams struct {
	SessionID string          `json:"sessionId"`
	Prompt    []PromptContent `json:"prompt"`
}

type PromptContent struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	Data     string `json:"data,omitempty"`
	MimeType string `json:"mimeType,omitempty"`
	URI      string `json:"uri,omitempty"`
}

type PromptResult struct {
	StopReason string      `json:"stopReason"`
	Usage      *UsageStats `json:"usage,omitempty"`
}

type UsageStats struct {
	InputTokens      int `json:"inputTokens"`
	OutputTokens     int `json:"outputTokens"`
	CachedReadTokens int `json:"cachedReadTokens"`
}

// Session SetModel & SetMode
type SetModelParams struct {
	SessionID string `json:"sessionId"`
	ModelID   string `json:"modelId"`
}

type SetModeParams struct {
	SessionID string `json:"sessionId"`
	ModeID    string `json:"modeId"`
}

// Session Cancel
type CancelParams struct {
	SessionID string `json:"sessionId"`
}

// Server-to-Client Request: session/request_permission
type RequestPermissionParams struct {
	SessionID string             `json:"sessionId"`
	ToolCall  ToolCallUpdate     `json:"toolCall"`
	Options   []PermissionOption `json:"options"`
}

type PermissionOption struct {
	OptionID string `json:"optionId"`
	Name     string `json:"name"`
	Kind     string `json:"kind"` // "allow_once", "allow_always", "reject_once", "reject_always"
}

type ToolCallUpdate struct {
	ToolCallID string          `json:"toolCallId"`
	Title      string          `json:"title,omitempty"`
	Kind       string          `json:"kind,omitempty"`
	Status     string          `json:"status,omitempty"`
	RawInput   json.RawMessage `json:"rawInput,omitempty"`
	RawOutput  json.RawMessage `json:"rawOutput,omitempty"`
}

type RequestPermissionResult struct {
	Outcome PermissionOutcome `json:"outcome"`
}

type PermissionOutcome struct {
	Outcome  string `json:"outcome"` // "selected" or "cancelled"
	OptionID string `json:"optionId,omitempty"`
}

// Notification: session/update
type SessionUpdateNotification struct {
	SessionID string          `json:"sessionId"`
	Update    json.RawMessage `json:"update"`
}

type SessionUpdateHeader struct {
	SessionUpdate string `json:"sessionUpdate"`
}

// Specific update payloads
type UserMessageChunkUpdate struct {
	SessionUpdate string        `json:"sessionUpdate"`
	Content       PromptContent `json:"content"`
}

type AgentMessageChunkUpdate struct {
	SessionUpdate string        `json:"sessionUpdate"`
	Content       PromptContent `json:"content"`
}

type AgentThoughtChunkUpdate struct {
	SessionUpdate string        `json:"sessionUpdate"`
	Content       PromptContent `json:"content"`
}

type SessionInfoUpdate struct {
	SessionUpdate string `json:"sessionUpdate"`
	Title         string `json:"title"`
}

type ToolCallUpdatePayload struct {
	SessionUpdate string          `json:"sessionUpdate"`
	ToolCallID    string          `json:"toolCallId"`
	Name          string          `json:"name,omitempty"`
	Title         string          `json:"title,omitempty"`
	Kind          string          `json:"kind,omitempty"`
	Status        string          `json:"status,omitempty"`
	Locations     json.RawMessage `json:"locations,omitempty"`
	RawInput      json.RawMessage `json:"rawInput,omitempty"`
	RawOutput     json.RawMessage `json:"rawOutput,omitempty"`
	Content       json.RawMessage `json:"content,omitempty"`
}

type UsageUpdatePayload struct {
	SessionUpdate string `json:"sessionUpdate"`
	Used          int    `json:"used"`
	Size          int    `json:"size"`
}
