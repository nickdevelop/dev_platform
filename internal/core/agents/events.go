package agents

import (
	"encoding/json"
	"fmt"
	"strings"
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
	// EventTurn carries a turn boundary of one session: a turn starts when
	// the agent begins working on a user message and ends with an outcome.
	// Consumers derive whether a session is busy from these events only;
	// lifecycle names are informational and never change the turn state.
	EventTurn EventType = "turn"
	// EventInteraction carries a tool confirmation or question form the
	// agent is waiting on, and its resolution. It replaces any side channel:
	// the workbench learns about pending interactions from the session stream.
	EventInteraction EventType = "interaction"
	// EventUnknown carries a backend event the adapter could not normalize.
	// Its original bytes travel in Raw so nothing the backend reports is
	// silently dropped.
	EventUnknown EventType = "unknown"
)

// Valid reports whether the event type is one of the defined kinds.
func (t EventType) Valid() bool {
	switch t {
	case EventLifecycle, EventMessage, EventToolCall, EventToolResult, EventThinking, EventError, EventTurn, EventInteraction, EventUnknown:
		return true
	default:
		return false
	}
}

// LifecycleEvent describes an instance or session state change. State uses
// the InstanceStatus values for instance-level events and backend-defined
// names for informational session-level ones (for example "created" or
// "text_started"); turn boundaries are EventTurn, never lifecycle. Detail
// carries human-readable context. Usage, when set, reports the turn's token
// usage (opencode's step finish); the workbench uses it as the freshest
// context anchor. ContextLimit, when set, reports the active model's dynamic
// context window in tokens (e.g. from session projections or context events);
// the workbench uses it as a dynamic context capacity limit.
type LifecycleEvent struct {
	State        string        `json:"state"`
	Detail       string        `json:"detail,omitempty"`
	Usage        *MessageUsage `json:"usage,omitempty"`
	ContextLimit *int          `json:"contextLimit,omitempty"`
}

// TurnPhase names the two boundaries of a turn.
type TurnPhase string

const (
	// TurnStarted opens a turn.
	TurnStarted TurnPhase = "started"
	// TurnEnded closes a turn and carries its Outcome.
	TurnEnded TurnPhase = "ended"
)

// TurnOutcome says how a turn ended. A turn cancelled by the user (interrupt
// or stop) is cancelled, never failed; a failed turn is followed by an
// EventError that explains it.
type TurnOutcome string

const (
	// TurnCompleted means the agent finished the turn normally.
	TurnCompleted TurnOutcome = "completed"
	// TurnCancelled means the turn was interrupted before it finished.
	TurnCancelled TurnOutcome = "cancelled"
	// TurnFailed means the turn ended because of an error.
	TurnFailed TurnOutcome = "failed"
)

// TurnPayload is one turn boundary. ID identifies the turn within its
// instance: the started and ended events of one turn share it, so consumers
// can pair them even when turns of different sessions interleave.
type TurnPayload struct {
	ID      string      `json:"id"`
	Phase   TurnPhase   `json:"phase"`
	Outcome TurnOutcome `json:"outcome,omitempty"`
}

// Validate reports whether the turn payload is well formed: a non-empty id, a
// known phase, and an outcome exactly when the turn ended.
func (p TurnPayload) Validate() error {
	if p.ID == "" {
		return fmt.Errorf("%w: turn id is empty", ErrInvalidEvent)
	}
	switch p.Phase {
	case TurnStarted:
		if p.Outcome != "" {
			return fmt.Errorf("%w: started turn carries outcome %q", ErrInvalidEvent, p.Outcome)
		}
	case TurnEnded:
		switch p.Outcome {
		case TurnCompleted, TurnCancelled, TurnFailed:
		default:
			return fmt.Errorf("%w: ended turn has unknown outcome %q", ErrInvalidEvent, p.Outcome)
		}
	default:
		return fmt.Errorf("%w: unknown turn phase %q", ErrInvalidEvent, p.Phase)
	}
	return nil
}

// InteractionKind names what the agent is waiting on.
type InteractionKind string

const (
	// InteractionPermission is a tool confirmation: the agent waits for an
	// approve or reject decision.
	InteractionPermission InteractionKind = "permission"
	// InteractionForm is a question form: the agent waits for an answer.
	InteractionForm InteractionKind = "form"
)

// InteractionPhase says whether an interaction is pending or has been settled.
type InteractionPhase string

const (
	// InteractionRequested means the agent is waiting on the interaction.
	InteractionRequested InteractionPhase = "requested"
	// InteractionResolved means the interaction was answered, cancelled or
	// expired; the consumer removes it from its pending set.
	InteractionResolved InteractionPhase = "resolved"
)

