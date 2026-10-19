package terminal

import (
	"context"
	"errors"

	"devplatform/internal/app/database"
	"gorm.io/gorm"
)

// Repository defines data access for terminal server records.
type Repository interface {
	List(ctx context.Context) ([]database.TerminalServer, error)
	Get(ctx context.Context, id uint) (*database.TerminalServer, error)
	Create(ctx context.Context, server *database.TerminalServer) error
	Update(ctx context.Context, server *database.TerminalServer) error
	Delete(ctx context.Context, id uint) error
}

type gormRepository struct {
	db *gorm.DB
}

// NewRepository creates a new GORM-backed terminal server repository.
func NewRepository(db *gorm.DB) Repository {
	return &gormRepository{db: db}
}

func (r *gormRepository) List(ctx context.Context) ([]database.TerminalServer, error) {
	var servers []database.TerminalServer
	if err := r.db.WithContext(ctx).Order("id ASC").Find(&servers).Error; err != nil {
		return nil, err
	}
	return servers, nil
}

func (r *gormRepository) Get(ctx context.Context, id uint) (*database.TerminalServer, error) {
	var server database.TerminalServer
	if err := r.db.WithContext(ctx).First(&server, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &server, nil
}

func (r *gormRepository) Create(ctx context.Context, server *database.TerminalServer) error {
	return r.db.WithContext(ctx).Create(server).Error
}

func (r *gormRepository) Update(ctx context.Context, server *database.TerminalServer) error {
	return r.db.WithContext(ctx).Save(server).Error
}

func (r *gormRepository) Delete(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Delete(&database.TerminalServer{}, id).Error
}
