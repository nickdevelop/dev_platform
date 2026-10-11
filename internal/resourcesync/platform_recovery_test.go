package resourcesync

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"devplatform/internal/agents"
	"devplatform/internal/resources"
)

// TestPlatformRecoveryCleansInterruptedWrite verifies that a temp file left by
// an interrupted write does not corrupt the next apply: the target keeps its
// last known-good content until the new write is fully installed, and the
// stale temp is removed.
func TestPlatformRecoveryCleansInterruptedWrite(t *testing.T) {
	dir, m := newManager(t)
	targetDir := filepath.Join(dir, "notes")

	if _, err := m.Apply(managedPatch("notes/lint.txt", "first", "create"), false); err != nil {
		t.Fatalf("first Apply: %v", err)
	}

	// Simulate a crash mid-write: a temp file was created but never renamed.
	stale := filepath.Join(targetDir, ".managed-interrupted.tmp")
	if err := os.WriteFile(stale, []byte("partial"), 0o644); err != nil {
		t.Fatalf("seed stale temp: %v", err)
	}

	if _, err := m.Apply(managedPatch("notes/lint.txt", "second", "update"), false); err != nil {
		t.Fatalf("second Apply: %v", err)
	}
	if got := readFile(t, filepath.Join(targetDir, "lint.txt")); got != "second" {
		t.Fatalf("target = %q, want the new content", got)
	}
	if _, err := os.Stat(stale); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale temp file was not cleaned up: %v", err)
	}
}

// TestPlatformRecoveryCaseAndEncoding verifies that targets are tracked
// case-sensitively (Linux/WSL semantics) and that Unicode resource names are
// supported.
func TestPlatformRecoveryCaseAndEncoding(t *testing.T) {
	adapter := &fakeAdapter{backend: agents.BackendPi, kinds: allKinds(), dir: ".pi/skills"}
	source := &fakeSource{defs: []resources.ResourceDefinition{
		testDefinition(1, resources.ResourceSkill, "Notes", "upper"),
		testDefinition(2, resources.ResourceSkill, "notes", "lower"),
		testDefinition(3, resources.ResourceSkill, "提交规范", "unicode"),
	}}
	c, dir := newCoordinator(t, source, adapter)
	ctx := context.Background()

	if _, err := c.Apply(ctx, 1, false); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	for path, want := range map[string]string{
		".pi/skills/Notes.txt": "upper",
		".pi/skills/notes.txt": "lower",
		".pi/skills/提交规范.txt":  "unicode",
	} {
		if got := readFile(t, filepath.Join(dir, filepath.FromSlash(path))); got != want {
			t.Fatalf("%s = %q, want %q", path, got, want)
		}
	}

	// The two case-differing targets are owned separately.
	upper, owned, err := c.manager.Manifest(ResourceTarget{ProjectID: 1, BackendID: agents.BackendPi, Path: ".pi/skills/Notes.txt"})
	if err != nil || !owned {
		t.Fatalf("upper manifest: owned=%v err=%v", owned, err)
	}
	lower, owned, err := c.manager.Manifest(ResourceTarget{ProjectID: 1, BackendID: agents.BackendPi, Path: ".pi/skills/notes.txt"})
	if err != nil || !owned {
		t.Fatalf("lower manifest: owned=%v err=%v", owned, err)
	}
	if upper.ManagedHash == lower.ManagedHash {
		t.Fatal("case-differing targets share a manifest hash")
	}
}

// TestPlatformRecoverySecretRotation verifies that rotating a credential and
// syncing a changed resource updates an already-owned target.
func TestPlatformRecoverySecretRotation(t *testing.T) {
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
	secrets := &fakeSecrets{values: map[string]string{"sec_1": "v1"}}
	c.secrets = secrets
	ctx := context.Background()

	if _, err := c.Apply(ctx, 1, false); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	target := filepath.Join(dir, "notes", "server.txt")
	if got := readFile(t, target); got != "credential=v1" {
		t.Fatalf("target = %q, want v1", got)
	}

	// Rotate the stored value; the owned target updates on the next sync.
	secrets.values["sec_1"] = "v2"
	if err := c.SyncChangedResource(ctx, 1, 1); err != nil {
		t.Fatalf("SyncChangedResource: %v", err)
	}
	if got := readFile(t, target); got != "credential=v2" {
		t.Fatalf("target after rotation = %q, want v2", got)
	}
}

