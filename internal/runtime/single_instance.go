// Package runtime provides deployment-runtime primitives for the platform
// service. It currently hosts the single-instance lock that enforces the
// "one platform process per data directory" rule.
//
// The lock is advisory and scoped to one data directory: two processes may
// run against different data directories concurrently, but the second
// process targeting the same data directory fails to start with a clear
// error instead of racing on the SQLite database and lock file. The OS
// mechanics (flock on unix, LockFileEx on windows) live behind the
// internal/platform Instance port; this package keeps the stable AcquireLock
// API and the user-facing error text.
package runtime

import (
	"errors"
	"fmt"

	"devplatform/internal/platform"
)

// Lock is a held single-instance lock on a platform data directory. It must
// be released before the process exits; Release is safe to call more than
// once and on a nil Lock.
type Lock struct {
	inner platform.Lock
}

// AcquireLock takes the exclusive single-instance lock for dataDir,
// creating the directory when missing. A second acquisition while another
// process (or goroutine) holds the lock fails with an error naming the data
// directory, so a confused double start is immediately diagnosable.
func AcquireLock(dataDir string) (*Lock, error) {
	if dataDir == "" {
		return nil, errors.New("runtime: data directory is required")
	}
	inner, err := platform.New().Instance().Acquire(dataDir)
	if err != nil {
		if errors.Is(err, platform.ErrLockHeld) {
			return nil, fmt.Errorf("runtime: data directory %s is already in use by another platform process", dataDir)
		}
		return nil, err
	}
	return &Lock{inner: inner}, nil
}

// Release drops the lock so another process may acquire it. Releasing an
// already released (or nil) lock is a no-op.
func (l *Lock) Release() error {
	if l == nil || l.inner == nil {
		return nil
	}
	err := l.inner.Release()
	l.inner = nil
	return err
}
