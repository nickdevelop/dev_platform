package chat

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"devplatform/internal/app/database"
)

// Repository persists chats and their messages. The interface is small and
// transactional boundaries live here so the service never depends on GORM.
type Repository interface {
	ListGroups() ([]ChatGroup, error)
	GetGroup(id uint) (ChatGroup, error)
	CreateGroup(group *ChatGroup) error
	UpdateGroup(group *ChatGroup) error
	DeleteGroup(id uint) error

	ListChats(limit, offset int) ([]Chat, int, error)
	ListChatsByGroup(groupID uint, limit, offset int) ([]Chat, int, error)
	GetChat(id uint) (Chat, error)
	CreateChat(chat *Chat, user, assistant *Message) error
	UpdateChat(chat *Chat) error
	TouchChat(id uint) error
	UpdateChatTitleIf(id uint, from, to string) (bool, error)
	DeleteChat(id uint) error

	ListMessages(chatID uint) ([]Message, error)
	ListRecentMessages(chatID uint, limit int) ([]Message, bool, error)
	GetMessage(chatID, messageID uint) (Message, error)
	AppendExchange(chat *Chat, user, assistant *Message) error
	ReplaceFromMessage(chat *Chat, fromMessageID uint, user, assistant *Message) error
	Regenerate(chat *Chat, lastUserMessageID uint, assistant *Message) error
	UpdateMessage(message *Message) error
	MarkPendingFailed(code, detail string) error
}

type gormRepository struct {
	db *gorm.DB
}

// NewGORMRepository builds the GORM-backed chat repository.
func NewGORMRepository(db *gorm.DB) Repository {
	return &gormRepository{db: db}
}

func (r *gormRepository) ListChats(limit, offset int) ([]Chat, int, error) {
	return r.listChats(nil, limit, offset)
}

func (r *gormRepository) ListChatsByGroup(groupID uint, limit, offset int) ([]Chat, int, error) {
	return r.listChats(&groupID, limit, offset)
}

// listChats applies the shared paging bounds and, when groupID is set, restricts
// the query to that conversation group. A nil groupID means every chat.
func (r *gormRepository) listChats(groupID *uint, limit, offset int) ([]Chat, int, error) {
	if limit <= 0 || limit > MaxListLimit {
		limit = DefaultListLimit
	}
	if offset < 0 {
		offset = 0
	}

	scope := func(db *gorm.DB) *gorm.DB {
		if groupID != nil {
			return db.Where("group_id = ?", *groupID)
		}
		return db
	}
	var total int64
	if err := scope(r.db.Model(&database.Chat{})).Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("chat: count chats: %w", err)
	}
	var rows []database.Chat
	if err := scope(r.db.Model(&database.Chat{})).
		Order("updated_at DESC, id DESC").Limit(limit).Offset(offset).
		Find(&rows).Error; err != nil {
		return nil, 0, fmt.Errorf("chat: list chats: %w", err)
	}
	out := make([]Chat, 0, len(rows))
	for _, row := range rows {
		out = append(out, chatFromRow(row))
	}
	return out, int(total), nil
}

func (r *gormRepository) GetChat(id uint) (Chat, error) {
	var row database.Chat
	if err := r.db.First(&row, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Chat{}, ErrChatNotFound
		}
		return Chat{}, fmt.Errorf("chat: load chat %d: %w", id, err)
	}
	return chatFromRow(row), nil
}

func (r *gormRepository) CreateChat(chat *Chat, user, assistant *Message) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		row := chatToRow(*chat)
		if err := tx.Create(&row).Error; err != nil {
			return fmt.Errorf("chat: create chat: %w", err)
		}
		chat.ID = row.ID
		chat.CreatedAt = row.CreatedAt
		chat.UpdatedAt = row.UpdatedAt

		if err := createMessage(tx, user, chat.ID); err != nil {
			return fmt.Errorf("chat: create first user message: %w", err)
		}
		if err := createMessage(tx, assistant, chat.ID); err != nil {
			return fmt.Errorf("chat: create first assistant message: %w", err)
		}
		return nil
	})
}

