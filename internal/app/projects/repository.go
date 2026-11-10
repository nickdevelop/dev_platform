package projects

import (
	"encoding/json"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"devplatform/internal/app/database"
	"devplatform/internal/core/agents"
)

// SettingKeyProjectDisplayConfig is the key used in database.Setting for project display preferences.
const SettingKeyProjectDisplayConfig = "projects.display_config"

// DisplayConfig captures the project ordering and hidden state in the sidebar.
type DisplayConfig struct {
	Order  []uint `json:"order"`
	Hidden []uint `json:"hidden"`
}

// Repository persists and retrieves project records and manages project unregistration.
type Repository interface {
	List() ([]Project, error)
	Get(id uint) (Project, error)
	Create(name, path string) (Project, error)
	Update(id uint, name, path string) (Project, error)
	RemoveProject(id uint) error
	GetDisplayConfig() (DisplayConfig, error)
	SetDisplayConfig(cfg DisplayConfig) error
	ListWorktrees(projectID uint) ([]Worktree, error)
	GetWorktree(projectID, id uint) (Worktree, error)
	CreateWorktree(worktree Worktree) (Worktree, error)
	UpdateWorktreeBranch(projectID, id uint, branch string) error
	DeleteWorktree(projectID, id uint) error
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

		for _, model := range []any{&database.AgentSession{}, &database.AgentInstance{}, &database.ProjectWorktree{}} {
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

func (r *gormRepository) ListWorktrees(projectID uint) ([]Worktree, error) {
	var rows []database.ProjectWorktree
	if err := r.db.Where("project_id = ?", projectID).Order("created_at ASC, id ASC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("projects: list worktrees of %d: %w", projectID, err)
	}
	out := make([]Worktree, 0, len(rows))
	for _, row := range rows {
		out = append(out, worktreeFromModel(row))
	}
	return out, nil
}

func (r *gormRepository) GetWorktree(projectID, id uint) (Worktree, error) {
	var row database.ProjectWorktree
	if err := r.db.Where("id = ? AND project_id = ?", id, projectID).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Worktree{}, ErrWorktreeNotFound
		}
		return Worktree{}, fmt.Errorf("projects: load worktree %d of project %d: %w", id, projectID, err)
	}
	return worktreeFromModel(row), nil
}

func (r *gormRepository) CreateWorktree(worktree Worktree) (Worktree, error) {
	row := database.ProjectWorktree{
		ProjectID:  worktree.ProjectID,
		Path:       worktree.Path,
		Branch:     worktree.Branch,
		BaseBranch: worktree.BaseBranch,
	}
	if err := r.db.Transaction(func(tx *gorm.DB) error { return tx.Create(&row).Error }); err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return Worktree{}, fmt.Errorf("%w: %q", ErrWorktreeExists, worktree.Branch)
		}
		return Worktree{}, fmt.Errorf("projects: record worktree %q: %w", worktree.Path, err)
	}
	return worktreeFromModel(row), nil
}

func (r *gormRepository) UpdateWorktreeBranch(projectID, id uint, branch string) error {
	res := r.db.Model(&database.ProjectWorktree{}).
		Where("id = ? AND project_id = ?", id, projectID).
		UpdateColumn("branch", branch)
	if res.Error != nil {
		return fmt.Errorf("projects: update worktree %d branch: %w", id, res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrWorktreeNotFound
	}
	return nil
}

func (r *gormRepository) DeleteWorktree(projectID, id uint) error {
	res := r.db.Where("id = ? AND project_id = ?", id, projectID).Delete(&database.ProjectWorktree{})
	if res.Error != nil {
		return fmt.Errorf("projects: delete worktree %d: %w", id, res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrWorktreeNotFound
	}
	return nil
}

func worktreeFromModel(row database.ProjectWorktree) Worktree {
	return Worktree{
		ID:         row.ID,
		ProjectID:  row.ProjectID,
		Path:       row.Path,
		Branch:     row.Branch,
		BaseBranch: row.BaseBranch,
		CreatedAt:  row.CreatedAt,
		UpdatedAt:  row.UpdatedAt,
	}
}

func (r *gormRepository) GetDisplayConfig() (DisplayConfig, error) {
	var row database.Setting
	result := r.db.Where("key = ?", SettingKeyProjectDisplayConfig).Limit(1).Find(&row)
	if result.Error != nil {
		return DisplayConfig{Order: []uint{}, Hidden: []uint{}}, fmt.Errorf("projects: load display config: %w", result.Error)
	}
	if result.RowsAffected == 0 || row.Value == "" {
		return DisplayConfig{Order: []uint{}, Hidden: []uint{}}, nil
	}
	var cfg DisplayConfig
	if err := json.Unmarshal([]byte(row.Value), &cfg); err != nil {
		return DisplayConfig{Order: []uint{}, Hidden: []uint{}}, nil
	}
	if cfg.Order == nil {
		cfg.Order = []uint{}
	}
	if cfg.Hidden == nil {
		cfg.Hidden = []uint{}
	}
	return cfg, nil
}

func (r *gormRepository) SetDisplayConfig(cfg DisplayConfig) error {
	if cfg.Order == nil {
		cfg.Order = []uint{}
	}
	if cfg.Hidden == nil {
		cfg.Hidden = []uint{}
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("projects: serialize display config: %w", err)
	}
	row := database.Setting{Key: SettingKeyProjectDisplayConfig, Value: string(data)}
	if err := r.db.Save(&row).Error; err != nil {
		return fmt.Errorf("projects: save display config: %w", err)
	}
	return nil
}
