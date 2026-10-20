package deepseek

import (
	"encoding/json"
	"testing"

	"devplatform/internal/core/agents"
)

func TestEventNormalization(t *testing.T) {
	norm := NewSessionEventNormalizer("inst-1", "ses-1")

	// 1. turn/start
	evs := norm.NormalizeFrame([]byte(`{
		"type": "event",
		"event": {
			"type": "turn/start",
			"data": {"turn": 1}
		}
	}`))
	if len(evs) != 1 || evs[0].Type != agents.EventTurn || evs[0].Turn.Phase != agents.TurnStarted {
		t.Fatalf("turn/start mismatch: %+v", evs)
	}
	if evs[0].Turn.ID != "turn-1" {
		t.Errorf("turn ID = %s, want turn-1", evs[0].Turn.ID)
	}

	// 2. assistant-stream text-delta
	evs = norm.NormalizeFrame([]byte(`{
		"type": "assistant-stream",
		"frame": {
			"type": "chunk",
			"chunk": {
				"type": "text-delta",
				"text": "Hello, world!"
			}
		}
	}`))
	if len(evs) != 1 || evs[0].Type != agents.EventMessage || evs[0].Message.Text != "Hello, world!" {
		t.Fatalf("text-delta mismatch: %+v", evs)
	}
	if evs[0].Message.Final {
		t.Error("streaming chunk must have Final=false")
	}

	// 3. assistant-stream reasoning-delta (thinking)
	evs = norm.NormalizeFrame([]byte(`{
		"type": "assistant-stream",
		"frame": {
			"type": "chunk",
			"chunk": {
				"type": "reasoning-delta",
				"text": "I think..."
			}
		}
	}`))
	if len(evs) != 1 || evs[0].Type != agents.EventThinking || evs[0].Thinking.Text != "I think..." {
		t.Fatalf("reasoning-delta mismatch: %+v", evs)
	}

	// 4. tool/call
	evs = norm.NormalizeFrame([]byte(`{
		"type": "event",
		"event": {
			"type": "tool/call",
			"data": {
				"callId": "call-1",
				"name": "read",
				"arguments": {"path": "main.go"}
			}
		}
	}`))
	if len(evs) != 1 || evs[0].Type != agents.EventToolCall || evs[0].ToolCall.CallID != "call-1" || evs[0].ToolCall.Name != "read" {
		t.Fatalf("tool/call mismatch: %+v", evs)
	}

	// 5. tool/result
	evs = norm.NormalizeFrame([]byte(`{
		"type": "event",
		"event": {
			"type": "tool/result",
			"data": {
				"message": {
					"toolCallId": "call-1",
					"isError": false,
					"content": "package main"
				}
			}
		}
	}`))
	if len(evs) != 1 || evs[0].Type != agents.EventToolResult || evs[0].ToolResult.CallID != "call-1" || evs[0].ToolResult.Output != "package main" {
		t.Fatalf("tool/result mismatch: %+v", evs)
	}

	// 6. token usage (assistant/message)
	evs = norm.NormalizeFrame([]byte(`{
		"type": "event",
		"event": {
			"type": "assistant/message",
			"data": {
				"usage": {
					"inputTokens": 100,
					"outputTokens": 50,
					"cacheReadTokens": 20,
					"cacheWriteTokens": 5
				}
			}
		}
	}`))
	if len(evs) != 1 || evs[0].Type != agents.EventLifecycle || evs[0].Lifecycle.Usage == nil {
		t.Fatalf("usage mismatch: %+v", evs)
	}
	u := evs[0].Lifecycle.Usage
	if u.Input != 100 || u.Output != 50 || u.CacheRead != 20 || u.CacheWrite != 5 {
		t.Errorf("usage values mismatch: %+v", u)
	}

	// 7. turn/end completed
	evs = norm.NormalizeFrame([]byte(`{
		"type": "event",
		"event": {
			"type": "turn/end",
			"data": {
				"reason": {"kind": "completed"}
			}
		}
	}`))
	if len(evs) != 1 || evs[0].Type != agents.EventTurn || evs[0].Turn.Phase != agents.TurnEnded || evs[0].Turn.Outcome != agents.TurnCompleted {
		t.Fatalf("turn/end completed mismatch: %+v", evs)
	}

	// 8. turn/end failed
	norm.currentTurnID = "turn-2"
	evs = norm.NormalizeFrame([]byte(`{
		"type": "event",
		"event": {
			"type": "turn/end",
			"data": {
				"reason": {"kind": "error", "error": {"message": "model error"}}
			}
		}
	}`))
	if len(evs) != 2 {
		t.Fatalf("turn/end failed should emit turn and error events, got %d", len(evs))
	}
	if evs[0].Type != agents.EventTurn || evs[0].Turn.Outcome != agents.TurnFailed {
		t.Errorf("event 0 = %+v, want TurnFailed", evs[0])
	}
	if evs[1].Type != agents.EventError || evs[1].Error.Message != "model error" {
		t.Errorf("event 1 = %+v, want EventError with 'model error'", evs[1])
	}

	// 9. Sequence monotonically increasing
	for i := 1; i < len(evs); i++ {
		if evs[i].Sequence <= evs[i-1].Sequence {
			t.Errorf("sequences not strictly increasing: %d <= %d", evs[i].Sequence, evs[i-1].Sequence)
		}
	}

	_ = json.Marshal
}
