//go:build linux

package projects

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// systemTrash is the OS trash implementation on Linux. It follows the
// freedesktop.org Trash specification for the current user's home trash:
// a deleted directory is moved into $XDG_DATA_HOME/Trash/files and described
// by a companion $XDG_DATA_HOME/Trash/info/<name>.trashinfo file recording the
// original absolute path and the deletion time. The freedesktop spec is what
// Linux desktop file managers use, so a directory deleted here can be restored
// through the normal desktop trash UI.
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

// Move moves path into the user's home trash. Any failure is reported as
// ErrTrashUnavailable so the caller refuses rather than permanently deletes.
func (systemTrash) Move(path string) error {
	base, err := trashBaseDir()
	if err != nil {
		return err
	}
	filesDir := filepath.Join(base, "Trash", "files")
	infoDir := filepath.Join(base, "Trash", "info")
	if err := os.MkdirAll(filesDir, 0o700); err != nil {
		return fmt.Errorf("%w: create %q: %v", ErrTrashUnavailable, filesDir, err)
	}
	if err := os.MkdirAll(infoDir, 0o700); err != nil {
		return fmt.Errorf("%w: create %q: %v", ErrTrashUnavailable, infoDir, err)
	}

	absolute, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("%w: resolve %q: %v", ErrTrashUnavailable, path, err)
	}

	name := uniqueTrashName(filesDir, infoDir, filepath.Base(path))
	infoPath := filepath.Join(infoDir, name+".trashinfo")
	info := fmt.Sprintf("[Trash Info]\nPath=%s\nDeletionDate=%s\n",
		escapeTrashPath(absolute), time.Now().Format("2006-01-02T15:04:05"))
	if err := os.WriteFile(infoPath, []byte(info), 0o600); err != nil {
		return fmt.Errorf("%w: write trash info: %v", ErrTrashUnavailable, err)
	}

	// The rename is the destructive step; it only runs after the .trashinfo
	// file exists, and the info file is removed if the move fails so the trash
	// never keeps an entry without its payload.
	if err := os.Rename(path, filepath.Join(filesDir, name)); err != nil {
		_ = os.Remove(infoPath)
		return fmt.Errorf("%w: move %q into trash: %v", ErrTrashUnavailable, path, err)
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
