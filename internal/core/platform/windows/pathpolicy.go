//go:build windows

package windows

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"devplatform/internal/core/platform/policy"
)

// CanonicalizeUserPath validates the raw input and returns its canonical
// absolute form. Drive letters and UNC paths are preserved (no WSL
// translation); relative inputs, bare drive-relative references
// (C:relative, C:, \rooted) and NUL bytes are rejected. Existing components
// are symlink-resolved exactly like the unix policy: a component that exists
// but cannot be resolved fails closed.
func (Dirs) CanonicalizeUserPath(p string) (string, error) {
	if p == "" {
		return "", fmt.Errorf("%w: path is empty", policy.ErrInvalidPath)
	}
	if strings.ContainsRune(p, 0) {
		return "", fmt.Errorf("%w: path contains a NUL byte", policy.ErrInvalidPath)
	}
	if !isDriveAbsolute(p) && !isUNC(p) {
		return "", fmt.Errorf("%w: %q is not absolute", policy.ErrInvalidPath, p)
	}
	// Abs also Cleans, which resolves interior `..` and `.` segments
	// lexically before any symlink is consulted.
	absolute, err := filepath.Abs(p)
	if err != nil {
		return "", fmt.Errorf("%w: %q is not absolute: %v", policy.ErrInvalidPath, p, err)
	}
	return canonicalize(absolute)
}

// SamePath compares paths for containment checks, ignoring case: Windows
// paths are case-insensitive.
func (Dirs) SamePath(a, b string) bool {
	return strings.EqualFold(a, b)
}

// isDriveAbsolute reports a volume-absolute path: a drive letter followed by
// a separator (C:\work, c:/work). Bare drive references (C:, C:relative)
// and rooted-but-driveless paths (\rooted) are not absolute: their volume
// depends on process state the platform does not track.
func isDriveAbsolute(p string) bool {
	return len(p) > 2 && p[1] == ':' && isDriveLetter(p[0]) && (p[2] == '\\' || p[2] == '/')
}

// isUNC reports a UNC path (\\server\share).
func isUNC(p string) bool {
	return strings.HasPrefix(p, `\\`)
}

func isDriveLetter(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

// canonicalize resolves path to its canonical form. If the path exists,
// EvalSymlinks resolves every component including the leaf. Otherwise the
// nearest existing ancestor is resolved and the non-existent remainder
// re-attached lexically. A component that exists but cannot be resolved — a
// dangling symlink, a path leading through a regular file (ENOTDIR), or an
// inaccessible ancestor — fails closed.
//
// It duplicates the unix policy on purpose: each platform keeps its path
// rules in its own pathpolicy file, and the shared error value keeps both
// failures identical to callers.
func canonicalize(path string) (string, error) {
	missing := ""
	dir := path
	for {
		resolved, err := filepath.EvalSymlinks(dir)
		if err == nil {
			return filepath.Join(resolved, missing), nil
		}
		if _, lstatErr := os.Lstat(dir); lstatErr == nil || !errors.Is(lstatErr, fs.ErrNotExist) {
			// The component exists (dangling symlink or non-directory
			// traversal) or its state cannot be determined; either way the
			// lexical rejoin would not describe a real path.
			return "", fmt.Errorf("%w: cannot canonicalize %q: %v", policy.ErrInvalidPath, dir, err)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("%w: cannot canonicalize %q: %v", policy.ErrInvalidPath, dir, err)
		}
		missing = filepath.Join(filepath.Base(dir), missing)
		dir = parent
	}
}
