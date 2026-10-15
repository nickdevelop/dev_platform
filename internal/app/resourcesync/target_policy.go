package resourcesync

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// resolvePath resolves a target to a canonical absolute path that is strictly
// inside the project directory. The relative path is validated the same way
// the contract validates it, and the joined path is prefix-checked against
// the project directory, so a target can never escape via traversal,
// absolute paths or separator tricks.
func (m *Manager) resolvePath(target ResourceTarget) (string, error) {
	if target.ProjectID == 0 {
		return "", fmt.Errorf("%w: project id is zero", ErrInvalidTarget)
	}
	if target.BackendID != "" && !target.BackendID.Valid() {
		return "", fmt.Errorf("%w: unknown backend %q", ErrInvalidTarget, target.BackendID)
	}
	if err := validateRelativePath(target.Path); err != nil {
		return "", err
	}

	projectDir, err := m.resolver.ResolveDirectory(target.ProjectID)
	if err != nil {
		return "", err
	}
	if projectDir == "" {
		return "", fmt.Errorf("resourcesync: project %d resolved to an empty directory", target.ProjectID)
	}

	abs := filepath.Join(projectDir, filepath.FromSlash(target.Path))
	if !strings.HasPrefix(abs, projectDir+string(os.PathSeparator)) {
		return "", fmt.Errorf("%w: path %q escapes project directory", ErrInvalidTarget, target.Path)
	}
	return abs, nil
}
