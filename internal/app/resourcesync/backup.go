package resourcesync

import (
	"fmt"
	"os"
	"path/filepath"

	"devplatform/internal/core/platform"
)

// backupFile copies the current target content into the manager's backup
// directory and returns the backup path. The backup is written atomically
// with owner-only permissions. The backup directory holds platform data, not
// project content.
func (m *Manager) backupFile(source string) (string, error) {
	if m.backupDir == "" {
		return "", fmt.Errorf("resourcesync: backup directory is not configured")
	}
	data, err := os.ReadFile(source)
	if err != nil {
		return "", fmt.Errorf("resourcesync: read target for backup: %w", err)
	}
	if err := platform.New().Perms().PrivateDir(m.backupDir); err != nil {
		return "", fmt.Errorf("resourcesync: create backup directory: %w", err)
	}

	name := fmt.Sprintf("%s-%s.bak", hashContent([]byte(source))[:16],
		m.now().Format("20060102T150405.000000000"))
	dest := filepath.Join(m.backupDir, name)
	if err := atomicWriteFile(dest, data, 0o600); err != nil {
		return "", fmt.Errorf("resourcesync: write backup: %w", err)
	}
	// Backups are as sensitive as the database: enforce owner-only
	// permissions through the port even where the platform degrades them.
	if err := platform.New().Perms().PrivateFile(dest); err != nil {
		return "", fmt.Errorf("resourcesync: restrict backup: %w", err)
	}
	return dest, nil
}

// restoreFile copies a backup back over the target, used to roll back a
// failed apply.
func restoreFile(backupPath, target string) error {
	data, err := os.ReadFile(backupPath)
	if err != nil {
		return fmt.Errorf("resourcesync: read backup: %w", err)
	}
	return atomicWriteFile(target, data, 0o644)
}

// cleanStaleTemps removes temp files left by an interrupted write. A leftover
// temp never carries the target's name, so removing it is safe; failures are
// ignored because the target itself is untouched.
func cleanStaleTemps(dir string) {
	matches, err := filepath.Glob(filepath.Join(dir, ".managed-*.tmp"))
	if err != nil {
		return
	}
	for _, match := range matches {
		_ = os.Remove(match)
	}
}

// atomicWriteFile writes data to path by writing a temp file in the same
// directory, syncing it and renaming it over the target, so a crash never
// leaves a partial file. Missing parent directories are created.
func atomicWriteFile(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("resourcesync: create target directory: %w", err)
	}
	cleanStaleTemps(dir)
	tmp, err := os.CreateTemp(dir, ".managed-*.tmp")
	if err != nil {
		return fmt.Errorf("resourcesync: create temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once the rename succeeds

	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("resourcesync: set target permissions: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("resourcesync: write temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("resourcesync: sync temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("resourcesync: close temp file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("resourcesync: install target: %w", err)
	}
	return nil
}
