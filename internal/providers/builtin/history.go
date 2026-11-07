package builtin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"devplatform/internal/core/agents"
)

// Compile-time proof the builtin adapter serves history reads.
var _ agents.HistoryReader = (*Adapter)(nil)

// loadTranscriptRecords reads the stored transcript of one session. A turn
// snapshots it before recording the fresh user message, so the rebuilt context
// carries prior turns only.
func (a *Adapter) loadTranscriptRecords(ctx context.Context, sessionID string) ([]TranscriptRecord, error) {
	records, err := a.store.Load(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("builtin: load transcript: %w", err)
	}
	return records, nil
}

// renderHistoryMessages renders stored records as the chat messages the turn
// sends back to the model, so a session keeps its memory across platform
// restarts. The mapping is the OpenAI loop format: user -> user, assistant ->
// assistant, tool_call -> assistant tool_calls, tool_result -> tool, thinking
// -> dropped, error -> inline note.
func renderHistoryMessages(records []TranscriptRecord, endpoint ModelEndpoint) []chatMessage {
	messages := make([]chatMessage, 0, len(records))
	for _, record := range records {
		switch record.Kind {
		case TranscriptUser:
			var payload userRecord
			_ = json.Unmarshal(record.Payload, &payload)
			message := textMessage("user", record.Content)
			if len(payload.Images) > 0 && endpoint.SupportsImage {
				parts := make([]contentPart, 0, len(payload.Images)+1)
				if record.Content != "" {
					parts = append(parts, contentPart{Type: "text", Text: record.Content})
				}
				for _, image := range payload.Images {
					if image.Data == "" {
						// An oversized attachment was stored by identity
						// only; the rebuilt message keeps its text.
						continue
					}
					parts = append(parts, contentPart{Type: "image_url", ImageURL: &imageURLRef{URL: "data:" + image.MediaType + ";base64," + image.Data}})
				}
				encoded, _ := json.Marshal(parts)
				message = chatMessage{Role: "user", Content: encoded}
			}
			messages = append(messages, message)
		case TranscriptThinking:
			// Reasoning is not replayed into the model context: only the
			// visible conversation and tool activity are.
		case TranscriptAssistant:
			messages = append(messages, textMessage("assistant", record.Content))
		case TranscriptToolCall:
			var payload toolCallRecord
			if err := json.Unmarshal(record.Payload, &payload); err != nil || payload.Name == "" {
				continue
			}
			messages = append(messages, chatMessage{
				Role: "assistant",
				ToolCalls: []wireToolCall{{
					ID:       payload.CallID,
					Type:     "function",
					Function: wireFunctionCall{Name: payload.Name, Arguments: string(payload.Input)},
				}},
			})
		case TranscriptToolResult:
			var payload toolResultRecord
			if err := json.Unmarshal(record.Payload, &payload); err != nil {
				payload.Output = compactJSONString(record.Payload)
			}
			messages = append(messages, chatMessage{
				Role:       "tool",
				Content:    jsonOrString(payload.Output),
				ToolCallID: payload.CallID,
			})
		case TranscriptError:
			// A failed turn is told back so the next round knows why the
			// previous one stopped.
			note := "系统提示：上一回合执行失败"
			if record.Content != "" {
				note += "：" + strings.TrimSpace(firstLine(record.Content))
			}
			messages = append(messages, textMessage("user", note))
		}
	}
	return messages
}

// compactJSONString renders a payload blob as the model-facing fallback text.
func compactJSONString(raw []byte) string {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return strings.TrimSpace(string(raw))
	}
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

// ListHistory replays a stored transcript as normalized events, oldest
// first, so the workbench rebuilds a conversation whose original runtime is
// long gone. Only the events the main conversation view consumes are stored,
// which is the same set a live stream produced.
func (a *Adapter) ListHistory(ctx context.Context, request agents.ListHistoryRequest) (agents.HistoryResult, error) {
	if err := ctx.Err(); err != nil {
		return agents.HistoryResult{}, err
	}
	if err := request.Validate(); err != nil {
		return agents.HistoryResult{}, err
	}
	records, err := a.store.Load(ctx, request.SessionID)
	if err != nil {
		return agents.HistoryResult{}, err
	}
	// The request's directory is authorized by the platform; a mismatch
	// between a transcript and the requested directory is treated as not
	// found rather than a leak of another project's transcript.
	exists, err := a.store.Exists(ctx, request.SessionID)
	if err != nil {
		return agents.HistoryResult{}, err
	}
	if !exists {
		return agents.HistoryResult{Events: []agents.NormalizedEvent{}}, nil
	}
	directory, err := a.sessionDirectory(request.SessionID)
	if err == nil && directory != "" && directory != request.Directory {
		return agents.HistoryResult{}, errors.New("builtin: transcript belongs to another working directory")
	}

	events, truncated := transcriptToEvents(records)
	return agents.HistoryResult{Events: events, Truncated: truncated}, nil
}

// transcriptToEvents converts stored records into replayed events,
// renumbered from one so sequence stays dense. A very long transcript is
// bounded like every other backend's history read.
func transcriptToEvents(records []TranscriptRecord) ([]agents.NormalizedEvent, bool) {
	truncated := false
	if len(records) > agents.MaxHistoryEvents {
		records = records[len(records)-agents.MaxHistoryEvents:]
		truncated = true
	}
	events := make([]agents.NormalizedEvent, 0, len(records))
	for index, record := range records {
		timestamp := record.CreatedAt
		event := agents.NormalizedEvent{
			Backend:   agents.BackendBuiltin,
			Replayed:  true,
			Timestamp: timestamp,
		}
		switch record.Kind {
		case TranscriptUser:
			event.Type = agents.EventMessage
			event.Message = &agents.MessagePayload{Role: "user", Text: record.Content, Final: true}
		case TranscriptAssistant:
			event.Type = agents.EventMessage
			event.Message = &agents.MessagePayload{Role: "assistant", Text: record.Content, Final: true}
		case TranscriptThinking:
			event.Type = agents.EventThinking
			event.Thinking = &agents.ThinkingPayload{Text: record.Content, Final: true}
		case TranscriptToolCall:
			var payload toolCallRecord
			if err := json.Unmarshal(record.Payload, &payload); err != nil {
				continue
			}
			event.Type = agents.EventToolCall
			event.ToolCall = &agents.ToolCallPayload{
				CallID: payload.CallID,
				Name:   payload.Name,
				Input:  agents.PruneToolInput(payload.Name, payload.Input),
			}
		case TranscriptToolResult:
			var payload toolResultRecord
			if err := json.Unmarshal(record.Payload, &payload); err != nil {
				continue
			}
			event.Type = agents.EventToolResult
			event.ToolResult = &agents.ToolResultPayload{
				CallID:  payload.CallID,
				Output:  payload.Output,
				IsError: payload.IsError,
			}
		case TranscriptError:
			event.Type = agents.EventError
			event.Error = &agents.ErrorPayload{Code: "turn_failed", Message: record.Content}
		default:
			continue
		}
		event.Sequence = uint64(index + 1)
		events = append(events, event)
	}
	return events, truncated
}
