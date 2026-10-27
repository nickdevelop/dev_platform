package opencode

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"devplatform/internal/core/agents"
)

// Compile-time proof that the opencode adapter can serve history reads.
var _ agents.HistoryReader = (*Adapter)(nil)

// ListHistory reads a session's native message log and translates it into the
// platform's normalized events.
//
// The translation mirrors the live stream: reasoning, replies and tool activity
// all become their own event kinds in both, so a replayed conversation looks
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

	inst, err := a.historyInstance(request.Directory)
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
// An instance serving the same working directory is preferred, because that is
// the server whose project the session belongs to. When none does — the session
// outlived its instance, which is exactly the case an instance-free read exists
// for — any running instance is used: the opencode server is machine-wide, the
// native message log lives on disk and the directory is a per-request scope, so
// any server reads any session's history. The smallest instance id is chosen
// only to make the selection reproducible.
func (a *Adapter) historyInstance(directory string) (*instance, error) {
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
	var fallback *instance
	for _, inst := range candidates {
		if inst.isClosed() {
			continue
		}
		if sameDirectory(inst.directory, directory) {
			return inst, nil
		}
		if fallback == nil {
			fallback = inst
		}
	}
	if fallback != nil {
		return fallback, nil
	}
	return nil, errors.New("opencode: no running instance; start the backend before reading session history")
}

// normalizeHistoryMessage translates one native message. A message can yield
// several events (an assistant entry that reasons, speaks and calls a tool), one,
// or none (turn markers, model switches). The reasoning block is kept as its own
// event kind so a replayed conversation matches the live one.
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
		var images []agents.ImageAttachment
		for i, f := range message.Files {
			mime := f.Mime
			data := f.Data
			if data == "" && strings.HasPrefix(f.URI, "data:") {
				comma := strings.Index(f.URI, ",")
				if comma != -1 {
					meta := f.URI[5:comma]
					data = f.URI[comma+1:]
					parts := strings.Split(meta, ";")
					if len(parts) > 0 && strings.HasPrefix(parts[0], "image/") {
						mime = parts[0]
					}
				}
			}
			if mime == "" && strings.HasPrefix(f.Mime, "image/") {
				mime = f.Mime
			}
			if strings.HasPrefix(mime, "image/") && data != "" {
				name := f.Name
				if name == "" {
					name = fmt.Sprintf("image_%d.png", i+1)
				}
				images = append(images, agents.ImageAttachment{
					ID:        fmt.Sprintf("img_%d", i+1),
					Name:      name,
					MediaType: mime,
					Data:      data,
				})
			}
		}
		if message.Text == "" && len(images) == 0 {
			return nil
		}
		event := envelope
		event.Type = agents.EventMessage
		event.Message = &agents.MessagePayload{Role: "user", Text: message.Text, Final: true, Images: images}
		return []agents.NormalizedEvent{event}

	case "assistant":
		events := make([]agents.NormalizedEvent, 0, len(message.Content))
		usage := message.Tokens.Usage()
		for _, block := range message.Content {
			switch block.Type {
			case "text":
				if block.Text == "" {
					continue
				}
				event := envelope
				event.Type = agents.EventMessage
				payload := &agents.MessagePayload{Role: "assistant", Text: block.Text, Final: true, Usage: usage}
				if message.Model != nil {
					// Each assistant message records the model that produced
					// it, so a replay is accurate per message even after a
					// mid-session switch.
					payload.Model = messageModel(*message.Model)
				}
				event.Message = payload
				// The turn's usage belongs to the message once: a multi-block
				// reply must not anchor it several times.
				usage = nil
				events = append(events, event)
			case "tool":
				events = append(events, toolHistoryEvents(envelope, block)...)
			case "reasoning":
				if block.Text == "" {
					continue
				}
				event := envelope
				event.Type = agents.EventThinking
				event.Thinking = &agents.ThinkingPayload{Text: block.Text, Final: true}
				events = append(events, event)
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
// stream emits for the same execution. The result text comes from the state's
// content blocks (the same shape the live session.tool.* events carry); a
// failed block without content falls back to its structured error so the
// replay never renders an empty result.
func toolHistoryEvents(envelope agents.NormalizedEvent, block ContentBlock) []agents.NormalizedEvent {
	call := envelope
	call.Type = agents.EventToolCall
	call.ToolCall = &agents.ToolCallPayload{CallID: block.ID, Name: block.Name}

	result := envelope
	result.Type = agents.EventToolResult
	result.ToolResult = &agents.ToolResultPayload{CallID: block.ID, IsError: true}

	if block.State != nil {
		call.ToolCall.Input = agents.PruneToolInput(block.Name, block.State.Input)
		isError := block.State.Status != "completed"
		rawOutput := flattenContent(block.State.Content)
		if isError && rawOutput == "" {
			rawOutput = historyErrorMessage(block.State)
		}
		summary := agents.SummarizeToolResult(block.Name, block.State.Input, rawOutput, isError)
		result.ToolResult.Output = summary
		result.ToolResult.IsError = isError
	}
	return []agents.NormalizedEvent{call, result}
}

// historyErrorMessage extracts a failed tool block's message from its
// structured error, mirroring the live path's structuredErrorMessage.
func historyErrorMessage(state *ToolState) string {
	if state == nil {
		return "opencode reported a failure"
	}
	if state.Error != nil {
		if state.Error.Message != "" {
			return state.Error.Message
		}
		if state.Error.Type != "" {
			return state.Error.Type
		}
	}
	return "opencode reported a failure"
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
