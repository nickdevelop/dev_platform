package agy

import (
	"os"
	"path/filepath"
	"testing"

	"devplatform/internal/core/agents"
)

func TestParseTranscriptFile(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "transcript.jsonl")

	content := `{"step_index":0,"source":"USER_EXPLICIT","type":"USER_INPUT","status":"DONE","created_at":"2026-11-12T07:10:19Z","content":"<USER_REQUEST>\nhello world\n</USER_REQUEST>\n<ADDITIONAL_METADATA>\ntime</ADDITIONAL_METADATA>"}
{"step_index":1,"source":"MODEL","type":"PLANNER_RESPONSE","status":"DONE","created_at":"2026-11-12T07:10:20Z","input_tokens":100,"output_tokens":50,"content":"Hello back!"}
{"step_index":2,"source":"MODEL","type":"PLANNER_RESPONSE","status":"DONE","created_at":"2026-11-12T07:10:21Z","tool_calls":[{"name":"date","args":{"format":"rfc3339"}}]}
{"step_index":3,"source":"MODEL","type":"GENERIC","status":"DONE","created_at":"2026-11-12T07:10:22Z","content":"2026-11-12T07:10:22Z"}
`
	if err := os.WriteFile(logPath, []byte(content), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	events, err := ParseTranscriptFile(logPath, "inst-1", "ses-1")
	if err != nil {
		t.Fatalf("parse transcript: %v", err)
	}

	// Expected:
	// TurnStarted
	// User message ("hello world")
	// Assistant message ("Hello back!")
	// ToolCall ("date")
	// ToolResult ("2026-11-12T07:10:22Z")
	// TurnEnded
	if len(events) != 6 {
		t.Fatalf("expected 6 events, got %d: %+v", len(events), events)
	}

	if events[0].Type != agents.EventTurn || events[0].Turn.Phase != agents.TurnStarted {
		t.Errorf("expected TurnStarted, got %+v", events[0])
	}
	if events[1].Type != agents.EventMessage || events[1].Message.Text != "hello world" {
		t.Errorf("expected clean user message 'hello world', got %q", events[1].Message.Text)
	}
	if events[2].Type != agents.EventMessage || events[2].Message.Text != "Hello back!" {
		t.Errorf("expected assistant message, got %+v", events[2])
	}
	if events[3].Type != agents.EventToolCall || events[3].ToolCall.Name != "date" {
		t.Errorf("expected tool call 'date', got %+v", events[3])
	}
	if events[4].Type != agents.EventToolResult || events[4].ToolResult.Output != "2026-11-12T07:10:22Z" {
		t.Errorf("expected tool result, got %+v", events[4])
	}
	if events[5].Type != agents.EventTurn || events[5].Turn.Phase != agents.TurnEnded {
		t.Errorf("expected TurnEnded, got %+v", events[5])
	}
}
