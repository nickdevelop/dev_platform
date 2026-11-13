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

func TestParseTranscriptFile_HydratesTruncatedFields(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "transcript.jsonl")
	fullLogPath := filepath.Join(tmpDir, "transcript_full.jsonl")

	compactContent := `{"step_index":0,"source":"USER_EXPLICIT","type":"USER_INPUT","status":"DONE","created_at":"2026-11-12T07:10:19Z","content":"<USER_REQUEST>tell me everything</USER_REQUEST>"}
{"step_index":1,"source":"MODEL","type":"PLANNER_RESPONSE","status":"DONE","created_at":"2026-11-12T07:10:20Z","content":"Part 1\n<truncated 5000 bytes>\nPart 2","truncated_fields":["content"]}
`
	fullContent := `{"step_index":0,"source":"USER_EXPLICIT","type":"USER_INPUT","status":"DONE","created_at":"2026-11-12T07:10:19Z","content":"<USER_REQUEST>tell me everything</USER_REQUEST>"}
{"step_index":1,"source":"MODEL","type":"PLANNER_RESPONSE","status":"DONE","created_at":"2026-11-12T07:10:20Z","content":"Part 1 - The Complete Unabridged Story from start to finish - Part 2"}
`
	if err := os.WriteFile(logPath, []byte(compactContent), 0644); err != nil {
		t.Fatalf("write compact log: %v", err)
	}
	if err := os.WriteFile(fullLogPath, []byte(fullContent), 0644); err != nil {
		t.Fatalf("write full log: %v", err)
	}

	events, err := ParseTranscriptFile(logPath, "inst-1", "ses-1")
	if err != nil {
		t.Fatalf("parse transcript: %v", err)
	}

	// TurnStarted, UserMessage, AssistantMessage, TurnEnded
	if len(events) != 4 {
		t.Fatalf("expected 4 events, got %d", len(events))
	}

	assistantMsg := events[2]
	if assistantMsg.Type != agents.EventMessage || assistantMsg.Message.Role != "assistant" {
		t.Fatalf("expected assistant message event, got %+v", assistantMsg)
	}

	expectedFull := "Part 1 - The Complete Unabridged Story from start to finish - Part 2"
	if assistantMsg.Message.Text != expectedFull {
		t.Errorf("expected hydrated full text %q, got %q", expectedFull, assistantMsg.Message.Text)
	}
}

func TestParseTranscriptFile_GracefulFallbackWhenFullMissing(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "transcript.jsonl")

	compactContent := `{"step_index":0,"source":"USER_EXPLICIT","type":"USER_INPUT","status":"DONE","created_at":"2026-11-12T07:10:19Z","content":"<USER_REQUEST>hi</USER_REQUEST>"}
{"step_index":1,"source":"MODEL","type":"PLANNER_RESPONSE","status":"DONE","created_at":"2026-11-12T07:10:20Z","content":"Partial <truncated> text","truncated_fields":["content"]}
`
	if err := os.WriteFile(logPath, []byte(compactContent), 0644); err != nil {
		t.Fatalf("write compact log: %v", err)
	}

	events, err := ParseTranscriptFile(logPath, "inst-1", "ses-1")
	if err != nil {
		t.Fatalf("parse transcript should succeed even without full transcript: %v", err)
	}

	if len(events) != 4 {
		t.Fatalf("expected 4 events, got %d", len(events))
	}
	if events[2].Message.Text != "Partial <truncated> text" {
		t.Errorf("expected fallback to compact text, got %q", events[2].Message.Text)
	}
}
