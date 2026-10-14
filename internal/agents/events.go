package agents

import (
	"encoding/json"
	"fmt"
	"time"
)

// MaxRawEventBytes bounds every backend JSON blob carried inside a
// normalized event: the Raw metadata body and a tool call's Input. It keeps a
// chatty or broken backend from inflating platform memory while still
// preserving the original bytes of unknown events.
const MaxRawEventBytes = 64 * 1024

// MaxHistoryEvents bounds how many prior events one history read returns.
// A transcript is unbounded in principle — a long session's pi JSONL file is
// routinely megabytes — so a reader keeps the most recent events and reports
// the truncation rather than materializing the whole file.
const MaxHistoryEvents = 5000

// EventType discriminates the NormalizedEvent union: it names which payload
// field carries the event body.
type EventType string

const (
	// EventLifecycle reports instance or session state changes. Lifecycle
	// events may be instance-scoped and carry no session id.
	EventLifecycle EventType = "lifecycle"
	// EventMessage carries a user, assistant or system message, or one
	// streaming chunk of it.
	EventMessage EventType = "message"
	// EventToolCall carries a tool invocation requested by the agent.
	EventToolCall EventType = "tool_call"
	// EventToolResult carries the outcome of a previously announced tool
	// call.
	EventToolResult EventType = "tool_result"
	// EventThinking carries the agent's reasoning, so the workbench can show it
	// as its own kind of entry instead of folding it into the reply. Like a
	// message it streams: chunks arrive with Final false and the last one
	// closes it. Backends that do not expose reasoning simply never emit it.
	EventThinking EventType = "thinking"
	// EventError carries a backend or adapter failure that is not attached
	// to a specific tool call.
	EventError EventType = "error"
	// EventUnknown carries a backend event the adapter could not normalize.
	// Its original bytes travel in Raw so nothing the backend reports is
	// silently dropped.
	EventUnknown EventType = "unknown"
)

// Valid reports whether the event type is one of the defined kinds.
func (t EventType) Valid() bool {
	switch t {
	case EventLifecycle, EventMessage, EventToolCall, EventToolResult, EventThinking, EventError, EventUnknown:
		return true
	default:
		return false
	}
}

// LifecycleEvent describes an instance or session state change. State uses
// the InstanceStatus values for instance-level events and backend-defined
// names for session-level ones; Detail carries human-readable context.
type LifecycleEvent struct {
	State  string `json:"state"`
	Detail string `json:"detail,omitempty"`
}

// MessagePayload carries one chat message or, on streaming backends, one
// incremental chunk of it: chunks arrive with Final false and the last chunk
// of a message closes it with Final true. Non-streaming backends emit the
// whole message once with Final true.
type MessagePayload struct {
	// Role is the message author: "user", "assistant" or "system".
	Role string `json:"role"`
	// Text is the message body, or the chunk's text delta.
	Text string `json:"text"`
	// Final reports whether this payload completes the message.
	Final bool `json:"final"`
	// Model identifies the model that produced an assistant message, when the
	// backend reports it. It is absent on user messages and on backends that
	// do not record a per-message model.
	Model *MessageModel `json:"model,omitempty"`
}

// MessageModel identifies the model that produced one assistant message. ID is
// the provider-scoped "provider/model" form the platform uses for model
// selection (ModelInfo.ID); Variant is set when the model ran a named variant.
type MessageModel struct {
	ID       string `json:"id"`
	Provider string `json:"provider"`
	Variant  string `json:"variant,omitempty"`
}

// ThinkingPayload carries the agent's reasoning, or one streaming chunk of it:
// chunks arrive with Final false and the last one closes the block with Final
// true. It mirrors MessagePayload's streaming contract so the two render
// identically.
type ThinkingPayload struct {
	// Text is the reasoning body, or the chunk's text delta.
	Text string `json:"text"`
	// Final reports whether this payload completes the reasoning block.
	Final bool `json:"final"`
}

// ToolCallPayload announces a tool invocation the agent requested. Input is
// the backend's argument JSON, bounded by MaxRawEventBytes.
type ToolCallPayload struct {
	CallID string          `json:"callId"`
	Name   string          `json:"name"`
	Input  json.RawMessage `json:"input,omitempty"`
}

// ToolResultPayload reports the outcome of the tool call named by CallID.
// Metadata optionally carries typed edit summaries (FileDiff list) alongside
// the human-readable Output; it never exceeds MaxRawEventBytes.
type ToolResultPayload struct {
	CallID   string          `json:"callId"`
	Output   string          `json:"output,omitempty"`
	IsError  bool            `json:"isError"`
	Metadata json.RawMessage `json:"metadata,omitempty"`
}

// ErrorPayload reports a backend or adapter failure. Code is a stable,
// backend-specific classification; Message is the human-readable text.
type ErrorPayload struct {
	Code    string `json:"code,omitempty"`
	Message string `json:"message"`
}

