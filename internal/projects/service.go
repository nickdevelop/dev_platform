// Package projects implements the platform's project records and the safe
// lifecycle of the managed project directories: register, create, rename,
// move, unregister and delete.
//
// Every filesystem operation is confined to the operator-authorized workspace
// roots and always acts on the canonical path returned by
// workspaces.ResolveAuthorizedPath; raw user input is never re-derived into a
// path after authorization. Record changes run in GORM transactions and no
// target is ever overwritten. Most operations change the filesystem first and
// update the record after; the compensating direction differs per operation
// (create removes the new directory on a failed insert, relocate moves it
// back on a failed update). Delete is record-first: the record is removed
// before the directory is touched and restored if the filesystem operation
// fails, so the platform never points at a directory this call deleted.
// Deletion prefers the OS trash; permanent deletion is only performed when
// the caller passes an explicit permanent flag, and never silently.
package projects

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gorm.io/gorm"

	"devplatform/internal/database"
	"devplatform/internal/workspaces"
)

var (
	// ErrInvalidName reports a project name that is empty, reserved, or
	// contains a path separator or NUL byte.
	ErrInvalidName = errors.New("projects: invalid project name")
	// ErrProjectNotFound reports that no project record with the given id
	// exists.
	ErrProjectNotFound = errors.New("projects: project not found")
	// ErrProjectExists reports that the directory is already registered.
	ErrProjectExists = errors.New("projects: path already registered")
	// ErrDirectoryMissing reports that the project directory (or the parent
	// of a requested target) does not exist.
	ErrDirectoryMissing = errors.New("projects: directory does not exist")
	// ErrNotDirectory reports a path that exists but is not a directory.
	ErrNotDirectory = errors.New("projects: path is not a directory")
	// ErrTargetExists reports that the rename/move target already exists; the
	// platform never overwrites it.
	ErrTargetExists = errors.New("projects: target path already exists")
	// ErrInvalidTarget reports a target inside the source directory, which
	// cannot be a valid move destination.
	ErrInvalidTarget = errors.New("projects: invalid target path")
	// ErrConfirmationMismatch reports that the caller's typed confirmation did
	// not match the project's current name.
	ErrConfirmationMismatch = errors.New("projects: confirmation does not match project name")
	// ErrTrashUnavailable reports that the OS trash could not be used, so
	// deletion requires an explicit permanent-delete confirmation.
	ErrTrashUnavailable = errors.New("projects: os trash is unavailable")
	// ErrPathChanged reports that a stored project path no longer resolves to
	// itself inside an authorized root — for example because a path component
	// was replaced by a symlink, or the authorizing root was removed. The path
	// is refused rather than mutated.
	ErrPathChanged = errors.New("projects: stored project path is no longer authorized or canonical")
	// ErrRestoreFailed reports that a failed delete operation could not restore
	// the project record, leaving the record and the directory out of step.
	ErrRestoreFailed = errors.New("projects: failed to restore project record after a failed delete")
	// ErrRollbackFailed reports that a directory was moved but its record could
	// not be updated and the compensating move back also failed.
	ErrRollbackFailed = errors.New("projects: failed to roll back directory move after a record update failure")
)

// Project is the JSON-stable API representation of a managed project. GORM
// models are never serialized directly.
type Project struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"`
}

// Trash moves a directory to the operating system's trash.
type Trash interface {
	Move(path string) error
}

// Option configures a Service.
type Option func(*Service)

// WithTrash overrides the OS trash implementation. It is primarily a test
// seam; production uses the platform's systemTrash.
func WithTrash(t Trash) Option {
	return func(s *Service) {
		if t != nil {
			s.trash = t
		}
	}
}

// Service manages project records and their directories. All directory
// mutations are serialized by mu so two concurrent requests can never both
// observe a free target and race the filesystem.
type Service struct {
	db    *gorm.DB
	trash Trash
	// renamePath renames a directory. It defaults to os.Rename and is a test
	// seam for exercising failure/compensation paths.
	renamePath func(oldpath, newpath string) error
	mu         sync.Mutex
}

