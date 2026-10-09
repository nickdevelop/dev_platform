//go:build unix

package runtime

import (
	"errors"
	"os"
	"syscall"
)

// lockFile takes an exclusive, non-blocking advisory lock (flock(2)) on the
// lock file. LOCK_NB keeps a mistaken double start failing fast with a
// clear error instead of silently queueing two servers.
func lockFile(file *os.File) error {
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return errLockHeld
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
