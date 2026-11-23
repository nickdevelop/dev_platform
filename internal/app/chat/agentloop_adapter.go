package chat

import (
	"context"

	"devplatform/internal/core/agentloop"
)

// AgentLoopCompleter wraps chat.Completer to implement agentloop.Completer.
type AgentLoopCompleter struct {
	inner Completer
}

// NewAgentLoopCompleter creates an adapter from chat.Completer to agentloop.Completer.
func NewAgentLoopCompleter(completer Completer) *AgentLoopCompleter {
	return &AgentLoopCompleter{inner: completer}
}

func (c *AgentLoopCompleter) Stream(ctx context.Context, req agentloop.CompletionRequest, onDelta func(agentloop.CompletionDelta) error) error {
	chatReq := CompletionRequest{
		BaseURL:  req.BaseURL,
		APIKey:   req.APIKey,
		Model:    req.Model,
		Messages: make([]CompletionMessage, 0, len(req.Messages)),
		Tools:    make([]ToolDefinition, 0, len(req.Tools)),
	}

	for _, m := range req.Messages {
		var toolCalls []ToolCall
		if len(m.ToolCalls) > 0 {
			toolCalls = make([]ToolCall, 0, len(m.ToolCalls))
			for _, tc := range m.ToolCalls {
				toolCalls = append(toolCalls, ToolCall{
					Index:     tc.Index,
					ID:        tc.ID,
					Type:      tc.Type,
					Name:      tc.Name,
					Arguments: tc.Arguments,
				})
			}
		}
		chatReq.Messages = append(chatReq.Messages, CompletionMessage{
			Role:       m.Role,
			Text:       m.Text,
			ToolCallID: m.ToolCallID,
			ToolCalls:  toolCalls,
		})
	}

	for _, t := range req.Tools {
		chatReq.Tools = append(chatReq.Tools, ToolDefinition{
			Type: t.Type,
			Function: FunctionSchema{
				Name:        t.Function.Name,
				Description: t.Function.Description,
				Parameters:  t.Function.Parameters,
			},
		})
	}

	return c.inner.Stream(ctx, chatReq, func(delta CompletionDelta) error {
		var loopToolCalls []agentloop.ToolCall
		if len(delta.ToolCalls) > 0 {
			loopToolCalls = make([]agentloop.ToolCall, 0, len(delta.ToolCalls))
			for _, tc := range delta.ToolCalls {
				loopToolCalls = append(loopToolCalls, agentloop.ToolCall{
					Index:     tc.Index,
					ID:        tc.ID,
					Type:      tc.Type,
					Name:      tc.Name,
					Arguments: tc.Arguments,
				})
			}
		}
		var usage *agentloop.Usage
		if delta.Usage != nil {
			usage = &agentloop.Usage{
				PromptTokens:     delta.Usage.Input,
				CompletionTokens: delta.Usage.Output,
				TotalTokens:      delta.Usage.Input + delta.Usage.Output,
			}
		}
		return onDelta(agentloop.CompletionDelta{
			Text:      delta.Text,
			Reasoning: delta.Reasoning,
			ToolCalls: loopToolCalls,
			Usage:     usage,
		})
	})
}

// ToolExecutorRegistry adapts chat.ToolExecutor to agentloop.Registry.
type ToolExecutorRegistry struct {
	executor ToolExecutor
}

// NewToolExecutorRegistry creates an adapter for ToolExecutor.
func NewToolExecutorRegistry(executor ToolExecutor) *ToolExecutorRegistry {
	return &ToolExecutorRegistry{executor: executor}
}

func (r *ToolExecutorRegistry) Definitions() []agentloop.ToolDefinition {
	if r.executor == nil {
		return nil
	}
	raw := r.executor.Definitions()
	out := make([]agentloop.ToolDefinition, 0, len(raw))
	for _, d := range raw {
		out = append(out, agentloop.ToolDefinition{
			Type: d.Type,
			Function: agentloop.FunctionSchema{
				Name:        d.Function.Name,
				Description: d.Function.Description,
				Parameters:  d.Function.Parameters,
			},
		})
	}
	return out
}

func (r *ToolExecutorRegistry) Execute(ctx context.Context, call agentloop.ToolCall) (agentloop.ToolResult, error) {
	if r.executor == nil {
		return agentloop.ToolResult{Status: agentloop.ToolStatusFailed, Output: "tool executor not available"}, nil
	}
	output, err := r.executor.Execute(ctx, call.Name, call.Arguments)
	if err != nil {
		return agentloop.ToolResult{
			Status: agentloop.ToolStatusFailed,
			Output: output,
			Error:  err.Error(),
		}, err
	}
	return agentloop.ToolResult{
		Status: agentloop.ToolStatusCompleted,
		Output: output,
	}, nil
}
