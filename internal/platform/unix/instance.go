//go:build unix

// Package unix is the Platform implementation shared by Linux and WSL2.
// It holds the OS mechanics (flock, process groups, /proc probes, XDG
// directories, POSIX permission bits); business packages only see the
// port interfaces in the parent platform package.
package unix

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// ErrLockHeld reports that another process holds the single-instance lock.
var ErrLockHeld = errors.New("single-instance lock is held by another process")

// lockFileName is the lock file placed inside the data directory. It stays
// behind after release so concurrent acquirers always contend on the same
// inode; ownership is decided by the advisory lock, not by file existence.
const lockFileName = "platform.lock"

// Instance is the unix single-instance lock (flock).
type Instance struct{}

// Acquire takes the exclusive, non-blocking single-instance lock for
// dataDir, creating the directory (0700) and lock file (0600) when missing.
// A second acquisition while another process holds the lock fails with an
// error wrapping ErrLockHeld.
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
	// Tighten a lock file left with wider permissions by an earlier version.
	if err := os.Chmod(path, 0o600); err != nil {
		file.Close()
		return nil, fmt.Errorf("platform: restrict single-instance lock %s: %w", path, err)
	}
	if err := lockFile(file); err != nil {
		file.Close()
		return nil, fmt.Errorf("platform: lock data directory %s: %w", dataDir, err)
	}
	return &Lock{file: file}, nil
}

// Lock is a held unix single-instance lock. Release is safe to call more
// than once and on a nil Lock.
type Lock struct {
	file *os.File
}

// Release drops the lock so another process may acquire it.
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
		return fmt.Errorf("platform: release single-instance lock: %w", err)
	}
	return nil
}

// lockFile takes an exclusive, non-blocking advisory lock (flock(2)) on the
// lock file. LOCK_NB keeps a mistaken double start failing fast with a
// clear error instead of silently queueing two servers.
func lockFile(file *os.File) error {
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return ErrLockHeld
		}
		return err
	}
	return nil
}

// unlockFile releases the advisory lock. The lock is also dropped when the
// file descriptor is closed, so a crashing process cannot pin it forever.
func unlockFile(file *os.File) error {
	return syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
}
