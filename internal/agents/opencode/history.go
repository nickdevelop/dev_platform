package opencode

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"devplatform/internal/agents"
)

// Compile-time proof that the opencode adapter can serve history reads.
var _ agents.HistoryReader = (*Adapter)(nil)

// ListHistory reads a session's native message log and translates it into the
// platform's normalized events.
//
// The translation mirrors the live stream: assistant reasoning and the
// endpoint's turn markers produce nothing, so a replayed conversation looks
// like the one the user watched. Returned events leave InstanceID and
// SessionID empty — the request names an instance-free scope — and are marked
// Replayed.
func (a *Adapter) ListHistory(ctx context.Context, request agents.ListHistoryRequest) (agents.HistoryResult, error) {
	if err := ctx.Err(); err != nil {
		return agents.HistoryResult{}, err
	}
	if err := request.Validate(); err != nil {
		return agents.HistoryResult{}, err
	}

	inst, err := a.historyInstance()
	if err != nil {
		return agents.HistoryResult{}, err
	}
	messages, err := inst.client.ListMessages(ctx, request.SessionID)
	if err != nil {
		return agents.HistoryResult{}, fmt.Errorf("opencode: read history for session %q: %w", request.SessionID, err)
	}

	events := make([]agents.NormalizedEvent, 0, len(messages))
	for _, message := range messages {
		events = append(events, normalizeHistoryMessage(message)...)
	}
	return boundHistory(events), nil
}

// historyInstance returns a running instance whose client can answer for the
// requested session.
//
// The opencode server is machine-global: the native message log lives on disk
// and the directory is a per-request scope, so any running server reads any
// session's history. The smallest instance id is chosen only to make the
// selection reproducible; no instance is preferred over another.
func (a *Adapter) historyInstance() (*instance, error) {
	a.mu.Lock()
	ids := make([]string, 0, len(a.instances))
	for id := range a.instances {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	candidates := make([]*instance, 0, len(ids))
	for _, id := range ids {
		candidates = append(candidates, a.instances[id])
	}
	a.mu.Unlock()

	// Checked outside the adapter lock: isClosed takes the instance lock, and
	// the instance teardown path must never wait on the adapter lock.
	for _, inst := range candidates {
		if !inst.isClosed() {
			return inst, nil
		}
	}
	return nil, errors.New("opencode: no running instance; start the backend before reading session history")
}

// normalizeHistoryMessage translates one native message. A message can yield
// two events (an assistant entry that both speaks and calls a tool), one, or
// none (reasoning, turn markers, model switches).
func normalizeHistoryMessage(message Message) []agents.NormalizedEvent {
	observed := time.Now().UTC()
	if message.Time.Created != 0 {
		observed = time.UnixMilli(message.Time.Created).UTC()
	}
	envelope := agents.NormalizedEvent{
		Backend:   agents.BackendOpenCodeV2,
		Timestamp: observed,
		Replayed:  true,
	}

	switch message.Type {
	case "user":
		if message.Text == "" {
			return nil
		}
		event := envelope
		event.Type = agents.EventMessage
		event.Message = &agents.MessagePayload{Role: "user", Text: message.Text, Final: true}
		return []agents.NormalizedEvent{event}

	case "assistant":
		events := make([]agents.NormalizedEvent, 0, len(message.Content))
		for _, block := range message.Content {
			switch block.Type {
			case "text":
				if block.Text == "" {
					continue
				}
				event := envelope
				event.Type = agents.EventMessage
				event.Message = &agents.MessagePayload{Role: "assistant", Text: block.Text, Final: true}
				events = append(events, event)
			case "tool":
				events = append(events, toolHistoryEvents(envelope, block)...)
			}
		}
		return events

	default:
		// "idle", "model-switched" and anything a future version adds: not
		// conversation.
		return nil
	}
}

// toolHistoryEvents turns one tool block into the call/result pair the live
// stream emits for the same execution.
func toolHistoryEvents(envelope agents.NormalizedEvent, block ContentBlock) []agents.NormalizedEvent {
	call := envelope
	call.Type = agents.EventToolCall
	call.ToolCall = &agents.ToolCallPayload{CallID: block.ID, Name: block.Name}

	result := envelope
	result.Type = agents.EventToolResult
	result.ToolResult = &agents.ToolResultPayload{CallID: block.ID, IsError: true}

	if block.State != nil {
		call.ToolCall.Input = block.State.Input
		result.ToolResult.Output = block.State.Output
		result.ToolResult.IsError = block.State.Status != "completed"
	}
	return []agents.NormalizedEvent{call, result}
}

// boundHistory keeps the most recent agents.MaxHistoryEvents and renumbers them
// from one, so an unbounded log is never materialized whole. It reports whether
// older events were dropped.
func boundHistory(events []agents.NormalizedEvent) agents.HistoryResult {
	truncated := len(events) > agents.MaxHistoryEvents
	if truncated {
		events = events[len(events)-agents.MaxHistoryEvents:]
	}
	for i := range events {
		events[i].Sequence = uint64(i + 1)
	}
	return agents.HistoryResult{Events: events, Truncated: truncated}
}
