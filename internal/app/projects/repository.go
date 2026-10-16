package projects

import (
	"errors"
	"fmt"

	"gorm.io/gorm"

	"devplatform/internal/app/database"
	"devplatform/internal/core/agents"
)

// Repository persists and retrieves project records and manages project unregistration.
type Repository interface {
	List() ([]Project, error)
	Get(id uint) (Project, error)
	Create(name, path string) (Project, error)
	Update(id uint, name, path string) (Project, error)
	RemoveProject(id uint) error
}

type gormRepository struct {
	db *gorm.DB
}

// NewGORMRepository creates a GORM-backed Repository for projects.
func NewGORMRepository(db *gorm.DB) Repository {
	return &gormRepository{db: db}
}

func (r *gormRepository) List() ([]Project, error) {
	var rows []database.Project
	if err := r.db.Order("id ASC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("projects: list projects: %w", err)
	}
	out := make([]Project, 0, len(rows))
	for _, row := range rows {
		out = append(out, projectFromModel(row))
	}
	return out, nil
}

func (r *gormRepository) Get(id uint) (Project, error) {
	var row database.Project
	if err := r.db.First(&row, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Project{}, ErrProjectNotFound
		}
		return Project{}, fmt.Errorf("projects: load project %d: %w", id, err)
	}
	return projectFromModel(row), nil
}

func (r *gormRepository) Create(name, path string) (Project, error) {
	row := database.Project{Name: name, Path: path}
	if err := r.db.Transaction(func(tx *gorm.DB) error { return tx.Create(&row).Error }); err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return Project{}, fmt.Errorf("%w: %q", ErrProjectExists, path)
		}
		return Project{}, fmt.Errorf("projects: record %q: %w", path, err)
	}
	return projectFromModel(row), nil
}

func (r *gormRepository) Update(id uint, name, path string) (Project, error) {
	var row database.Project
	if err := r.db.First(&row, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Project{}, ErrProjectNotFound
		}
		return Project{}, fmt.Errorf("projects: load project %d: %w", id, err)
	}
	row.Name = name
	row.Path = path
	if err := r.db.Transaction(func(tx *gorm.DB) error { return tx.Save(&row).Error }); err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return Project{}, fmt.Errorf("%w: %q", ErrProjectExists, path)
		}
		return Project{}, fmt.Errorf("projects: update record for %q: %w", path, err)
	}
	return projectFromModel(row), nil
}

func (r *gormRepository) RemoveProject(id uint) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
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

// projectFromModel maps the persistence model to the JSON-stable DTO.
func projectFromModel(row database.Project) Project {
	return Project{ID: row.ID, Name: row.Name, Path: row.Path}
}