// NewService builds the project service over the platform database.
func NewService(db *gorm.DB, opts ...Option) *Service {
	s := &Service{db: db, trash: systemTrash{}, renamePath: os.Rename}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// List returns the registered projects in creation order.
func (s *Service) List() ([]Project, error) {
	var rows []database.Project
	if err := s.db.Order("id ASC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("projects: list projects: %w", err)
	}
	projects := make([]Project, 0, len(rows))
	for _, row := range rows {
		projects = append(projects, projectFromModel(row))
	}
	return projects, nil
}

// Register records an existing directory under an authorized workspace root.
// The directory is never modified; the canonical path is stored and the name
// is derived from the directory's basename. A directory already registered is
// rejected with ErrProjectExists.
func (s *Service) Register(input string) (Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	resolved, err := s.resolve(input)
	if err != nil {
		return Project{}, err
	}
	if err := s.requireDirectory(resolved.Path); err != nil {
		return Project{}, err
	}

	row, err := s.insertProject(resolved.Path, filepath.Base(resolved.Path))
	if err != nil {
		return Project{}, err
	}
	return projectFromModel(row), nil
}

// Create creates a new project directory named name under the authorized
// parent directory parentInput. The name must be a single safe path element;
// the target is derived from the canonical parent and that validated name. An
// existing target is never overwritten.
func (s *Service) Create(parentInput, name string) (Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := validateName(name); err != nil {
		return Project{}, err
	}
	parent, err := s.resolveExistingDirectory(parentInput)
	if err != nil {
		return Project{}, err
	}

	target := filepath.Join(parent.Path, name)
	if err := os.Mkdir(target, 0o755); err != nil {
		if errors.Is(err, os.ErrExist) {
			return Project{}, fmt.Errorf("%w: %q", ErrTargetExists, target)
		}
		return Project{}, fmt.Errorf("projects: create directory %q: %w", target, err)
	}

	row, err := s.insertProject(target, name)
	if err != nil {
		// The directory was created by this call; remove it so a failed
		// record insert does not leave an unmanaged directory behind.
		_ = os.Remove(target)
		return Project{}, err
	}
	return projectFromModel(row), nil
}

// Rename renames the project directory within its current parent. The typed
// confirmation must match the project's current name. An existing target is
// never overwritten.
func (s *Service) Rename(id uint, name, confirmName string) (Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	row, err := s.load(id)
	if err != nil {
		return Project{}, err
	}
	if err := confirmNameMatches(row, confirmName); err != nil {
		return Project{}, err
	}
	if err := s.authorizeStoredPath(row.Path); err != nil {
		return Project{}, err
	}
	if err := validateName(name); err != nil {
		return Project{}, err
	}
	if err := s.requireDirectory(row.Path); err != nil {
		return Project{}, err
	}

	target, err := s.resolve(filepath.Join(filepath.Dir(row.Path), name))
	if err != nil {
		return Project{}, err
	}
	if target.Path == row.Path {
		return projectFromModel(row), nil
	}
	return s.relocate(row, target.Path, name)
}

// Move moves the project directory to the authorized destination path. The
// typed confirmation must match the project's current name. An existing
// destination, or one inside the source directory, is rejected.
func (s *Service) Move(id uint, destinationInput, confirmName string) (Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	row, err := s.load(id)
	if err != nil {
		return Project{}, err
	}
	if err := confirmNameMatches(row, confirmName); err != nil {
		return Project{}, err
	}
	if err := s.authorizeStoredPath(row.Path); err != nil {
		return Project{}, err
	}
	if err := s.requireDirectory(row.Path); err != nil {
		return Project{}, err
	}

	destination, err := s.resolve(destinationInput)
	if err != nil {
		return Project{}, err
	}
	if destination.Path == row.Path {
		return projectFromModel(row), nil
	}
	if pathWithin(row.Path, destination.Path) {
		return Project{}, fmt.Errorf("%w: %q is inside the source %q", ErrInvalidTarget, destination.Path, row.Path)
	}
	parent := filepath.Dir(destination.Path)
	if err := s.requireDirectory(parent); err != nil {
		return Project{}, err
	}
	return s.relocate(row, destination.Path, filepath.Base(destination.Path))
}

// Unregister removes only the platform record; the directory on disk is never
// touched.
func (s *Service) Unregister(id uint) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.deleteRecord(id)
}

