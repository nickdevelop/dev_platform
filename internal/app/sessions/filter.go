package sessions

import (
	"strings"
	"sync"

	"devplatform/internal/core/agents"
)

var questionToolNames = map[string]struct{}{
	"question":              {},
	"ask":                   {},
	"askuserquestion":       {},
	"ask_user":              {},
	"ask_user_question":     {},
	"ask_question":          {},
	"ask_followup_question": {},
	"inquire":               {},
}

// isQuestionTool reports whether a tool name indicates an interactive user question.
func isQuestionTool(name string) bool {
	lower := strings.ToLower(strings.TrimSpace(name))
	if _, ok := questionToolNames[lower]; ok {
		return true
	}
	return strings.Contains(lower, "ask_user") || strings.Contains(lower, "question")
}

// FilterDeliverableEvents filters a slice of normalized events (such as a replayed
// history page) to eliminate thinking blocks and general tool activity (bash, edit, read, etc.),
// keeping only conversations (messages), interactive questions, tool confirmation/interactions,
// turn boundaries, lifecycle, and errors.
func FilterDeliverableEvents(events []agents.NormalizedEvent) []agents.NormalizedEvent {
	if len(events) == 0 {
		return events
	}
	questionCallIDs := make(map[string]struct{})
	for _, ev := range events {
		if ev.Type == agents.EventToolCall && ev.ToolCall != nil && isQuestionTool(ev.ToolCall.Name) {
			if ev.ToolCall.CallID != "" {
				questionCallIDs[ev.ToolCall.CallID] = struct{}{}
			}
		}
	}

	filtered := make([]agents.NormalizedEvent, 0, len(events))
	for _, ev := range events {
		switch ev.Type {
		case agents.EventThinking:
			// Strip thinking from deliverable transcripts
			continue
		case agents.EventToolCall:
			if ev.ToolCall != nil && isQuestionTool(ev.ToolCall.Name) {
				filtered = append(filtered, ev)
			}
		case agents.EventToolResult:
			if ev.ToolResult != nil && ev.ToolResult.CallID != "" {
				if _, ok := questionCallIDs[ev.ToolResult.CallID]; ok {
					filtered = append(filtered, ev)
				}
			}
		default:
			filtered = append(filtered, ev)
		}
	}
	return filtered
}

// eventDeliverFilter provides stateful filtering for live streaming events of a session.
type eventDeliverFilter struct {
	mu              sync.Mutex
	questionCallIDs map[string]struct{}
}

func newEventDeliverFilter() *eventDeliverFilter {
	return &eventDeliverFilter{
		questionCallIDs: make(map[string]struct{}),
	}
}

// Filter decides whether an event should be delivered to the client and tracks question call IDs.
func (f *eventDeliverFilter) Filter(ev agents.NormalizedEvent) (agents.NormalizedEvent, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()

	switch ev.Type {
	case agents.EventThinking:
		return ev, false
	case agents.EventToolCall:
		if ev.ToolCall != nil && isQuestionTool(ev.ToolCall.Name) {
			if ev.ToolCall.CallID != "" {
				f.questionCallIDs[ev.ToolCall.CallID] = struct{}{}
			}
			return ev, true
		}
		return ev, false
	case agents.EventToolResult:
		if ev.ToolResult != nil && ev.ToolResult.CallID != "" {
			if _, ok := f.questionCallIDs[ev.ToolResult.CallID]; ok {
				return ev, true
			}
		}
		return ev, false
	case agents.EventTurn:
		if ev.Turn != nil && ev.Turn.Phase == agents.TurnEnded {
			// Clear question call IDs at the end of the turn
			f.questionCallIDs = make(map[string]struct{})
		}
		return ev, true
	default:
		return ev, true
	}
}
