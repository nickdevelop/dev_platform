package roles

import (
	"errors"
	"strings"

	"gorm.io/gorm"

	"devplatform/internal/app/database"
	"devplatform/internal/core/agents"
)

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func toDB(r Role) database.AgentRole {
	return database.AgentRole{
		ID:           r.ID,
		Name:         r.Name,
		Description:  r.Description,
		Backend:      string(r.Backend),
		Model:        r.Model,
		PromptID:     r.PromptID,
		SystemPrompt: r.SystemPrompt,
		Icon:         r.Icon,
		Color:        r.Color,
		CreatedAt:    r.CreatedAt,
		UpdatedAt:    r.UpdatedAt,
	}
}

func fromDB(row database.AgentRole) Role {
	return Role{
		ID:           row.ID,
		Name:         row.Name,
		Description:  row.Description,
		Backend:      agents.BackendID(row.Backend),
		Model:        row.Model,
		PromptID:     row.PromptID,
		SystemPrompt: row.SystemPrompt,
		Icon:         row.Icon,
		Color:        row.Color,
		CreatedAt:    row.CreatedAt,
		UpdatedAt:    row.UpdatedAt,
	}
}

func (r *Repository) Create(role Role) (Role, error) {
	row := toDB(role)
	if err := r.db.Create(&row).Error; err != nil {
		return Role{}, err
	}
	return fromDB(row), nil
}

func (r *Repository) Get(id uint) (Role, error) {
	var row database.AgentRole
	if err := r.db.First(&row, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Role{}, ErrRoleNotFound
		}
		return Role{}, err
	}
	return fromDB(row), nil
}

func (r *Repository) List(keyword string) ([]Role, error) {
	var rows []database.AgentRole
	q := r.db.Model(&database.AgentRole{})
	if strings.TrimSpace(keyword) != "" {
		pattern := "%" + strings.TrimSpace(keyword) + "%"
		q = q.Where("name LIKE ? OR description LIKE ? OR model LIKE ?", pattern, pattern, pattern)
	}
	if err := q.Order("id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]Role, len(rows))
	for i, row := range rows {
		out[i] = fromDB(row)
	}
	return out, nil
}

func (r *Repository) Update(role Role) error {
	row := toDB(role)
	res := r.db.Model(&database.AgentRole{}).Where("id = ?", role.ID).Updates(map[string]any{
		"name":          row.Name,
		"description":   row.Description,
		"backend":       row.Backend,
		"model":         row.Model,
		"prompt_id":     row.PromptID,
		"system_prompt": row.SystemPrompt,
		"icon":          row.Icon,
		"color":         row.Color,
		"updated_at":    row.UpdatedAt,
	})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrRoleNotFound
	}
	return nil
}

func (r *Repository) Delete(id uint) error {
	res := r.db.Delete(&database.AgentRole{}, "id = ?", id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrRoleNotFound
	}
	return nil
}
