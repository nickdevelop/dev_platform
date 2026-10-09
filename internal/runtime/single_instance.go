// Package runtime provides deployment-runtime primitives for the platform
// service. It currently hosts the single-instance lock that enforces the
// "one platform process per data directory" rule.
//
// The lock is advisory (flock(2) on Unix platforms, which covers the
// supported Linux and Windows WSL deployment targets) and is scoped to one
// data directory: two processes may run against different data directories
// concurrently, but the second process targeting the same data directory
// fails to start with a clear error instead of racing on the SQLite
// database and lock file.
package runtime

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// lockFileName is the lock file placed inside the data directory. It stays
// behind after release so concurrent acquirers always contend on the same
// inode; ownership is decided by the advisory lock, not by file existence.
const lockFileName = "platform.lock"

// Lock is a held single-instance lock on a platform data directory. It must
// be released before the process exits; Release is safe to call more than
// once and on a nil Lock.
type Lock struct {
	file *os.File
}

// errLockHeld reports that another process holds the single-instance lock.
// Platform-specific lockFile implementations return it when the data
// directory is contended.
var errLockHeld = errors.New("single-instance lock is held by another process")

// AcquireLock takes the exclusive single-instance lock for dataDir,
// creating the directory when missing. A second acquisition while another
// process (or goroutine) holds the lock fails with an error naming the data
// directory, so a confused double start is immediately diagnosable.
func AcquireLock(dataDir string) (*Lock, error) {
	if dataDir == "" {
		return nil, errors.New("runtime: data directory is required")
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, fmt.Errorf("runtime: create data directory: %w", err)
	}

	path := filepath.Join(dataDir, lockFileName)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("runtime: open single-instance lock %s: %w", path, err)
	}
	// Tighten a lock file left with wider permissions by an earlier version.
	if err := os.Chmod(path, 0o600); err != nil {
		file.Close()
		return nil, fmt.Errorf("runtime: restrict single-instance lock %s: %w", path, err)
	}
	if err := lockFile(file); err != nil {
		file.Close()
		if errors.Is(err, errLockHeld) {
			return nil, fmt.Errorf("runtime: data directory %s is already in use by another platform process", dataDir)
		}
		return nil, fmt.Errorf("runtime: lock data directory %s: %w", dataDir, err)
	}
	return &Lock{file: file}, nil
}

// Release drops the lock so another process may acquire it. Releasing an
// already released (or nil) lock is a no-op.
func (l *Lock) Release() error {
	if l == nil || l.file == nil {
		return nil
	}
	err := unlockFile(l.file)
	if closeErr := l.file.Close(); err == nil {
		err = closeErr
	}
	l.file = nil
	if err != nil {
		return fmt.Errorf("runtime: release single-instance lock: %w", err)
	}
	return nil
}
