//go:build windows

package windows

import (
	"os"
)

// Perms accepts the documented degradation on Windows: no ACL tightening
// happens, so PrivateDir and PrivateFile only ensure existence. Protection
// of secrets.json and friends relies on the user profile directory's
// default ACL instead; the startup log says so on every boot (see
// Environment.Describe) rather than silently degrading.
type Perms struct{}

// PrivateDir creates path, inheriting the parent ACL. The mode is ignored
// on Windows.
func (Perms) PrivateDir(path string) error {
	return os.MkdirAll(path, 0o777)
}

// PrivateFile ensures path exists without touching its ACL, mirroring the
// unix failure when the file is missing.
func (Perms) PrivateFile(path string) error {
	if _, err := os.Stat(path); err != nil {
		return err
	}
	return nil
}

// Enforced reports false: Windows performs no private-permission
// enforcement.
func (Perms) Enforced() bool {
	return false
}
