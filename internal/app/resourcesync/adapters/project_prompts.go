// Package adapters provides the per-backend resource translators that render
// canonical Skill, MCP and prompt definitions into the managed files each
// backend reads. Each adapter declares the kinds it supports and produces a
// project-relative target plus content; the coordinator owns when a patch is
// previewed, applied or paused on conflict.
//
// These are the platform's managed formats. They are deliberately explicit
// and versioned in docs/resource-sync.md; when a backend's native format is
// verified against a live installation, only the adapter's render function
// changes, never the ownership or conflict machinery.
package adapters

import (
	"context"
	"fmt"
	"strings"

	"devplatform/internal/core/agents"
	"devplatform/internal/app/resources"
	"devplatform/internal/app/resourcesync"
)

// ProjectPrompts synchronizes project-scoped prompt definitions into a
// project-level platform-managed directory.
type ProjectPrompts struct{}

// BackendID is empty: project prompts are not backend-specific.
func (ProjectPrompts) BackendID() agents.BackendID { return "" }

// Supports reports that the project adapter represents prompts only.
func (ProjectPrompts) Supports(kind resources.ResourceKind) bool {
	return kind == resources.ResourcePrompt
}

// Target is the platform-managed prompt file inside the project.
func (ProjectPrompts) Target(projectID uint, resource resources.ResourceDefinition) (resourcesync.ResourceTarget, error) {
	return resourcesync.ResourceTarget{
		ProjectID: projectID,
		Path:      ".devplatform/prompts/" + resource.Name + ".md",
	}, nil
}

// Render produces the markdown prompt file.
func (a ProjectPrompts) Render(_ context.Context, resource resources.ResourceDefinition, rc resourcesync.RenderContext) (resourcesync.ManagedPatch, error) {
	target, err := a.Target(rc.ProjectID, resource)
	if err != nil {
		return resourcesync.ManagedPatch{}, err
	}
	return managedPatch(resource, target, renderMarkdown(resource)), nil
}

// renderMarkdown renders a prompt or skill body with a stable, deterministic
// header so the file is readable on its own. Line endings are normalized to
// LF first: a definition carrying CRLF (a Windows checkout) must render the
// same bytes as its LF twin, or every sync would report the file dirty.
func renderMarkdown(resource resources.ResourceDefinition) []byte {
	var builder strings.Builder
	builder.WriteString("# ")
	builder.WriteString(resource.Name)
	builder.WriteString("\n\n")
	if resource.Description != "" {
		builder.WriteString(resource.Description)
		builder.WriteString("\n\n")
	}
	content := strings.ReplaceAll(resource.Content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")
	builder.WriteString(content)
	if !strings.HasSuffix(content, "\n") {
		builder.WriteString("\n")
	}
	return []byte(builder.String())
}

// managedPatch assembles a patch with a summary that never contains content.
func managedPatch(resource resources.ResourceDefinition, target resourcesync.ResourceTarget, content []byte) resourcesync.ManagedPatch {
	return resourcesync.ManagedPatch{
		ResourceID:      resource.ID,
		Kind:            resource.Kind,
		Target:          target,
		Content:         content,
		Summary:         fmt.Sprintf("write %s %q to %s", resource.Kind, resource.Name, target.Path),
		ResourceVersion: resource.UpdatedAt.UTC().Format("2006-01-02T15:04:05.999999999Z07:00"),
	}
}

// resolveSecret resolves an opaque reference, never echoing the value.
func resolveSecret(ctx context.Context, rc resourcesync.RenderContext, ref string) (string, error) {
	if ref == "" {
		return "", nil
	}
	if rc.Secrets == nil {
		return "", fmt.Errorf("resourcesync/adapters: a secret store is required to resolve %s", ref)
	}
	value, err := rc.Secrets.Get(ctx, ref)
	if err != nil {
		return "", fmt.Errorf("resourcesync/adapters: resolve secret %s: %w", ref, err)
	}
	return value, nil
}
