package agy

import (
	"encoding/json"
	"testing"
	"time"

	"devplatform/internal/core/agents"
)

func TestEventNormalizer(t *testing.T) {
	fixedTime := time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)
	now := func() time.Time { return fixedTime }
	normalizer := NewEventNormalizer("inst-1", "ses-1", now)

	// 1. Test init event
	initJSON := `{"event":"init","conversation_id":"ses-1","init":{"cwd":"/workspace","tools":["run_command"],"permission_mode":"always-proceed"}}`
	events, err := normalizer.Normalize([]byte(initJSON))
	if err != nil {
		t.Fatalf("normalize init: %v", err)
	}
	if len(events) != 1 || events[0].Type != agents.EventLifecycle {
		t.Fatalf("expected 1 lifecycle event, got %+v", events)
	}
	if events[0].Lifecycle.State != "initialized" {
		t.Errorf("lifecycle state = %q, want initialized", events[0].Lifecycle.State)
	}

	// 2. Test text delta
	deltaJSON := `{"event":"step_update","step_update":{"conversation_id":"ses-1","step_index":1,"state":"ACTIVE","step_type":"agent_response","text_delta":"Hello world"}}`
	events, err = normalizer.Normalize([]byte(deltaJSON))
	if err != nil {
		t.Fatalf("normalize delta: %v", err)
	}
	// First non-user step should start a turn, then emit message
	if len(events) != 2 {
		t.Fatalf("expected 2 events (turn start + message), got %d: %+v", len(events), events)
	}
	if events[0].Type != agents.EventTurn || events[0].Turn.Phase != agents.TurnStarted {
		t.Errorf("event[0] = %+v, want TurnStarted", events[0])
	}
	if events[1].Type != agents.EventMessage || events[1].Message.Text != "Hello world" {
		t.Errorf("event[1] = %+v, want message 'Hello world'", events[1])
	}

	// 3. Test tool call active
	toolActiveJSON := `{"event":"step_update","step_update":{"conversation_id":"ses-1","step_index":2,"state":"ACTIVE","step_type":"tool","tool_name":"run_command","tool_info":{"name":"run_command","parameters":{"CommandLine":"date"}}}}`
	events, err = normalizer.Normalize([]byte(toolActiveJSON))
	if err != nil {
		t.Fatalf("normalize tool active: %v", err)
	}
	if len(events) != 1 || events[0].Type != agents.EventToolCall {
		t.Fatalf("expected 1 tool call event, got %+v", events)
	}
	if events[0].ToolCall.Name != "run_command" || events[0].ToolCall.CallID != "call-2" {
		t.Errorf("unexpected tool call: %+v", events[0].ToolCall)
	}

	// 4. Test tool call done
	toolDoneJSON := `{"event":"step_update","step_update":{"conversation_id":"ses-1","step_index":2,"state":"DONE","step_type":"tool","tool_name":"run_command","tool_info":{"name":"run_command","parameters":{"CommandLine":"date"},"output":"Thu Nov 12 10:00:00 UTC 2026"}}}`
	events, err = normalizer.Normalize([]byte(toolDoneJSON))
	if err != nil {
		t.Fatalf("normalize tool done: %v", err)
	}
	if len(events) != 1 || events[0].Type != agents.EventToolResult {
		t.Fatalf("expected 1 tool result event, got %+v", events)
	}
	if events[0].ToolResult.CallID != "call-2" || events[0].ToolResult.Output != "Thu Nov 12 10:00:00 UTC 2026" || events[0].ToolResult.IsError {
		t.Errorf("unexpected tool result: %+v", events[0].ToolResult)
	}

	// 5. Test tool call error
	toolErrJSON := `{"event":"step_update","step_update":{"conversation_id":"ses-1","step_index":3,"state":"ERROR","step_type":"tool","tool_name":"read_file","tool_info":{"name":"read_file","error":{"type":"TOOL_ERROR","message":"not found"}}}}`
	events, err = normalizer.Normalize([]byte(toolErrJSON))
	if err != nil {
		t.Fatalf("normalize tool error: %v", err)
	}
	if len(events) != 1 || events[0].Type != agents.EventToolResult {
		t.Fatalf("expected 1 tool result event, got %+v", events)
	}
	if !events[0].ToolResult.IsError || events[0].ToolResult.Output != "not found" {
		t.Errorf("unexpected error tool result: %+v", events[0].ToolResult)
	}

	// 6. Test result event
	resultJSON := `{"event":"result","result":{"conversation_id":"ses-1","status":"SUCCESS","response":"Done!","duration_seconds":2.5,"num_turns":1,"usage":{"input_tokens":100,"output_tokens":50,"thinking_tokens":10,"cache_read_tokens":0,"total_tokens":160}}}`
	events, err = normalizer.Normalize([]byte(resultJSON))
	if err != nil {
		t.Fatalf("normalize result: %v", err)
	}
	// Expect final message + TurnEnded
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d: %+v", len(events), events)
	}
	if events[0].Type != agents.EventMessage || !events[0].Message.Final {
		t.Errorf("event[0] = %+v, want final message", events[0])
	}
	if events[0].Message.Usage == nil || events[0].Message.Usage.Input != 100 || events[0].Message.Usage.Reasoning != 10 {
		t.Errorf("unexpected usage: %+v", events[0].Message.Usage)
	}
	if events[1].Type != agents.EventTurn || events[1].Turn.Phase != agents.TurnEnded || events[1].Turn.Outcome != agents.TurnCompleted {
		t.Errorf("event[1] = %+v, want TurnCompleted", events[1])
	}

	// 7. Test unknown event / raw fallback
	unknownJSON := `{"event":"custom_alien_event","data":{"foo":"bar"}}`
	events, err = normalizer.Normalize([]byte(unknownJSON))
	if err != nil {
		t.Fatalf("normalize unknown: %v", err)
	}
	if len(events) != 1 || events[0].Type != agents.EventUnknown {
		t.Fatalf("expected 1 unknown event, got %+v", events)
	}
	var rawData map[string]any
	if err := json.Unmarshal(events[0].Raw, &rawData); err != nil || rawData["event"] != "custom_alien_event" {
		t.Errorf("unknown raw payload not preserved: %s", events[0].Raw)
	}
}
