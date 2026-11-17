package builtin

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"devplatform/internal/core/agents"
)

// debugLogger writes structured JSONL interaction events to
// <dataDir>/debug_logs/session-<sessionId>.jsonl when debug mode is enabled.
type debugLogger struct {
	dir     string
	debugOn func() bool
	mu      sync.Mutex
}

func newDebugLogger(dataDir string, debugOn func() bool) *debugLogger {
	dir := ""
	if strings.TrimSpace(dataDir) != "" {
		dir = filepath.Join(dataDir, "debug_logs")
	}
	return &debugLogger{
		dir:     dir,
		debugOn: debugOn,
	}
}

// sanitizeSessionID cleans the session id and verifies it is safe for filenames.
func sanitizeSessionID(sessionID string) (string, error) {
	clean := strings.TrimSpace(sessionID)
	if clean == "" || strings.Contains(clean, "..") || strings.ContainsAny(clean, `/\:*?"<>|`) {
		return "", errors.New("builtin: invalid session id for debug log")
	}
	return clean, nil
}

func (l *debugLogger) filePath(sessionID string) (string, error) {
	if l == nil || l.dir == "" {
		return "", errors.New("builtin: dataDir is not configured for debug logger")
	}
	cleanID, err := sanitizeSessionID(sessionID)
	if err != nil {
		return "", err
	}
	return filepath.Join(l.dir, fmt.Sprintf("session-%s.jsonl", cleanID)), nil
}

// LogEntry is one JSON line in the debug log file.
type LogEntry struct {
	Timestamp string `json:"timestamp"`
	TurnID    string `json:"turnId,omitempty"`
	Step      int    `json:"step,omitempty"`
	Type      string `json:"type"`
	Payload   any    `json:"payload"`
}

func (l *debugLogger) isEnabled() bool {
	if l == nil || l.dir == "" || l.debugOn == nil {
		return false
	}
	return l.debugOn()
}

// Log writes one structured event if debug mode is active. It is non-blocking,
// fail-safe and never returns an error to the turn loop.
func (l *debugLogger) Log(sessionID string, turnID string, step int, entryType string, payload any) {
	if !l.isEnabled() {
		return
	}
	path, err := l.filePath(sessionID)
	if err != nil {
		return
	}

	entry := LogEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		TurnID:    turnID,
		Step:      step,
		Type:      entryType,
		Payload:   payload,
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return
	}
	data = append(data, '\n')

	l.mu.Lock()
	defer l.mu.Unlock()

	if err := os.MkdirAll(l.dir, 0700); err != nil {
		return
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(data)
}

// Open returns a read handle and content size for one session's debug log.
func (l *debugLogger) Open(sessionID string) (io.ReadCloser, int64, error) {
	path, err := l.filePath(sessionID)
	if err != nil {
		return nil, 0, err
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	stat, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, 0, agents.ErrDebugLogNotFound
		}
		return nil, 0, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	return f, stat.Size(), nil
}

// Purge removes the session's debug log file if it exists.
func (l *debugLogger) Purge(sessionID string) error {
	path, err := l.filePath(sessionID)
	if err != nil {
		return err
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
