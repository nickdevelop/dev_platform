package agents

import (
	"fmt"
	"strconv"
)

// MaxHistoryPageSize bounds one paginated history page. It is deliberately
// larger than the workbench's first page (200) but small enough that a hostile
// client cannot ask for an unbounded response.
const MaxHistoryPageSize = 1000

// PageHistory slices one history page out of the adapter's normalized events.
//
// A zero limit preserves the legacy read: the newest MaxHistoryEvents events,
// with Truncated reporting that older ones were dropped. A positive limit
// returns a page ending at the cursor (the whole tail for an empty cursor) and
// walks the page start back to the nearest user message, so a turn boundary is
// never split across two pages. Limit is therefore a soft bound: one long turn
// can make the page larger.
//
// The cursor is the page's start index in the full event list, opaque to
// clients. It assumes an append-only transcript; after the agent rewrites its
// transcript the cursor may no longer address the same position, in which case
// PageHistory reports ErrInvalidHistoryCursor rather than returning a
// misleading page.
func PageHistory(events []NormalizedEvent, limit int, before string) (HistoryResult, error) {
	total := len(events)

	if limit <= 0 {
		if before != "" {
			return HistoryResult{}, fmt.Errorf("%w: cursor requires a limit", ErrInvalidHistoryCursor)
		}
		start := 0
		truncated := total > MaxHistoryEvents
		if truncated {
			start = total - MaxHistoryEvents
		}
		page := cloneHistoryEvents(events[start:])
		renumberHistory(page)
		return HistoryResult{Events: page, Truncated: truncated}, nil
	}

	if limit > MaxHistoryPageSize {
		limit = MaxHistoryPageSize
	}

	end := total
	if before != "" {
		index, err := strconv.Atoi(before)
		if err != nil || index < 0 || index > total {
			return HistoryResult{}, fmt.Errorf("%w: %q", ErrInvalidHistoryCursor, before)
		}
		end = index
	}
	if end == 0 {
		return HistoryResult{Events: []NormalizedEvent{}}, nil
	}

	start := historyPageStart(events, end, limit)

	page := cloneHistoryEvents(events[start:end])
	renumberHistory(page)
	result := HistoryResult{Events: page, HasMore: start > 0}
	if result.HasMore {
		result.NextCursor = strconv.Itoa(start)
	}
	return result, nil
}

// historyPageStart chooses a page start that does not cut a turn in half.
//
// It prefers the first user message at or after the nominal lower bound, which
// keeps the page within the requested size. When the lower bound falls inside
// the final turn and no later user message exists, it walks back to that
// turn's user message; the page can then exceed the soft limit rather than
// returning an empty page or an orphaned tool result.
func historyPageStart(events []NormalizedEvent, end, limit int) int {
	lower := end - limit
	if lower < 0 {
		lower = 0
	}
	for index := lower; index < end; index++ {
		if isUserTurnStart(events[index]) {
			return index
		}
	}
	for index := lower; index >= 0; index-- {
		if isUserTurnStart(events[index]) {
			return index
		}
	}
	return 0
}

// isUserTurnStart reports whether event begins a user turn. PageHistory uses
// it to keep one turn's thinking/tool events on the same side of a page
// boundary as the user message that opened it.
func isUserTurnStart(event NormalizedEvent) bool {
	return event.Type == EventMessage && event.Message != nil && event.Message.Role == "user"
}

func cloneHistoryEvents(events []NormalizedEvent) []NormalizedEvent {
	if len(events) == 0 {
		return []NormalizedEvent{}
	}
	page := make([]NormalizedEvent, len(events))
	copy(page, events)
	return page
}

// renumberHistory restores the API's dense sequence contract for one page.
func renumberHistory(events []NormalizedEvent) {
	for i := range events {
		events[i].Sequence = uint64(i + 1)
	}
}
