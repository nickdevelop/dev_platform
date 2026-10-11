package resourcesync

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"devplatform/internal/agents"
	"devplatform/internal/database"
	"devplatform/internal/resources"
)

// openTestDB opens and migrates a throwaway platform database.
func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := database.Open(t.TempDir())
	if err != nil {
		t.Fatalf("database.Open: %v", err)
	}
	if err := database.Migrate(db); err != nil {
		t.Fatalf("database.Migrate: %v", err)
	}
	db.Logger = logger.Discard
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

// fakeResolver returns a fixed project directory, standing in for the project
// service's authorized-path resolution.
type fakeResolver struct {
	dir string
	err error
}

func (f fakeResolver) ResolveDirectory(uint) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return f.dir, nil
}

func newManager(t *testing.T) (string, *Manager) {
	t.Helper()
	db := openTestDB(t)
	dir := t.TempDir()
	m := NewManager(db, fakeResolver{dir: dir}, t.TempDir())
	return dir, m
}

func managedTarget(path string) ResourceTarget {
	return ResourceTarget{ProjectID: 1, BackendID: agents.BackendPi, Path: path}
}

func managedPatch(path, content, summary string) ManagedPatch {
	return ManagedPatch{
		ResourceID:      5,
		Kind:            resources.ResourcePrompt,
		Target:          managedTarget(path),
		Content:         []byte(content),
		Summary:         summary,
		ResourceVersion: "v1",
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func TestManagedFilePreviewDoesNotMutate(t *testing.T) {
	dir, m := newManager(t)
	patch := managedPatch("managed/notes.md", "hello", "create managed file")

	preview, err := m.Preview(patch)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	if preview.State != SyncStatePending {
		t.Fatalf("Preview state = %q, want %q", preview.State, SyncStatePending)
	}
	if preview.Conflict != nil {
		t.Fatalf("Preview conflict = %+v, want nil", preview.Conflict)
	}
	if strings.TrimSpace(preview.ChangeSummary) == "" {
		t.Fatal("Preview returned an empty change summary")
	}
	if err := preview.Validate(); err != nil {
		t.Fatalf("Preview returned an invalid preview: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "managed", "notes.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Preview mutated the filesystem: %v", err)
	}
}

func TestManagedFileFirstApplyUnmanagedCollision(t *testing.T) {
	dir, m := newManager(t)
	target := filepath.Join(dir, "notes.md")
	if err := os.WriteFile(target, []byte("user content"), 0o644); err != nil {
		t.Fatalf("seed user file: %v", err)
	}
	patch := managedPatch("notes.md", "platform content", "write managed content")

	preview, err := m.Preview(patch)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	if preview.State != SyncStateConflict || preview.Conflict == nil || preview.Conflict.Kind != ConflictUnmanagedTarget {
		t.Fatalf("Preview = %+v, want an unmanaged-target conflict", preview)
	}

	if _, err := m.Apply(patch, false); !errors.Is(err, ErrConflict) {
		t.Fatalf("Apply over unmanaged target = %v, want ErrConflict", err)
	}
	if got := readFile(t, target); got != "user content" {
		t.Fatalf("Apply modified an unmanaged file: %q", got)
	}
	if _, owned, err := m.Manifest(patch.Target); err != nil || owned {
		t.Fatalf("ownership recorded despite refusal: owned=%v err=%v", owned, err)
	}
}

func TestManagedFileManagedUpdateAndBackup(t *testing.T) {
	dir, m := newManager(t)
	target := filepath.Join(dir, "notes.md")

	if _, err := m.Apply(managedPatch("notes.md", "first", "create"), false); err != nil {
		t.Fatalf("first Apply: %v", err)
	}
	if got := readFile(t, target); got != "first" {
		t.Fatalf("after first Apply = %q, want %q", got, "first")
	}
	if _, owned, err := m.Manifest(managedTarget("notes.md")); err != nil || !owned {
		t.Fatalf("ownership not recorded after apply: owned=%v err=%v", owned, err)
	}

	patch := managedPatch("notes.md", "second", "update")
	preview, err := m.Preview(patch)
	if err != nil {
		t.Fatalf("Preview update: %v", err)
	}
	if preview.State != SyncStateApplied || preview.Conflict != nil {
		t.Fatalf("Preview update = %+v, want a clean applied preview", preview)
	}

	if _, err := m.Apply(patch, false); err != nil {
		t.Fatalf("second Apply: %v", err)
	}
	if got := readFile(t, target); got != "second" {
		t.Fatalf("after second Apply = %q, want %q", got, "second")
	}

	manifest, owned, err := m.Manifest(managedTarget("notes.md"))
	if err != nil || !owned {
		t.Fatalf("Manifest after update: owned=%v err=%v", owned, err)
	}
	if manifest.ManagedHash != hashContent([]byte("second")) {
		t.Fatalf("managed hash = %q, want the new content hash", manifest.ManagedHash)
	}
	if manifest.ResourceVersion != "v1" {
		t.Fatalf("resource version = %q, want v1", manifest.ResourceVersion)
	}
	if manifest.BackupPath == "" {
		t.Fatal("update did not record a backup reference")
	}
	if got := readFile(t, manifest.BackupPath); got != "first" {
		t.Fatalf("backup content = %q, want the previous content", got)
	}
}

func TestManagedFileExternalDrift(t *testing.T) {
	dir, m := newManager(t)
	target := filepath.Join(dir, "notes.md")

	if _, err := m.Apply(managedPatch("notes.md", "first", "create"), false); err != nil {
		t.Fatalf("first Apply: %v", err)
	}
	if err := os.WriteFile(target, []byte("external edit"), 0o644); err != nil {
		t.Fatalf("external edit: %v", err)
	}

	patch := managedPatch("notes.md", "second", "update")
	preview, err := m.Preview(patch)
	if err != nil {
		t.Fatalf("Preview drift: %v", err)
	}
	if preview.State != SyncStateConflict || preview.Conflict == nil || preview.Conflict.Kind != ConflictExternalDrift {
		t.Fatalf("Preview drift = %+v, want an external-drift conflict", preview)
	}
	if _, err := m.Apply(patch, false); !errors.Is(err, ErrConflict) {
		t.Fatalf("Apply over drift = %v, want ErrConflict", err)
	}
	if got := readFile(t, target); got != "external edit" {
		t.Fatalf("Apply overwrote external drift: %q", got)
	}
}

func TestManagedFileTargetTraversal(t *testing.T) {
	dir, m := newManager(t)
	patch := managedPatch("../escape.md", "x", "escape")

	if _, err := m.Preview(patch); !errors.Is(err, ErrInvalidTarget) {
		t.Fatalf("Preview traversal = %v, want ErrInvalidTarget", err)
	}
	if _, err := m.Apply(patch, false); !errors.Is(err, ErrInvalidTarget) {
		t.Fatalf("Apply traversal = %v, want ErrInvalidTarget", err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(dir), "escape.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("traversal wrote outside the project: %v", err)
	}
}

func TestManagedFileTargetInvalid(t *testing.T) {
	dir, m := newManager(t)
	if err := os.MkdirAll(filepath.Join(dir, "adir"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	patch := managedPatch("adir", "x", "write to a directory")

	preview, err := m.Preview(patch)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	if preview.State != SyncStateConflict || preview.Conflict == nil || preview.Conflict.Kind != ConflictTargetInvalid {
		t.Fatalf("Preview = %+v, want a target-invalid conflict", preview)
	}
	if _, err := m.Apply(patch, false); !errors.Is(err, ErrConflict) {
		t.Fatalf("Apply to a directory = %v, want ErrConflict", err)
	}
}

func TestManagedFileRollbackAfterWriteFailure(t *testing.T) {
	dir, m := newManager(t)
	target := filepath.Join(dir, "notes.md")

	if _, err := m.Apply(managedPatch("notes.md", "first", "create"), false); err != nil {
		t.Fatalf("first Apply: %v", err)
	}

	// Simulate a write that reports failure after touching the file; Apply
	// must restore the backup so the last known-good content survives.
	m.writeFile = func(path string, data []byte, mode os.FileMode) error {
		if err := os.WriteFile(path, data, mode); err != nil {
			return err
		}
		return errors.New("simulated write failure")
	}

	if _, err := m.Apply(managedPatch("notes.md", "second", "update"), false); err == nil {
		t.Fatal("Apply with a failing writer = nil, want error")
	}
	if got := readFile(t, target); got != "first" {
		t.Fatalf("rollback left %q, want the previous content", got)
	}
	if _, owned, err := m.Manifest(managedTarget("notes.md")); err != nil || !owned {
		t.Fatalf("rollback removed ownership: owned=%v err=%v", owned, err)
	}
}

func TestManagedFileApplyRefusesUnknownProject(t *testing.T) {
	db := openTestDB(t)
	m := NewManager(db, fakeResolver{err: errors.New("projects: project not found")}, t.TempDir())

	if _, err := m.Preview(managedPatch("notes.md", "x", "create")); err == nil {
		t.Fatal("Preview with an unresolvable project = nil, want error")
	}
}