func (r *gormRepository) UpdateChat(chat *Chat) error {
	now := time.Now().UTC()
	res := r.db.Model(&database.Chat{}).Where("id = ?", chat.ID).Updates(map[string]any{
		"group_id":   chat.GroupID,
		"title":      chat.Title,
		"model_id":   chat.ModelID,
		"model_name": chat.ModelName,
		"updated_at": now,
	})
	if res.Error != nil {
		return fmt.Errorf("chat: update chat %d: %w", chat.ID, res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrChatNotFound
	}
	chat.UpdatedAt = now
	return nil
}

func (r *gormRepository) ListGroups() ([]ChatGroup, error) {
	var rows []database.ChatGroup
	if err := r.db.
		Order("CASE WHEN system_key IS NOT NULL AND system_key <> '' THEN 0 ELSE 1 END, id ASC").
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("chat: list groups: %w", err)
	}
	out := make([]ChatGroup, 0, len(rows))
	for _, row := range rows {
		out = append(out, chatGroupFromRow(row))
	}
	return out, nil
}

func (r *gormRepository) GetGroup(id uint) (ChatGroup, error) {
	var row database.ChatGroup
	if err := r.db.First(&row, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ChatGroup{}, ErrGroupNotFound
		}
		return ChatGroup{}, fmt.Errorf("chat: load group %d: %w", id, err)
	}
	return chatGroupFromRow(row), nil
}

func (r *gormRepository) CreateGroup(group *ChatGroup) error {
	row := chatGroupToRow(*group)
	if err := r.db.Create(&row).Error; err != nil {
		return fmt.Errorf("chat: create group: %w", err)
	}
	group.ID = row.ID
	group.CreatedAt = row.CreatedAt
	group.UpdatedAt = row.UpdatedAt
	return nil
}

func (r *gormRepository) UpdateGroup(group *ChatGroup) error {
	now := time.Now().UTC()
	res := r.db.Model(&database.ChatGroup{}).Where("id = ?", group.ID).Updates(map[string]any{
		"name":       group.Name,
		"updated_at": now,
	})
	if res.Error != nil {
		return fmt.Errorf("chat: update group %d: %w", group.ID, res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrGroupNotFound
	}
	group.UpdatedAt = now
	return nil
}

func (r *gormRepository) DeleteGroup(id uint) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		var chatIDs []uint
		if err := tx.Model(&database.Chat{}).Where("group_id = ?", id).Pluck("id", &chatIDs).Error; err != nil {
			return fmt.Errorf("chat: find chats for group %d: %w", id, err)
		}
		if len(chatIDs) > 0 {
			if err := tx.Where("chat_id IN ?", chatIDs).Delete(&database.ChatMessage{}).Error; err != nil {
				return fmt.Errorf("chat: delete messages for group chats: %w", err)
			}
			if err := tx.Where("id IN ?", chatIDs).Delete(&database.Chat{}).Error; err != nil {
				return fmt.Errorf("chat: delete chats for group %d: %w", id, err)
			}
		}
		res := tx.Delete(&database.ChatGroup{}, id)
		if res.Error != nil {
			return fmt.Errorf("chat: delete group %d: %w", id, res.Error)
		}
		if res.RowsAffected == 0 {
			return ErrGroupNotFound
		}
		return nil
	})
}

// TouchChat updates only the activity timestamp. It is used when a generation
// finishes: the user may have renamed the chat or switched models while the
// answer streamed, and those changes must not be overwritten.
func (r *gormRepository) TouchChat(id uint) error {
	res := r.db.Model(&database.Chat{}).Where("id = ?", id).
		Update("updated_at", time.Now().UTC())
	if res.Error != nil {
		return fmt.Errorf("chat: touch chat %d: %w", id, res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrChatNotFound
	}
	return nil
}

// UpdateChatTitleIf applies an automatic fallback-title change only while the
// expected title is still current. A manual rename (or an already generated
// title) therefore wins.
func (r *gormRepository) UpdateChatTitleIf(id uint, from, to string) (bool, error) {
	if from == to {
		return false, nil
	}
	res := r.db.Model(&database.Chat{}).Where("id = ? AND title = ?", id, from).
		Updates(map[string]any{"title": to, "updated_at": time.Now().UTC()})
	if res.Error != nil {
		return false, fmt.Errorf("chat: update title of chat %d: %w", id, res.Error)
	}
	return res.RowsAffected > 0, nil
}

func (r *gormRepository) DeleteChat(id uint) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("chat_id = ?", id).Delete(&database.ChatMessage{}).Error; err != nil {
			return fmt.Errorf("chat: delete messages for chat %d: %w", id, err)
		}
		res := tx.Delete(&database.Chat{}, id)
		if res.Error != nil {
			return fmt.Errorf("chat: delete chat %d: %w", id, res.Error)
		}
		if res.RowsAffected == 0 {
			return ErrChatNotFound
		}
		return nil
	})
}

