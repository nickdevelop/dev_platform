//go:build windows

// Package windows is the Platform implementation for native Windows. It
// mirrors the unix package port by port (LockFileEx, Job Objects, PATHEXT
// lookup, known folders); business packages only see the port interfaces in
// the parent platform package.
package windows

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

// ErrLockHeld reports that another process holds the single-instance lock.
// It is the implementation's copy of the canonical port value; the New
// adapter maps it to platform.ErrLockHeld.
var ErrLockHeld = errors.New("single-instance lock is held by another process")

// lockFileName is the lock file placed inside the data directory. It stays
// behind after release so concurrent acquirers always contend on the same
// file; ownership is decided by the LockFileEx lock, not by file existence.
const lockFileName = "platform.lock"

// Instance is the Windows single-instance lock (LockFileEx).
type Instance struct{}

// Acquire takes the exclusive, non-blocking single-instance lock for
// dataDir. The lock file keeps the unix layout (platform.lock) and modes
// (ignored on Windows) so a data directory stays portable. The lock is tied
// to the open handle: it is released by Release and, by the OS, when the
// process exits for any reason.
func (Instance) Acquire(dataDir string) (*Lock, error) {
	if dataDir == "" {
		return nil, errors.New("platform: data directory is required")
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, fmt.Errorf("platform: create data directory: %w", err)
	}

	path := filepath.Join(dataDir, lockFileName)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("platform: open single-instance lock %s: %w", path, err)
	}
	var overlapped windows.Overlapped
	err = windows.LockFileEx(
		windows.Handle(file.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
		0, 1, 0, &overlapped)
	if err != nil {
		file.Close()
		if err == windows.ERROR_LOCK_VIOLATION {
			return nil, fmt.Errorf("platform: lock data directory %s: %w", dataDir, ErrLockHeld)
		}
		return nil, fmt.Errorf("platform: lock data directory %s: %w", dataDir, err)
	}
	return &Lock{file: file}, nil
}

// Lock is a held Windows single-instance lock. Release is safe to call more
// than once and on a nil Lock.
type Lock struct {
	file *os.File
}

// Release drops the lock so another process may acquire it.
func (l *Lock) Release() error {
	if l == nil || l.file == nil {
		return nil
	}
	var overlapped windows.Overlapped
	err := windows.UnlockFileEx(windows.Handle(l.file.Fd()), 0, 1, 0, &overlapped)
	if closeErr := l.file.Close(); err == nil {
		err = closeErr
	}
	l.file = nil
	if err != nil {
		return fmt.Errorf("platform: release single-instance lock: %w", err)
	}
	return nil
}
