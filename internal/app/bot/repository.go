package bot

import (
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"devplatform/internal/app/database"
)

const maxContextsPerChannel = 1000

// Repository handles database persistence for bot channels, reply contexts and session watches.
type Repository struct {
	db *gorm.DB
}

// NewRepository constructs a GORM-backed Repository.
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) ListChannels() ([]database.BotChannel, error) {
	var rows []database.BotChannel
	if err := r.db.Order("id ASC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("bot: list channels: %w", err)
	}
	return rows, nil
}

func (r *Repository) GetChannel(id uint) (database.BotChannel, error) {
	var row database.BotChannel
	if err := r.db.First(&row, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return database.BotChannel{}, ErrChannelNotFound
		}
		return database.BotChannel{}, fmt.Errorf("bot: get channel %d: %w", id, err)
	}
	return row, nil
}

func (r *Repository) GetChannelByChatID(chatID uint) (database.BotChannel, error) {
	var row database.BotChannel
	if err := r.db.Where("chat_id = ?", chatID).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return database.BotChannel{}, ErrChannelNotFound
		}
		return database.BotChannel{}, fmt.Errorf("bot: get channel by chat %d: %w", chatID, err)
	}
	return row, nil
}

func (r *Repository) CreateChannel(row *database.BotChannel) error {
	if err := r.db.Create(row).Error; err != nil {
		return fmt.Errorf("bot: create channel: %w", err)
	}
	return nil
}

func (r *Repository) UpdateChannel(row *database.BotChannel) error {
	res := r.db.Save(row)
	if res.Error != nil {
		return fmt.Errorf("bot: update channel %d: %w", row.ID, res.Error)
	}
	return nil
}

func (r *Repository) UpdateChannelState(id uint, status, lastError string, lastConnected *time.Time) error {
	updates := map[string]any{
		"status":     status,
		"last_error": lastError,
		"updated_at": time.Now().UTC(),
	}
	if lastConnected != nil {
		updates["last_connected_at"] = *lastConnected
	}
	if err := r.db.Model(&database.BotChannel{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return fmt.Errorf("bot: update channel state %d: %w", id, err)
	}
	return nil
}

func (r *Repository) UpdateChannelCursor(id uint, cursor string) error {
	if err := r.db.Model(&database.BotChannel{}).Where("id = ?", id).Updates(map[string]any{
		"update_cursor": cursor,
		"updated_at":    time.Now().UTC(),
	}).Error; err != nil {
		return fmt.Errorf("bot: update channel cursor %d: %w", id, err)
	}
	return nil
}

func (r *Repository) DeleteChannel(id uint) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("channel_id = ?", id).Delete(&database.BotChannelContext{}).Error; err != nil {
			return fmt.Errorf("bot: delete contexts for channel %d: %w", id, err)
		}
		if err := tx.Where("channel_id = ?", id).Delete(&database.BotSessionWatch{}).Error; err != nil {
			return fmt.Errorf("bot: delete watches for channel %d: %w", id, err)
		}
		res := tx.Delete(&database.BotChannel{}, id)
		if res.Error != nil {
			return fmt.Errorf("bot: delete channel %d: %w", id, res.Error)
		}
		if res.RowsAffected == 0 {
			return ErrChannelNotFound
		}
		return nil
	})
}

// UpsertContextToken persists the latest context_token for (channelID, userID)
// and prunes the oldest entries once a channel exceeds maxContextsPerChannel.
func (r *Repository) UpsertContextToken(channelID uint, externalUserID, contextToken string, at time.Time) error {
	if channelID == 0 || externalUserID == "" {
		return nil
	}
	return r.db.Transaction(func(tx *gorm.DB) error {
		var existing database.BotChannelContext
		err := tx.Where("channel_id = ? AND external_user_id = ?", channelID, externalUserID).First(&existing).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			row := database.BotChannelContext{
				ChannelID:      channelID,
				ExternalUserID: externalUserID,
				ContextToken:   contextToken,
				LastMessageAt:  at,
			}
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		} else {
			updates := map[string]any{
				"last_message_at": at,
				"updated_at":      time.Now().UTC(),
			}
			if contextToken != "" {
				updates["context_token"] = contextToken
			}
			if err := tx.Model(&existing).Updates(updates).Error; err != nil {
				return err
			}
		}

		var count int64
		if err := tx.Model(&database.BotChannelContext{}).Where("channel_id = ?", channelID).Count(&count).Error; err == nil && count > maxContextsPerChannel {
			var oldest []uint
			if err := tx.Model(&database.BotChannelContext{}).
				Where("channel_id = ?", channelID).
				Order("last_message_at ASC, id ASC").
				Limit(int(count-maxContextsPerChannel)).
				Pluck("id", &oldest).Error; err == nil && len(oldest) > 0 {
				_ = tx.Where("id IN ?", oldest).Delete(&database.BotChannelContext{}).Error
			}
		}
		return nil
	})
}

func (r *Repository) GetContextToken(channelID uint, externalUserID string) (string, error) {
	var row database.BotChannelContext
	if externalUserID != "" {
		err := r.db.Where("channel_id = ? AND external_user_id = ?", channelID, externalUserID).First(&row).Error
		if err == nil {
			return row.ContextToken, nil
		}
	}
	// Fallback to the most recently active user for this channel.
	if err := r.db.Where("channel_id = ?", channelID).Order("last_message_at DESC, id DESC").First(&row).Error; err != nil {
		return "", err
	}
	return row.ContextToken, nil
}

func (r *Repository) LatestUserForChannel(channelID uint) (string, string, error) {
	var row database.BotChannelContext
	if err := r.db.Where("channel_id = ?", channelID).Order("last_message_at DESC, id DESC").First(&row).Error; err != nil {
		return "", "", err
	}
	return row.ExternalUserID, row.ContextToken, nil
}

func (r *Repository) UpsertSessionWatch(watch database.BotSessionWatch) (database.BotSessionWatch, error) {
	var existing database.BotSessionWatch
	err := r.db.Where("session_id = ? AND watch_channel_id = ?", watch.SessionID, watch.WatchChannelID).First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		if err := r.db.Create(&watch).Error; err != nil {
			return database.BotSessionWatch{}, err
		}
		return watch, nil
	}
	if err != nil {
		return database.BotSessionWatch{}, err
	}
	existing.ExternalUserID = watch.ExternalUserID
	existing.ProjectID = watch.ProjectID
	existing.Status = WatchWatching
	existing.LastError = ""
	if err := r.db.Save(&existing).Error; err != nil {
		return database.BotSessionWatch{}, err
	}
	return existing, nil
}

func (r *Repository) ListActiveWatches() ([]database.BotSessionWatch, error) {
	var rows []database.BotSessionWatch
	if err := r.db.Where("status = ?", WatchWatching).Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *Repository) MarkWatchStatus(id uint, status, lastError string) error {
	return r.db.Model(&database.BotSessionWatch{}).Where("id = ?", id).Updates(map[string]any{
		"status":     status,
		"last_error": lastError,
		"updated_at": time.Now().UTC(),
	}).Error
}