// Delete removes the project directory and its record. Preferring the OS
// trash, it moves the directory there; when trash is unavailable (or fails)
// the call fails with ErrTrashUnavailable and the directory is left in place,
// so the caller must re-issue the request with permanent=true after explicit
// confirmation. The typed confirmation must match the project's current name
// in every case.
//
// The record is removed before the directory so the platform can never end up
// holding a record that points at a directory this call deleted. If the
// filesystem operation then fails, the record is restored; if that restore
// itself fails the call reports ErrRestoreFailed rather than a misleading
// success. The stored path is re-authorized against the current roots first,
// so a project whose authorizing root was removed is refused, not deleted.
func (s *Service) Delete(id uint, confirmName string, permanent bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	row, err := s.load(id)
	if err != nil {
		return err
	}
	if err := confirmNameMatches(row, confirmName); err != nil {
		return err
	}
	if err := s.authorizeStoredPath(row.Path); err != nil {
		return err
	}
	if err := s.requireDirectory(row.Path); err != nil {
		return err
	}

	if err := s.deleteRecord(id); err != nil {
		return err
	}

	operationErr := s.removeDirectory(row.Path, permanent)
	if operationErr == nil {
		return nil
	}
	if restoreErr := s.restoreProject(row); restoreErr != nil {
		return fmt.Errorf("%w: delete of %q failed (%v), and record %d could not be restored: %v",
			ErrRestoreFailed, row.Path, operationErr, row.ID, restoreErr)
	}
	return operationErr
}

// removeDirectory performs the destructive half of Delete: the OS trash by
// default, or an explicit permanent removal.
func (s *Service) removeDirectory(path string, permanent bool) error {
	if permanent {
		if err := os.RemoveAll(path); err != nil {
			return fmt.Errorf("projects: permanently delete %q: %w", path, err)
		}
		return nil
	}
	if err := s.trash.Move(path); err != nil {
		return fmt.Errorf("%w: %v", ErrTrashUnavailable, err)
	}
	return nil
}

// relocate moves the directory at row.Path to target and updates the record.
// The target must not exist. If the record update fails, the directory is
// moved back so the record never points at a location that changed silently;
// if that compensating move also fails the call reports ErrRollbackFailed with
// both affected paths rather than discarding the error.
func (s *Service) relocate(row database.Project, target, name string) (Project, error) {
	if _, err := os.Lstat(target); err == nil {
		return Project{}, fmt.Errorf("%w: %q", ErrTargetExists, target)
	} else if !errors.Is(err, os.ErrNotExist) {
		return Project{}, fmt.Errorf("projects: inspect target %q: %w", target, err)
	}

	if err := s.renamePath(row.Path, target); err != nil {
		return Project{}, fmt.Errorf("projects: move %q to %q: %w", row.Path, target, err)
	}

	previous := row.Path
	row.Path, row.Name = target, name
	if err := s.db.Transaction(func(tx *gorm.DB) error { return tx.Save(&row).Error }); err != nil {
		if rollbackErr := s.renamePath(target, previous); rollbackErr != nil {
			return Project{}, fmt.Errorf("%w: directory now at %q but record %d still points at %q (%v); record update error: %v",
				ErrRollbackFailed, target, row.ID, previous, rollbackErr, err)
		}
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return Project{}, fmt.Errorf("%w: %q", ErrProjectExists, target)
		}
		return Project{}, fmt.Errorf("projects: update record for %q: %w", target, err)
	}
	return projectFromModel(row), nil
}

// restoreProject re-inserts a project record after a failed delete, preserving
// its identity and timestamps.
func (s *Service) restoreProject(row database.Project) error {
	return s.db.Transaction(func(tx *gorm.DB) error { return tx.Create(&row).Error })
}

// insertProject records a directory as a project in its own transaction.
func (s *Service) insertProject(path, name string) (database.Project, error) {
	row := database.Project{Name: name, Path: path}
	if err := s.db.Transaction(func(tx *gorm.DB) error { return tx.Create(&row).Error }); err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return database.Project{}, fmt.Errorf("%w: %q", ErrProjectExists, path)
		}
		return database.Project{}, fmt.Errorf("projects: record %q: %w", path, err)
	}
	return row, nil
}

