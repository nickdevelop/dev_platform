package builtin

import (
	"encoding/json"

	"devplatform/internal/providers/builtin/tools"
)

// requiresApproval reports whether a tool mutates the workspace or executes
// processes: those are the invocations the confirm permission mode gates on.
func requiresApproval(toolName string) bool {
	switch toolName {
	case tools.ToolWriteFile, tools.ToolEditFile, tools.ToolExecuteCommand:
		return true
	default:
		return false
	}
}

// buildSchemas renders the registry's tool definitions for the wire format.
func buildSchemas(registry *tools.Registry) []toolSchema {
	definitions := registry.Definitions()
	schemas := make([]toolSchema, 0, len(definitions))
	for _, definition := range definitions {
		schemas = append(schemas, toolSchema{
			Type: "function",
			Function: toolFunctionSpec{
				Name:        definition.Function.Name,
				Description: definition.Function.Description,
				// Parameters is already a raw JSON schema object.
				Parameters: mustRawObject(definition.Function.Parameters),
			},
		})
	}
	return schemas
}

// mustRawObject converts a raw JSON schema into the parameters map shape the
// completer re-encodes; the conversion must never fail because the input came
// from the registry's own literals.
func mustRawObject(raw json.RawMessage) map[string]any {
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return map[string]any{"type": "object", "properties": map[string]any{}}
	}
	return out
}
