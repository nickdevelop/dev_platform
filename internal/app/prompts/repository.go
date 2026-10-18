package prompts

import (
	"errors"
	"strings"

	"gorm.io/gorm"

	"devplatform/internal/app/database"
)

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func toDB(p Prompt) database.Prompt {
	return database.Prompt{
		ID:          p.ID,
		Name:        p.Name,
		Description: p.Description,
		Content:     p.Content,
		CreatedAt:   p.CreatedAt,
		UpdatedAt:   p.UpdatedAt,
	}
}

func fromDB(row database.Prompt) Prompt {
	return Prompt{
		ID:          row.ID,
		Name:        row.Name,
		Description: row.Description,
		Content:     row.Content,
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
	}
}

func (r *Repository) Create(p Prompt) (Prompt, error) {
	row := toDB(p)
	if err := r.db.Create(&row).Error; err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return Prompt{}, ErrDuplicatePromptName
		}
		return Prompt{}, err
	}
	return fromDB(row), nil
}

func (r *Repository) Get(id uint) (Prompt, error) {
	var row database.Prompt
	if err := r.db.First(&row, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Prompt{}, ErrPromptNotFound
		}
		return Prompt{}, err
	}
	return fromDB(row), nil
}

func (r *Repository) List(keyword string) ([]Prompt, error) {
	var rows []database.Prompt
	q := r.db.Model(&database.Prompt{})
	if strings.TrimSpace(keyword) != "" {
		pattern := "%" + strings.TrimSpace(keyword) + "%"
		q = q.Where("name LIKE ? OR description LIKE ?", pattern, pattern)
	}
	if err := q.Order("id DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]Prompt, 0, len(rows))
	for _, row := range rows {
		out = append(out, fromDB(row))
	}
	return out, nil
}

func (r *Repository) Update(p Prompt) (Prompt, error) {
	row := toDB(p)
	if err := r.db.Save(&row).Error; err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return Prompt{}, ErrDuplicatePromptName
		}
		return Prompt{}, err
	}
	return fromDB(row), nil
}

func (r *Repository) Delete(id uint) error {
	res := r.db.Delete(&database.Prompt{}, "id = ?", id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrPromptNotFound
	}
	return nil
}
