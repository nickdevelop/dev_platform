package resourcesync

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"devplatform/internal/core/agents"
	"devplatform/internal/app/resources"
)

// ErrFirstApplyRequired reports that a resource has not been applied to a
// project yet, so an automatic sync must not create its target: the first
// apply requires explicit user consent.
var ErrFirstApplyRequired = errors.New("resourcesync: first apply requires explicit consent")

// ResourceSource supplies the canonical definitions and their project links.
// It is satisfied by the resource service.
type ResourceSource interface {
	// ListProjectResources returns the resources associated with a project.
	ListProjectResources(projectID uint) ([]resources.ResourceDefinition, error)
	// Get returns one resource definition.
	Get(id uint) (resources.ResourceDefinition, error)
}

// Coordinator renders canonical resources through the registered adapters and
// applies them through the managed-file manager. It reports per-resource
// status: one resource failing to render or apply never hides or aborts
// another resource's result.
type Coordinator struct {
	manager  *Manager
	source   ResourceSource
	secrets  SecretResolver
	project  ResourceSyncAdapter
	adapters map[agents.BackendID]ResourceSyncAdapter
}

// NewCoordinator builds a coordinator. An adapter with an empty BackendID is
// the project-level adapter; the rest are keyed by their backend.
func NewCoordinator(manager *Manager, source ResourceSource, secretResolver SecretResolver, adapters ...ResourceSyncAdapter) *Coordinator {
	c := &Coordinator{
		manager:  manager,
		source:   source,
		secrets:  secretResolver,
		adapters: map[agents.BackendID]ResourceSyncAdapter{},
	}
	for _, adapter := range adapters {
		if adapter == nil {
			continue
		}
		if adapter.BackendID() == "" {
			c.project = adapter
			continue
		}
		c.adapters[adapter.BackendID()] = adapter
	}
	return c
}

// Supports reports whether the backend's adapter can represent the kind. The
// empty backend names the project-level adapter.
func (c *Coordinator) Supports(kind resources.ResourceKind, backend agents.BackendID) bool {
	adapter := c.adapterFor(backend)
	return adapter != nil && adapter.Supports(kind)
}

func (c *Coordinator) adapterFor(backend agents.BackendID) ResourceSyncAdapter {
	if backend == "" {
		return c.project
	}
	return c.adapters[backend]
}

// adaptersFor selects the adapters a definition synchronizes through.
// Project-scoped prompts go to the project adapter; session prompts are
// selected per session and are not file-synchronized; skills and MCP servers
// go to every applicable backend adapter.
func (c *Coordinator) adaptersFor(def resources.ResourceDefinition) []ResourceSyncAdapter {
	if def.Kind == resources.ResourcePrompt {
		if def.PromptScope == resources.PromptScopeProject && c.project != nil {
			return []ResourceSyncAdapter{c.project}
		}
		return nil
	}

	backends := make([]agents.BackendID, 0, len(c.adapters))
	for backend := range c.adapters {
		backends = append(backends, backend)
	}
	sort.Slice(backends, func(i, j int) bool { return backends[i] < backends[j] })

	adapters := make([]ResourceSyncAdapter, 0, len(backends))
	for _, backend := range backends {
		if len(def.Backends) > 0 && !backendApplies(def.Backends, backend) {
			continue
		}
		adapters = append(adapters, c.adapters[backend])
	}
	return adapters
}

func backendApplies(backends []agents.BackendID, backend agents.BackendID) bool {
	for _, candidate := range backends {
		if candidate == backend {
			return true
		}
	}
	return false
}

// Preview renders every resource associated with a project and reports what
// applying it would do, without touching the filesystem.
func (c *Coordinator) Preview(ctx context.Context, projectID uint) ([]SyncPreview, error) {
	definitions, err := c.source.ListProjectResources(projectID)
	if err != nil {
		return nil, err
	}
	previews := make([]SyncPreview, 0, len(definitions))
	for _, def := range definitions {
		for _, adapter := range c.adaptersFor(def) {
			previews = append(previews, c.previewOne(ctx, adapter, def, projectID))
		}
	}
	return previews, nil
}

// Apply renders and applies every resource associated with a project. force
// allows overwriting an unmanaged target or external drift after the user has
// resolved it; without force, conflicts are returned in the previews and the
// conflicting targets are left untouched.
func (c *Coordinator) Apply(ctx context.Context, projectID uint, force bool) ([]SyncPreview, error) {
	definitions, err := c.source.ListProjectResources(projectID)
	if err != nil {
		return nil, err
	}
	previews := make([]SyncPreview, 0, len(definitions))
	for _, def := range definitions {
		for _, adapter := range c.adaptersFor(def) {
			previews = append(previews, c.applyOne(ctx, adapter, def, projectID, force))
		}
	}
	return previews, nil
}

