package agents

import "testing"

func TestDeliveryGateAdmitAll(t *testing.T) {
	events := []NormalizedEvent{
		{Type: EventMessage, Message: &MessagePayload{Role: "user", Text: "hello"}},
		{Type: EventThinking, Thinking: &ThinkingPayload{Text: "thinking..."}},
		{Type: EventToolCall, ToolCall: &ToolCallPayload{CallID: "bash-1", Name: "execute_command"}},
		{Type: EventToolResult, ToolResult: &ToolResultPayload{CallID: "bash-1", Output: "done"}},
		{Type: EventToolCall, ToolCall: &ToolCallPayload{CallID: "q-1", Name: "ask_user_question"}},
		{Type: EventToolResult, ToolResult: &ToolResultPayload{CallID: "q-1", Output: "selected B"}},
		{Type: EventUnknown, Raw: []byte(`{"type":"message_update"}`)},
		{Type: EventInteraction, Interaction: &InteractionPayload{ID: "perm-1"}},
		{Type: EventTurn, Turn: &TurnPayload{ID: "t-1", Phase: TurnEnded}},
		{Type: EventMessage, Message: &MessagePayload{Role: "assistant", Text: "reply"}},
	}

	admitted := NewDeliveryGate().AdmitAll(events)
	want := []EventType{EventMessage, EventToolCall, EventToolResult, EventInteraction, EventTurn, EventMessage}
	if len(admitted) != len(want) {
		t.Fatalf("admitted %d events, want %d", len(admitted), len(want))
	}
	for i, typ := range want {
		if admitted[i].Type != typ {
			t.Errorf("admitted[%d].Type = %s, want %s", i, admitted[i].Type, typ)
		}
	}
	if admitted[1].ToolCall.CallID != "q-1" || admitted[2].ToolResult.CallID != "q-1" {
		t.Errorf("question call and result not paired: %+v %+v", admitted[1].ToolCall, admitted[2].ToolResult)
	}
}

func TestDeliveryGateStreaming(t *testing.T) {
	g := NewDeliveryGate()

	if g.Admit(NormalizedEvent{Type: EventThinking, Thinking: &ThinkingPayload{Text: "r"}}) {
		t.Error("thinking admitted, want dropped")
	}
	if g.Admit(NormalizedEvent{Type: EventUnknown, Raw: []byte(`{}`)}) {
		t.Error("unknown admitted, want dropped")
	}
	if g.Admit(NormalizedEvent{Type: EventToolCall, ToolCall: &ToolCallPayload{CallID: "read-1", Name: "read_file"}}) {
		t.Error("read_file tool_call admitted, want dropped")
	}
	if g.Admit(NormalizedEvent{Type: EventToolResult, ToolResult: &ToolResultPayload{CallID: "read-1"}}) {
		t.Error("read_file tool_result admitted, want dropped")
	}
	if !g.Admit(NormalizedEvent{Type: EventToolCall, ToolCall: &ToolCallPayload{CallID: "ask-1", Name: "ask_question"}}) {
		t.Fatal("ask_question tool_call dropped, want admitted")
	}
	if !g.Admit(NormalizedEvent{Type: EventToolResult, ToolResult: &ToolResultPayload{CallID: "ask-1"}}) {
		t.Error("ask_question tool_result dropped, want admitted")
	}
	if !g.Admit(NormalizedEvent{Type: EventTurn, Turn: &TurnPayload{ID: "turn-1", Phase: TurnEnded}}) {
		t.Fatal("turn event dropped, want admitted")
	}
	if g.Admit(NormalizedEvent{Type: EventToolResult, ToolResult: &ToolResultPayload{CallID: "ask-1"}}) {
		t.Error("stale question tool_result after TurnEnded admitted, want dropped")
	}
}
