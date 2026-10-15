//go:build unix

package unix

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"devplatform/internal/core/platform/policy"
)

// windowsDrivePattern matches Windows drive paths with a separator
// (C:\work, c:/work, C:\) but not drive-relative forms (C:relative), which
// are matched by windowsDrivePrefix and rejected as ambiguous.
var windowsDrivePattern = regexp.MustCompile(`(?i)^([a-z]):[\\/](.*)$`)

// windowsDrivePrefix matches a bare drive prefix such as "C:relative" or
// "C:" — a Windows drive-relative reference whose meaning depends on the
// drive's current directory, which the platform does not track.
var windowsDrivePrefix = regexp.MustCompile(`(?i)^[a-z]:`)

// CanonicalizeUserPath validates the raw input and returns its canonical
// absolute form: Windows drive paths are translated to WSL mount paths,
// relative and ambiguous inputs are rejected, `..` segments are applied
// lexically, and existing components are symlink-resolved.
func (Dirs) CanonicalizeUserPath(p string) (string, error) {
	cleaned, err := normalizeInput(p)
	if err != nil {
		return "", err
	}
	return canonicalize(cleaned)
}

// SamePath compares paths for containment checks. Unix is case sensitive:
// a differently-cased path is a different path.
func (Dirs) SamePath(a, b string) bool {
	return a == b
}

// TranslateWindowsPath maps Windows-style drive paths to their WSL mount
// points (C:\work → /mnt/c/work, lowercasing the drive letter) and rejects
// ambiguous forms: UNC paths and backslashes outside a drive prefix. On
// Linux a backslash is a legal filename character, so mixed-separator input
// cannot be interpreted safely.
func TranslateWindowsPath(input string) (string, error) {
	if strings.HasPrefix(input, `\\`) {
		return "", fmt.Errorf("%w: UNC paths are not supported", policy.ErrInvalidPath)
	}
	if match := windowsDrivePattern.FindStringSubmatch(input); match != nil {
		rest := strings.ReplaceAll(match[2], "\\", "/")
		return filepath.Join("/mnt/"+strings.ToLower(match[1]), rest), nil
	}
	if windowsDrivePrefix.MatchString(input) {
		return "", fmt.Errorf("%w: drive-relative paths are not supported", policy.ErrInvalidPath)
	}
	if strings.ContainsRune(input, '\\') {
		return "", fmt.Errorf("%w: backslash is only valid in a Windows drive path", policy.ErrInvalidPath)
	}
	return input, nil
}

// normalizeInput validates the raw input and returns its cleaned absolute
// form: Windows drive paths are translated to WSL mount paths, relative and
// ambiguous inputs are rejected, and `..` segments are applied lexically by
// filepath.Abs.
func normalizeInput(input string) (string, error) {
	if input == "" {
		return "", fmt.Errorf("%w: path is empty", policy.ErrInvalidPath)
	}
	if strings.ContainsRune(input, 0) {
		return "", fmt.Errorf("%w: path contains a NUL byte", policy.ErrInvalidPath)
	}

	cleaned, err := TranslateWindowsPath(input)
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(cleaned) {
		return "", fmt.Errorf("%w: %q is not absolute", policy.ErrInvalidPath, input)
	}
	// Abs also Cleans, which resolves interior `..` and `.` segments
	// lexically before any symlink is consulted.
	return filepath.Abs(cleaned)
}

// canonicalize resolves path to its canonical form. If the path exists,
// EvalSymlinks resolves every component including the leaf. Otherwise the
// nearest existing ancestor is resolved and the non-existent remainder
// re-attached lexically. A component that exists but cannot be resolved — a
// dangling symlink, a path leading through a regular file (ENOTDIR), or an
// inaccessible ancestor — fails closed.
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
