//go:build unix

package unix

import (
	"os"
	"os/exec"
)

// Executables locates commands and tests executability on unix.
type Executables struct{}

// Resolve finds name on PATH via exec.LookPath.
func (Executables) Resolve(name string) (string, error) {
	return exec.LookPath(name)
}

// IsExecutable reports whether path is an existing, executable regular file.
func (Executables) IsExecutable(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	return info.Mode()&0o111 != 0
}
