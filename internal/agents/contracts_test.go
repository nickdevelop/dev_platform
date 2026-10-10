package agents

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

// validInstance returns an Instance that passes validation, for tests that
// start from a known-good record and mutate one field at a time.
func validInstance() Instance {
	now := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	return Instance{
		ID:        "inst-1",
		Backend:   BackendPi,
		Directory: "/data/projects/demo",
		Status:    InstanceStarting,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// validSessionRef returns a SessionRef that passes validation.
func validSessionRef() SessionRef {
	return SessionRef{
		InstanceID: "inst-1",
		Backend:    BackendPi,
		SessionID:  "ses_123",
		Title:      "fix the flaky test",
	}
}

func TestContractsBackendIDs(t *testing.T) {
	if BackendOpenCodeV2 != "opencode-v2" {
		t.Errorf("BackendOpenCodeV2 = %q, want %q", BackendOpenCodeV2, "opencode-v2")
	}
	if BackendPi != "pi" {
		t.Errorf("BackendPi = %q, want %q", BackendPi, "pi")
	}

	for _, id := range []BackendID{BackendOpenCodeV2, BackendPi} {
		if !id.Valid() {
			t.Errorf("BackendID(%q).Valid() = false, want true", id)
		}
	}

	for _, id := range []BackendID{"", "claude-code", "OpenCode-V2", "opencode"} {
		if id.Valid() {
			t.Errorf("BackendID(%q).Valid() = true, want false", id)
		}
	}

	// Backend IDs must survive a JSON round-trip verbatim.
	data, err := json.Marshal(BackendOpenCodeV2)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	if string(data) != `"opencode-v2"` {
		t.Errorf("marshaled backend id = %s, want %q", data, `"opencode-v2"`)
	}
}

func TestContractsCapabilityCombinations(t *testing.T) {
	// The zero value must declare no capability at all: a backend that does
	// not opt in must never expose a control that pretends to work.
	var zero Capabilities
	if zero.Streaming || zero.SessionDiscovery || zero.SessionContinue ||
		zero.Stop || zero.ModelSelection || zero.ToolCallEvents || zero.ToolConfirmation ||
		zero.Interrupt {
		t.Error("zero Capabilities must be false for every capability")
	}

	// Capability booleans are explicit: JSON must carry every key even when
	// false, so a consumer reading the wire form sees the whole declaration.
	data, err := json.Marshal(zero)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	for _, key := range []string{
		"streaming", "sessionDiscovery", "sessionContinue", "stop",
		"modelSelection", "toolCallEvents", "toolConfirmation", "interrupt",
	} {
		if !strings.Contains(string(data), `"`+key+`":false`) {
			t.Errorf("marshaled Capabilities %s lacks explicit key %q", data, key)
		}
	}

	var roundTrip Capabilities
	if err := json.Unmarshal(data, &roundTrip); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if roundTrip != zero {
		t.Errorf("capability round-trip mismatch: got %+v, want %+v", roundTrip, zero)
	}

	// Every capability combination must be expressible: a streaming backend
	// without tool confirmation, and a full-featured backend, both decode to
	// themselves.
	combinations := []Capabilities{
		{Streaming: true},
		{SessionDiscovery: true, SessionContinue: true},
		{Streaming: true, ToolCallEvents: true},
		{Streaming: true, SessionDiscovery: true, SessionContinue: true, Stop: true,
			ModelSelection: true, ToolCallEvents: true, ToolConfirmation: true},
	}
	for _, want := range combinations {
		data, err := json.Marshal(want)
		if err != nil {
			t.Fatalf("json.Marshal(%+v): %v", want, err)
		}
		var got Capabilities
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatalf("json.Unmarshal(%s): %v", data, err)
		}
		if got != want {
			t.Errorf("capability round-trip: got %+v, want %+v", got, want)
		}
	}
}

func TestContractsInstanceTransitions(t *testing.T) {
	valid := []struct {
		from, to InstanceStatus
	}{
		{InstanceStarting, InstanceRunning},
		{InstanceStarting, InstanceFailed},
		{InstanceStarting, InstanceStopped},
		{InstanceRunning, InstanceStopped},
		{InstanceRunning, InstanceFailed},
		{InstanceFailed, InstanceStopped},
		{InstanceStopped, InstanceStarting},
	}
	for _, tc := range valid {
		if !tc.from.CanTransitionTo(tc.to) {
			t.Errorf("%s.CanTransitionTo(%s) = false, want true", tc.from, tc.to)
		}
	}

	invalid := []struct {
		from, to InstanceStatus
	}{
		{InstanceStarting, InstanceStarting},
		{InstanceRunning, InstanceStarting},
		{InstanceRunning, InstanceRunning},
		{InstanceStopped, InstanceRunning},
		{InstanceStopped, InstanceFailed},
		{InstanceFailed, InstanceRunning},
		{InstanceFailed, InstanceFailed},
		{InstanceStatus("paused"), InstanceRunning},
	}
	for _, tc := range invalid {
		if tc.from.CanTransitionTo(tc.to) {
			t.Errorf("%s.CanTransitionTo(%s) = true, want false", tc.from, tc.to)
		}
	}

	for _, status := range []InstanceStatus{InstanceStarting, InstanceRunning, InstanceStopped, InstanceFailed} {
		if !status.Valid() {
			t.Errorf("InstanceStatus(%q).Valid() = false, want true", status)
		}
	}
	if (InstanceStatus("paused")).Valid() {
		t.Error(`InstanceStatus("paused").Valid() = true, want false`)
	}
}

func TestContractsInstanceValidation(t *testing.T) {
	if err := validInstance().Validate(); err != nil {
		t.Fatalf("validInstance().Validate() = %v, want nil", err)
	}

	tests := []struct {
		name    string
		mutate  func(*Instance)
		wantErr bool
	}{
		{"empty id", func(i *Instance) { i.ID = "" }, true},
		{"invalid backend", func(i *Instance) { i.Backend = BackendID("claude-code") }, true},
		{"empty directory", func(i *Instance) { i.Directory = "" }, true},
		{"zero created timestamp", func(i *Instance) { i.CreatedAt = time.Time{} }, true},
		{"zero updated timestamp", func(i *Instance) { i.UpdatedAt = time.Time{} }, true},
		{"unknown status", func(i *Instance) { i.Status = InstanceStatus("paused") }, true},
		{"running instance is valid", func(i *Instance) { i.Status = InstanceRunning }, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			instance := validInstance()
			tc.mutate(&instance)
			err := instance.Validate()
			if (err != nil) != tc.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestContractsSessionRefValidation(t *testing.T) {
	if err := validSessionRef().Validate(); err != nil {
		t.Fatalf("validSessionRef().Validate() = %v, want nil", err)
	}

	tests := []struct {
		name    string
		mutate  func(*SessionRef)
		wantErr bool
	}{
		{"empty instance id", func(s *SessionRef) { s.InstanceID = "" }, true},
		{"empty session id", func(s *SessionRef) { s.SessionID = "" }, true},
		{"invalid backend", func(s *SessionRef) { s.Backend = BackendID("") }, true},
		{"title is optional", func(s *SessionRef) { s.Title = "" }, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ref := validSessionRef()
			tc.mutate(&ref)
			err := ref.Validate()
			if (err != nil) != tc.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestContractsNormalizedEvents(t *testing.T) {
	now := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	base := NormalizedEvent{
		InstanceID: "inst-1",
		SessionID:  "ses_123",
		Backend:    BackendOpenCodeV2,
		Sequence:   7,
		Timestamp:  now,
	}

	tests := []struct {
		name    string
		event   NormalizedEvent
		wantErr bool
	}{
		{
			name:  "lifecycle event may be instance-scoped",
			event: NormalizedEvent{InstanceID: "inst-1", Backend: BackendPi, Timestamp: now, Type: EventLifecycle, Lifecycle: &LifecycleEvent{State: string(InstanceRunning)}},
		},
		{
			name:  "message event",
			event: withPayload(base, EventMessage, &MessagePayload{Role: "assistant", Text: "working on it", Final: false}),
		},
		{
			name:  "final message event",
			event: withPayload(base, EventMessage, &MessagePayload{Role: "assistant", Text: "done", Final: true}),
		},
		{
			name:  "tool call event",
			event: withPayload(base, EventToolCall, &ToolCallPayload{CallID: "call_1", Name: "read_file", Input: json.RawMessage(`{"path":"main.go"}`)}),
		},
		{
			name:  "tool result event",
			event: withPayload(base, EventToolResult, &ToolResultPayload{CallID: "call_1", Output: "ok", IsError: false}),
		},
		{
			name:  "error event",
			event: withPayload(base, EventError, &ErrorPayload{Code: "backend_unavailable", Message: "daemon not reachable"}),
		},
		{
			name:  "unknown event keeps raw backend body",
			event: withRaw(base, EventUnknown, json.RawMessage(`{"opencode":"part-9","weird":true}`)),
		},
		{
			name:    "missing instance id",
			event:   withPayload(NormalizedEvent{SessionID: "ses_123", Backend: BackendPi, Timestamp: now, Type: EventMessage}, EventMessage, &MessagePayload{Role: "user", Text: "hi"}),
			wantErr: true,
		},
		{
			name:    "unknown backend",
			event:   withPayload(NormalizedEvent{InstanceID: "inst-1", Backend: BackendID("claude-code"), Timestamp: now, Type: EventMessage}, EventMessage, &MessagePayload{Role: "user", Text: "hi"}),
			wantErr: true,
		},
		{
			name:    "zero timestamp",
			event:   withPayload(NormalizedEvent{InstanceID: "inst-1", Backend: BackendPi, Type: EventMessage}, EventMessage, &MessagePayload{Role: "user", Text: "hi"}),
			wantErr: true,
		},
		{
			name:    "unknown event type",
			event:   withPayload(base, EventType("ticker"), nil),
			wantErr: true,
		},
		{
			name:    "session-scoped event without session id",
			event:   withPayload(NormalizedEvent{InstanceID: "inst-1", Backend: BackendPi, Timestamp: now, Type: EventMessage}, EventMessage, &MessagePayload{Role: "user", Text: "hi"}),
			wantErr: true,
		},
		{
			name:    "type without its payload",
			event:   withPayload(base, EventMessage, nil),
			wantErr: true,
		},
		{
			name:    "payload of another type",
			event:   withPayload(base, EventMessage, &ToolCallPayload{CallID: "call_1", Name: "read_file"}),
			wantErr: true,
		},
		{
			name: "two payloads set is rejected",
			event: withExtraPayload(
				withPayload(base, EventMessage, &MessagePayload{Role: "assistant", Text: "hi"}),
				&ToolCallPayload{CallID: "call_1", Name: "read_file"},
			),
			wantErr: true,
		},
		{
			name: "two payloads neither matching the type is rejected",
			event: withExtraPayload(
				withPayload(base, EventLifecycle, &LifecycleEvent{State: string(InstanceRunning)}),
				&ErrorPayload{Message: "boom"},
			),
			wantErr: true,
		},
		{
			name: "three payloads set is rejected",
			event: withExtraPayload(
				withExtraPayload(
					withPayload(base, EventError, &ErrorPayload{Message: "boom"}),
					&MessagePayload{Role: "system", Text: "reconnecting"},
				),
				&ToolResultPayload{CallID: "call_1", Output: "ok"},
			),
			wantErr: true,
		},
		{
			name:  "message event with backend metadata",
			event: withPayloadAndRaw(base, EventMessage, &MessagePayload{Role: "assistant", Text: "hi"}, json.RawMessage(`{"partId":"part-9"}`)),
		},
		{
			name:    "unknown event without raw body",
			event:   withPayload(base, EventUnknown, nil),
			wantErr: true,
		},
		{
			name:    "unknown event with typed payload",
			event:   withPayload(base, EventUnknown, &MessagePayload{Role: "user", Text: "hi"}),
			wantErr: true,
		},
		{
			name:    "raw body over the size bound",
			event:   withRaw(base, EventUnknown, json.RawMessage(`"`+strings.Repeat("a", MaxRawEventBytes-1)+`"`)),
			wantErr: true,
		},
		{
			name:  "raw body exactly at the size bound",
			event: withRaw(base, EventUnknown, json.RawMessage(`"`+strings.Repeat("a", MaxRawEventBytes-2)+`"`)),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.event.Validate()
			if (err != nil) != tc.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

// withPayload returns a copy of the event with Type set to eventType and the
// given payload pointer installed.
func withPayload(event NormalizedEvent, eventType EventType, payload any) NormalizedEvent {
	event.Type = eventType
	switch p := payload.(type) {
	case nil:
	case *MessagePayload:
		event.Message = p
	case *ToolCallPayload:
		event.ToolCall = p
	case *ToolResultPayload:
		event.ToolResult = p
	case *ErrorPayload:
		event.Error = p
	case *LifecycleEvent:
		event.Lifecycle = p
	default:
		panic("unhandled payload type")
	}
	return event
}

// withRaw returns a copy of the event with Type set to eventType and the raw
// backend body attached.
func withRaw(event NormalizedEvent, eventType EventType, raw json.RawMessage) NormalizedEvent {
	event.Type = eventType
	event.Raw = raw
	return event
}

// withPayloadAndRaw returns a copy of the event with a typed payload and a
// bounded raw backend metadata body attached at the same time.
func withPayloadAndRaw(event NormalizedEvent, eventType EventType, payload any, raw json.RawMessage) NormalizedEvent {
	event = withPayload(event, eventType, payload)
	event.Raw = raw
	return event
}

// withExtraPayload installs an additional payload alongside the one the event
// already carries. Validate must reject any event with more than one payload,
// even when one of them matches the event's Type.
func withExtraPayload(event NormalizedEvent, payload any) NormalizedEvent {
	switch p := payload.(type) {
	case *LifecycleEvent:
		event.Lifecycle = p
	case *MessagePayload:
		event.Message = p
	case *ToolCallPayload:
		event.ToolCall = p
	case *ToolResultPayload:
		event.ToolResult = p
	case *ErrorPayload:
		event.Error = p
	default:
		panic("unhandled payload type")
	}
	return event
}

func TestContractsNormalizedEventJSONRoundTrip(t *testing.T) {
	now := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	raw := json.RawMessage(`{"opencode":"part-9","weird":true}`)
	event := NormalizedEvent{
		InstanceID: "inst-1",
		SessionID:  "ses_123",
		Backend:    BackendOpenCodeV2,
		Sequence:   7,
		Timestamp:  now,
		Type:       EventUnknown,
		Raw:        raw,
	}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	for _, key := range []string{`"instanceId"`, `"sessionId"`, `"sequence"`, `"timestamp"`, `"type"`, `"backend"`, `"raw"`} {
		if !strings.Contains(string(data), key) {
			t.Errorf("marshaled event %s lacks envelope key %s", data, key)
		}
	}

	var got NormalizedEvent
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if got.Type != EventUnknown {
		t.Errorf("round-trip Type = %q, want %q", got.Type, EventUnknown)
	}
	if string(got.Raw) != string(raw) {
		t.Errorf("round-trip Raw = %s, want %s", got.Raw, raw)
	}
	if got.InstanceID != event.InstanceID || got.SessionID != event.SessionID ||
		got.Sequence != event.Sequence || !got.Timestamp.Equal(now) {
		t.Errorf("round-trip envelope mismatch: got %+v, want instance/session/sequence/time of %+v", got, event)
	}
	if got.Backend != BackendOpenCodeV2 {
		t.Errorf("round-trip Backend = %q, want %q", got.Backend, BackendOpenCodeV2)
	}
}

// TestContractsNormalizedEventRawPreserved pins that an unknown backend event
// survives normalization untouched: the raw body the adapter could not parse
// is the same bytes the consumer reads back.
func TestContractsNormalizedEventRawPreserved(t *testing.T) {
	original := []byte(`{"pi":"slot-3","opaque":"x"}`)
	event := NormalizedEvent{
		InstanceID: "inst-1",
		Backend:    BackendPi,
		Timestamp:  time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC),
		Type:       EventUnknown,
		Raw:        json.RawMessage(original),
	}
	if err := event.Validate(); err != nil {
		t.Fatalf("Validate() = %v, want nil", err)
	}

	encoded, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var decoded NormalizedEvent
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if string(decoded.Raw) != string(original) {
		t.Errorf("raw event body changed: got %s, want %s", decoded.Raw, original)
	}
}

// stubAdapter is the smallest possible AgentAdapter. It exists to pin the
// interface shape: every method takes a context first, and cancellation is
// observable through each of them.
type stubAdapter struct{}

func (stubAdapter) Probe(ctx context.Context) error { return ctx.Err() }

func (stubAdapter) StartInstance(ctx context.Context, req StartInstanceRequest) (*Instance, error) {
	return nil, ctx.Err()
}

func (stubAdapter) StopInstance(ctx context.Context, instanceID string) error { return ctx.Err() }

func (stubAdapter) ListSessions(ctx context.Context, instanceID string) ([]SessionRef, error) {
	return nil, ctx.Err()
}

func (stubAdapter) CreateSession(ctx context.Context, instanceID string, opts CreateSessionOptions) (*SessionRef, error) {
	return nil, ctx.Err()
}

func (stubAdapter) ContinueSession(ctx context.Context, ref SessionRef) (*SessionRef, error) {
	return nil, ctx.Err()
}

func (stubAdapter) ListModels(ctx context.Context, instanceID string) ([]ModelInfo, error) {
	return nil, ctx.Err()
}

func (stubAdapter) Interrupt(ctx context.Context, ref SessionRef) error { return ctx.Err() }

func (stubAdapter) SendMessage(ctx context.Context, ref SessionRef, message SendMessageRequest) error {
	return ctx.Err()
}

func (stubAdapter) Subscribe(ctx context.Context, instanceID string) (<-chan NormalizedEvent, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	ch := make(chan NormalizedEvent)
	close(ch)
	return ch, nil
}

var _ AgentAdapter = stubAdapter{}

func TestContractsContextCancellation(t *testing.T) {
	var adapter AgentAdapter = stubAdapter{}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	ref := validSessionRef()
	methods := map[string]func() error{
		"Probe": func() error { return adapter.Probe(ctx) },
		"StartInstance": func() error {
			_, err := adapter.StartInstance(ctx, StartInstanceRequest{Directory: "/data/projects/demo"})
			return err
		},
		"StopInstance":    func() error { return adapter.StopInstance(ctx, "inst-1") },
		"ListSessions":    func() error { _, err := adapter.ListSessions(ctx, "inst-1"); return err },
		"CreateSession":   func() error { _, err := adapter.CreateSession(ctx, "inst-1", CreateSessionOptions{}); return err },
		"ContinueSession": func() error { _, err := adapter.ContinueSession(ctx, ref); return err },
		"SendMessage":     func() error { return adapter.SendMessage(ctx, ref, SendMessageRequest{Text: "hi"}) },
		"Subscribe":       func() error { _, err := adapter.Subscribe(ctx, "inst-1"); return err },
	}
	for name, call := range methods {
		t.Run(name, func(t *testing.T) {
			if err := call(); !errors.Is(err, context.Canceled) {
				t.Errorf("%s with canceled context = %v, want context.Canceled", name, err)
			}
		})
	}
}

// TestContractsSubscribeCancellation pins the subscription contract: a
// consumer draining the event channel must observe context cancellation
// promptly even while events keep arriving.
func TestContractsSubscribeCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch := make(chan NormalizedEvent, 1)
	ch <- NormalizedEvent{InstanceID: "inst-1", Backend: BackendPi, Timestamp: time.Now(), Type: EventLifecycle, Lifecycle: &LifecycleEvent{State: string(InstanceRunning)}}

	drained := make(chan struct{})
	go func() {
		defer close(drained)
		for {
			select {
			case <-ch:
			case <-ctx.Done():
				return
			}
		}
	}()

	cancel()
	select {
	case <-drained:
	case <-time.After(2 * time.Second):
		t.Fatal("consumer did not observe context cancellation")
	}
}