// NormalizedEvent is the discriminated union every backend adapter emits.
// The envelope (instance id, session id, backend, sequence, timestamp and
// type) is uniform across backends; exactly one typed payload matches Type,
// and Raw carries optional backend-specific metadata — or the untouched
// original bytes when the adapter could not normalize the event at all.
type NormalizedEvent struct {
	// InstanceID is the instance the event belongs to.
	InstanceID string `json:"instanceId"`
	// SessionID is the agent-native session the event belongs to. It is
	// empty for instance-scoped lifecycle events and for unknown events,
	// whose scope could not be determined.
	SessionID string `json:"sessionId,omitempty"`
	// Backend names the adapter that emitted the event.
	Backend BackendID `json:"backend"`
	// Sequence increases by one per event per instance and orders the
	// stream.
	Sequence uint64 `json:"sequence"`
	// Replayed is true for an event read from an agent-native transcript
	// rather than observed on the live stream. It keeps a replayed event
	// distinguishable from a realtime one without changing the envelope for
	// realtime events, which omit the field entirely.
	Replayed bool `json:"replayed,omitempty"`
	// Timestamp is when the event was observed by the adapter.
	Timestamp time.Time `json:"timestamp"`
	// Type discriminates the union and names the non-nil payload field.
	Type EventType `json:"type"`

	Lifecycle  *LifecycleEvent    `json:"lifecycle,omitempty"`
	Message    *MessagePayload    `json:"message,omitempty"`
	Thinking   *ThinkingPayload   `json:"thinking,omitempty"`
	ToolCall   *ToolCallPayload   `json:"toolCall,omitempty"`
	ToolResult *ToolResultPayload `json:"toolResult,omitempty"`
	Error      *ErrorPayload      `json:"error,omitempty"`

	// Raw preserves bounded backend JSON: extra metadata alongside a
	// normalized event, or the untouched original event body for
	// EventUnknown. It never replaces a typed payload and never exceeds
	// MaxRawEventBytes.
	Raw json.RawMessage `json:"raw,omitempty"`
}

// Validate reports whether the event satisfies the contract: a complete
// envelope, a known type with exactly the matching payload, and every
// embedded backend JSON blob within the size bound.
func (e NormalizedEvent) Validate() error {
	if e.InstanceID == "" {
		return fmt.Errorf("%w: instance id is empty", ErrInvalidEvent)
	}
	if !e.Backend.Valid() {
		return fmt.Errorf("%w: unknown backend %q", ErrUnknownBackend, e.Backend)
	}
	if e.Timestamp.IsZero() {
		return fmt.Errorf("%w: timestamp is zero", ErrInvalidEvent)
	}
	if !e.Type.Valid() {
		return fmt.Errorf("%w: unknown event type %q", ErrInvalidEvent, e.Type)
	}
	if e.Type != EventLifecycle && e.Type != EventUnknown && e.SessionID == "" {
		return fmt.Errorf("%w: %s event is missing its session id", ErrInvalidEvent, e.Type)
	}
	if len(e.Raw) > MaxRawEventBytes {
		return fmt.Errorf("%w: raw body is %d bytes, limit is %d", ErrRawEventTooLarge, len(e.Raw), MaxRawEventBytes)
	}

	fields := []struct {
		name string
		set  bool
	}{
		{"lifecycle", e.Lifecycle != nil},
		{"message", e.Message != nil},
		{"thinking", e.Thinking != nil},
		{"toolCall", e.ToolCall != nil},
		{"toolResult", e.ToolResult != nil},
		{"error", e.Error != nil},
	}
	got := ""
	setCount := 0
	for _, f := range fields {
		if f.set {
			setCount++
			got = f.name
		}
	}

	// Type is known at this point, so an absent map entry means
	// EventUnknown.
	want, typed := map[EventType]string{
		EventLifecycle:  "lifecycle",
		EventMessage:    "message",
		EventThinking:   "thinking",
		EventToolCall:   "toolCall",
		EventToolResult: "toolResult",
		EventError:      "error",
	}[e.Type]

	if typed {
		switch {
		case setCount == 0:
			return fmt.Errorf("%w: %s event is missing its %s payload", ErrInvalidEvent, e.Type, want)
		case setCount > 1:
			return fmt.Errorf("%w: %s event carries %d payloads, want exactly one", ErrInvalidEvent, e.Type, setCount)
		case got != want:
			return fmt.Errorf("%w: %s event carries a %s payload", ErrInvalidEvent, e.Type, got)
		}
		if e.Type == EventToolCall && len(e.ToolCall.Input) > MaxRawEventBytes {
			return fmt.Errorf("%w: tool call input is %d bytes, limit is %d", ErrRawEventTooLarge, len(e.ToolCall.Input), MaxRawEventBytes)
		}
		if e.Type == EventToolResult && len(e.ToolResult.Metadata) > MaxRawEventBytes {
			return fmt.Errorf("%w: tool result metadata is %d bytes, limit is %d", ErrRawEventTooLarge, len(e.ToolResult.Metadata), MaxRawEventBytes)
		}
		return nil
	}

	if setCount > 0 {
		return fmt.Errorf("%w: unknown event carries a %s payload", ErrInvalidEvent, got)
	}
	if len(e.Raw) == 0 {
		return fmt.Errorf("%w: unknown event is missing its raw body", ErrInvalidEvent)
	}
	return nil
}
