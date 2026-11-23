package agentloop

import (
	"context"
	"fmt"
	"sync"
)

// Registry manages and routes tool calls to their respective handlers.
type Registry interface {
	Definitions() []ToolDefinition
	Execute(ctx context.Context, call ToolCall) (ToolResult, error)
}

// MemoryRegistry is an in-memory concurrent-safe implementation of Registry.
type MemoryRegistry struct {
	mu    sync.RWMutex
	tools map[string]Tool
	order []string
}

// NewRegistry creates a new empty Tool Registry.
func NewRegistry(tools ...Tool) *MemoryRegistry {
	r := &MemoryRegistry{
		tools: make(map[string]Tool),
		order: make([]string, 0),
	}
	for _, t := range tools {
		r.Register(t)
	}
	return r
}

// Register adds a tool into the registry. If a tool with the same name exists, it is replaced.
func (r *MemoryRegistry) Register(tool Tool) {
	if tool == nil {
		return
	}
	def := tool.Definition()
	name := def.Function.Name
	if name == "" {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.tools[name]; !exists {
		r.order = append(r.order, name)
	}
	r.tools[name] = tool
}

// Definitions returns OpenAI-compatible definitions for all registered tools in registered order.
func (r *MemoryRegistry) Definitions() []ToolDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	defs := make([]ToolDefinition, 0, len(r.order))
	for _, name := range r.order {
		if t, ok := r.tools[name]; ok {
			defs = append(defs, t.Definition())
		}
	}
	return defs
}

// Execute routes and executes the requested tool call.
func (r *MemoryRegistry) Execute(ctx context.Context, call ToolCall) (ToolResult, error) {
	r.mu.RLock()
	tool, ok := r.tools[call.Name]
	r.mu.RUnlock()

	if !ok {
		err := fmt.Errorf("tool %q not found in registry", call.Name)
		return ToolResult{
			Status: ToolStatusFailed,
			Output: fmt.Sprintf(`{"error": %q}`, err.Error()),
			Error:  err.Error(),
		}, err
	}

	res, err := tool.Execute(ctx, call)
	if err != nil && res.Status == "" {
		res.Status = ToolStatusFailed
	}
	if err != nil && res.Output == "" {
		res.Output = fmt.Sprintf(`{"error": %q}`, err.Error())
	}
	return res, err
}

// FuncTool is a convenience adapter to convert a standalone function into a Tool.
type FuncTool struct {
	Def ToolDefinition
	Fn  func(ctx context.Context, call ToolCall) (ToolResult, error)
}

func (f *FuncTool) Definition() ToolDefinition {
	return f.Def
}

func (f *FuncTool) Execute(ctx context.Context, call ToolCall) (ToolResult, error) {
	if f.Fn == nil {
		return ToolResult{Status: ToolStatusCompleted}, nil
	}
	return f.Fn(ctx, call)
}
