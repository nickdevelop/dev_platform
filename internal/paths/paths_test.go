package paths

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"devplatform/internal/platform/policy"
)

// canonicalTempDir returns the symlink-resolved path of a fresh temp dir, so
// tests compare against what EvalSymlinks reports even under a symlinked /tmp.
func canonicalTempDir(t *testing.T) string {
	t.Helper()

	canonical, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}
	return canonical
}

func TestResolveAcceptsAbsolutePaths(t *testing.T) {
	root := canonicalTempDir(t)
	sub := filepath.Join(root, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("mkdir sub: %v", err)
	}

	if got, err := Resolve(root); err != nil || got != root {
		t.Errorf("Resolve(root) = %q, %v; want %q", got, err, root)
	}
	// Trailing separators and interior dot segments are normalized away.
	if got, err := Resolve(sub + "/"); err != nil || got != sub {
		t.Errorf("Resolve(trailing slash) = %q, %v; want %q", got, err, sub)
	}
	if got, err := Resolve(filepath.Join(root, "sub", "..", "sub")); err != nil || got != sub {
		t.Errorf("Resolve(dot-dot) = %q, %v; want %q", got, err, sub)
	}
}

func TestResolveResolvesSymlinks(t *testing.T) {
	root := canonicalTempDir(t)
	real := filepath.Join(root, "real")
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatalf("mkdir real: %v", err)
	}
	alias := filepath.Join(root, "alias")
	if err := os.Symlink(real, alias); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	if got, err := Resolve(alias); err != nil || got != real {
		t.Errorf("Resolve(alias) = %q, %v; want canonical %q", got, err, real)
	}
}

func TestResolveRejectsAmbiguousInputs(t *testing.T) {
	for _, input := range []string{
		"",
		"relative/path",
		`C:relative`,
		`C:`,
		`\\server\share\proj`,
		`/tmp/mixed\separator`,
		"/tmp/with\x00nul",
	} {
		if _, err := Resolve(input); !errors.Is(err, ErrInvalidPath) {
			t.Errorf("Resolve(%q) error = %v, want ErrInvalidPath", input, err)
		}
	}
}

func TestResolveHandlesNonexistentDescendants(t *testing.T) {
	root := canonicalTempDir(t)

	// A not-yet-existing target: the nearest existing ancestor is canonical
	// and the full path is re-attached lexically.
	planned := filepath.Join(root, "new-project", "deep", "child")
	if got, err := Resolve(planned); err != nil || got != planned {
		t.Errorf("Resolve(nonexistent) = %q, %v; want %q", got, err, planned)
	}

	// A path leading through an existing regular file can never come into
	// existence and is rejected as ambiguous.
	file := filepath.Join(root, "file.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if _, err := Resolve(filepath.Join(file, "sub")); !errors.Is(err, ErrInvalidPath) {
		t.Errorf("Resolve(through file) error = %v, want ErrInvalidPath", err)
	}
}

func TestResolveRejectsDanglingSymlink(t *testing.T) {
	root := canonicalTempDir(t)
	dangling := filepath.Join(root, "dangling")
	if err := os.Symlink(filepath.Join(root, "gone"), dangling); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	if _, err := Resolve(dangling); !errors.Is(err, ErrInvalidPath) {
		t.Errorf("Resolve(dangling) error = %v, want ErrInvalidPath", err)
	}
}

func TestResolveNormalizesWSLDrivePaths(t *testing.T) {
	if got, err := Resolve(`C:\work\project`); err != nil || got != "/mnt/c/work/project" {
		t.Errorf("Resolve(C:\\work\\project) = %q, %v; want /mnt/c/work/project", got, err)
	}
}

// TestErrInvalidPathSharedWithPolicy pins the load-bearing alias: the unix
// path policy wraps the policy value, so errors from Resolve must match both.
func TestErrInvalidPathSharedWithPolicy(t *testing.T) {
	if ErrInvalidPath != policy.ErrInvalidPath {
		t.Fatal("paths.ErrInvalidPath is not the shared policy value")
	}
}
