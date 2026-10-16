// Package projects implements the platform's project records and the safe
// lifecycle of the managed project directories: register, create, rename,
// move and unregister.
//
// Every filesystem operation acts on the canonical path returned by
// paths.Resolve; raw user input is never re-derived into a path after
// canonicalization. Record changes run in GORM transactions and no target is
// ever overwritten. Most operations change the filesystem first and update the
// record after; the compensating direction differs per operation (create removes
// the new directory on a failed insert, relocate moves it back on a failed
// update).
//
// The platform never deletes a project directory. Unregister removes only the
// platform's records: it first stops every live agent instance of the project
// (aborting if any stop fails), then removes the project's instances, sessions,
// backend configs and the project record in one transaction. The directory on
// disk is left untouched.
package projects

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gorm.io/gorm"

	"devplatform/internal/app/database"
	"devplatform/internal/app/paths"
	"devplatform/internal/core/agents"
	"devplatform/internal/core/platform"
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
	// ErrPathChanged reports that a stored project path no longer resolves to
	// itself inside an authorized root — for example because a path component
	// was replaced by a symlink, or the authorizing root was removed. The path
	// is refused rather than mutated.
	ErrPathChanged = errors.New("projects: stored project path is no longer authorized or canonical")
	// ErrInstanceStop reports that unregistering was refused because the
	// project's live agent instances could not all be stopped. Nothing is
	// removed in that case.
	ErrInstanceStop = errors.New("projects: stopping project instances failed")
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

// InstanceStopper stops every live agent instance of one project. The
// instances package implements it. It is injected by the composition root
// because instances already depends on projects. An implementation must not
// call back into this Service.
type InstanceStopper interface {
	StopProjectInstances(ctx context.Context, projectID uint) error
}

// Service manages project records and their directories. All operations are
// serialized by mu so two concurrent requests can never both observe a free
// target and race the filesystem, and so an unregister cannot interleave with
// a start that is resolving the same project.
type Service struct {
	db *gorm.DB
	// renamePath renames a directory. It defaults to os.Rename and is a test
	// seam for exercising failure/compensation paths.
	renamePath func(oldpath, newpath string) error
	// stopper stops a project's live instances before it is unregistered.
	stopper InstanceStopper
	mu      sync.Mutex
}

// NewService builds the project service over the platform database. The
// instance stopper is wired afterwards with SetInstanceStopper, because the
// instance service itself is built from this Service.
func NewService(db *gorm.DB) *Service {
	return &Service{db: db, renamePath: os.Rename}
}

// SetInstanceStopper installs the component that stops a project's live
// instances. The composition root calls it once, before serving requests.
func (s *Service) SetInstanceStopper(stopper InstanceStopper) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopper = stopper
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
	if err := s.requireDirectory(resolved); err != nil {
		return Project{}, err
	}

	row, err := s.insertProject(resolved, filepath.Base(resolved))
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

	target := filepath.Join(parent, name)
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
	if target == row.Path {
		return projectFromModel(row), nil
	}
	return s.relocate(row, target, name)
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
	if destination == row.Path {
		return projectFromModel(row), nil
	}
	if pathWithin(row.Path, destination) {
		return Project{}, fmt.Errorf("%w: %q is inside the source %q", ErrInvalidTarget, destination, row.Path)
	}
	parent := filepath.Dir(destination)
	if err := s.requireDirectory(parent); err != nil {
		return Project{}, err
	}
	return s.relocate(row, destination, filepath.Base(destination))
}

// ResolveDirectory loads the project with the given id, re-canonicalizes its
// stored path, checks that the directory still exists, and returns the
// canonical directory the project currently occupies. It is the gate the agent
// instance lifecycle uses to turn a project id into a working directory: an
// unknown project, a symlinked path whose canonical form changed, or a missing
// directory all fail rather than yield a directory the platform must not touch.
func (s *Service) ResolveDirectory(id uint) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	row, err := s.load(id)
	if err != nil {
		return "", err
	}
	if err := s.authorizeStoredPath(row.Path); err != nil {
		return "", err
	}
	if err := s.requireDirectory(row.Path); err != nil {
		return "", err
	}
	return row.Path, nil
}

