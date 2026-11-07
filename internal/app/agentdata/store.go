package agentdata

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"devplatform/internal/app/database"
)

// Sentinel errors. Handlers map them to HTTP status codes; callers test them
// with errors.Is.
var (
	// ErrSessionNotFound reports an operation on a transcript session the
	// store does not know.
	ErrSessionNotFound = errors.New("agentdata: session not found")
	// ErrSessionBusy reports a delete refused because the session's turn is
	// still running.
	ErrSessionBusy = errors.New("agentdata: session is running")
)

// Record is one transcript record: a transcript-ordered conversation entry.
// Kind discriminates the entry (user, assistant, thinking, tool_call,
// tool_result, error); Content carries the text body and Payload the JSON
// payload the agent loop needs to rebuild an LLM context.
type Record struct {
	Seq       int
	Kind      string
	Content   string
	Payload   []byte
	CreatedAt time.Time
}

// SessionSummary is one transcript session as the settings UI lists it.
type SessionSummary struct {
	SessionID    string    `json:"sessionId"`
	Title        string    `json:"title"`
	Directory    string    `json:"directory"`
	Model        string    `json:"model,omitempty"`
	MessageCount int64     `json:"messageCount"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

// Service owns the transcript tables of one backend. Writes are serialized by
// the platform's single-connection pool; every batch appends in one
// transaction so a partial turn is never visible. The index purger and
// activity gate are function hooks so this package stays free of both the
// sessions service and any backend identifier: the composition root binds
// them.
type Service struct {
	db          *gorm.DB
	indexPurger func(nativeSessions []string) error
	activity    func(sessionID string) bool
}

// Option customizes the transcript service.
type Option func(*Service)

// WithIndexPurger installs the hook that removes the session index rows of
// deleted native conversations, so the workbench list never offers a session
// whose conversation is gone.
func WithIndexPurger(fn func(nativeSessions []string) error) Option {
	return func(s *Service) { s.indexPurger = fn }
}

// WithActivityGate installs the hook that reports whether a session's turn is
// still running; deletes over busy sessions are refused so a live turn never
// writes into rows being removed.
func WithActivityGate(fn func(sessionID string) bool) Option {
	return func(s *Service) { s.activity = fn }
}

// NewService builds the transcript service over the platform database.
func NewService(db *gorm.DB, options ...Option) *Service {
	if db == nil {
		panic("agentdata: nil gorm.DB")
	}
	service := &Service{db: db}
	for _, option := range options {
		option(service)
	}
	return service
}

// CreateSession inserts the owning session row. An existing row with the
// same id is left untouched so a re-created id keeps its records.
func (s *Service) CreateSession(ctx context.Context, sessionID, directory, title string, createdAt time.Time) error {
	if sessionID == "" || directory == "" {
		return errors.New("agentdata: session id and directory are required")
	}
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	row := database.BuiltinAgentSession{
		SessionID: sessionID,
		Directory: directory,
		Title:     title,
		CreatedAt: createdAt,
		UpdatedAt: createdAt,
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&row).Error; err != nil {
			// The provider may create the same id again (e.g. a continue on
			// a fresh runtime); an existing row wins.
			if errors.Is(err, gorm.ErrDuplicatedKey) {
				return nil
			}
			return fmt.Errorf("agentdata: create session: %w", err)
		}
		return nil
	})
}

// SetModel stamps the session's last-used model.
func (s *Service) SetModel(ctx context.Context, sessionID, model string) error {
	return s.db.WithContext(ctx).Model(&database.BuiltinAgentSession{}).
		Where("session_id = ?", sessionID).
		Update("model", model).Error
}

// Append writes a batch of records and bumps the session timestamp in one
// transaction. The caller owns Seq numbering; it must increase monotonically
// within a session.
func (s *Service) Append(ctx context.Context, sessionID string, records []Record) error {
	if sessionID == "" {
		return ErrSessionNotFound
	}
	if len(records) == 0 {
		return nil
	}
	rows := make([]database.BuiltinAgentMessage, 0, len(records))
	for _, record := range records {
		created := record.CreatedAt
		if created.IsZero() {
			created = time.Now().UTC()
		}
		rows = append(rows, database.BuiltinAgentMessage{
			SessionID: sessionID,
			Seq:       record.Seq,
			Kind:      record.Kind,
			Content:   record.Content,
			Payload:   string(record.Payload),
			CreatedAt: created,
		})
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&rows).Error; err != nil {
			return fmt.Errorf("agentdata: append %s: %w", sessionID, err)
		}
		if err := tx.Model(&database.BuiltinAgentSession{}).
			Where("session_id = ?", sessionID).
			Update("updated_at", time.Now().UTC()).Error; err != nil {
			return fmt.Errorf("agentdata: touch session %s: %w", sessionID, err)
		}
		return nil
	})
}

// Load returns a session's records ordered oldest first.
func (s *Service) Load(ctx context.Context, sessionID string) ([]Record, error) {
	rows := make([]database.BuiltinAgentMessage, 0)
	if err := s.db.WithContext(ctx).
		Where("session_id = ?", sessionID).
		Order("seq ASC, id ASC").
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("agentdata: load %s: %w", sessionID, err)
	}
	records := make([]Record, 0, len(rows))
	for _, row := range rows {
		records = append(records, Record{
			Seq:       row.Seq,
			Kind:      row.Kind,
			Content:   row.Content,
			Payload:   []byte(row.Payload),
			CreatedAt: row.CreatedAt,
		})
	}
	return records, nil
}

// Exists reports whether the session has a transcript owning row.
func (s *Service) Exists(ctx context.Context, sessionID string) (bool, error) {
	var count int64
	if err := s.db.WithContext(ctx).Model(&database.BuiltinAgentSession{}).
		Where("session_id = ?", sessionID).Count(&count).Error; err != nil {
		return false, fmt.Errorf("agentdata: exists %s: %w", sessionID, err)
	}
	return count > 0, nil
}

// isBusy reports whether the session's turn is running.
func (s *Service) isBusy(sessionID string) bool {
	return s.activity != nil && s.activity(sessionID)
}

// ListSessions returns one page of transcript sessions by recent use, with
// the total count. limit <= 0 means all.
func (s *Service) ListSessions(ctx context.Context, limit, offset int) ([]SessionSummary, int64, error) {
	var total int64
	if err := s.db.WithContext(ctx).Model(&database.BuiltinAgentSession{}).Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("agentdata: count sessions: %w", err)
	}

	query := s.db.WithContext(ctx).Table("agent_builtin_sessions AS s").
		Select(`s.session_id, s.title, s.directory, s.model, s.created_at, s.updated_at,
			(SELECT COUNT(*) FROM agent_builtin_messages AS m WHERE m.session_id = s.session_id) AS message_count`).
		Order("s.updated_at DESC, s.id DESC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}
	var rows []SessionSummary
	if err := query.Scan(&rows).Error; err != nil {
		return nil, 0, fmt.Errorf("agentdata: list sessions: %w", err)
	}
	return rows, total, nil
}

// DeleteSession removes one session's messages and owning row, then purges
// the platform session-index rows that point at it. A session whose turn is
// running is refused with ErrSessionBusy.
func (s *Service) DeleteSession(ctx context.Context, sessionID string) error {
	if s.isBusy(sessionID) {
		return fmt.Errorf("%w: %s", ErrSessionBusy, sessionID)
	}
	exists, err := s.Exists(ctx, sessionID)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("%w: %s", ErrSessionNotFound, sessionID)
	}
	if s.indexPurger != nil {
		if err := s.indexPurger([]string{sessionID}); err != nil {
			return err
		}
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("session_id = ?", sessionID).
			Delete(&database.BuiltinAgentMessage{}).Error; err != nil {
			return fmt.Errorf("agentdata: delete messages %s: %w", sessionID, err)
		}
		if err := tx.Where("session_id = ?", sessionID).
			Delete(&database.BuiltinAgentSession{}).Error; err != nil {
			return fmt.Errorf("agentdata: delete session %s: %w", sessionID, err)
		}
		return nil
	})
}

// ClearSessions removes every transcript session and its messages, purging
// the matching index rows first. It returns the number of sessions removed.
func (s *Service) ClearSessions(ctx context.Context) (int64, error) {
	all, _, err := s.ListSessions(ctx, 0, 0)
	if err != nil {
		return 0, err
	}
	natives := make([]string, 0, len(all))
	for _, session := range all {
		if s.activity != nil && s.activity(session.SessionID) {
			continue
		}
		natives = append(natives, session.SessionID)
	}
	if len(natives) == 0 {
		return 0, nil
	}
	if s.indexPurger != nil {
		if err := s.indexPurger(natives); err != nil {
			return 0, err
		}
	}
	var removed int64
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Where("session_id IN ?", natives).Delete(&database.BuiltinAgentSession{})
		if result.Error != nil {
			return fmt.Errorf("agentdata: clear sessions: %w", result.Error)
		}
		removed = result.RowsAffected
		if err := tx.Where("session_id IN ?", natives).Delete(&database.BuiltinAgentMessage{}).Error; err != nil {
			return fmt.Errorf("agentdata: clear messages: %w", err)
		}
		return nil
	}); err != nil {
		return 0, err
	}
	return removed, nil
}
