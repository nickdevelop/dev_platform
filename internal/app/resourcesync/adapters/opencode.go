package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"devplatform/internal/core/agents"
	"devplatform/internal/app/resources"
	"devplatform/internal/app/resourcesync"
)

// OpenCode renders resources for the opencode backend into the
// platform-managed .opencode directory.
type OpenCode struct{}

// BackendID identifies the opencode backend.
func (OpenCode) BackendID() agents.BackendID { return agents.BackendOpenCodeV2 }

// Supports reports the kinds opencode can represent.
func (OpenCode) Supports(kind resources.ResourceKind) bool {
	switch kind {
	case resources.ResourcePrompt, resources.ResourceSkill, resources.ResourceMCP:
		return true
	default:
		return false
	}
}

// Target is the opencode directory entry for a resource.
func (OpenCode) Target(projectID uint, resource resources.ResourceDefinition) (resourcesync.ResourceTarget, error) {
	dir, ext, err := openCodeLocation(resource.Kind)
	if err != nil {
		return resourcesync.ResourceTarget{}, err
	}
	return resourcesync.ResourceTarget{
		ProjectID: projectID,
		BackendID: agents.BackendOpenCodeV2,
		Path:      ".opencode/" + dir + "/" + resource.Name + ext,
	}, nil
}

func openCodeLocation(kind resources.ResourceKind) (dir, ext string, err error) {
	switch kind {
	case resources.ResourcePrompt:
		return "prompts", ".md", nil
	case resources.ResourceSkill:
		return "skills", ".md", nil
	case resources.ResourceMCP:
		return "mcp", ".json", nil
	default:
		return "", "", fmt.Errorf("resourcesync/adapters: opencode does not support %s", kind)
	}
}

// Render produces the opencode file for a resource. Prompts and skills are
// markdown; MCP servers are JSON with the backend's credentials block.
func (a OpenCode) Render(ctx context.Context, resource resources.ResourceDefinition, rc resourcesync.RenderContext) (resourcesync.ManagedPatch, error) {
	target, err := a.Target(rc.ProjectID, resource)
	if err != nil {
		return resourcesync.ManagedPatch{}, err
	}
	switch resource.Kind {
	case resources.ResourcePrompt, resources.ResourceSkill:
		return managedPatch(resource, target, renderMarkdown(resource)), nil
	case resources.ResourceMCP:
		content, err := renderOpenCodeMCP(ctx, resource, rc)
		if err != nil {
			return resourcesync.ManagedPatch{}, err
		}
		return managedPatch(resource, target, content), nil
	default:
		return resourcesync.ManagedPatch{}, fmt.Errorf("resourcesync/adapters: opencode does not support %s", resource.Kind)
	}
}

// renderOpenCodeMCP wraps the neutral MCP definition in opencode's shape and
// injects the resolved credential under credentials.token.
func renderOpenCodeMCP(ctx context.Context, resource resources.ResourceDefinition, rc resourcesync.RenderContext) ([]byte, error) {
	definition, err := parseMCPDefinition(resource)
	if err != nil {
		return nil, err
	}
	doc := map[string]any{"name": resource.Name}
	if len(definition) > 0 {
		doc["server"] = definition
	}
	value, err := resolveSecret(ctx, rc, resource.SecretRef)
	if err != nil {
		return nil, err
	}
	if value != "" {
		doc["credentials"] = map[string]string{"token": value}
	}
	return marshalDocument(doc)
}

// parseMCPDefinition decodes the neutral MCP body as a JSON object. An empty
// body is allowed; a non-object body is rejected with a message that never
// includes the body.
func parseMCPDefinition(resource resources.ResourceDefinition) (map[string]any, error) {
	if strings.TrimSpace(resource.Content) == "" {
		return nil, nil
	}
	var definition map[string]any
	if err := json.Unmarshal([]byte(resource.Content), &definition); err != nil {
		return nil, fmt.Errorf("resourcesync/adapters: mcp %q content is not a JSON object", resource.Name)
	}
	return definition, nil
}

// marshalDocument renders a JSON document with a stable indent and newline.
func marshalDocument(doc map[string]any) ([]byte, error) {
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("resourcesync/adapters: encode document: %w", err)
	}
	return append(data, '\n'), nil
}
