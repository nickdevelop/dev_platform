//go:build !unix

package runtime

import (
	"errors"
	"os"
)

// errLockUnsupported reports that the platform has no single-instance lock
// implementation. The supported deployment targets are Linux and Windows
// WSL (both Unix); rather than run unsupervised where two processes could
// corrupt one data directory, startup refuses to proceed.
var errLockUnsupported = errors.New("runtime: single-instance locking requires a Unix platform (Linux or WSL)")

// lockFile always fails on platforms without a lock implementation.
func lockFile(*os.File) error { return errLockUnsupported }

// unlockFile mirrors lockFile for the non-Unix fallback.
func unlockFile(*os.File) error { return errLockUnsupported }