// SyncChangedResource re-renders one resource and updates the targets the
// platform already owns. A target that has never been applied is skipped
// (first apply needs consent), and a target that drifted pauses with
// ErrConflict rather than being overwritten.
func (c *Coordinator) SyncChangedResource(ctx context.Context, projectID, resourceID uint) error {
	def, err := c.source.Get(resourceID)
	if err != nil {
		return err
	}
	for _, adapter := range c.adaptersFor(def) {
		if !adapter.Supports(def.Kind) {
			continue
		}
		patch, err := adapter.Render(ctx, def, RenderContext{ProjectID: projectID, Secrets: c.secrets})
		if err != nil {
			return err
		}
		_, owned, err := c.manager.Manifest(patch.Target)
		if err != nil {
			return err
		}
		if !owned {
			continue
		}
		if _, err := c.manager.Apply(patch, false); err != nil {
			return err
		}
	}
	return nil
}

// ResolveConflict applies one resource with force, recording the user's
// decision to adopt or overwrite a conflicting target.
func (c *Coordinator) ResolveConflict(ctx context.Context, projectID, resourceID uint) ([]SyncPreview, error) {
	def, err := c.source.Get(resourceID)
	if err != nil {
		return nil, err
	}
	previews := make([]SyncPreview, 0, 1)
	for _, adapter := range c.adaptersFor(def) {
		previews = append(previews, c.applyOne(ctx, adapter, def, projectID, true))
	}
	return previews, nil
}

func (c *Coordinator) previewOne(ctx context.Context, adapter ResourceSyncAdapter, def resources.ResourceDefinition, projectID uint) SyncPreview {
	target := c.targetFor(adapter, def, projectID)
	if !adapter.Supports(def.Kind) {
		return SyncPreview{
			ResourceID:    def.ID,
			Kind:          def.Kind,
			Target:        target,
			State:         SyncStateUnsupported,
			ChangeSummary: fmt.Sprintf("backend %q does not support %s resources", adapter.BackendID(), def.Kind),
		}
	}
	patch, err := adapter.Render(ctx, def, RenderContext{ProjectID: projectID, Secrets: c.secrets})
	if err != nil {
		return failedPreview(def, target, err)
	}
	preview, err := c.manager.Preview(patch)
	if err != nil {
		return failedPreview(def, patch.Target, err)
	}
	return preview
}

func (c *Coordinator) applyOne(ctx context.Context, adapter ResourceSyncAdapter, def resources.ResourceDefinition, projectID uint, force bool) SyncPreview {
	target := c.targetFor(adapter, def, projectID)
	if !adapter.Supports(def.Kind) {
		return SyncPreview{
			ResourceID:    def.ID,
			Kind:          def.Kind,
			Target:        target,
			State:         SyncStateUnsupported,
			ChangeSummary: fmt.Sprintf("backend %q does not support %s resources", adapter.BackendID(), def.Kind),
		}
	}
	patch, err := adapter.Render(ctx, def, RenderContext{ProjectID: projectID, Secrets: c.secrets})
	if err != nil {
		return failedPreview(def, target, err)
	}
	preview, err := c.manager.Apply(patch, force)
	if err != nil {
		if errors.Is(err, ErrConflict) {
			return preview
		}
		return failedPreview(def, patch.Target, err)
	}
	return preview
}

// targetFor resolves a target for status reporting, falling back to a
// best-effort valid target when the adapter cannot name one.
func (c *Coordinator) targetFor(adapter ResourceSyncAdapter, def resources.ResourceDefinition, projectID uint) ResourceTarget {
	target, err := adapter.Target(projectID, def)
	if err == nil && target.ProjectID != 0 && target.Path != "" {
		return target
	}
	return ResourceTarget{ProjectID: projectID, BackendID: adapter.BackendID(), Path: def.Name}
}

// failedPreview reports a per-resource failure without leaking content or
// credentials into the summary.
func failedPreview(def resources.ResourceDefinition, target ResourceTarget, err error) SyncPreview {
	detail := agents.RedactSecrets(err.Error())
	if len(detail) > 300 {
		detail = detail[:300]
	}
	return SyncPreview{
		ResourceID:    def.ID,
		Kind:          def.Kind,
		Target:        target,
		State:         SyncStateFailed,
		ChangeSummary: detail,
	}
}
