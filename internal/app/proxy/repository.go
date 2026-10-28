package proxy

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

func toDB(p ServiceProxy) database.ServiceProxy {
	return database.ServiceProxy{
		ID:        p.ID,
		Name:      p.Name,
		Alias:     p.Alias,
		Type:      p.Type,
		Target:    p.Target,
		Enabled:   p.Enabled,
		CreatedAt: p.CreatedAt,
		UpdatedAt: p.UpdatedAt,
	}
}

func fromDB(row database.ServiceProxy) ServiceProxy {
	return ServiceProxy{
		ID:        row.ID,
		Name:      row.Name,
		Alias:     row.Alias,
		Type:      row.Type,
		Target:    row.Target,
		Enabled:   row.Enabled,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
}

func (r *Repository) Create(p ServiceProxy) (ServiceProxy, error) {
	row := toDB(p)
	if err := r.db.Create(&row).Error; err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return ServiceProxy{}, ErrDuplicateProxyAlias
		}
		return ServiceProxy{}, err
	}
	return fromDB(row), nil
}

func (r *Repository) Get(id uint) (ServiceProxy, error) {
	var row database.ServiceProxy
	if err := r.db.First(&row, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ServiceProxy{}, ErrProxyNotFound
		}
		return ServiceProxy{}, err
	}
	return fromDB(row), nil
}

func (r *Repository) GetByAlias(alias string) (ServiceProxy, error) {
	var row database.ServiceProxy
	if err := r.db.First(&row, "alias = ?", alias).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ServiceProxy{}, ErrProxyNotFound
		}
		return ServiceProxy{}, err
	}
	return fromDB(row), nil
}

func (r *Repository) List() ([]ServiceProxy, error) {
	var rows []database.ServiceProxy
	if err := r.db.Order("id DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]ServiceProxy, 0, len(rows))
	for _, row := range rows {
		out = append(out, fromDB(row))
	}
	return out, nil
}

func (r *Repository) Update(id uint, p ServiceProxy) (ServiceProxy, error) {
	var row database.ServiceProxy
	if err := r.db.First(&row, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ServiceProxy{}, ErrProxyNotFound
		}
		return ServiceProxy{}, err
	}

	row.Name = p.Name
	row.Alias = p.Alias
	row.Type = p.Type
	row.Target = p.Target
	row.Enabled = p.Enabled

	if err := r.db.Save(&row).Error; err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return ServiceProxy{}, ErrDuplicateProxyAlias
		}
		return ServiceProxy{}, err
	}
	return fromDB(row), nil
}

func (r *Repository) Delete(id uint) error {
	res := r.db.Delete(&database.ServiceProxy{}, "id = ?", id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrProxyNotFound
	}
	return nil
}

func (r *Repository) ToggleEnabled(id uint) (ServiceProxy, error) {
	var row database.ServiceProxy
	if err := r.db.First(&row, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ServiceProxy{}, ErrProxyNotFound
		}
		return ServiceProxy{}, err
	}
	row.Enabled = !row.Enabled
	if err := r.db.Save(&row).Error; err != nil {
		return ServiceProxy{}, err
	}
	return fromDB(row), nil
}
