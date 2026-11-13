package sessions

import (
	"testing"

	"devplatform/internal/core/agents"
)

func TestFilterDeliverableEvents(t *testing.T) {
	events := []agents.NormalizedEvent{
		{
			Type:    agents.EventMessage,
			Message: &agents.MessagePayload{Role: "user", Text: "hello"},
		},
		{
			Type:     agents.EventThinking,
			Thinking: &agents.ThinkingPayload{Text: "thinking..."},
		},
		{
			Type:     agents.EventToolCall,
			ToolCall: &agents.ToolCallPayload{CallID: "bash-1", Name: "execute_command"},
		},
		{
			Type:       agents.EventToolResult,
			ToolResult: &agents.ToolResultPayload{CallID: "bash-1", Output: "done"},
		},
		{
			Type:     agents.EventToolCall,
			ToolCall: &agents.ToolCallPayload{CallID: "q-1", Name: "ask_user_question"},
		},
		{
			Type:       agents.EventToolResult,
			ToolResult: &agents.ToolResultPayload{CallID: "q-1", Output: "selected B"},
		},
		{
			Type:        agents.EventInteraction,
			Interaction: &agents.InteractionPayload{ID: "perm-1"},
		},
		{
			Type: agents.EventTurn,
			Turn: &agents.TurnPayload{ID: "t-1", Phase: agents.TurnEnded},
		},
		{
			Type:    agents.EventMessage,
			Message: &agents.MessagePayload{Role: "assistant", Text: "reply"},
		},
	}

	filtered := FilterDeliverableEvents(events)
	if len(filtered) != 6 {
		t.Fatalf("FilterDeliverableEvents returned %d events, want 6", len(filtered))
	}

	expectedTypes := []agents.EventType{
		agents.EventMessage,
		agents.EventToolCall,
		agents.EventToolResult,
		agents.EventInteraction,
		agents.EventTurn,
		agents.EventMessage,
	}

	for i, want := range expectedTypes {
		if filtered[i].Type != want {
			t.Errorf("filtered[%d].Type = %s, want %s", i, filtered[i].Type, want)
		}
	}
	if filtered[1].ToolCall.CallID != "q-1" {
		t.Errorf("filtered[1].ToolCall.CallID = %s, want q-1", filtered[1].ToolCall.CallID)
	}
	if filtered[2].ToolResult.CallID != "q-1" {
		t.Errorf("filtered[2].ToolResult.CallID = %s, want q-1", filtered[2].ToolResult.CallID)
	}
}

func TestEventDeliverFilterStreaming(t *testing.T) {
	f := newEventDeliverFilter()

	// 1. Thinking is dropped
	ev, ok := f.Filter(agents.NormalizedEvent{
		Type:     agents.EventThinking,
		Thinking: &agents.ThinkingPayload{Text: "reasoning..."},
	})
	if ok {
		t.Errorf("thinking event delivered, want dropped")
	}

	// 2. Regular tool_call (e.g. read_file) is dropped
	ev, ok = f.Filter(agents.NormalizedEvent{
		Type:     agents.EventToolCall,
		ToolCall: &agents.ToolCallPayload{CallID: "read-1", Name: "read_file"},
	})
	if ok {
		t.Errorf("read_file tool_call delivered, want dropped")
	}

	// 3. Regular tool_result is dropped
	ev, ok = f.Filter(agents.NormalizedEvent{
		Type:       agents.EventToolResult,
		ToolResult: &agents.ToolResultPayload{CallID: "read-1", Output: "file content"},
	})
	if ok {
		t.Errorf("read_file tool_result delivered, want dropped")
	}

	// 4. Question tool_call is kept and tracked
	ev, ok = f.Filter(agents.NormalizedEvent{
		Type:     agents.EventToolCall,
		ToolCall: &agents.ToolCallPayload{CallID: "ask-1", Name: "ask_question"},
	})
	if !ok || ev.ToolCall.CallID != "ask-1" {
		t.Errorf("ask_question tool_call dropped or mismatch, ok=%v", ok)
	}

	// 5. Question tool_result is kept
	ev, ok = f.Filter(agents.NormalizedEvent{
		Type:       agents.EventToolResult,
		ToolResult: &agents.ToolResultPayload{CallID: "ask-1", Output: "answer"},
	})
	if !ok || ev.ToolResult.CallID != "ask-1" {
		t.Errorf("ask_question tool_result dropped or mismatch, ok=%v", ok)
	}

	// 6. Interaction is kept
	ev, ok = f.Filter(agents.NormalizedEvent{
		Type:        agents.EventInteraction,
		Interaction: &agents.InteractionPayload{ID: "i-1"},
	})
	if !ok {
		t.Errorf("interaction event dropped, want delivered")
	}

	// 7. TurnEnded cleans tracked question IDs
	ev, ok = f.Filter(agents.NormalizedEvent{
		Type: agents.EventTurn,
		Turn: &agents.TurnPayload{ID: "turn-1", Phase: agents.TurnEnded},
	})
	if !ok {
		t.Errorf("turn event dropped, want delivered")
	}

	// 8. Next turn: old ask-1 tool_result should now be dropped
	ev, ok = f.Filter(agents.NormalizedEvent{
		Type:       agents.EventToolResult,
		ToolResult: &agents.ToolResultPayload{CallID: "ask-1", Output: "stale answer"},
	})
	if ok {
		t.Errorf("stale question tool_result after TurnEnded was delivered, want dropped")
	}
}
