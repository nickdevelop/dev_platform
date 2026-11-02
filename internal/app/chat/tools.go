package chat

import (
	"context"
	"encoding/json"
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

// ToolExecutor executes platform tools requested by the model.
type ToolExecutor interface {
	Definitions() []ToolDefinition
	Execute(ctx context.Context, name string, argumentsJSON string) (string, error)
}
