// Package paths canonicalizes filesystem paths for the platform.
//
// Unlike the removed workspace-root authorization layer, this package does NOT
// restrict a path to any pre-registered directory: the platform may operate on
// any absolute path the process itself can access. What it still guarantees is
// that the caller operates on a canonical, unambiguous location:
//
//   - Windows-style drive paths (C:\work, c:/work) are translated to their WSL
//     mount form (/mnt/c/work); UNC paths (\\server\share), drive-relative paths
//     (C:relative), and mid-path backslashes are ambiguous on Linux and are
//     rejected outright.
//   - Relative paths and NUL bytes are rejected.
//   - `..` segments are applied lexically before any symlink is consulted.
//   - Existing components are symlink-resolved; for a not-yet-existing target
//     the nearest existing ancestor is resolved and the remainder re-attached.
//     A component that exists but cannot be resolved (a dangling symlink, a
//     path leading through a regular file, an inaccessible ancestor) fails
//     closed with ErrInvalidPath.
//
// Case behavior is exact-case: on Linux a differently-cased path is a
// different path.
//
// The OS-specific rules live behind the internal/platform Dirs port; this
// package keeps the stable Resolve API and the shared error value.
package paths

import (
	"devplatform/internal/platform"
	"devplatform/internal/platform/policy"
)

// ErrInvalidPath reports a malformed, relative, ambiguous, or unresolvable
// input path that cannot be canonicalized safely. It aliases the shared
// policy value so the unix implementation can wrap it without creating an
// import cycle.
var ErrInvalidPath = policy.ErrInvalidPath

// Resolve validates input and returns the canonical absolute path to operate
// on. The input must be absolute (after Windows drive translation); callers
// must operate on the returned path, never on the raw input, so the validated
// location is the accessed location.
func Resolve(input string) (string, error) {
	return platform.New().Dirs().CanonicalizeUserPath(input)
}