// TestPlatformRecoveryMultiTargetPartialFailure verifies that one unwritable
// target does not prevent another target of the same resource from syncing,
// and that each result is reported.
func TestPlatformRecoveryMultiTargetPartialFailure(t *testing.T) {
	oc := &fakeAdapter{backend: agents.BackendOpenCodeV2, kinds: allKinds(), dir: ".opencode/skills"}
	pi := &fakeAdapter{backend: agents.BackendPi, kinds: allKinds(), dir: ".pi/skills"}
	source := &fakeSource{defs: []resources.ResourceDefinition{testDefinition(1, resources.ResourceSkill, "lint", "body")}}
	c, dir := newCoordinator(t, source, oc, pi)

	// Make the pi target unwritable: a directory sits where the file goes.
	if err := os.MkdirAll(filepath.Join(dir, ".pi", "skills", "lint.txt"), 0o755); err != nil {
		t.Fatalf("mkdir blocking target: %v", err)
	}

	previews, err := c.Preview(context.Background(), 1)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	states := map[agents.BackendID]SyncState{}
	for _, preview := range previews {
		states[preview.Target.BackendID] = preview.State
	}
	if states[agents.BackendOpenCodeV2] != SyncStatePending {
		t.Fatalf("opencode state = %q, want pending", states[agents.BackendOpenCodeV2])
	}
	if states[agents.BackendPi] != SyncStateConflict {
		t.Fatalf("pi state = %q, want conflict", states[agents.BackendPi])
	}

	applied, err := c.Apply(context.Background(), 1, false)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	appliedStates := map[agents.BackendID]SyncState{}
	for _, preview := range applied {
		appliedStates[preview.Target.BackendID] = preview.State
	}
	if appliedStates[agents.BackendOpenCodeV2] != SyncStateApplied {
		t.Fatalf("opencode state after apply = %q, want applied", appliedStates[agents.BackendOpenCodeV2])
	}
	if appliedStates[agents.BackendPi] != SyncStateConflict {
		t.Fatalf("pi state after apply = %q, want conflict", appliedStates[agents.BackendPi])
	}
	if got := readFile(t, filepath.Join(dir, ".opencode", "skills", "lint.txt")); got != "body" {
		t.Fatalf("opencode target = %q, want the resource content", got)
	}
}

// TestPlatformRecoverySecretNotInDiagnostics verifies that a secret-store
// failure is reported by reference only, never by value.
func TestPlatformRecoverySecretNotInDiagnostics(t *testing.T) {
	adapter := &fakeAdapter{
		backend: agents.BackendPi,
		kinds:   allKinds(),
		dir:     "notes",
		render: func(def resources.ResourceDefinition, rc RenderContext) (string, error) {
			_, err := rc.Secrets.Get(context.Background(), def.SecretRef)
			if err != nil {
				return "", fmt.Errorf("resolve secret %s: %w", def.SecretRef, err)
			}
			return "", nil
		},
	}
	def := testDefinition(1, resources.ResourceMCP, "server", "")
	def.SecretRef = "sec_1"
	source := &fakeSource{defs: []resources.ResourceDefinition{def}}
	c, _ := newCoordinator(t, source, adapter)
	c.secrets = &fakeSecrets{err: errors.New("store unavailable")}

	previews, err := c.Preview(context.Background(), 1)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	if previews[0].State != SyncStateFailed {
		t.Fatalf("state = %q, want failed", previews[0].State)
	}
	if !strings.Contains(previews[0].ChangeSummary, "sec_1") {
		t.Fatalf("summary does not name the reference: %s", previews[0].ChangeSummary)
	}
}
