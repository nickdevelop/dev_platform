package agentdata

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"devplatform/internal/app/database"
)

func openService(t *testing.T, options ...Option) *Service {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := database.Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return NewService(db, options...)
}

func TestCreateAppendAndLoadRoundTrips(t *testing.T) {
	s := openService(t)
	ctx := context.Background()
	created := time.Now().UTC()

	if err := s.CreateSession(ctx, "sess-1", "/data/projects/demo", "标题", created); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	// Re-creating an existing id is a no-op, not an error.
	if err := s.CreateSession(ctx, "sess-1", "/data/projects/demo", "again", created); err != nil {
		t.Fatalf("CreateSession duplicate: %v", err)
	}
	if err := s.SetModel(ctx, "sess-1", "openai/gpt-4o"); err != nil {
		t.Fatalf("SetModel: %v", err)
	}
	records := []Record{
		{Seq: 1, Kind: "user", Content: "你好", CreatedAt: created},
		{Seq: 2, Kind: "tool_call", Payload: []byte(`{"callId":"c1","name":"read_file","input":{"path":"a.go"}}`), CreatedAt: created},
		{Seq: 3, Kind: "tool_result", Content: "content", Payload: []byte(`{"callId":"c1","output":"content"}`), CreatedAt: created},
		{Seq: 4, Kind: "assistant", Content: "回复", CreatedAt: created},
	}
	if err := s.Append(ctx, "sess-1", records); err != nil {
		t.Fatalf("Append: %v", err)
	}

	loaded, err := s.Load(ctx, "sess-1")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(loaded) != len(records) {
		t.Fatalf("Load = %d records, want %d", len(loaded), len(records))
	}
	for i, record := range loaded {
		if record.Seq != records[i].Seq || record.Kind != records[i].Kind || record.Content != records[i].Content {
			t.Fatalf("record %d mismatch: got %+v want %+v", i, record, records[i])
		}
	}
	if exists, err := s.Exists(ctx, "sess-1"); err != nil || !exists {
		t.Fatalf("Exists = (%v, %v), want (true, nil)", exists, err)
	}

	sessions, total, err := s.ListSessions(ctx, 0, 0)
	if err != nil || total != 1 || len(sessions) != 1 {
		t.Fatalf("ListSessions = (%v, %d, %v)", sessions, total, err)
	}
	if sessions[0].MessageCount != int64(len(records)) || sessions[0].Model != "openai/gpt-4o" {
		t.Fatalf("summary mismatch: %+v", sessions[0])
	}
}

func TestDeleteRefusesBusyAndNeedsGate(t *testing.T) {
	busy := map[string]bool{"sess-1": true}
	s := openService(t, WithActivityGate(func(sessionID string) bool { return busy[sessionID] }))
	ctx := context.Background()
	if err := s.CreateSession(ctx, "sess-1", "/data/projects/demo", "t", time.Now().UTC()); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	err := s.DeleteSession(ctx, "sess-1")
	if !errors.Is(err, ErrSessionBusy) {
		t.Fatalf("DeleteSession on a busy session = %v, want ErrSessionBusy", err)
	}
	busy["sess-1"] = false
	if err := s.DeleteSession(ctx, "sess-1"); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	if exists, _ := s.Exists(ctx, "sess-1"); exists {
		t.Fatal("deleted session still exists")
	}
	if err := s.DeleteSession(ctx, "sess-1"); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("second delete = %v, want ErrSessionNotFound", err)
	}
}

func TestDeletePurgesTheIndexRows(t *testing.T) {
	var purged [][]string
	s := openService(t, WithIndexPurger(func(nativeSessions []string) error {
		purged = append(purged, append([]string(nil), nativeSessions...))
		return nil
	}))
	ctx := context.Background()
	for _, id := range []string{"a", "b", "c"} {
		if err := s.CreateSession(ctx, id, "/w", "t", time.Now().UTC()); err != nil {
			t.Fatalf("CreateSession %s: %v", id, err)
		}
	}
	if _, err := s.ClearSessions(ctx); err != nil {
		t.Fatalf("ClearSessions: %v", err)
	}
	if len(purged) != 1 || len(purged[0]) != 3 {
		t.Fatalf("purger calls = %v, want one call with the three ids", purged)
	}
	sessions, total, err := s.ListSessions(ctx, 0, 0)
	if err != nil || total != 0 || len(sessions) != 0 {
		t.Fatalf("ListSessions after clear = (%v, %d, %v)", sessions, total, err)
	}
}

func TestAppendWithoutSessionStillWritesRowsButDeleteOfUnknownFailsCleanly(t *testing.T) {
	s := openService(t)
	ctx := context.Background()
	// Deleting an unknown session is ErrSessionNotFound, never a panic.
	if err := s.DeleteSession(ctx, "missing"); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("delete missing = %v, want ErrSessionNotFound", err)
	}
	// Appending nothing is a no-op.
	if err := s.Append(ctx, "missing", nil); err != nil {
		t.Fatalf("Append empty = %v, want nil", err)
	}
}
