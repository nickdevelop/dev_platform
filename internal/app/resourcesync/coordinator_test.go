package resourcesync

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"devplatform/internal/core/agents"
	"devplatform/internal/app/resources"
)

// fakeAdapter is a scriptable ResourceSyncAdapter over a fixed directory.
type fakeAdapter struct {
	backend agents.BackendID
	kinds   map[resources.ResourceKind]bool
	dir     string
	render  func(def resources.ResourceDefinition, rc RenderContext) (string, error)
}

func (a *fakeAdapter) BackendID() agents.BackendID { return a.backend }

func (a *fakeAdapter) Supports(kind resources.ResourceKind) bool { return a.kinds[kind] }

func (a *fakeAdapter) Target(projectID uint, def resources.ResourceDefinition) (ResourceTarget, error) {
	return ResourceTarget{ProjectID: projectID, BackendID: a.backend, Path: a.dir + "/" + def.Name + ".txt"}, nil
}

func (a *fakeAdapter) Render(_ context.Context, def resources.ResourceDefinition, rc RenderContext) (ManagedPatch, error) {
	target, err := a.Target(rc.ProjectID, def)
	if err != nil {
		return ManagedPatch{}, err
	}
	content := def.Content
	if a.render != nil {
		content, err = a.render(def, rc)
		if err != nil {
			return ManagedPatch{}, err
		}
	}
	return ManagedPatch{
		ResourceID:      def.ID,
		Kind:            def.Kind,
		Target:          target,
		Content:         []byte(content),
		Summary:         "sync " + def.Name,
		ResourceVersion: "v1",
	}, nil
}

func allKinds() map[resources.ResourceKind]bool {
	return map[resources.ResourceKind]bool{
		resources.ResourcePrompt: true,
		resources.ResourceSkill:  true,
		resources.ResourceMCP:    true,
	}
}

// fakeSource is a fixed resource collection.
type fakeSource struct {
	defs []resources.ResourceDefinition
}

func (f *fakeSource) ListProjectResources(uint) ([]resources.ResourceDefinition, error) {
	return f.defs, nil
}

func (f *fakeSource) Get(id uint) (resources.ResourceDefinition, error) {
	for _, def := range f.defs {
		if def.ID == id {
			return def, nil
		}
	}
	return resources.ResourceDefinition{}, fmt.Errorf("%w: %d", resources.ErrResourceNotFound, id)
}

