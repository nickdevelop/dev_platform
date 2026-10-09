//go:build linux

package projects

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// systemTrash is the OS trash implementation on Linux. It follows the
// freedesktop.org Trash specification:
//
//   - a directory on the same filesystem as the user's home trash is moved
//     into $XDG_DATA_HOME/Trash (the "home trash"), and
//   - a directory on another filesystem (the common WSL case, e.g. a project
//     under /mnt/c) is moved into a per-volume trash directory on that same
//     volume, either the shared $topdir/.Trash/$uid when the volume already
//     has a usable shared trash, or the per-user $topdir/.Trash-$uid
//     otherwise.
//
// Trashing on the same filesystem matters: os.Rename across filesystems
// fails with EXDEV, which is exactly why the home-trash-only implementation
// degraded every /mnt/c project to a permanent delete. Each moved directory
// is described by a companion info/<name>.trashinfo file recording the
// original absolute path and deletion time, so a desktop trash UI can restore
// it. Any failure is reported as ErrTrashUnavailable so the caller refuses
// rather than permanently deletes.
type systemTrash struct{}

// trashBaseDir resolves the current user's XDG data home: XDG_DATA_HOME when
// it is an absolute path, otherwise $HOME/.local/share. It is resolved on
// every call so tests can point the trash at a temporary directory.
func trashBaseDir() (string, error) {
	if base := os.Getenv("XDG_DATA_HOME"); filepath.IsAbs(base) {
		return base, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("%w: cannot locate home directory: %v", ErrTrashUnavailable, err)
	}
	if home == "" {
		return "", fmt.Errorf("%w: home directory is empty", ErrTrashUnavailable)
	}
	return filepath.Join(home, ".local", "share"), nil
}

// Move moves path into the appropriate freedesktop trash directory. It picks
// the home trash when path shares the home trash's filesystem and a per-volume
// trash otherwise.
func (systemTrash) Move(path string) error {
	base, err := trashBaseDir()
	if err != nil {
		return err
	}

	absolute, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("%w: resolve %q: %v", ErrTrashUnavailable, path, err)
	}

	homeDev, err := deviceOfExisting(base)
	if err != nil {
		return fmt.Errorf("%w: inspect home trash %q: %v", ErrTrashUnavailable, base, err)
	}
	pathDev, err := deviceOfExisting(absolute)
	if err != nil {
		return fmt.Errorf("%w: inspect %q: %v", ErrTrashUnavailable, absolute, err)
	}

	trashRoot := filepath.Join(base, "Trash")
	if pathDev != homeDev {
		topdir, err := findTopDir(absolute)
		if err != nil {
			return fmt.Errorf("%w: locate mount point of %q: %v", ErrTrashUnavailable, absolute, err)
		}
		trashRoot = selectTrashRoot(trashRoot, homeDev, pathDev, topdir, os.Getuid())
	}
	return moveIntoTrash(trashRoot, absolute)
}

// selectTrashRoot chooses the trash root directory for a path. It is a pure
// function of the two device numbers and the path's mount point (topdir), so
// the cross-volume choice is testable without mounting a second filesystem.
func selectTrashRoot(homeTrashRoot string, homeDev, pathDev uint64, topdir string, uid int) string {
	if pathDev == homeDev {
		return homeTrashRoot
	}
	return perVolumeTrashRoot(topdir, uid)
}

// perVolumeTrashRoot returns the per-volume trash directory for uid under the
// volume's top directory. It prefers the shared $topdir/.Trash/$uid layout
// when a usable (sticky, world-writable) shared .Trash already exists, and
// otherwise uses the per-user $topdir/.Trash-$uid directory, which avoids
// creating a world-writable directory on the volume.
func perVolumeTrashRoot(topdir string, uid int) string {
	shared := filepath.Join(topdir, ".Trash")
	if info, err := os.Stat(shared); err == nil && info.IsDir() && isUsableSharedTrash(info) {
		return filepath.Join(shared, strconv.Itoa(uid))
	}
	return filepath.Join(topdir, fmt.Sprintf(".Trash-%d", uid))
}

