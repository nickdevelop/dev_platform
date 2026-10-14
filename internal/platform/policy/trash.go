// Package policy holds portable trash mechanics shared by the platform
// implementations: pure path logic plus filesystem operations that need no
// OS-specific calls. It carries no build tag, so its tests run on every
// platform. The freedesktop.org Trash specification details live here; the
// unix package adds device detection and the windows implementation reports
// the trash unavailable.
package policy

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// ErrTrashUnavailable is the implementation's copy of the canonical port
// value (ErrTrashUnavailable). Policy cannot import the port
// package without creating an import cycle, so the New adapter maps this
// copy to the canonical value at the port boundary.
var ErrTrashUnavailable = errors.New("projects: os trash is unavailable")

// SelectTrashRoot chooses the trash root directory for a path. It is a pure
// function of the two device numbers and the path's mount point (topdir), so
// the cross-volume choice is testable without mounting a second filesystem.
func SelectTrashRoot(homeTrashRoot string, homeDev, pathDev uint64, topdir string, uid int) string {
	if pathDev == homeDev {
		return homeTrashRoot
	}
	return PerVolumeTrashRoot(topdir, uid)
}

// PerVolumeTrashRoot returns the per-volume trash directory for uid under the
// volume's top directory. It prefers the shared $topdir/.Trash/$uid layout
// when a usable (sticky, world-writable) shared .Trash already exists, and
// otherwise uses the per-user $topdir/.Trash-$uid directory, which avoids
// creating a world-writable directory on the volume.
func PerVolumeTrashRoot(topdir string, uid int) string {
	shared := filepath.Join(topdir, ".Trash")
	if info, err := os.Stat(shared); err == nil && isUsableSharedTrash(info) {
		return filepath.Join(shared, strconv.Itoa(uid))
	}
	return filepath.Join(topdir, fmt.Sprintf(".Trash-%d", uid))
}

// isUsableSharedTrash reports whether a shared .Trash directory satisfies the
// spec's requirements: a directory with the sticky bit and world-writable.
func isUsableSharedTrash(info os.FileInfo) bool {
	return info.IsDir() && info.Mode()&os.ModeSticky != 0 && info.Mode().Perm()&0o002 != 0
}

// MoveIntoTrash moves absolute into trashRoot/files and writes the companion
// trashRoot/info/<name>.trashinfo. The rename is the destructive step; it only
// runs after the info file exists, and the info file is removed if the move
// fails so the trash never keeps an entry without its payload.
func MoveIntoTrash(trashRoot, absolute string) error {
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
		EscapeTrashPath(absolute), time.Now().Format("2006-01-02T15:04:05"))
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

// EscapeTrashPath percent-escapes each path segment while preserving the '/'
// separators, as the Trash specification requires for the Path field.
func EscapeTrashPath(path string) string {
	segments := strings.Split(path, "/")
	for i, segment := range segments {
		segments[i] = url.PathEscape(segment)
	}
	return strings.Join(segments, "/")
}
