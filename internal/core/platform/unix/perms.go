//go:build unix

package unix

import "os"

// Perms applies owner-only permissions on unix, where mode bits are
// enforced. The 0700/0600 values match the constants the business packages
// historically used (secrets.FileMode and dirMode).
type Perms struct{}

// PrivateDir creates path with owner-only permissions.
func (Perms) PrivateDir(path string) error {
	return os.MkdirAll(path, 0o700)
}

// PrivateFile restricts an existing file to owner-only permissions.
func (Perms) PrivateFile(path string) error {
	return os.Chmod(path, 0o600)
}

// Enforced reports true: unix mode bits take effect.
func (Perms) Enforced() bool {
	return true
}
