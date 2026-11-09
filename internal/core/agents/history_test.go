package agents

import (
	"errors"
	"testing"
)

func userMessage(text string) NormalizedEvent {
	return NormalizedEvent{
		Type:    EventMessage,
		Message: &MessagePayload{Role: "user", Text: text, Final: true},
	}
}

func assistantMessage(text string) NormalizedEvent {
	return NormalizedEvent{
		Type:    EventMessage,
		Message: &MessagePayload{Role: "assistant", Text: text, Final: true},
	}
}

func TestPageHistoryLegacyBound(t *testing.T) {
	events := make([]NormalizedEvent, MaxHistoryEvents+5)
	for i := range events {
		events[i] = assistantMessage("event")
	}

	result, err := PageHistory(events, 0, "")
	if err != nil {
		t.Fatalf("PageHistory: %v", err)
	}
	if !result.Truncated {
		t.Error("Truncated = false, want true for a transcript past the hard bound")
	}
	if result.HasMore {
		t.Error("HasMore = true on a legacy read, want false")
	}
	if len(result.Events) != MaxHistoryEvents {
		t.Fatalf("len(Events) = %d, want %d", len(result.Events), MaxHistoryEvents)
	}
	if result.Events[0].Sequence != 1 {
		t.Errorf("first Sequence = %d, want 1", result.Events[0].Sequence)
	}
}

func TestPageHistoryWalksTurnsWithoutDuplicates(t *testing.T) {
	events := []NormalizedEvent{
		userMessage("u1"),
		assistantMessage("a1"),
		userMessage("u2"),
		{Type: EventThinking, Thinking: &ThinkingPayload{Text: "think", Final: true}},
		assistantMessage("a2"),
		userMessage("u3"),
		{Type: EventToolCall, ToolCall: &ToolCallPayload{CallID: "c1", Name: "bash"}},
		{Type: EventToolResult, ToolResult: &ToolResultPayload{CallID: "c1", Output: "ok"}},
		assistantMessage("a3"),
	}

	var collected []string
	cursor := ""
	for page := 0; page < 5; page++ {
		result, err := PageHistory(events, 4, cursor)
		if err != nil {
			t.Fatalf("PageHistory page %d: %v", page, err)
		}
		var pageTexts []string
		for _, event := range result.Events {
			if event.Message != nil {
				pageTexts = append(pageTexts, event.Message.Text)
			}
		}
		collected = append(pageTexts, collected...)
		if !result.HasMore {
			break
		}
		if result.NextCursor == "" {
			t.Fatal("HasMore = true but NextCursor is empty")
		}
		if result.NextCursor == cursor {
			t.Fatal("cursor did not advance")
		}
		cursor = result.NextCursor
	}

	want := []string{"u1", "a1", "u2", "a2", "u3", "a3"}
	if len(collected) != len(want) {
		t.Fatalf("collected = %v, want %v", collected, want)
	}
	for i := range want {
		if collected[i] != want[i] {
			t.Fatalf("collected = %v, want %v", collected, want)
		}
	}
}

func TestPageHistoryExtendsPageRatherThanSplittingFinalTurn(t *testing.T) {
	events := []NormalizedEvent{
		userMessage("u1"),
		{Type: EventToolCall, ToolCall: &ToolCallPayload{CallID: "c1", Name: "bash"}},
		{Type: EventToolResult, ToolResult: &ToolResultPayload{CallID: "c1", Output: "1"}},
		{Type: EventToolCall, ToolCall: &ToolCallPayload{CallID: "c2", Name: "bash"}},
		{Type: EventToolResult, ToolResult: &ToolResultPayload{CallID: "c2", Output: "2"}},
		{Type: EventToolCall, ToolCall: &ToolCallPayload{CallID: "c3", Name: "bash"}},
		userMessage("u2"),
		assistantMessage("a2"),
	}

	first, err := PageHistory(events, 3, "")
	if err != nil {
		t.Fatalf("PageHistory: %v", err)
	}
	if len(first.Events) != 2 || first.Events[0].Message.Text != "u2" {
		t.Fatalf("first page = %+v, want the final turn only", first.Events)
	}
	if !first.HasMore || first.NextCursor == "" {
		t.Fatalf("first page HasMore/NextCursor = %v/%q", first.HasMore, first.NextCursor)
	}

	second, err := PageHistory(events, 3, first.NextCursor)
	if err != nil {
		t.Fatalf("PageHistory second: %v", err)
	}
	if len(second.Events) != 6 || second.Events[0].Message.Text != "u1" {
		t.Fatalf("second page = %+v, want the whole first turn", second.Events)
	}
	if second.HasMore {
		t.Error("second page HasMore = true, want false")
	}
}

func TestPageHistoryRejectsInvalidCursor(t *testing.T) {
	events := []NormalizedEvent{userMessage("u"), assistantMessage("a")}
	for _, cursor := range []string{"x", "-1", "3", "1.5"} {
		if _, err := PageHistory(events, 1, cursor); !errors.Is(err, ErrInvalidHistoryCursor) {
			t.Errorf("PageHistory cursor %q error = %v, want ErrInvalidHistoryCursor", cursor, err)
		}
	}
}

func TestPageHistoryRequiresLimitForCursor(t *testing.T) {
	events := []NormalizedEvent{userMessage("u")}
	if _, err := PageHistory(events, 0, "1"); !errors.Is(err, ErrInvalidHistoryCursor) {
		t.Errorf("PageHistory error = %v, want ErrInvalidHistoryCursor", err)
	}
}

func TestPageHistoryEmpty(t *testing.T) {
	result, err := PageHistory(nil, 20, "")
	if err != nil {
		t.Fatalf("PageHistory: %v", err)
	}
	if len(result.Events) != 0 || result.HasMore || result.Truncated || result.NextCursor != "" {
		t.Fatalf("result = %+v, want an empty terminal page", result)
	}
}

func TestPageHistoryRenumbersPageAndStopsAtZero(t *testing.T) {
	events := []NormalizedEvent{
		userMessage("u1"), assistantMessage("a1"),
		userMessage("u2"), assistantMessage("a2"),
	}
	result, err := PageHistory(events, 100, "2")
	if err != nil {
		t.Fatalf("PageHistory: %v", err)
	}
	if len(result.Events) != 2 || result.Events[0].Sequence != 1 || result.Events[1].Sequence != 2 {
		t.Fatalf("page = %+v, want two events renumbered from one", result.Events)
	}
	if result.HasMore || result.NextCursor != "" {
		t.Errorf("HasMore/NextCursor = %v/%q, want false/empty", result.HasMore, result.NextCursor)
	}
}
