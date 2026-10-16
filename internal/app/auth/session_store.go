package auth

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"time"

	"gorm.io/gorm"
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
	repo Repository
	ttl  time.Duration
	now  func() time.Time
}

// NewSessionStore builds a session store over a GORM database.
func NewSessionStore(db *gorm.DB, ttl time.Duration, now func() time.Time) *SessionStore {
	return NewSessionStoreWithRepository(NewGORMRepository(db), ttl, now)
}

// NewSessionStoreWithRepository builds a session store over a custom repository.
// A non-positive ttl falls back to DefaultSessionTTL and a nil clock falls back
// to time.Now.
func NewSessionStoreWithRepository(repo Repository, ttl time.Duration, now func() time.Time) *SessionStore {
	if ttl <= 0 {
		ttl = DefaultSessionTTL
	}
	if now == nil {
		now = time.Now
	}
	return &SessionStore{repo: repo, ttl: ttl, now: now}
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
	_ = s.repo.DeleteExpiredSessions(now)

	record := SessionRecord{ID: session.ID, ExpiresAt: session.ExpiresAt}
	if err := s.repo.CreateSession(record); err != nil {
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

	record, ok, err := s.repo.GetSession(id)
	if err != nil {
		return Session{}, false, fmt.Errorf("auth: load session: %w", err)
	}
	if !ok {
		return Session{}, false, nil
	}

	if !record.ExpiresAt.After(s.now().UTC()) {
		_ = s.repo.DeleteSession(id)
		return Session{}, false, nil
	}
	return Session{ID: record.ID, ExpiresAt: record.ExpiresAt}, true, nil
}

// Delete removes a session, invalidating it immediately.
func (s *SessionStore) Delete(id string) error {
	if id == "" {
		return nil
	}
	if err := s.repo.DeleteSession(id); err != nil {
		return fmt.Errorf("auth: delete session: %w", err)
	}
	return nil
}
