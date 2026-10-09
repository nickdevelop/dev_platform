//go:build linux

package projects

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSystemTrashMovesDirectoryIntoUserTrash verifies the production trash
// mover against the freedesktop.org home-trash layout, using an isolated
// XDG_DATA_HOME so the test never touches the real user trash.
func TestSystemTrashMovesDirectoryIntoUserTrash(t *testing.T) {
	base := t.TempDir()
	t.Setenv("XDG_DATA_HOME", base)

	src := filepath.Join(t.TempDir(), "myproj")
	if err := os.MkdirAll(filepath.Join(src, "sub"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(src, "sub", "f.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if err := (systemTrash{}).Move(src); err != nil {
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

// TestSystemTrashEscapesSpacesAndAvoidsCollisions verifies the Path escaping
// and the ".N" name collision rule from the spec.
func TestSystemTrashEscapesSpacesAndAvoidsCollisions(t *testing.T) {
	base := t.TempDir()
	t.Setenv("XDG_DATA_HOME", base)

	first := filepath.Join(t.TempDir(), "dir one", "proj")
	second := filepath.Join(t.TempDir(), "dir two", "proj")
	for _, dir := range []string{first, second} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("MkdirAll(%q): %v", dir, err)
		}
		if err := (systemTrash{}).Move(dir); err != nil {
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

// TestSelectTrashRootChoosesHomeForSameDevice verifies a path on the home
// trash's own filesystem uses the home trash.
func TestSelectTrashRootChoosesHomeForSameDevice(t *testing.T) {
	homeTrashRoot := filepath.Join(t.TempDir(), "Trash")
	topdir := t.TempDir()

	got := selectTrashRoot(homeTrashRoot, 4, 4, topdir, 1000)
	if got != homeTrashRoot {
		t.Errorf("selectTrashRoot(same device) = %q, want home trash %q", got, homeTrashRoot)
	}
}

// TestSelectTrashRootUsesPerVolumeForForeignDevice simulates the WSL case
// where the project lives on another mount (/mnt/c): the selected trash root
// must be the per-user trash directory on that volume, never the home trash.
func TestSelectTrashRootUsesPerVolumeForForeignDevice(t *testing.T) {
	homeTrashRoot := filepath.Join(t.TempDir(), "Trash")
	topdir := t.TempDir()

	got := selectTrashRoot(homeTrashRoot, 4, 99, topdir, 1000)
	want := filepath.Join(topdir, ".Trash-1000")
	if got != want {
		t.Fatalf("selectTrashRoot(foreign device) = %q, want per-volume %q", got, want)
	}
	if got == homeTrashRoot {
		t.Errorf("foreign-device path still used the home trash %q", homeTrashRoot)
	}

	// When the volume exposes a usable shared .Trash (sticky + world
	// writable), the per-user directory inside it is preferred.
	shared := filepath.Join(topdir, ".Trash")
	if err := os.Mkdir(shared, 0o777); err != nil {
		t.Fatalf("create shared .Trash: %v", err)
	}
	if err := os.Chmod(shared, os.ModeSticky|0o777); err != nil {
		t.Fatalf("chmod shared .Trash: %v", err)
	}
	if got := perVolumeTrashRoot(topdir, 1000); got != filepath.Join(shared, "1000") {
		t.Errorf("perVolumeTrashRoot with shared trash = %q, want %q", got, filepath.Join(shared, "1000"))
	}
}

// TestMoveIntoTrashWritesFreedesktopLayout verifies the per-volume move
// mechanics: the payload and its .trashinfo land under files/ and info/ with
// the original absolute path recorded.
func TestMoveIntoTrashWritesFreedesktopLayout(t *testing.T) {
	trashRoot := filepath.Join(t.TempDir(), ".Trash-1000")
	src := filepath.Join(t.TempDir(), "proj")
	if err := os.MkdirAll(filepath.Join(src, "sub"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	absolute, err := filepath.Abs(src)
	if err != nil {
		t.Fatalf("Abs: %v", err)
	}

	if err := moveIntoTrash(trashRoot, absolute); err != nil {
		t.Fatalf("moveIntoTrash: %v", err)
	}
	if _, err := os.Lstat(src); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("source still exists after moveIntoTrash: %v", err)
	}
	if _, err := os.Stat(filepath.Join(trashRoot, "files", "proj", "sub")); err != nil {
		t.Errorf("trashed payload missing: %v", err)
	}
	info, err := os.ReadFile(filepath.Join(trashRoot, "info", "proj.trashinfo"))
	if err != nil {
		t.Fatalf("read trash info: %v", err)
	}
	if !strings.Contains(string(info), "Path="+absolute) {
		t.Errorf("trash info = %q, want Path=%s", info, absolute)
	}
}

// TestSystemTrashReportsUnavailableWithoutHome verifies that a missing home
// directory is reported as ErrTrashUnavailable rather than silently deleting.
func TestSystemTrashReportsUnavailableWithoutHome(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("HOME", "")

	src := filepath.Join(t.TempDir(), "proj")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := (systemTrash{}).Move(src); !errors.Is(err, ErrTrashUnavailable) {
		t.Fatalf("Move error = %v, want ErrTrashUnavailable", err)
	}
	if _, err := os.Stat(src); err != nil {
		t.Errorf("directory removed despite unavailable trash: %v", err)
	}
}
