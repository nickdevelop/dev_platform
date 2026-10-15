//go:build windows

package windows

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Executables locates commands and tests executability on Windows.
type Executables struct{}

// Resolve finds name on PATH. Lookup follows PATHEXT (opencode.cmd and
// friends), while extensionless sh shims never match: only files carrying a
// PATHEXT extension are accepted.
func (Executables) Resolve(name string) (string, error) {
	found, err := exec.LookPath(name)
	if err != nil {
		return "", err
	}
	if !(Executables{}).IsExecutable(found) {
		return "", fmt.Errorf("%w: %q resolves to %q, which carries no executable extension", exec.ErrNotFound, name, found)
	}
	return found, nil
}

// IsExecutable reports whether path is an existing regular file carrying a
// PATHEXT extension.
func (Executables) IsExecutable(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	ext := strings.ToLower(filepath.Ext(path))
	if ext == "" {
		return false
	}
	for _, candidate := range pathExtList() {
		if ext == candidate {
			return true
		}
	}
	return false
}

// pathExtList returns the executable extensions from PATHEXT, lowercased.
// An empty PATHEXT falls back to the system default set.
func pathExtList() []string {
	raw := os.Getenv("PATHEXT")
	if raw == "" {
		return []string{".com", ".exe", ".bat", ".cmd"}
	}
	var exts []string
	for _, ext := range strings.Split(raw, ";") {
		ext = strings.ToLower(strings.TrimSpace(ext))
		if ext == "" {
			continue
		}
		if !strings.HasPrefix(ext, ".") {
			ext = "." + ext
		}
		exts = append(exts, ext)
	}
	return exts
}