// InteractionPayload is one interaction boundary. ID is the backend's request
// or form id, which the reply endpoints take back. A requested interaction
// carries the matching PermissionRequest or FormInfo; a resolved one carries
// only the id.
type InteractionPayload struct {
	ID         string             `json:"id"`
	Kind       InteractionKind    `json:"kind"`
	Phase      InteractionPhase   `json:"phase"`
	Permission *PermissionRequest `json:"permission,omitempty"`
	Form       *FormInfo          `json:"form,omitempty"`
}

// Validate reports whether the interaction payload is well formed: a known kind
// and phase, and for a requested interaction exactly the payload its kind
// names, carrying the same id.
func (p InteractionPayload) Validate() error {
	if p.ID == "" {
		return fmt.Errorf("%w: interaction id is empty", ErrInvalidEvent)
	}
	switch p.Kind {
	case InteractionPermission, InteractionForm:
	default:
		return fmt.Errorf("%w: unknown interaction kind %q", ErrInvalidEvent, p.Kind)
	}
	switch p.Phase {
	case InteractionResolved:
		if p.Permission != nil || p.Form != nil {
			return fmt.Errorf("%w: resolved interaction carries a payload", ErrInvalidEvent)
		}
		return nil
	case InteractionRequested:
	default:
		return fmt.Errorf("%w: unknown interaction phase %q", ErrInvalidEvent, p.Phase)
	}
	if p.Kind == InteractionPermission {
		if p.Permission == nil || p.Form != nil {
			return fmt.Errorf("%w: permission interaction must carry exactly a permission payload", ErrInvalidEvent)
		}
		if p.Permission.ID != p.ID {
			return fmt.Errorf("%w: permission payload id %q does not match interaction id %q", ErrInvalidEvent, p.Permission.ID, p.ID)
		}
		return nil
	}
	if p.Form == nil || p.Permission != nil {
		return fmt.Errorf("%w: form interaction must carry exactly a form payload", ErrInvalidEvent)
	}
	if p.Form.ID != p.ID {
		return fmt.Errorf("%w: form payload id %q does not match interaction id %q", ErrInvalidEvent, p.Form.ID, p.ID)
	}
	return nil
}

// MessageUsage is one message's (or turn's) token usage as the backend
// reports it. All fields are prompt-visible tokens; Total is their sum.
// It is always exact backend data, never an estimate: estimates are computed
// in the workbench and never travel in this struct.
type MessageUsage struct {
	Input      int `json:"input"`
	Output     int `json:"output"`
	Reasoning  int `json:"reasoning,omitempty"`
	CacheRead  int `json:"cacheRead,omitempty"`
	CacheWrite int `json:"cacheWrite,omitempty"`
}

// Total reports the prompt-visible token sum.
func (u *MessageUsage) Total() int {
	if u == nil {
		return 0
	}
	return u.Input + u.Output + u.Reasoning + u.CacheRead + u.CacheWrite
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
	// Images carries optional image attachments, e.g. for user turns.
	Images []ImageAttachment `json:"images,omitempty"`
	// Final reports whether this payload completes the message.
	Final bool `json:"final"`
	// Model identifies the model that produced an assistant message, when the
	// backend reports it. It is absent on user messages and on backends that
	// do not record a per-message model.
	Model *MessageModel `json:"model,omitempty"`
	// Usage carries the message's token usage, when the backend reports it.
	// It is set on final assistant messages only; streaming chunks never
	// carry it.
	Usage *MessageUsage `json:"usage,omitempty"`
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

	Lifecycle   *LifecycleEvent     `json:"lifecycle,omitempty"`
	Message     *MessagePayload     `json:"message,omitempty"`
	Thinking    *ThinkingPayload    `json:"thinking,omitempty"`
	ToolCall    *ToolCallPayload    `json:"toolCall,omitempty"`
	ToolResult  *ToolResultPayload  `json:"toolResult,omitempty"`
	Error       *ErrorPayload       `json:"error,omitempty"`
	Turn        *TurnPayload        `json:"turn,omitempty"`
	Interaction *InteractionPayload `json:"interaction,omitempty"`

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
		{"turn", e.Turn != nil},
		{"interaction", e.Interaction != nil},
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
		EventLifecycle:   "lifecycle",
		EventMessage:     "message",
		EventThinking:    "thinking",
		EventToolCall:    "toolCall",
		EventToolResult:  "toolResult",
		EventError:       "error",
		EventTurn:        "turn",
		EventInteraction: "interaction",
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
		if e.Type == EventTurn {
			return e.Turn.Validate()
		}
		if e.Type == EventInteraction {
			return e.Interaction.Validate()
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

// IsQuestionTool reports whether toolName identifies a question/interaction tool
// whose invocations and answers belong in the primary chat message flow.
func IsQuestionTool(name string) bool {
	lower := strings.ToLower(strings.TrimSpace(name))
	if _, ok := questionToolNames[lower]; ok {
		return true
	}
	return strings.Contains(lower, "ask_user") || strings.Contains(lower, "question")
}

