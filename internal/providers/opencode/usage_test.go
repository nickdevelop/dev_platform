package opencode

import (
	"encoding/json"
	"testing"

	"devplatform/internal/core/agents"
)

// tokensMessage builds an assistant history message carrying wire tokens.
func tokensMessage(input, output, reasoning, read, write int) Message {
	tokens := &WireTokens{Input: input, Output: output, Reasoning: reasoning}
	tokens.Cache.Read = read
	tokens.Cache.Write = write
	return Message{
		Type:   "assistant",
		Tokens: tokens,
		Content: []ContentBlock{
			{Type: "text", Text: "first"},
			{Type: "text", Text: "second"},
		},
	}
}

// TestNormalizeHistoryMessageCarriesTokensOnce verifies the turn's usage
// anchors the first text block exactly once: a multi-block reply must not
// report its tokens several times.
func TestNormalizeHistoryMessageCarriesTokensOnce(t *testing.T) {
	events := normalizeHistoryMessage(tokensMessage(100, 20, 5, 1000, 50))
	if len(events) != 2 {
		t.Fatalf("events = %d, want 2 text events", len(events))
	}
	usage := events[0].Message.Usage
	if usage == nil {
		t.Fatal("first event carries no usage")
	}
	if usage.Input != 100 || usage.Output != 20 || usage.Reasoning != 5 || usage.CacheRead != 1000 || usage.CacheWrite != 50 {
		t.Errorf("usage = %+v, want the wire tokens", usage)
	}
	if total := usage.Total(); total != 1175 {
		t.Errorf("usage total = %d, want 1175", total)
	}
	if events[1].Message.Usage != nil {
		t.Errorf("second event usage = %+v, want nil", events[1].Message.Usage)
	}
}

// TestNormalizeHistoryMessageDropsZeroTokens verifies placeholder token
// blocks never become a misleading anchor.
func TestNormalizeHistoryMessageDropsZeroTokens(t *testing.T) {
	message := tokensMessage(0, 0, 0, 0, 0)
	for _, event := range normalizeHistoryMessage(message) {
		if event.Message != nil && event.Message.Usage != nil {
			t.Fatalf("usage = %+v, want nil for zero tokens", event.Message.Usage)
		}
	}
}

// TestStepEndedCarriesUsage verifies a finished step's tokens ride its
// lifecycle event as the workbench's freshest context anchor.
func TestStepEndedCarriesUsage(t *testing.T) {
	adapter, _, _ := newAdapterWithServer(t, infoHandler("2.0.24"))
	instance := startTestInstance(t, adapter)
	inst := adapter.instances[instance.ID]
	inst.registerSession("ses_1", "")

	got := inst.normalize(&Event{
		ID:   "evt_1",
		Type: "session.step.ended",
		Data: json.RawMessage(`{"sessionID":"ses_1","tokens":{"input":500,"output":60,"reasoning":7,"cache":{"read":2000,"write":100}}}`),
	})
	if len(got) != 1 || got[0].Type != agents.EventLifecycle || got[0].Lifecycle == nil {
		t.Fatalf("normalize = %+v, want one lifecycle event", got)
	}
	usage := got[0].Lifecycle.Usage
	if usage == nil {
		t.Fatal("step.ended carries no usage")
	}
	if usage.Input != 500 || usage.Output != 60 || usage.Reasoning != 7 || usage.CacheRead != 2000 || usage.CacheWrite != 100 {
		t.Errorf("usage = %+v, want the step tokens", usage)
	}
}

// TestStepEndedWithoutTokensStaysSilent verifies a step event without a
// tokens body still maps to a plain lifecycle event rather than failing.
func TestStepEndedWithoutTokensStaysSilent(t *testing.T) {
	adapter, _, _ := newAdapterWithServer(t, infoHandler("2.0.24"))
	instance := startTestInstance(t, adapter)
	inst := adapter.instances[instance.ID]
	inst.registerSession("ses_1", "")

	got := inst.normalize(&Event{
		ID:   "evt_1",
		Type: "session.step.ended",
		Data: json.RawMessage(`{"sessionID":"ses_1"}`),
	})
	if len(got) != 1 || got[0].Lifecycle == nil {
		t.Fatalf("normalize = %+v, want one lifecycle event", got)
	}
	if got[0].Lifecycle.Usage != nil {
		t.Errorf("usage = %+v, want nil", got[0].Lifecycle.Usage)
	}
}
