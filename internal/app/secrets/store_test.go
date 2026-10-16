package secrets

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"devplatform/internal/core/platform"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	return store
}

func TestStoreSetGetDelete(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	if _, err := store.Get(ctx, "sec_missing"); !errors.Is(err, ErrSecretNotFound) {
		t.Fatalf("Get missing = %v, want ErrSecretNotFound", err)
	}

	if err := store.Set(ctx, "sec_1", "value-one"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := store.Get(ctx, "sec_1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != "value-one" {
		t.Fatalf("Get = %q, want %q", got, "value-one")
	}

	// Rotation overwrites the value for the same reference.
	if err := store.Set(ctx, "sec_1", "value-two"); err != nil {
		t.Fatalf("rotate: %v", err)
	}
	got, err = store.Get(ctx, "sec_1")
	if err != nil {
		t.Fatalf("Get after rotate: %v", err)
	}
	if got != "value-two" {
		t.Fatalf("Get after rotate = %q, want %q", got, "value-two")
	}

	// A second reference is independent.
	if err := store.Set(ctx, "sec_2", "other"); err != nil {
		t.Fatalf("Set second: %v", err)
	}

	if err := store.Delete(ctx, "sec_1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := store.Get(ctx, "sec_1"); !errors.Is(err, ErrSecretNotFound) {
		t.Fatalf("Get after delete = %v, want ErrSecretNotFound", err)
	}
	if got, err := store.Get(ctx, "sec_2"); err != nil || got != "other" {
		t.Fatalf("second reference affected by delete: %q, %v", got, err)
	}

	if err := store.Delete(ctx, "sec_missing"); !errors.Is(err, ErrSecretNotFound) {
		t.Fatalf("Delete missing = %v, want ErrSecretNotFound", err)
	}

	exists, err := store.Exists(ctx, "sec_2")
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}
	if !exists {
		t.Fatal("Exists(sec_2) = false, want true")
	}
}

func TestStoreRejectsInvalidID(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	for _, id := range []string{"", " ", "../escape", "a/b", `a\b`, "a\x00b", strings.Repeat("x", 129)} {
		if err := store.Set(ctx, id, "value"); err == nil {
			t.Errorf("Set(%q) = nil, want error", id)
		}
		if _, err := store.Get(ctx, id); err == nil {
			t.Errorf("Get(%q) = nil error, want error", id)
		}
	}
}

func TestStoreFilePermissions(t *testing.T) {
	// Skip by capability, not by GOOS: the assertions only run where the
	// platform enforces private permissions.
	if !platform.New().Perms().Enforced() {
		t.Skip("private permissions are not enforced on this platform")
	}
	store := newTestStore(t)
	if err := store.Set(context.Background(), "sec_1", "value"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	info, err := os.Stat(store.Path())
	if err != nil {
		t.Fatalf("stat secret file: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("secret file mode = %o, want 600", perm)
	}

	// Rotating must not widen the permissions.
	if err := store.Set(context.Background(), "sec_1", "value-two"); err != nil {
		t.Fatalf("rotate: %v", err)
	}
	info, err = os.Stat(store.Path())
	if err != nil {
		t.Fatalf("stat after rotate: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("secret file mode after rotate = %o, want 600", perm)
	}
}

func TestStoreRejectsSymlinkedFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on Windows")
	}
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	target := filepath.Join(dir, "attacker-target.json")
	if err := os.WriteFile(target, []byte(`{"sec_1":"stolen"}`), 0o600); err != nil {
		t.Fatalf("write target: %v", err)
	}
	if err := os.Symlink(target, store.Path()); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	// Reading through the symlink would read the attacker's file; it must be
	// refused instead.
	if _, err := store.Get(context.Background(), "sec_1"); err == nil {
		t.Fatal("Get through symlink = nil error, want refusal")
	}

	// Writing must refuse too, rather than replace or follow the symlink, so a
	// planted link can neither redirect the write nor be silently destroyed.
	if err := store.Set(context.Background(), "sec_1", "ours"); err == nil {
		t.Fatal("Set over symlink = nil error, want refusal")
	}
	if info, err := os.Lstat(store.Path()); err != nil {
		t.Fatalf("lstat: %v", err)
	} else if info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("secret path is no longer a symlink after Set")
	}
	targetData, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if string(targetData) != `{"sec_1":"stolen"}` {
		t.Fatalf("secret operations modified the symlink target: %s", targetData)
	}
}

func TestStoreRejectsEmptyValue(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	if err := store.Set(ctx, "sec_1", ""); !errors.Is(err, ErrEmptySecretValue) {
		t.Fatalf("Set with an empty value = %v, want ErrEmptySecretValue", err)
	}
	if _, err := store.Get(ctx, "sec_1"); !errors.Is(err, ErrSecretNotFound) {
		t.Fatalf("empty Set stored a value: %v", err)
	}

	// Rotation to an empty value is refused too.
	if err := store.Set(ctx, "sec_1", "value"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := store.Set(ctx, "sec_1", ""); !errors.Is(err, ErrEmptySecretValue) {
		t.Fatalf("rotate to empty = %v, want ErrEmptySecretValue", err)
	}
	if got, err := store.Get(ctx, "sec_1"); err != nil || got != "value" {
		t.Fatalf("value after refused rotation = %q, %v", got, err)
	}
}

func TestStoreErrorsNeverContainValue(t *testing.T) {
	store := newTestStore(t)
	const value = "super-secret-value"

	// An invalid id fails before anything is written; the error must not echo
	// the value.
	err := store.Set(context.Background(), "bad/id", value)
	if err == nil {
		t.Fatal("Set with invalid id = nil, want error")
	}
	if strings.Contains(err.Error(), value) {
		t.Fatalf("error leaked the secret value: %v", err)
	}
}

func TestRedact(t *testing.T) {
	text := "connecting with token s3cr3t-value and key other-value"
	got := Redact(text, "s3cr3t-value", "other-value")
	if strings.Contains(got, "s3cr3t-value") || strings.Contains(got, "other-value") {
		t.Fatalf("Redact left a secret in place: %q", got)
	}
	if !strings.Contains(got, "[REDACTED]") {
		t.Fatalf("Redact = %q, want a [REDACTED] marker", got)
	}

	// Empty values must not turn every position into a marker.
	if got := Redact("plain text", "", "  "); got != "plain text" {
		t.Fatalf("Redact with empty values = %q, want unchanged", got)
	}
}