func (r *gormRepository) ListMessages(chatID uint) ([]Message, error) {
	var rows []database.ChatMessage
	if err := r.db.Where("chat_id = ?", chatID).Order("id ASC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("chat: list messages for chat %d: %w", chatID, err)
	}
	return messagesFromRows(rows)
}

func (r *gormRepository) ListRecentMessages(chatID uint, limit int) ([]Message, bool, error) {
	if limit <= 0 {
		limit = MaxSnapshotMessages
	}
	var rows []database.ChatMessage
	if err := r.db.Where("chat_id = ?", chatID).Order("id DESC").Limit(limit + 1).Find(&rows).Error; err != nil {
		return nil, false, fmt.Errorf("chat: list recent messages for chat %d: %w", chatID, err)
	}
	truncated := len(rows) > limit
	if truncated {
		rows = rows[:limit]
	}
	// The query is newest-first for the limit; the API and renderer want the
	// transcript oldest-first.
	for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
		rows[i], rows[j] = rows[j], rows[i]
	}
	out, err := messagesFromRows(rows)
	if err != nil {
		return nil, false, err
	}
	return out, truncated, nil
}

func (r *gormRepository) GetMessage(chatID, messageID uint) (Message, error) {
	var row database.ChatMessage
	if err := r.db.Where("chat_id = ? AND id = ?", chatID, messageID).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Message{}, ErrMessageNotFound
		}
		return Message{}, fmt.Errorf("chat: load message %d: %w", messageID, err)
	}
	return messageFromRowChecked(row)
}

func (r *gormRepository) AppendExchange(chat *Chat, user, assistant *Message) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := createMessage(tx, user, chat.ID); err != nil {
			return fmt.Errorf("chat: append user message: %w", err)
		}
		if err := createMessage(tx, assistant, chat.ID); err != nil {
			return fmt.Errorf("chat: append assistant message: %w", err)
		}
		return touchChat(tx, chat)
	})
}

func (r *gormRepository) ReplaceFromMessage(chat *Chat, fromMessageID uint, user, assistant *Message) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("chat_id = ? AND id >= ?", chat.ID, fromMessageID).
			Delete(&database.ChatMessage{}).Error; err != nil {
			return fmt.Errorf("chat: truncate from message %d: %w", fromMessageID, err)
		}
		if err := createMessage(tx, user, chat.ID); err != nil {
			return fmt.Errorf("chat: insert edited user message: %w", err)
		}
		if err := createMessage(tx, assistant, chat.ID); err != nil {
			return fmt.Errorf("chat: insert assistant message: %w", err)
		}
		return touchChat(tx, chat)
	})
}

func (r *gormRepository) Regenerate(chat *Chat, lastUserMessageID uint, assistant *Message) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("chat_id = ? AND id > ?", chat.ID, lastUserMessageID).
			Delete(&database.ChatMessage{}).Error; err != nil {
			return fmt.Errorf("chat: truncate after message %d: %w", lastUserMessageID, err)
		}
		if err := createMessage(tx, assistant, chat.ID); err != nil {
			return fmt.Errorf("chat: insert regenerated assistant message: %w", err)
		}
		return touchChat(tx, chat)
	})
}

