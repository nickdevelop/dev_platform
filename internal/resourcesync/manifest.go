package resourcesync

import (
	"errors"
	"fmt"
	"os"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"devplatform/internal/agents"
	"devplatform/internal/database"
)

// ErrConflict reports a managed-file operation refused because the target is
// in a state that needs explicit user resolution (unmanaged target, external
// drift, or an unwritable target).
var ErrConflict = errors.New("resourcesync: managed file conflict")

// ProjectResolver turns a project id into its canonical, authorized
// directory. It is satisfied by the project service and by test fakes.
type ProjectResolver interface {
	ResolveDirectory(projectID uint) (string, error)
}

// Manifest is the platform's ownership record for one managed target file.
type Manifest struct {
	ProjectID       uint
	Backend         agents.BackendID
	Path            string
	ManagedHash     string
	BackupPath      string
	ResourceVersion string
	UpdatedAt       time.Time
}

// Manager owns the managed-file lifecycle: it previews the effect of a patch,
// detects unmanaged targets and external drift, backs up prior content, and
// writes atomically. Ownership is recorded only after a successful apply.
type Manager struct {
	db        *gorm.DB
	resolver  ProjectResolver
	backupDir string
	now       func() time.Time
	// writeFile is the write primitive; a test seam for exercising failure
	// and rollback.
	writeFile func(path string, data []byte, mode os.FileMode) error
}

// NewManager builds a managed-file manager. backupDir is where prior content
// is preserved; it should live under the platform data directory, never
// inside a managed project.
func NewManager(db *gorm.DB, resolver ProjectResolver, backupDir string) *Manager {
	return &Manager{
		db:        db,
		resolver:  resolver,
		backupDir: backupDir,
		now:       func() time.Time { return time.Now().UTC() },
		writeFile: atomicWriteFile,
	}
}

// Manifest returns the ownership record for a target, if the platform owns it.
func (m *Manager) Manifest(target ResourceTarget) (Manifest, bool, error) {
	var row database.ManagedResourceFile
	err := m.db.
		Where("project_id = ? AND backend = ? AND path = ?", target.ProjectID, string(target.BackendID), target.Path).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Manifest{}, false, nil
	}
	if err != nil {
		return Manifest{}, false, fmt.Errorf("resourcesync: load manifest: %w", err)
	}
	return Manifest{
		ProjectID:       row.ProjectID,
		Backend:         agents.BackendID(row.Backend),
		Path:            row.Path,
		ManagedHash:     row.ManagedHash,
		BackupPath:      row.BackupPath,
		ResourceVersion: row.ResourceVersion,
		UpdatedAt:       row.UpdatedAt,
	}, true, nil
}

// saveManifest upserts the ownership record for a target.
func (m *Manager) saveManifest(target ResourceTarget, managedHash, backupPath, version string) error {
	row := database.ManagedResourceFile{
		ProjectID:       target.ProjectID,
		Backend:         string(target.BackendID),
		Path:            target.Path,
		ManagedHash:     managedHash,
		BackupPath:      backupPath,
		ResourceVersion: version,
		UpdatedAt:       m.now(),
	}
	err := m.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "project_id"}, {Name: "backend"}, {Name: "path"}},
		DoUpdates: clause.AssignmentColumns([]string{"managed_hash", "backup_path", "resource_version", "updated_at"}),
	}).Create(&row).Error
	if err != nil {
		return fmt.Errorf("resourcesync: save manifest: %w", err)
	}
	return nil
}

