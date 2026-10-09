// Package workspaces manages the operator-authorized workspace roots and the
// canonical path authorization policy that every platform filesystem
// operation must pass through.
//
// Security model: the platform only touches paths that resolve inside a
// registered workspace root. ResolveAuthorizedPath is the single gate for
// that decision; callers must operate on the returned canonical path, never
// on the raw input, so that the validated location is the accessed location.
//
// Path semantics: the platform targets Linux and WSL. Windows-style drive
// paths (C:\work, c:/work) are translated to their WSL mount form
// (/mnt/c/work) before authorization; UNC paths (\\server\share),
// drive-relative paths (C:relative), and mid-path backslashes are ambiguous
// on Linux and are rejected outright.
//
// Case behavior: authorization is exact-case. On Linux filesystems a
// differently-cased path is a different, unauthorized path. On
// case-insensitive mounts (WSL /mnt/<drive>drvfs) the OS might resolve a case
// variant to the same directory, but the string containment check still
// refuses it, so operators must register roots with the canonical casing
// returned by the filesystem and use that casing consistently.
package workspaces

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	// ErrInvalidPath reports a malformed, relative, or ambiguous input path
	// that cannot be authorized safely.
	ErrInvalidPath = errors.New("workspaces: invalid path")
	// ErrOutsideRoots reports a well-formed path that does not resolve inside
	// any authorized workspace root.
	ErrOutsideRoots = errors.New("workspaces: path is outside every authorized workspace root")
)

// ResolvedPath is the authorization decision for one input path: the
// canonical location the platform may operate on, and the authorized root
// that contains it.
type ResolvedPath struct {
	// Path is the canonical absolute path. Existing components are
	// symlink-resolved; for a not-yet-existing path the nearest existing
	// ancestor is resolved and the non-existent remainder re-attached.
	Path string
	// Root is the canonical path of the deepest workspace root containing
	// Path.
	Root string
}

// windowsDrivePattern matches Windows drive paths with a separator
// (C:\work, c:/work, C:\) but not drive-relative forms (C:relative), which
// are matched by windowsDrivePrefix and rejected as ambiguous.
var windowsDrivePattern = regexp.MustCompile(`(?i)^([a-z]):[\\/](.*)$`)

// windowsDrivePrefix matches a bare drive prefix such as "C:relative" or
// "C:" — a Windows drive-relative reference whose meaning depends on the
// drive's current directory, which the platform does not track.
var windowsDrivePrefix = regexp.MustCompile(`(?i)^[a-z]:`)

// ResolveAuthorizedPath validates that input resolves inside one of the
// given authorized workspace roots and returns the canonical path to operate
// on.
//
// The input must be absolute (after Windows drive translation); `..`
// segments are applied lexically by filepath.Abs before symlink resolution,
// so any traversal that escapes a root produces a path outside it and is
// rejected. Existing components are resolved through filepath.EvalSymlinks;
// for a not-yet-existing target the nearest existing ancestor is resolved and
// required to be inside a root, and the full path must remain inside it.
// Ambiguous inputs — relative paths, stray backslashes, UNC paths, and paths
// leading through an existing non-directory — fail closed with
// ErrInvalidPath; everything outside every root fails with ErrOutsideRoots.
//
// The roots are canonicalized with EvalSymlinks before the containment
// checks; a root that no longer resolves cannot authorize anything and is
// skipped. When several roots contain the path, the deepest one is reported.
func ResolveAuthorizedPath(input string, roots []string) (ResolvedPath, error) {
	cleaned, err := normalizeInput(input)
	if err != nil {
		return ResolvedPath{}, err
	}

	canonicalRoots := make([]string, 0, len(roots))
	for _, root := range roots {
		if root == "" || !filepath.IsAbs(root) {
			continue
		}
		if resolved, err := filepath.EvalSymlinks(root); err == nil {
			canonicalRoots = append(canonicalRoots, resolved)
		}
	}

	canonical, err := canonicalize(cleaned)
	if err != nil {
		return ResolvedPath{}, err
	}

	root, ok := containingRoot(canonical, canonicalRoots)
	if !ok {
		return ResolvedPath{}, fmt.Errorf("%w: %q", ErrOutsideRoots, canonical)
	}
	return ResolvedPath{Path: canonical, Root: root}, nil
}

// normalizeInput validates the raw input and returns its cleaned absolute
// form: Windows drive paths are translated to WSL mount paths, relative and
// ambiguous inputs are rejected, and `..` segments are applied lexically by
// filepath.Abs.
func normalizeInput(input string) (string, error) {
	if input == "" {
		return "", fmt.Errorf("%w: path is empty", ErrInvalidPath)
	}
	if strings.ContainsRune(input, 0) {
		return "", fmt.Errorf("%w: path contains a NUL byte", ErrInvalidPath)
	}

	cleaned, err := translateWindowsPath(input)
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(cleaned) {
		return "", fmt.Errorf("%w: %q is not absolute", ErrInvalidPath, input)
	}
	// Abs also Cleans, which resolves interior `..` and `.` segments
	// lexically before any symlink is consulted.
	return filepath.Abs(cleaned)
}

// translateWindowsPath maps Windows-style drive paths to their WSL mount
// points (C:\work → /mnt/c/work, lowercasing the drive letter) and rejects
// ambiguous forms: UNC paths and backslashes outside a drive prefix. On
// Linux a backslash is a legal filename character, so mixed-separator input
// cannot be interpreted safely.
func translateWindowsPath(input string) (string, error) {
	if strings.HasPrefix(input, `\\`) {
		return "", fmt.Errorf("%w: UNC paths are not supported", ErrInvalidPath)
	}
	if match := windowsDrivePattern.FindStringSubmatch(input); match != nil {
		rest := strings.ReplaceAll(match[2], "\\", "/")
		return filepath.Join("/mnt/"+strings.ToLower(match[1]), rest), nil
	}
	if windowsDrivePrefix.MatchString(input) {
		return "", fmt.Errorf("%w: drive-relative paths are not supported", ErrInvalidPath)
	}
	if strings.ContainsRune(input, '\\') {
		return "", fmt.Errorf("%w: backslash is only valid in a Windows drive path", ErrInvalidPath)
	}
	return input, nil
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
			return "", fmt.Errorf("%w: cannot canonicalize %q: %v", ErrInvalidPath, dir, err)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("%w: cannot canonicalize %q: %v", ErrInvalidPath, dir, err)
		}
		missing = filepath.Join(filepath.Base(dir), missing)
		dir = parent
	}
}

// containingRoot returns the deepest root that contains path, following the
// same rule as pathInsideRoot for every candidate.
func containingRoot(path string, roots []string) (string, bool) {
	best := ""
	found := false
	for _, root := range roots {
		if !pathInsideRoot(path, root) {
			continue
		}
		if !found || len(root) > len(best) {
			best, found = root, true
		}
	}
	return best, found
}

// pathInsideRoot reports whether path is root itself or lies below it. The
// root "/" contains everything; any other root must match on exact segment
// boundaries so that /data/roots does not authorize /data/roots-extra.
func pathInsideRoot(path, root string) bool {
	if root == "/" {
		return true
	}
	return path == root || strings.HasPrefix(path, root+"/")
}
