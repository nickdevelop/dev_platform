package adapters

import (
	"context"
	"fmt"

	"devplatform/internal/agents"
	"devplatform/internal/resources"
	"devplatform/internal/resourcesync"
)

// Pi renders resources for the pi CLI backend into the platform-managed .pi
// directory.
type Pi struct{}

// BackendID identifies the pi backend.
func (Pi) BackendID() agents.BackendID { return agents.BackendPi }

// Supports reports the kinds pi can represent.
func (Pi) Supports(kind resources.ResourceKind) bool {
	switch kind {
	case resources.ResourcePrompt, resources.ResourceSkill, resources.ResourceMCP:
		return true
	default:
		return false
	}
}

// Target is the pi directory entry for a resource.
func (Pi) Target(projectID uint, resource resources.ResourceDefinition) (resourcesync.ResourceTarget, error) {
	dir, ext, err := piLocation(resource.Kind)
	if err != nil {
		return resourcesync.ResourceTarget{}, err
	}
	return resourcesync.ResourceTarget{
		ProjectID: projectID,
		BackendID: agents.BackendPi,
		Path:      ".pi/" + dir + "/" + resource.Name + ext,
	}, nil
}

func piLocation(kind resources.ResourceKind) (dir, ext string, err error) {
	switch kind {
	case resources.ResourcePrompt:
		return "prompts", ".md", nil
	case resources.ResourceSkill:
		return "skills", ".md", nil
	case resources.ResourceMCP:
		return "mcp", ".json", nil
	default:
		return "", "", fmt.Errorf("resourcesync/adapters: pi does not support %s", kind)
	}
}

// Render produces the pi file for a resource. Prompts and skills are
// markdown; MCP servers are JSON with the backend's env block.
func (a Pi) Render(ctx context.Context, resource resources.ResourceDefinition, rc resourcesync.RenderContext) (resourcesync.ManagedPatch, error) {
	target, err := a.Target(rc.ProjectID, resource)
	if err != nil {
		return resourcesync.ManagedPatch{}, err
	}
	switch resource.Kind {
	case resources.ResourcePrompt, resources.ResourceSkill:
		return managedPatch(resource, target, renderMarkdown(resource)), nil
	case resources.ResourceMCP:
		content, err := renderPiMCP(ctx, resource, rc)
		if err != nil {
			return resourcesync.ManagedPatch{}, err
		}
		return managedPatch(resource, target, content), nil
	default:
		return resourcesync.ManagedPatch{}, fmt.Errorf("resourcesync/adapters: pi does not support %s", resource.Kind)
	}
}

// renderPiMCP wraps the neutral MCP definition in pi's shape and injects the
// resolved credential under env.MCP_TOKEN.
func renderPiMCP(ctx context.Context, resource resources.ResourceDefinition, rc resourcesync.RenderContext) ([]byte, error) {
	definition, err := parseMCPDefinition(resource)
	if err != nil {
		return nil, err
	}
	doc := map[string]any{"serverName": resource.Name}
	if len(definition) > 0 {
		doc["definition"] = definition
	}
	value, err := resolveSecret(ctx, rc, resource.SecretRef)
	if err != nil {
		return nil, err
	}
	if value != "" {
		doc["env"] = map[string]string{"MCP_TOKEN": value}
	}
	return marshalDocument(doc)
}
