package pi

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestHistoryRecordCarriesAssistantUsage verifies a transcript assistant
// message forwards its provider-reported usage as the context anchor.
func TestHistoryRecordCarriesAssistantUsage(t *testing.T) {
	record := historyRecord{Type: "message"}
	record.Timestamp = parserClock()
	record.Message = &struct {
		Role       string          `json:"role"`
		Content    json.RawMessage `json:"content"`
		ToolCallID string          `json:"toolCallId"`
		IsError    bool            `json:"isError"`
		Usage      *wireUsage      `json:"usage"`
	}{
		Role:    "assistant",
		Content: json.RawMessage(`"hello"`),
		Usage:   &wireUsage{Input: 800, Output: 40, CacheRead: 5000},
	}

	events := normalizeHistoryRecord(record, nil, nil)
	if len(events) != 1 || events[0].Message == nil {
		t.Fatalf("events = %+v, want one message", events)
	}
	usage := events[0].Message.Usage
	if usage == nil {
		t.Fatal("assistant message carries no usage")
	}
	if usage.Input != 800 || usage.Output != 40 || usage.CacheRead != 5000 {
		t.Errorf("usage = %+v, want the transcript usage", usage)
	}
	if total := usage.Total(); total != 5840 {
		t.Errorf("usage total = %d, want 5840", total)
	}
}

// TestHistoryRecordSkipsStandaloneUsage verifies cost-bookkeeping usage
// records (not messages) never surface as conversation or context.
func TestHistoryRecordSkipsStandaloneUsage(t *testing.T) {
	record := historyRecord{Type: "usage"}
	if events := normalizeHistoryRecord(record, nil, nil); len(events) != 0 {
		t.Fatalf("events = %+v, want none", events)
	}
}

// TestParserFinalMessageCarriesEnvelopeUsage verifies the live stream's
// message_update usage lands on the closing message as its anchor, while
// streaming deltas never carry it.
func TestParserFinalMessageCarriesEnvelopeUsage(t *testing.T) {
	input := strings.Join([]string{
		`{"type":"session","version":3,"id":"sess-1","cwd":"/data/projects/demo"}`,
		`{"type":"message_update","usage":{"input":900,"output":30,"cacheRead":100},"assistantMessageEvent":{"type":"text_delta","contentIndex":0,"delta":"Hi"}}`,
		`{"type":"message_end","usage":{"input":900,"output":30,"cacheRead":100},"message":{"role":"assistant","content":[{"type":"text","text":"Hi"}],"stopReason":"stop"}}`,
	}, "\n") + "\n"

	got := collect(t, input)
	if len(got) != 3 {
		t.Fatalf("got %d events, want 3: %+v", len(got), got)
	}
	if got[1].Message == nil || got[1].Message.Final {
		t.Fatalf("event[1] = %+v, want a non-final delta", got[1])
	}
	if got[1].Message.Usage != nil {
		t.Errorf("delta usage = %+v, want nil", got[1].Message.Usage)
	}
	usage := got[2].Message.Usage
	if usage == nil {
		t.Fatal("final message carries no usage")
	}
	if usage.Input != 900 || usage.Output != 30 || usage.CacheRead != 100 {
		t.Errorf("usage = %+v, want the envelope usage", usage)
	}
}
