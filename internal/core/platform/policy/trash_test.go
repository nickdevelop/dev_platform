package policy

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSelectTrashRootChoosesHomeForSameDevice verifies a path on the home
// trash's own filesystem uses the home trash.
func TestSelectTrashRootChoosesHomeForSameDevice(t *testing.T) {
	homeTrashRoot := filepath.Join(t.TempDir(), "Trash")
	topdir := t.TempDir()

	got := SelectTrashRoot(homeTrashRoot, 4, 4, topdir, 1000)
	if got != homeTrashRoot {
		t.Errorf("SelectTrashRoot(same device) = %q, want home trash %q", got, homeTrashRoot)
	}
}

// TestSelectTrashRootUsesPerVolumeForForeignDevice simulates the WSL case
// where the project lives on another mount (/mnt/c): the selected trash root
// must be the per-user trash directory on that volume, never the home trash.
func TestSelectTrashRootUsesPerVolumeForForeignDevice(t *testing.T) {
	homeTrashRoot := filepath.Join(t.TempDir(), "Trash")
	topdir := t.TempDir()

	got := SelectTrashRoot(homeTrashRoot, 4, 99, topdir, 1000)
	want := filepath.Join(topdir, ".Trash-1000")
	if got != want {
		t.Fatalf("SelectTrashRoot(foreign device) = %q, want per-volume %q", got, want)
	}
	if got == homeTrashRoot {
		t.Errorf("foreign-device path still used the home trash %q", homeTrashRoot)
	}
}

// TestPerVolumeTrashRootPrefersSharedTrash verifies the shared .Trash layout
// is preferred only when it is sticky and world-writable.
func TestPerVolumeTrashRootPrefersSharedTrash(t *testing.T) {
	topdir := t.TempDir()

	if got, want := PerVolumeTrashRoot(topdir, 1000), filepath.Join(topdir, ".Trash-1000"); got != want {
		t.Errorf("PerVolumeTrashRoot without shared trash = %q, want %q", got, want)
	}

	shared := filepath.Join(topdir, ".Trash")
	if err := os.Mkdir(shared, 0o777); err != nil {
		t.Fatalf("create shared .Trash: %v", err)
	}
	if err := os.Chmod(shared, os.ModeSticky|0o777); err != nil {
		t.Fatalf("chmod shared .Trash: %v", err)
	}
	if got, want := PerVolumeTrashRoot(topdir, 1000), filepath.Join(shared, "1000"); got != want {
		t.Errorf("PerVolumeTrashRoot with shared trash = %q, want %q", got, want)
	}
}

// TestEscapeTrashPathKeepsSeparators verifies segments are escaped while '/'
// separators survive.
func TestEscapeTrashPathKeepsSeparators(t *testing.T) {
	got := EscapeTrashPath("/home/user/dir one/proj")
	if strings.Contains(got, " ") {
		t.Errorf("EscapeTrashPath = %q, want no raw spaces", got)
	}
	if !strings.Contains(got, "dir%20one") {
		t.Errorf("EscapeTrashPath = %q, want the space escaped", got)
	}
	if !strings.HasPrefix(got, "/home/user/") {
		t.Errorf("EscapeTrashPath = %q, want separators preserved", got)
	}
}

// TestMoveIntoTrashWritesFreedesktopLayout verifies the move mechanics: the
// payload and its .trashinfo land under files/ and info/ with the original
// absolute path recorded.
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

	if err := MoveIntoTrash(trashRoot, absolute); err != nil {
		t.Fatalf("MoveIntoTrash: %v", err)
	}
	if _, err := os.Lstat(src); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("source still exists after MoveIntoTrash: %v", err)
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

// TestMoveIntoTrashReportsUnavailable keeps failures in the trash-unavailable
// family so callers refuse rather than permanently delete.
func TestMoveIntoTrashReportsUnavailable(t *testing.T) {
	// A trash root under a regular file cannot hold files/info directories.
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatalf("write blocker: %v", err)
	}
	err := MoveIntoTrash(filepath.Join(blocker, "trash"), filepath.Join(t.TempDir(), "missing"))
	if err == nil {
		t.Fatal("MoveIntoTrash into an unusable root succeeded, want an error")
	} else if !errors.Is(err, ErrTrashUnavailable) {
		t.Fatalf("MoveIntoTrash error = %v, want ErrTrashUnavailable", err)
	}
}
