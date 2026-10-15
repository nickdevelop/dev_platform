//go:build linux

package unix

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"devplatform/internal/core/platform/policy"
)

// Trash is the freedesktop.org Trash implementation on Linux:
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
// fails with EXDEV, which is exactly why a home-trash-only implementation
// would degrade every /mnt/c project to a permanent delete. Each moved
// directory is described by a companion info/<name>.trashinfo file recording
// the original absolute path and deletion time, so a desktop trash UI can
// restore it. Any failure is reported as ErrTrashUnavailable so the caller
// refuses rather than permanently deletes.
type Trash struct{}

// trashBaseDir resolves the current user's XDG data home: XDG_DATA_HOME when
// it is an absolute path, otherwise $HOME/.local/share. It is resolved on
// every call so tests can point the trash at a temporary directory.
func trashBaseDir() (string, error) {
	if base := os.Getenv("XDG_DATA_HOME"); filepath.IsAbs(base) {
		return base, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("%w: cannot locate home directory: %v", policy.ErrTrashUnavailable, err)
	}
	if home == "" {
		return "", fmt.Errorf("%w: home directory is empty", policy.ErrTrashUnavailable)
	}
	return filepath.Join(home, ".local", "share"), nil
}

// Move moves path into the appropriate freedesktop trash directory. It picks
// the home trash when path shares the home trash's filesystem and a per-volume
// trash otherwise.
func (Trash) Move(path string) error {
	base, err := trashBaseDir()
	if err != nil {
		return err
	}

	absolute, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("%w: resolve %q: %v", policy.ErrTrashUnavailable, path, err)
	}

	homeDev, err := deviceOfExisting(base)
	if err != nil {
		return fmt.Errorf("%w: inspect home trash %q: %v", policy.ErrTrashUnavailable, base, err)
	}
	pathDev, err := deviceOfExisting(absolute)
	if err != nil {
		return fmt.Errorf("%w: inspect %q: %v", policy.ErrTrashUnavailable, absolute, err)
	}

	trashRoot := filepath.Join(base, "Trash")
	if pathDev != homeDev {
		topdir, err := findTopDir(absolute)
		if err != nil {
			return fmt.Errorf("%w: locate mount point of %q: %v", policy.ErrTrashUnavailable, absolute, err)
		}
		trashRoot = policy.SelectTrashRoot(trashRoot, homeDev, pathDev, topdir, os.Getuid())
	}
	return policy.MoveIntoTrash(trashRoot, absolute)
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