// Preview reports what applying the patch would do without touching the
// filesystem. It returns a conflict preview for an unmanaged target, an
// externally edited managed target, or a target that cannot be written; a
// pending preview for a first apply; and an applied preview for a safe
// update.
func (m *Manager) Preview(patch ManagedPatch) (SyncPreview, error) {
	abs, err := m.resolvePath(patch.Target)
	if err != nil {
		return SyncPreview{}, err
	}

	info, statErr := os.Lstat(abs)
	exists := statErr == nil
	if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return SyncPreview{}, fmt.Errorf("resourcesync: inspect target: %w", statErr)
	}
	if exists {
		if info.Mode()&os.ModeSymlink != 0 {
			return m.conflictPreview(patch, ConflictTargetInvalid, "target is a symlink; refusing to follow it"), nil
		}
		if info.IsDir() {
			return m.conflictPreview(patch, ConflictTargetInvalid, "target is a directory, not a file"), nil
		}
	}

	manifest, owned, err := m.Manifest(patch.Target)
	if err != nil {
		return SyncPreview{}, err
	}

	if !exists {
		return SyncPreview{
			ResourceID:    patch.ResourceID,
			Kind:          patch.Kind,
			Target:        patch.Target,
			State:         SyncStatePending,
			ChangeSummary: patch.Summary,
		}, nil
	}
	if !owned {
		return m.conflictPreview(patch, ConflictUnmanagedTarget, "target exists and is not platform-managed"), nil
	}

	current, err := os.ReadFile(abs)
	if err != nil {
		return SyncPreview{}, fmt.Errorf("resourcesync: read target: %w", err)
	}
	if hashContent(current) != manifest.ManagedHash {
		return m.conflictPreview(patch, ConflictExternalDrift, "target was edited outside the platform"), nil
	}
	return SyncPreview{
		ResourceID:    patch.ResourceID,
		Kind:          patch.Kind,
		Target:        patch.Target,
		State:         SyncStateApplied,
		ChangeSummary: patch.Summary,
	}, nil
}

// Apply writes the patch. It refuses when the preview reports a conflict
// unless force is set (a user resolution). Prior content is backed up before
// the write, and a failed write or manifest update rolls the target back to
// the last known-good content.
func (m *Manager) Apply(patch ManagedPatch, force bool) (SyncPreview, error) {
	preview, err := m.Preview(patch)
	if err != nil {
		return SyncPreview{}, err
	}
	if preview.State == SyncStateConflict && !force {
		return preview, fmt.Errorf("%w: %s", ErrConflict, preview.Conflict.Kind)
	}

	abs, err := m.resolvePath(patch.Target)
	if err != nil {
		return SyncPreview{}, err
	}
	_, statErr := os.Lstat(abs)
	existed := statErr == nil
	if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return SyncPreview{}, fmt.Errorf("resourcesync: inspect target: %w", statErr)
	}

	var backupPath string
	if existed {
		backupPath, err = m.backupFile(abs)
		if err != nil {
			return SyncPreview{}, err
		}
	}

	if err := m.writeFile(abs, patch.Content, 0o644); err != nil {
		m.rollback(abs, existed, backupPath)
		return SyncPreview{}, err
	}
	if err := m.saveManifest(patch.Target, hashContent(patch.Content), backupPath, patch.ResourceVersion); err != nil {
		m.rollback(abs, existed, backupPath)
		return SyncPreview{}, err
	}

	return SyncPreview{
		ResourceID:    patch.ResourceID,
		Kind:          patch.Kind,
		Target:        patch.Target,
		State:         SyncStateApplied,
		ChangeSummary: patch.Summary,
	}, nil
}

// rollback restores the last known-good content after a failed apply.
func (m *Manager) rollback(abs string, existed bool, backupPath string) {
	if existed && backupPath != "" {
		_ = restoreFile(backupPath, abs)
		return
	}
	_ = os.Remove(abs)
}

// conflictPreview builds a conflict preview for a patch.
func (m *Manager) conflictPreview(patch ManagedPatch, kind ConflictKind, detail string) SyncPreview {
	conflict := &SyncConflict{
		Kind:       kind,
		ResourceID: patch.ResourceID,
		Target:     patch.Target,
		Detail:     detail,
	}
	return SyncPreview{
		ResourceID:    patch.ResourceID,
		Kind:          patch.Kind,
		Target:        patch.Target,
		State:         SyncStateConflict,
		ChangeSummary: patch.Summary,
		Conflict:      conflict,
	}
}
