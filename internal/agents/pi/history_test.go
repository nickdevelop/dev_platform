package pi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"devplatform/internal/agents"
)

// writeTranscriptFixture copies the committed structural fixture into the
// adapter's session directory under the folder name pi derives from the
// working directory, substituting the runtime directory and session id.
func writeTranscriptFixture(t *testing.T, sessionRoot, directory, sessionID string) {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join("testdata", "transcript.jsonl"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	content := strings.NewReplacer("__DIR__", directory, "__SESSION_ID__", sessionID).Replace(string(raw))

	writeTranscriptFile(t, sessionRoot, directory, sessionID, content)
}

// writeTranscriptFile writes one session transcript into the pi session
// layout, creating the per-directory folder.
func writeTranscriptFile(t *testing.T, sessionRoot, directory, sessionID, content string) {
	t.Helper()

	folder := filepath.Join(sessionRoot, sessionDirName(directory))
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	name := fmt.Sprintf("2026-10-09T00-00-00-000Z_%s.jsonl", sessionID)
	if err := os.WriteFile(filepath.Join(folder, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestListHistoryMapsPiTranscript pins the whole translation: which records
// become events, in what order, with which payloads, and which are skipped.
// The skipped records (system preamble, thinking) are the ones that would make
// a replayed conversation look different from the live one.
func TestListHistoryMapsPiTranscript(t *testing.T) {
	dir := t.TempDir()
	sessionRoot := t.TempDir()
	const sessionID = "sess-hist-1"
	writeTranscriptFixture(t, sessionRoot, dir, sessionID)

	adapter := New(Options{Executable: "pi", SessionDir: sessionRoot})
	events, err := adapter.ListHistory(context.Background(), agents.ListHistoryRequest{Directory: dir, SessionID: sessionID})
	if err != nil {
		t.Fatalf("ListHistory() error = %v", err)
	}
	if len(events) != 4 {
		t.Fatalf("ListHistory() returned %d events, want 4 (user, assistant text, tool call, tool result): %+v", len(events), events)
	}

	user := events[0]
	if user.Type != agents.EventMessage || user.Message == nil ||
		user.Message.Role != "user" || user.Message.Text != "历史里的用户消息" || !user.Message.Final {
		t.Errorf("events[0] = %+v, want a final user message", user)
	}

	assistant := events[1]
	if assistant.Type != agents.EventMessage || assistant.Message == nil ||
		assistant.Message.Role != "assistant" || assistant.Message.Text != "历史里的助手回复" || !assistant.Message.Final {
		t.Errorf("events[1] = %+v, want a final assistant message (thinking skipped)", assistant)
	}

	call := events[2]
	if call.Type != agents.EventToolCall || call.ToolCall == nil ||
		call.ToolCall.CallID != "call_1" || call.ToolCall.Name != "bash" {
		t.Fatalf("events[2] = %+v, want the bash tool call", call)
	}
	var input map[string]string
	if err := json.Unmarshal(call.ToolCall.Input, &input); err != nil {
		t.Fatalf("tool call input %s is not the fixture arguments: %v", call.ToolCall.Input, err)
	}
	if input["command"] != "ls -la" {
		t.Errorf("tool call input = %v, want the fixture command", input)
	}

	result := events[3]
	if result.Type != agents.EventToolResult || result.ToolResult == nil ||
		result.ToolResult.CallID != "call_1" || result.ToolResult.Output != "历史里的工具输出" || !result.ToolResult.IsError {
		t.Errorf("events[3] = %+v, want a failed tool result for call_1", result)
	}

	for i, event := range events {
		if event.Replayed != true {
			t.Errorf("events[%d].Replayed = false, want every history event marked replayed", i)
		}
		if event.InstanceID != "" {
			t.Errorf("events[%d].InstanceID = %q, want empty: the request names no instance", i, event.InstanceID)
		}
		if event.Backend != agents.BackendPi {
			t.Errorf("events[%d].Backend = %q, want pi", i, event.Backend)
		}
		if event.Sequence != uint64(i+1) {
			t.Errorf("events[%d].Sequence = %d, want %d", i, event.Sequence, i+1)
		}
		if event.Timestamp.IsZero() {
			t.Errorf("events[%d].Timestamp is zero, want the record timestamp", i)
		}
	}
}

// TestListHistoryMissingSessionFile pins the empty case: an unknown session id
// yields nothing to replay and is not an error, matching how ListSessions
// treats a directory with no sessions.
func TestListHistoryMissingSessionFile(t *testing.T) {
	dir := t.TempDir()
	sessionRoot := t.TempDir()
	adapter := New(Options{Executable: "pi", SessionDir: sessionRoot})

	// No per-directory folder exists yet.
	events, err := adapter.ListHistory(context.Background(), agents.ListHistoryRequest{Directory: dir, SessionID: "sess-absent"})
	if err != nil {
		t.Fatalf("ListHistory() for a directory with no transcripts error = %v", err)
	}
	if len(events) != 0 {
		t.Errorf("ListHistory() = %+v, want empty", events)
	}

	// The folder exists but holds a different session.
	writeTranscriptFixture(t, sessionRoot, dir, "sess-other")
	events, err = adapter.ListHistory(context.Background(), agents.ListHistoryRequest{Directory: dir, SessionID: "sess-absent"})
	if err != nil {
		t.Fatalf("ListHistory() for an unknown session error = %v", err)
	}
	if len(events) != 0 {
		t.Errorf("ListHistory() = %+v, want empty for an unknown session id", events)
	}
}

// TestListHistoryTruncatesToMostRecent pins the bound: an unbounded transcript
// must not be materialized whole, and the events that survive must be the most
// recent ones, renumbered from one.
func TestListHistoryTruncatesToMostRecent(t *testing.T) {
	dir := t.TempDir()
	sessionRoot := t.TempDir()
	const sessionID = "sess-long"

	total := agents.MaxHistoryEvents + 10
	lines := make([]string, 0, total+1)
	lines = append(lines, fmt.Sprintf(
		`{"type":"session","version":3,"id":%q,"timestamp":"2026-10-09T00:00:00.000Z","cwd":%q}`, sessionID, dir))
	for i := 0; i < total; i++ {
		lines = append(lines, fmt.Sprintf(
			`{"type":"message","id":"m%d","parentId":null,"timestamp":"2026-10-09T00:00:01.000Z","message":{"role":"user","content":[{"type":"text","text":"消息 %d"}]}}`,
			i, i))
	}
	writeTranscriptFile(t, sessionRoot, dir, sessionID, strings.Join(lines, "\n")+"\n")

	adapter := New(Options{Executable: "pi", SessionDir: sessionRoot})
	events, err := adapter.ListHistory(context.Background(), agents.ListHistoryRequest{Directory: dir, SessionID: sessionID})
	if err != nil {
		t.Fatalf("ListHistory() error = %v", err)
	}
	if len(events) != agents.MaxHistoryEvents {
		t.Fatalf("ListHistory() returned %d events, want the bound of %d", len(events), agents.MaxHistoryEvents)
	}
	// The first surviving event is the oldest of the most recent window: the
	// dropped ten are 消息 0..9.
	if got := events[0].Message.Text; got != "消息 10" {
		t.Errorf("first replayed event = %q, want the oldest event inside the window (消息 10)", got)
	}
	if got := events[len(events)-1].Message.Text; got != fmt.Sprintf("消息 %d", total-1) {
		t.Errorf("last replayed event = %q, want the newest event", got)
	}
	if events[0].Sequence != 1 {
		t.Errorf("first replayed event Sequence = %d, want renumbering from 1", events[0].Sequence)
	}
}

// TestListHistoryRejectsEmptyRequest pins that an unscoped read is refused
// rather than silently returning another directory's transcript.
func TestListHistoryRejectsEmptyRequest(t *testing.T) {
	adapter := New(Options{Executable: "pi", SessionDir: t.TempDir()})
	cases := map[string]agents.ListHistoryRequest{
		"empty directory": {SessionID: "sess-1"},
		"empty session":   {Directory: t.TempDir()},
	}
	for name, request := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := adapter.ListHistory(context.Background(), request); !errors.Is(err, agents.ErrInvalidHistoryRequest) {
				t.Errorf("ListHistory(%+v) error = %v, want ErrInvalidHistoryRequest", request, err)
			}
		})
	}
}