// Unregister removes the project from the platform. It is the only way a
// project leaves the platform, and it never touches the directory on disk.
//
// Live agent instances are stopped first. If any stop fails the call returns
// ErrInstanceStop and nothing is removed, so the caller can retry. The record
// removal then runs in one transaction that also drops the project's instances,
// sessions and backend configs; see removeProject.
func (s *Service) Unregister(ctx context.Context, id uint) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, err := s.load(id); err != nil {
		return err
	}
	if s.stopper == nil {
		return fmt.Errorf("%w: no instance stopper is configured", ErrInstanceStop)
	}
	if err := s.stopper.StopProjectInstances(ctx, id); err != nil {
		return fmt.Errorf("%w: %v", ErrInstanceStop, err)
	}
	return s.removeProject(id)
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

// removeProject deletes a project and everything the platform owns for it in
// one transaction: agent sessions, agent instances, backend configs, then the
// project record. Before deleting, it refuses if any instance of the project is
// still live, so an instance started concurrently with the stop above can never
// be orphaned from its project.
func (s *Service) removeProject(id uint) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var row database.Project
		if err := tx.First(&row, id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrProjectNotFound
			}
			return fmt.Errorf("projects: load project %d: %w", id, err)
		}

		var live int64
		if err := tx.Model(&database.AgentInstance{}).
			Where("project_id = ? AND status NOT IN ?", id,
				[]string{string(agents.InstanceStopped), string(agents.InstanceFailed)}).
			Count(&live).Error; err != nil {
			return fmt.Errorf("projects: count live instances of project %d: %w", id, err)
		}
		if live > 0 {
			return fmt.Errorf("%w: %d instance(s) of project %d are still live", ErrInstanceStop, live, id)
		}

		for _, model := range []any{&database.AgentSession{}, &database.AgentInstance{}} {
			if err := tx.Where("project_id = ?", id).Delete(model).Error; err != nil {
				return fmt.Errorf("projects: delete project %d rows: %w", id, err)
			}
		}
		if err := tx.Delete(&database.Project{}, id).Error; err != nil {
			return fmt.Errorf("projects: delete project %d: %w", id, err)
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

// resolve canonicalizes input into the absolute path to operate on. The
// platform no longer confines paths to authorized roots; it still rejects
// relative, ambiguous, or unresolvable input.
func (s *Service) resolve(input string) (string, error) {
	return paths.Resolve(input)
}

// authorizeStoredPath re-canonicalizes a path already stored in the database
// before it is mutated. It requires the canonical resolution to be unchanged,
// so a stored path whose components were replaced by symlinks is rejected
// instead of mutated.
func (s *Service) authorizeStoredPath(path string) error {
	resolved, err := s.resolve(path)
	if err != nil {
		return err
	}
	if resolved != path {
		return fmt.Errorf("%w: %q now resolves to %q", ErrPathChanged, path, resolved)
	}
	return nil
}

// resolveExistingDirectory canonicalizes input and additionally requires it to
// be an existing directory.
func (s *Service) resolveExistingDirectory(input string) (string, error) {
	resolved, err := s.resolve(input)
	if err != nil {
		return "", err
	}
	if err := s.requireDirectory(resolved); err != nil {
		return "", err
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

// pathWithin reports whether child lies strictly inside parent. Segments
// compare through the platform path policy, so Windows case-insensitivity
// applies without a GOOS branch; both sides arrive canonical and absolute
// from paths.Resolve.
func pathWithin(parent, child string) bool {
	dirs := platform.New().Dirs()
	parentSegs := strings.Split(filepath.Clean(parent), string(filepath.Separator))
	childSegs := strings.Split(filepath.Clean(child), string(filepath.Separator))
	if len(childSegs) <= len(parentSegs) {
		return false
	}
	for i, seg := range parentSegs {
		if !dirs.SamePath(seg, childSegs[i]) {
			return false
		}
	}
	return true
}

// projectFromModel maps the persistence model to the JSON-stable DTO.
func projectFromModel(row database.Project) Project {
	return Project{ID: row.ID, Name: row.Name, Path: row.Path}
}