func (r *gormRepository) UpdateMessage(message *Message) error {
	now := time.Now().UTC()
	res := r.db.Model(&database.ChatMessage{}).Where("id = ?", message.ID).Updates(map[string]any{
		"content":       message.Text,
		"reasoning":     message.Reasoning,
		"status":        message.Status,
		"error_code":    message.ErrorCode,
		"error_message": message.Error,
		"input_tokens":  usageInput(message.Usage),
		"output_tokens": usageOutput(message.Usage),
		"updated_at":    now,
	})
	if res.Error != nil {
		return fmt.Errorf("chat: update message %d: %w", message.ID, res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrMessageNotFound
	}
	message.UpdatedAt = now
	return nil
}

func (r *gormRepository) MarkPendingFailed(code, detail string) error {
	return r.db.Model(&database.ChatMessage{}).
		Where("role = ? AND status = ?", RoleAssistant, StatusPending).
		Updates(map[string]any{
			"status":        StatusFailed,
			"error_code":    code,
			"error_message": detail,
			"updated_at":    time.Now().UTC(),
		}).Error
}

func touchChat(tx *gorm.DB, chat *Chat) error {
	now := time.Now().UTC()
	// Deliberately excludes title: an exchange may race a manual rename, and
	// the rename must win. Title changes are explicit calls to UpdateChat or
	// UpdateChatTitleIf.
	res := tx.Model(&database.Chat{}).Where("id = ?", chat.ID).Updates(map[string]any{
		"model_id":   chat.ModelID,
		"model_name": chat.ModelName,
		"updated_at": now,
	})
	if res.Error != nil {
		return fmt.Errorf("chat: touch chat %d: %w", chat.ID, res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrChatNotFound
	}
	chat.UpdatedAt = now
	return nil
}

func messagesFromRows(rows []database.ChatMessage) ([]Message, error) {
	out := make([]Message, 0, len(rows))
	for _, row := range rows {
		message, err := messageFromRowChecked(row)
		if err != nil {
			return nil, err
		}
		out = append(out, message)
	}
	return out, nil
}

func messageFromRowChecked(row database.ChatMessage) (Message, error) {
	var images []ImageAttachment
	if row.Images != "" {
		if err := json.Unmarshal([]byte(row.Images), &images); err != nil {
			return Message{}, fmt.Errorf("chat: decode images of message %d: %w", row.ID, err)
		}
	}
	var usage *Usage
	if row.InputTokens > 0 || row.OutputTokens > 0 {
		usage = &Usage{Input: row.InputTokens, Output: row.OutputTokens}
	}
	return Message{
		ID:        row.ID,
		ChatID:    row.ChatID,
		Role:      row.Role,
		Text:      row.Content,
		Reasoning: row.Reasoning,
		Images:    images,
		Status:    row.Status,
		ModelID:   row.ModelID,
		ModelName: row.ModelName,
		Usage:     usage,
		ErrorCode: row.ErrorCode,
		Error:     row.ErrorMessage,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}, nil
}

func chatFromRow(row database.Chat) Chat {
	return Chat{
		ID:        row.ID,
		GroupID:   row.GroupID,
		Title:     row.Title,
		ModelID:   row.ModelID,
		ModelName: row.ModelName,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
}

func chatToRow(chat Chat) database.Chat {
	return database.Chat{
		ID:        chat.ID,
		GroupID:   chat.GroupID,
		Title:     chat.Title,
		ModelID:   chat.ModelID,
		ModelName: chat.ModelName,
	}
}

func createMessage(tx *gorm.DB, message *Message, chatID uint) error {
	images := ""
	if len(message.Images) > 0 {
		encoded, err := json.Marshal(message.Images)
		if err != nil {
			return err
		}
		images = string(encoded)
	}
	row := database.ChatMessage{
		ChatID:       chatID,
		Role:         message.Role,
		Content:      message.Text,
		Reasoning:    message.Reasoning,
		Images:       images,
		Status:       message.Status,
		ModelID:      message.ModelID,
		ModelName:    message.ModelName,
		ErrorCode:    message.ErrorCode,
		ErrorMessage: message.Error,
		InputTokens:  usageInput(message.Usage),
		OutputTokens: usageOutput(message.Usage),
	}
	if err := tx.Create(&row).Error; err != nil {
		return err
	}
	message.ID = row.ID
	message.ChatID = chatID
	message.CreatedAt = row.CreatedAt
	message.UpdatedAt = row.UpdatedAt
	return nil
}

func usageInput(usage *Usage) int {
	if usage == nil {
		return 0
	}
	return usage.Input
}

func usageOutput(usage *Usage) int {
	if usage == nil {
		return 0
	}
	return usage.Output
}

func chatGroupFromRow(row database.ChatGroup) ChatGroup {
	group := ChatGroup{
		ID:        row.ID,
		Name:      row.Name,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
	if row.SystemKey != nil {
		group.SystemKey = *row.SystemKey
	}
	return group
}

func chatGroupToRow(group ChatGroup) database.ChatGroup {
	row := database.ChatGroup{
		ID:   group.ID,
		Name: group.Name,
	}
	if group.SystemKey != "" {
		key := group.SystemKey
		row.SystemKey = &key
	}
	return row
}
