package builtin

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"devplatform/internal/core/agents"
)

func TestDebugLoggerLifecycle(t *testing.T) {
	tempDir := t.TempDir()
	enabled := false
	logger := newDebugLogger(tempDir, func() bool { return enabled })

	sessionID := "sess-test-123"

	// 1. When disabled, Log does not create file
	logger.Log(sessionID, "turn-1", 1, "test_event", map[string]string{"foo": "bar"})
	_, _, err := logger.Open(sessionID)
	if err == nil || err != agents.ErrDebugLogNotFound {
		t.Fatalf("expected ErrDebugLogNotFound when disabled, got %v", err)
	}

	// 2. Enable debug logging and write events
	enabled = true
	logger.Log(sessionID, "turn-1", 1, "llm_request", map[string]string{"prompt": "hello"})
	logger.Log(sessionID, "turn-1", 1, "llm_response", map[string]string{"text": "world"})

	reader, size, err := logger.Open(sessionID)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer reader.Close()

	if size <= 0 {
		t.Fatalf("expected positive file size, got %d", size)
	}

	scanner := bufio.NewScanner(reader)
	var lines []LogEntry
	for scanner.Scan() {
		var entry LogEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			t.Fatalf("unmarshal line %q: %v", scanner.Text(), err)
		}
		lines = append(lines, entry)
	}
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
	if lines[0].Type != "llm_request" || lines[1].Type != "llm_response" {
		t.Errorf("unexpected event types: %s, %s", lines[0].Type, lines[1].Type)
	}

	// 3. Purge removes file
	if err := logger.Purge(sessionID); err != nil {
		t.Fatalf("Purge failed: %v", err)
	}
	_, _, err = logger.Open(sessionID)
	if err != agents.ErrDebugLogNotFound {
		t.Fatalf("expected ErrDebugLogNotFound after purge, got %v", err)
	}
}

func TestDebugLoggerPathTraversalSanitization(t *testing.T) {
	tempDir := t.TempDir()
	logger := newDebugLogger(tempDir, func() bool { return true })

	badIDs := []string{
		"../escape",
		"foo/bar",
		`foo\bar`,
		"foo:bar",
		"",
		"   ",
	}

	for _, badID := range badIDs {
		_, _, err := logger.Open(badID)
		if err == nil {
			t.Errorf("expected error for bad session id %q, got nil", badID)
		}
	}
}

func TestDebugLoggerConcurrentWrites(t *testing.T) {
	tempDir := t.TempDir()
	logger := newDebugLogger(tempDir, func() bool { return true })
	sessionID := "sess-concurrent"

	var wg sync.WaitGroup
	count := 50
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(step int) {
			defer wg.Done()
			logger.Log(sessionID, "turn-1", step, "concurrent_event", map[string]int{"step": step})
		}(i)
	}
	wg.Wait()

	reader, _, err := logger.Open(sessionID)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer reader.Close()

	scanner := bufio.NewScanner(reader)
	lines := 0
	for scanner.Scan() {
		lines++
	}
	if lines != count {
		t.Fatalf("expected %d lines, got %d", count, lines)
	}
}

func TestAdapterDebugLogExporterContract(t *testing.T) {
	tempDir := t.TempDir()
	dbDir := filepath.Join(tempDir, "db")
	_ = os.MkdirAll(dbDir, 0700)

	debugActive := true
	adapter := New(Options{
		TranscriptStore: &fakeTranscriptStore{},
		ModelResolver:   &fakeModelResolver{},
		DataDir:         tempDir,
		DebugMode:       func() bool { return debugActive },
	})

	var _ agents.DebugLogExporter = adapter

	ref := agents.SessionRef{
		Backend:   agents.BackendBuiltin,
		SessionID: "sess-contract-test",
	}

	ctx := context.Background()

	// Currently no file exists
	_, _, err := adapter.OpenDebugLog(ctx, ref)
	if err != agents.ErrDebugLogNotFound {
		t.Fatalf("expected ErrDebugLogNotFound, got %v", err)
	}

	// Write via internal logger
	adapter.logger.Log(ref.SessionID, "turn-1", 1, "test", "content")

	r, size, err := adapter.OpenDebugLog(ctx, ref)
	if err != nil {
		t.Fatalf("OpenDebugLog failed: %v", err)
	}
	r.Close()
	if size <= 0 {
		t.Errorf("size <= 0: %d", size)
	}

	// Purge via adapter
	if err := adapter.PurgeDebugLog(ctx, ref); err != nil {
		t.Fatalf("PurgeDebugLog failed: %v", err)
	}
	_, _, err = adapter.OpenDebugLog(ctx, ref)
	if err != agents.ErrDebugLogNotFound {
		t.Fatalf("expected ErrDebugLogNotFound after purge, got %v", err)
	}
}

type fakeTranscriptStore struct{}

func (f *fakeTranscriptStore) CreateSession(ctx context.Context, s TranscriptSession) error {
	return nil
}
func (f *fakeTranscriptStore) SetModel(ctx context.Context, sessionID, model string) error {
	return nil
}
func (f *fakeTranscriptStore) Append(ctx context.Context, sessionID string, records []TranscriptRecord) error {
	return nil
}
func (f *fakeTranscriptStore) Load(ctx context.Context, sessionID string) ([]TranscriptRecord, error) {
	return nil, nil
}
func (f *fakeTranscriptStore) Exists(ctx context.Context, sessionID string) (bool, error) {
	return false, nil
}

type fakeModelResolver struct{}

func (f *fakeModelResolver) ListModels(ctx context.Context) ([]agents.ModelInfo, error) {
	return []agents.ModelInfo{{ID: "fake-model"}}, nil
}
func (f *fakeModelResolver) ResolveModel(ctx context.Context, modelRef string) (ModelEndpoint, error) {
	return ModelEndpoint{ModelID: modelRef, BaseURL: "http://localhost"}, nil
}
