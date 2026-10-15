package auth

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"time"

	"gorm.io/gorm"

	"devplatform/internal/app/database"
)

// DefaultSessionTTL is how long a freshly issued login session stays valid.
const DefaultSessionTTL = 12 * time.Hour

// sessionIDBytes is the amount of entropy in a session identifier.
const sessionIDBytes = 32

// Session is an authenticated login session. The ID is the opaque cookie
// value; it is never logged.
type Session struct {
	ID        string
	ExpiresAt time.Time
}

// SessionStore persists and validates login sessions in the auth_sessions
// table. All time comparisons go through now so tests can control expiry.
type SessionStore struct {
	db  *gorm.DB
	ttl time.Duration
	now func() time.Time
}

// NewSessionStore builds a session store. A non-positive ttl falls back to
// DefaultSessionTTL and a nil clock falls back to time.Now.
func NewSessionStore(db *gorm.DB, ttl time.Duration, now func() time.Time) *SessionStore {
	if ttl <= 0 {
		ttl = DefaultSessionTTL
	}
	if now == nil {
		now = time.Now
	}
	return &SessionStore{db: db, ttl: ttl, now: now}
}

// newSessionID returns a cryptographically random, URL-safe session token.
func newSessionID() (string, error) {
	buf := make([]byte, sessionIDBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("auth: generate session id: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// Create issues a new session and stores it. Expired sessions are pruned
// opportunistically so the table stays bounded over time.
func (s *SessionStore) Create() (Session, error) {
	id, err := newSessionID()
	if err != nil {
		return Session{}, err
	}
	now := s.now().UTC()
	session := Session{ID: id, ExpiresAt: now.Add(s.ttl)}

	// Best-effort cleanup; a failure here must not block issuing a session.
	_ = s.db.Where("expires_at < ?", now).Delete(&database.AuthSession{}).Error

	record := database.AuthSession{ID: session.ID, ExpiresAt: session.ExpiresAt, CreatedAt: now}
	if err := s.db.Create(&record).Error; err != nil {
		return Session{}, fmt.Errorf("auth: store session: %w", err)
	}
	return session, nil
}

// Validate loads and checks a session ID. It returns ok=false for an unknown
// or expired session. Expired sessions are removed as a side effect.
func (s *SessionStore) Validate(id string) (Session, bool, error) {
	if id == "" {
		return Session{}, false, nil
	}

	// Find (rather than First) avoids GORM's "record not found" error log for
	// a tampered or stale cookie.
	var records []database.AuthSession
	if err := s.db.Where("id = ?", id).Limit(1).Find(&records).Error; err != nil {
		return Session{}, false, fmt.Errorf("auth: load session: %w", err)
	}
	if len(records) == 0 {
		return Session{}, false, nil
	}

	record := records[0]
	if !record.ExpiresAt.After(s.now().UTC()) {
		_ = s.db.Where("id = ?", id).Delete(&database.AuthSession{}).Error
		return Session{}, false, nil
	}
	return Session{ID: record.ID, ExpiresAt: record.ExpiresAt}, true, nil
}

// Delete removes a session, invalidating it immediately.
func (s *SessionStore) Delete(id string) error {
	if id == "" {
		return nil
	}
	if err := s.db.Where("id = ?", id).Delete(&database.AuthSession{}).Error; err != nil {
		return fmt.Errorf("auth: delete session: %w", err)
	}
	return nil
}