// isUsableSharedTrash reports whether a shared .Trash directory satisfies the
// spec's requirements: a directory with the sticky bit and world-writable.
func isUsableSharedTrash(info os.FileInfo) bool {
	return info.Mode()&os.ModeSticky != 0 && info.Mode().Perm()&0o002 != 0
}

// findTopDir returns the mount point (the "top directory") containing path by
// walking up until the parent lives on a different device.
func findTopDir(path string) (string, error) {
	dev, err := deviceOf(path)
	if err != nil {
		return "", err
	}
	dir := path
	for {
		parent := filepath.Dir(dir)
		if parent == dir {
			return dir, nil
		}
		parentDev, err := deviceOf(parent)
		if err != nil {
			return "", err
		}
		if parentDev != dev {
			return dir, nil
		}
		dir = parent
	}
}

// deviceOf returns the device id of an existing path.
func deviceOf(path string) (uint64, error) {
	var st syscall.Stat_t
	if err := syscall.Stat(path, &st); err != nil {
		return 0, err
	}
	return uint64(st.Dev), nil
}

// deviceOfExisting returns the device id of path or of its nearest existing
// ancestor, so a not-yet-created trash base still resolves to its volume.
func deviceOfExisting(path string) (uint64, error) {
	dir := path
	for {
		dev, err := deviceOf(dir)
		if err == nil {
			return dev, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return 0, err
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return 0, err
		}
		dir = parent
	}
}

// moveIntoTrash moves absolute into trashRoot/files and writes the companion
// trashRoot/info/<name>.trashinfo. The rename is the destructive step; it only
// runs after the info file exists, and the info file is removed if the move
// fails so the trash never keeps an entry without its payload.
func moveIntoTrash(trashRoot, absolute string) error {
	filesDir := filepath.Join(trashRoot, "files")
	infoDir := filepath.Join(trashRoot, "info")
	if err := os.MkdirAll(filesDir, 0o700); err != nil {
		return fmt.Errorf("%w: create %q: %v", ErrTrashUnavailable, filesDir, err)
	}
	if err := os.MkdirAll(infoDir, 0o700); err != nil {
		return fmt.Errorf("%w: create %q: %v", ErrTrashUnavailable, infoDir, err)
	}

	name := uniqueTrashName(filesDir, infoDir, filepath.Base(absolute))
	infoPath := filepath.Join(infoDir, name+".trashinfo")
	info := fmt.Sprintf("[Trash Info]\nPath=%s\nDeletionDate=%s\n",
		escapeTrashPath(absolute), time.Now().Format("2006-01-02T15:04:05"))
	if err := os.WriteFile(infoPath, []byte(info), 0o600); err != nil {
		return fmt.Errorf("%w: write trash info: %v", ErrTrashUnavailable, err)
	}

	if err := os.Rename(absolute, filepath.Join(filesDir, name)); err != nil {
		_ = os.Remove(infoPath)
		return fmt.Errorf("%w: move %q into trash: %v", ErrTrashUnavailable, absolute, err)
	}
	return nil
}

// uniqueTrashName picks a name not yet used in either trash directory, so two
// directories that share a basename never overwrite each other in the trash.
func uniqueTrashName(filesDir, infoDir, base string) string {
	name := base
	for i := 2; ; i++ {
		if !trashEntryExists(filesDir, infoDir, name) {
			return name
		}
		name = fmt.Sprintf("%s.%d", base, i)
	}
}

// trashEntryExists reports whether the trash already holds an entry with name.
func trashEntryExists(filesDir, infoDir, name string) bool {
	if _, err := os.Lstat(filepath.Join(filesDir, name)); err == nil {
		return true
	}
	if _, err := os.Lstat(filepath.Join(infoDir, name+".trashinfo")); err == nil {
		return true
	}
	return false
}

// escapeTrashPath percent-escapes each path segment while preserving the '/'
// separators, as the Trash specification requires for the Path field.
func escapeTrashPath(path string) string {
	segments := strings.Split(path, "/")
	for i, segment := range segments {
		segments[i] = url.PathEscape(segment)
	}
	return strings.Join(segments, "/")
}
