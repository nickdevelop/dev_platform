package auth

import (
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"devplatform/internal/app/database"
)

// SessionRecord is the persistence representation of an authenticated web session.
type SessionRecord struct {
	ID        string
	ExpiresAt time.Time
	CreatedAt time.Time
}

// Repository defines the persistence contract for authentication: passwords
// and web sessions.
type Repository interface {
	GetPasswordHash() (string, bool, error)
	SetPasswordHash(hash string) error

	CreateSession(record SessionRecord) error
	GetSession(id string) (SessionRecord, bool, error)
	DeleteSession(id string) error
	DeleteExpiredSessions(now time.Time) error
}

type gormRepository struct {
	db *gorm.DB
}

// NewGORMRepository builds a GORM-backed auth repository.
func NewGORMRepository(db *gorm.DB) Repository {
	return &gormRepository{db: db}
}

func (r *gormRepository) GetPasswordHash() (string, bool, error) {
	var settings []database.Setting
	if err := r.db.Where("key = ?", database.SettingKeyPasswordHash).
		Limit(1).Find(&settings).Error; err != nil {
		return "", false, fmt.Errorf("auth: load password hash: %w", err)
	}
	if len(settings) == 0 {
		return "", false, nil
	}
	return settings[0].Value, true, nil
}

func (r *gormRepository) SetPasswordHash(hash string) error {
	setting := database.Setting{Key: database.SettingKeyPasswordHash, Value: hash}
	if err := r.db.Create(&setting).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return ErrPasswordAlreadySet
		}
		return fmt.Errorf("auth: store password hash: %w", err)
	}
	return nil
}

func (r *gormRepository) CreateSession(record SessionRecord) error {
	row := database.AuthSession{
		ID:        record.ID,
		ExpiresAt: record.ExpiresAt,
		CreatedAt: record.CreatedAt,
	}
	return r.db.Create(&row).Error
}

func (r *gormRepository) GetSession(id string) (SessionRecord, bool, error) {
	var records []database.AuthSession
	if err := r.db.Where("id = ?", id).Limit(1).Find(&records).Error; err != nil {
		return SessionRecord{}, false, err
	}
	if len(records) == 0 {
		return SessionRecord{}, false, nil
	}
	rec := records[0]
	return SessionRecord{
		ID:        rec.ID,
		ExpiresAt: rec.ExpiresAt,
		CreatedAt: rec.CreatedAt,
	}, true, nil
}

func (r *gormRepository) DeleteSession(id string) error {
	return r.db.Where("id = ?", id).Delete(&database.AuthSession{}).Error
}

func (r *gormRepository) DeleteExpiredSessions(now time.Time) error {
	return r.db.Where("expires_at < ?", now).Delete(&database.AuthSession{}).Error
}
