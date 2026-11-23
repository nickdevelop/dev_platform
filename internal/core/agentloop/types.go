package agentloop

import (
	"context"
	"encoding/json"
)

// Role constants for completion messages.
const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleTool      = "tool"
)

// ToolDefinition describes an OpenAI-compatible function tool.
type ToolDefinition struct {
	Type     string         `json:"type"` // always "function"
	Function FunctionSchema `json:"function"`
}

// FunctionSchema specifies the function tool name, description and parameters schema.
type FunctionSchema struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// ToolCall represents a function call requested by the model.
type ToolCall struct {
	Index     int    `json:"index"`
	ID        string `json:"id"`
	Type      string `json:"type"` // "function"
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ToolStatus represents the execution state of a tool invocation.
type ToolStatus string

const (
	ToolStatusCompleted       ToolStatus = "completed"
	ToolStatusSuspended       ToolStatus = "suspended"        // Workflow suspended (e.g. flow_ask_user)
	ToolStatusRequireApproval ToolStatus = "require_approval" // Requires user authorization
	ToolStatusFailed          ToolStatus = "failed"
)

// ToolResult captures the outcome of a tool execution.
type ToolResult struct {
	Status  ToolStatus `json:"status"`
	Output  string     `json:"output"`  // String representation returned to the model context
	Error   string     `json:"error,omitempty"`
	Payload any        `json:"payload,omitempty"` // Arbitrary domain metadata
}

// Tool is the standard interface all modular tools implement.
type Tool interface {
	Definition() ToolDefinition
	Execute(ctx context.Context, call ToolCall) (ToolResult, error)
}

// CompletionMessage represents a message in the conversation transcript.
type CompletionMessage struct {
	Role       string     `json:"role"`
	Text       string     `json:"text,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	Images     []string   `json:"images,omitempty"`
}

// CompletionDelta carries incremental chunks during model streaming.
type CompletionDelta struct {
	Text      string     `json:"text,omitempty"`
	Reasoning string     `json:"reasoning,omitempty"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
	Usage     *Usage     `json:"usage,omitempty"`
}

// Usage captures token accounting for a completion.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// CompletionRequest provides parameters required to run a completion turn.
type CompletionRequest struct {
	BaseURL     string              `json:"base_url,omitempty"`
	APIKey      string              `json:"api_key,omitempty"`
	Model       string              `json:"model"`
	Messages    []CompletionMessage `json:"messages"`
	Tools       []ToolDefinition    `json:"tools,omitempty"`
	Temperature *float64            `json:"temperature,omitempty"`
	MaxTokens   *int                `json:"max_tokens,omitempty"`
}

// Completer abstracts OpenAI-compatible chat streaming and completions.
type Completer interface {
	Stream(ctx context.Context, req CompletionRequest, onDelta func(CompletionDelta) error) error
}

// LoopOptions specifies options controlling the ReAct execution loop.
type LoopOptions struct {
	MaxSteps     int                         // Maximum tool iteration steps before terminating or forcing final answer
	ForceFinal   bool                        // If true, strips tools when MaxSteps is reached to prompt a text summary
	OnDelta      func(delta CompletionDelta) // Stream delta callback for real-time frontend updates
	OnToolStart  func(call ToolCall)         // Hook triggered right before tool execution begins
	OnToolEnd    func(call ToolCall, res ToolResult) // Hook triggered right after tool execution finishes
	OnStepFinish func(step int, calls []ToolCall)    // Hook triggered when an entire ReAct step completes
}

// RunOutcome summarizes the end result of a ReAct execution.
type RunOutcome struct {
	Messages     []CompletionMessage // Full updated transcript including tool calls and outputs
	FinalText    string              // Accumulated final assistant text response
	Reasoning    string              // Accumulated reasoning/thinking text
	StepsTaken   int                 // Total tool loop iterations executed
	Suspended    bool                // True if execution stopped due to ToolStatusSuspended
	ApprovalReq  bool                // True if execution stopped due to ToolStatusRequireApproval
	PendingCall  *ToolCall           // The tool call that triggered suspend or approval
	TotalUsage   Usage               // Aggregate token usage if reported by the completer
}
