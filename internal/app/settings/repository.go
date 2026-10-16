package settings

import (
	"fmt"

	"gorm.io/gorm"

	"devplatform/internal/app/database"
)

// Repository persists and retrieves key-value configuration records.
type Repository interface {
	Get(key string) (string, bool, error)
	Set(key, value string) error
}

type gormRepository struct {
	db *gorm.DB
}

// NewGORMRepository creates a GORM-backed Repository for settings.
func NewGORMRepository(db *gorm.DB) Repository {
	return &gormRepository{db: db}
}

func (r *gormRepository) Get(key string) (string, bool, error) {
	var row database.Setting
	result := r.db.Where("key = ?", key).Limit(1).Find(&row)
	if result.Error != nil {
		return "", false, fmt.Errorf("settings: load key %q: %w", key, result.Error)
	}
	if result.RowsAffected == 0 {
		return "", false, nil
	}
	return row.Value, true, nil
}

func (r *gormRepository) Set(key, value string) error {
	row := database.Setting{Key: key, Value: value}
	if err := r.db.Save(&row).Error; err != nil {
		return fmt.Errorf("settings: save key %q: %w", key, err)
	}
	return nil
}
