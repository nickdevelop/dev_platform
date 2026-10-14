//go:build linux

package unix_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"devplatform/internal/platform"
)

// TestTrashMovesDirectoryIntoUserTrash verifies the production trash mover
// against the freedesktop.org home-trash layout, using an isolated
// XDG_DATA_HOME so the test never touches the real user trash.
func TestTrashMovesDirectoryIntoUserTrash(t *testing.T) {
	base := t.TempDir()
	t.Setenv("XDG_DATA_HOME", base)

	src := filepath.Join(t.TempDir(), "myproj")
	if err := os.MkdirAll(filepath.Join(src, "sub"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(src, "sub", "f.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if err := platform.New().Trash().Move(src); err != nil {
		t.Fatalf("Move: %v", err)
	}

	if _, err := os.Lstat(src); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("source still exists after Move: %v", err)
	}
	payload := filepath.Join(base, "Trash", "files", "myproj", "sub", "f.txt")
	if _, err := os.Stat(payload); err != nil {
		t.Errorf("trashed payload missing at %s: %v", payload, err)
	}
	info, err := os.ReadFile(filepath.Join(base, "Trash", "info", "myproj.trashinfo"))
	if err != nil {
		t.Fatalf("read trash info: %v", err)
	}
	if !strings.HasPrefix(string(info), "[Trash Info]") {
		t.Errorf("trash info = %q, want [Trash Info] header", info)
	}
	if !strings.Contains(string(info), "Path="+src) {
		t.Errorf("trash info = %q, want Path=%s", info, src)
	}
	if !strings.Contains(string(info), "DeletionDate=") {
		t.Errorf("trash info = %q, want a DeletionDate", info)
	}
}

// TestTrashEscapesSpacesAndAvoidsCollisions verifies the Path escaping
// and the ".N" name collision rule from the spec.
func TestTrashEscapesSpacesAndAvoidsCollisions(t *testing.T) {
	base := t.TempDir()
	t.Setenv("XDG_DATA_HOME", base)

	first := filepath.Join(t.TempDir(), "dir one", "proj")
	second := filepath.Join(t.TempDir(), "dir two", "proj")
	for _, dir := range []string{first, second} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("MkdirAll(%q): %v", dir, err)
		}
		if err := platform.New().Trash().Move(dir); err != nil {
			t.Fatalf("Move(%q): %v", dir, err)
		}
	}

	for _, name := range []string{"proj", "proj.2"} {
		if _, err := os.Stat(filepath.Join(base, "Trash", "files", name)); err != nil {
			t.Errorf("trash entry %q missing: %v", name, err)
		}
	}
	info, err := os.ReadFile(filepath.Join(base, "Trash", "info", "proj.trashinfo"))
	if err != nil {
		t.Fatalf("read trash info: %v", err)
	}
	for _, line := range strings.Split(string(info), "\n") {
		if strings.HasPrefix(line, "Path=") && strings.Contains(line, " ") {
			t.Errorf("trash info Path still contains a raw space: %q", line)
		}
	}
	if !strings.Contains(string(info), "dir%20one") {
		t.Errorf("trash info = %q, want the space escaped", info)
	}
}

// TestTrashReportsUnavailableWithoutHome verifies that a missing home
// directory is reported as ErrTrashUnavailable rather than silently deleting.
func TestTrashReportsUnavailableWithoutHome(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("HOME", "")

	src := filepath.Join(t.TempDir(), "proj")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := platform.New().Trash().Move(src); !errors.Is(err, platform.ErrTrashUnavailable) {
		t.Fatalf("Move error = %v, want ErrTrashUnavailable", err)
	}
	if _, err := os.Stat(src); err != nil {
		t.Errorf("directory removed despite unavailable trash: %v", err)
	}
}