// deleteRecord removes a project record in its own transaction.
func (s *Service) deleteRecord(id uint) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		result := tx.Delete(&database.Project{}, id)
		if result.Error != nil {
			return fmt.Errorf("projects: delete record %d: %w", id, result.Error)
		}
		if result.RowsAffected == 0 {
			return ErrProjectNotFound
		}
		return nil
	})
}

// load reads the project record with the given id.
func (s *Service) load(id uint) (database.Project, error) {
	var row database.Project
	if err := s.db.First(&row, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return database.Project{}, ErrProjectNotFound
		}
		return database.Project{}, fmt.Errorf("projects: load project %d: %w", id, err)
	}
	return row, nil
}

// resolve authorizes input against the registered workspace roots and returns
// the canonical path to operate on.
func (s *Service) resolve(input string) (workspaces.ResolvedPath, error) {
	roots, err := s.authorizedRoots()
	if err != nil {
		return workspaces.ResolvedPath{}, err
	}
	resolved, err := workspaces.ResolveAuthorizedPath(input, roots)
	if err != nil {
		return workspaces.ResolvedPath{}, err
	}
	return resolved, nil
}

// authorizeStoredPath re-authorizes a path already stored in the database
// against the currently configured workspace roots before it is mutated. It
// additionally requires the canonical resolution to be unchanged, so a stored
// path whose components were replaced by symlinks (or whose authorizing root
// was removed) is rejected instead of mutated.
func (s *Service) authorizeStoredPath(path string) error {
	resolved, err := s.resolve(path)
	if err != nil {
		return err
	}
	if resolved.Path != path {
		return fmt.Errorf("%w: %q now resolves to %q", ErrPathChanged, path, resolved.Path)
	}
	return nil
}

// resolveExistingDirectory authorizes input and additionally requires it to be
// an existing directory.
func (s *Service) resolveExistingDirectory(input string) (workspaces.ResolvedPath, error) {
	resolved, err := s.resolve(input)
	if err != nil {
		return workspaces.ResolvedPath{}, err
	}
	if err := s.requireDirectory(resolved.Path); err != nil {
		return workspaces.ResolvedPath{}, err
	}
	return resolved, nil
}

// requireDirectory fails unless path exists and is a directory.
func (s *Service) requireDirectory(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%w: %q", ErrDirectoryMissing, path)
		}
		return fmt.Errorf("projects: inspect %q: %w", path, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%w: %q", ErrNotDirectory, path)
	}
	return nil
}

// authorizedRoots loads the registered workspace roots from the database.
func (s *Service) authorizedRoots() ([]string, error) {
	var rows []database.WorkspaceRoot
	if err := s.db.Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("projects: load workspace roots: %w", err)
	}
	roots := make([]string, 0, len(rows))
	for _, row := range rows {
		roots = append(roots, row.Path)
	}
	return roots, nil
}

// validateName rejects names that are empty, reserved, or would name a path
// other than a single direct child directory.
func validateName(name string) error {
	switch {
	case name == "":
		return fmt.Errorf("%w: name is empty", ErrInvalidName)
	case name == "." || name == "..":
		return fmt.Errorf("%w: %q is reserved", ErrInvalidName, name)
	case strings.ContainsAny(name, `/\`):
		return fmt.Errorf("%w: %q contains a path separator", ErrInvalidName, name)
	case strings.ContainsRune(name, 0):
		return fmt.Errorf("%w: %q contains a NUL byte", ErrInvalidName, name)
	default:
		return nil
	}
}

// confirmNameMatches requires the caller to have typed the project's current
// name before a destructive or relocating operation.
func confirmNameMatches(row database.Project, confirmName string) error {
	if confirmName != row.Name {
		return fmt.Errorf("%w: want %q", ErrConfirmationMismatch, row.Name)
	}
	return nil
}

// pathWithin reports whether child lies strictly inside parent.
func pathWithin(parent, child string) bool {
	return strings.HasPrefix(child, parent+string(filepath.Separator))
}

// projectFromModel maps the persistence model to the JSON-stable DTO.
func projectFromModel(row database.Project) Project {
	return Project{ID: row.ID, Name: row.Name, Path: row.Path}
}
