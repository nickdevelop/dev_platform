package notes

import (
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"

	"devplatform/internal/app/database"
)

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func toDB(n Note) database.Note {
	return database.Note{
		ID:        n.ID,
		ProjectID: n.ProjectID,
		Title:     n.Title,
		Content:   n.Content,
		CreatedAt: n.CreatedAt,
		UpdatedAt: n.UpdatedAt,
	}
}

func fromDB(row database.Note) Note {
	return Note{
		ID:        row.ID,
		ProjectID: row.ProjectID,
		Title:     row.Title,
		Content:   row.Content,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
}

func (r *Repository) Create(n Note) (Note, error) {
	row := toDB(n)
	if row.CreatedAt.IsZero() {
		row.CreatedAt = time.Now().UTC()
	}
	if row.UpdatedAt.IsZero() {
		row.UpdatedAt = row.CreatedAt
	}
	if err := r.db.Create(&row).Error; err != nil {
		return Note{}, err
	}
	return fromDB(row), nil
}

func (r *Repository) Get(id uint) (Note, error) {
	var row database.Note
	if err := r.db.First(&row, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Note{}, ErrNoteNotFound
		}
		return Note{}, err
	}
	return fromDB(row), nil
}

func (r *Repository) List(keyword string, projectID *uint) ([]Note, error) {
	var rows []database.Note
	q := r.db.Model(&database.Note{})
	if projectID != nil {
		q = q.Where("project_id = ?", *projectID)
	}
	if trimmed := strings.TrimSpace(keyword); trimmed != "" {
		pattern := "%" + trimmed + "%"
		q = q.Where("title LIKE ? OR content LIKE ?", pattern, pattern)
	}
	if err := q.Order("updated_at DESC, id DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]Note, 0, len(rows))
	for _, row := range rows {
		out = append(out, fromDB(row))
	}
	return out, nil
}

func (r *Repository) Update(n Note) (Note, error) {
	row := toDB(n)
	row.UpdatedAt = time.Now().UTC()
	if err := r.db.Save(&row).Error; err != nil {
		return Note{}, err
	}
	return fromDB(row), nil
}

func (r *Repository) Delete(id uint) error {
	res := r.db.Delete(&database.Note{}, "id = ?", id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNoteNotFound
	}
	return nil
}
