package platformtools

import (
	"context"

	"devplatform/internal/core/agentloop"
)

// AsAgentLoopRegistry wraps Executor into an agentloop.Registry.
func (e *Executor) AsAgentLoopRegistry() agentloop.Registry {
	defs := e.Definitions()
	tools := make([]agentloop.Tool, 0, len(defs))

	for _, d := range defs {
		// Convert chat.ToolDefinition to agentloop.ToolDefinition
		loopDef := agentloop.ToolDefinition{
			Type: d.Type,
			Function: agentloop.FunctionSchema{
				Name:        d.Function.Name,
				Description: d.Function.Description,
				Parameters:  d.Function.Parameters,
			},
		}
		name := d.Function.Name
		tools = append(tools, &agentloop.FuncTool{
			Def: loopDef,
			Fn: func(ctx context.Context, call agentloop.ToolCall) (agentloop.ToolResult, error) {
				out, err := e.Execute(ctx, name, call.Arguments)
				if err != nil {
					return agentloop.ToolResult{
						Status: agentloop.ToolStatusFailed,
						Output: out,
						Error:  err.Error(),
					}, err
				}
				return agentloop.ToolResult{
					Status: agentloop.ToolStatusCompleted,
					Output: out,
				}, nil
			},
		})
	}
	return agentloop.NewRegistry(tools...)
}