func testDefinition(id uint, kind resources.ResourceKind, name, content string) resources.ResourceDefinition {
	now := time.Date(2026, 10, 11, 10, 0, 0, 0, time.UTC)
	return resources.ResourceDefinition{
		ID:        id,
		Kind:      kind,
		Name:      name,
		Content:   content,
		Enabled:   true,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func newCoordinator(t *testing.T, source ResourceSource, adapters ...ResourceSyncAdapter) (*Coordinator, string) {
	t.Helper()
	db := openTestDB(t)
	dir := t.TempDir()
	m := NewManager(db, fakeResolver{dir: dir}, t.TempDir())
	return NewCoordinator(m, source, nil, adapters...), dir
}

func TestCoordinatorFormatTranslation(t *testing.T) {
	oc := &fakeAdapter{backend: agents.BackendOpenCodeV2, kinds: allKinds(), dir: ".opencode/skills"}
	pi := &fakeAdapter{backend: agents.BackendPi, kinds: allKinds(), dir: ".pi/skills"}
	source := &fakeSource{defs: []resources.ResourceDefinition{testDefinition(1, resources.ResourceSkill, "lint", "body")}}
	c, _ := newCoordinator(t, source, oc, pi)

	previews, err := c.Preview(context.Background(), 1)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	if len(previews) != 2 {
		t.Fatalf("Preview returned %d previews, want 2 (one per backend)", len(previews))
	}
	if previews[0].Target.Path == previews[1].Target.Path {
		t.Fatalf("both adapters produced the same target path %q", previews[0].Target.Path)
	}
	for _, preview := range previews {
		if preview.State != SyncStatePending {
			t.Errorf("preview for %s = %q, want %q", preview.Target.Path, preview.State, SyncStatePending)
		}
		if err := preview.Validate(); err != nil {
			t.Errorf("preview invalid: %v", err)
		}
	}
}

func TestCoordinatorUnsupportedDeclaration(t *testing.T) {
	pi := &fakeAdapter{backend: agents.BackendPi, kinds: map[resources.ResourceKind]bool{resources.ResourcePrompt: true}, dir: ".pi/skills"}
	def := testDefinition(1, resources.ResourceSkill, "lint", "body")
	def.Backends = []agents.BackendID{agents.BackendPi}
	c, _ := newCoordinator(t, &fakeSource{defs: []resources.ResourceDefinition{def}}, pi)

	if c.Supports(resources.ResourceSkill, agents.BackendPi) {
		t.Fatal("Supports(skill, pi) = true, want false")
	}
	previews, err := c.Preview(context.Background(), 1)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	if len(previews) != 1 || previews[0].State != SyncStateUnsupported {
		t.Fatalf("Preview = %+v, want one unsupported preview", previews)
	}
}

func TestCoordinatorFirstApplyConsent(t *testing.T) {
	adapter := &fakeAdapter{backend: agents.BackendPi, kinds: allKinds(), dir: "notes"}
	source := &fakeSource{defs: []resources.ResourceDefinition{testDefinition(1, resources.ResourceSkill, "lint", "body")}}
	c, dir := newCoordinator(t, source, adapter)

	target := filepath.Join(dir, "notes", "lint.txt")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(target, []byte("user content"), 0o644); err != nil {
		t.Fatalf("seed target: %v", err)
	}

	previews, err := c.Preview(context.Background(), 1)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	if len(previews) != 1 || previews[0].State != SyncStateConflict || previews[0].Conflict.Kind != ConflictUnmanagedTarget {
		t.Fatalf("Preview = %+v, want an unmanaged-target conflict", previews)
	}

	applied, err := c.Apply(context.Background(), 1, false)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if applied[0].State != SyncStateConflict {
		t.Fatalf("Apply without consent = %+v, want the conflict to stand", applied[0])
	}
	data, _ := os.ReadFile(target)
	if string(data) != "user content" {
		t.Fatalf("Apply overwrote the unmanaged target: %q", data)
	}
}

func TestCoordinatorPostConsentAutoUpdate(t *testing.T) {
	adapter := &fakeAdapter{backend: agents.BackendPi, kinds: allKinds(), dir: "notes"}
	source := &fakeSource{defs: []resources.ResourceDefinition{testDefinition(1, resources.ResourceSkill, "lint", "first")}}
	c, dir := newCoordinator(t, source, adapter)
	ctx := context.Background()

	applied, err := c.Apply(ctx, 1, false)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if applied[0].State != SyncStateApplied {
		t.Fatalf("Apply = %+v, want applied", applied[0])
	}
	target := filepath.Join(dir, "notes", "lint.txt")
	if data, _ := os.ReadFile(target); string(data) != "first" {
		t.Fatalf("target = %q, want %q", data, "first")
	}

	// The definition changes; the already-consented target updates silently.
	source.defs[0].Content = "second"
	if err := c.SyncChangedResource(ctx, 1, 1); err != nil {
		t.Fatalf("SyncChangedResource: %v", err)
	}
	if data, _ := os.ReadFile(target); string(data) != "second" {
		t.Fatalf("target after sync = %q, want %q", data, "second")
	}
}

func TestCoordinatorExternalConflictPauses(t *testing.T) {
	adapter := &fakeAdapter{backend: agents.BackendPi, kinds: allKinds(), dir: "notes"}
	source := &fakeSource{defs: []resources.ResourceDefinition{testDefinition(1, resources.ResourceSkill, "lint", "first")}}
	c, dir := newCoordinator(t, source, adapter)
	ctx := context.Background()

	if _, err := c.Apply(ctx, 1, false); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	target := filepath.Join(dir, "notes", "lint.txt")
	if err := os.WriteFile(target, []byte("external edit"), 0o644); err != nil {
		t.Fatalf("external edit: %v", err)
	}

	source.defs[0].Content = "second"
	if err := c.SyncChangedResource(ctx, 1, 1); !errors.Is(err, ErrConflict) {
		t.Fatalf("SyncChangedResource over drift = %v, want ErrConflict", err)
	}
	if data, _ := os.ReadFile(target); string(data) != "external edit" {
		t.Fatalf("drift was overwritten: %q", data)
	}
}

func TestCoordinatorPartialFailure(t *testing.T) {
	adapter := &fakeAdapter{
		backend: agents.BackendPi,
		kinds:   allKinds(),
		dir:     "notes",
		render: func(def resources.ResourceDefinition, _ RenderContext) (string, error) {
			if def.Name == "bad" {
				return "", errors.New("render failed: unsupported content")
			}
			return def.Content, nil
		},
	}
	source := &fakeSource{defs: []resources.ResourceDefinition{
		testDefinition(1, resources.ResourceSkill, "good", "ok"),
		testDefinition(2, resources.ResourceSkill, "bad", "boom"),
	}}
	c, dir := newCoordinator(t, source, adapter)
	ctx := context.Background()

	previews, err := c.Preview(ctx, 1)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	if len(previews) != 2 {
		t.Fatalf("Preview returned %d previews, want 2", len(previews))
	}
	states := map[uint]SyncState{}
	for _, preview := range previews {
		states[preview.ResourceID] = preview.State
	}
	if states[1] != SyncStatePending || states[2] != SyncStateFailed {
		t.Fatalf("states = %+v, want resource 1 pending and resource 2 failed", states)
	}

	applied, err := c.Apply(ctx, 1, false)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(applied) != 2 {
		t.Fatalf("Apply returned %d results, want 2", len(applied))
	}
	// The good resource is written even though its sibling failed.
	if _, err := os.Stat(filepath.Join(dir, "notes", "good.txt")); err != nil {
		t.Fatalf("good resource not applied: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "notes", "bad.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed resource left a file behind: %v", err)
	}
}

// fakeSecrets is a scriptable SecretResolver.
type fakeSecrets struct {
	values map[string]string
	err    error
}

func (f *fakeSecrets) Get(_ context.Context, id string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	value, ok := f.values[id]
	if !ok {
		return "", fmt.Errorf("secrets: secret not found: %s", id)
	}
	return value, nil
}

func TestCoordinatorSecretResolutionWithoutLeak(t *testing.T) {
	adapter := &fakeAdapter{
		backend: agents.BackendPi,
		kinds:   allKinds(),
		dir:     "notes",
		render: func(def resources.ResourceDefinition, rc RenderContext) (string, error) {
			value, err := rc.Secrets.Get(context.Background(), def.SecretRef)
			if err != nil {
				return "", err
			}
			return "credential=" + value, nil
		},
	}
	def := testDefinition(1, resources.ResourceMCP, "server", "")
	def.SecretRef = "sec_1"
	source := &fakeSource{defs: []resources.ResourceDefinition{def}}
	c, dir := newCoordinator(t, source, adapter)
	c.secrets = &fakeSecrets{values: map[string]string{"sec_1": "topsecret"}}
	ctx := context.Background()

	previews, err := c.Preview(ctx, 1)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	if strings.Contains(previews[0].ChangeSummary, "topsecret") {
		t.Fatalf("preview summary leaked the secret: %s", previews[0].ChangeSummary)
	}

	if _, err := c.Apply(ctx, 1, false); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "notes", "server.txt"))
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if string(data) != "credential=topsecret" {
		t.Fatalf("target = %q, want the resolved credential", data)
	}

	// A resolver failure must not leak the value into the reported summary.
	c.secrets = &fakeSecrets{err: errors.New("store unavailable")}
	previews, err = c.Preview(ctx, 1)
	if err != nil {
		t.Fatalf("Preview with failing store: %v", err)
	}
	if previews[0].State != SyncStateFailed {
		t.Fatalf("preview = %+v, want failed", previews[0])
	}
	if strings.Contains(previews[0].ChangeSummary, "topsecret") {
		t.Fatalf("failure summary leaked the secret: %s", previews[0].ChangeSummary)
	}
}
