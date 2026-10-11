package resourcesync

import (
	"context"

	"devplatform/internal/agents"
	"devplatform/internal/resources"
)

// SecretResolver resolves an opaque credential reference to its plaintext
// value for rendering. Implementations must not log or echo the value.
type SecretResolver interface {
	Get(ctx context.Context, id string) (string, error)
}

// RenderContext carries what an adapter needs beyond the definition itself.
type RenderContext struct {
	// ProjectID is the project the patch is rendered for.
	ProjectID uint
	// Secrets resolves MCP credential references, when the adapter has one.
	Secrets SecretResolver
}

// ResourceSyncAdapter translates one canonical resource definition into a
// backend-specific managed patch. An adapter that does not support a kind
// must say so through Supports; the coordinator reports it unavailable rather
// than ignoring it. Target is exposed separately so the coordinator can build
// a status preview even when rendering fails.
type ResourceSyncAdapter interface {
	// BackendID is the backend the adapter renders for; empty means
	// project-level.
	BackendID() agents.BackendID
	// Supports reports whether the adapter can represent the resource kind.
	Supports(kind resources.ResourceKind) bool
	// Target is the location the resource synchronizes to for a project.
	Target(projectID uint, resource resources.ResourceDefinition) (ResourceTarget, error)
	// Render produces the managed patch for the resource.
	Render(ctx context.Context, resource resources.ResourceDefinition, rc RenderContext) (ManagedPatch, error)
}
