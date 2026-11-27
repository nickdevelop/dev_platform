package builtin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"devplatform/internal/core/agents"
)

// transcriptMemory is an in-memory TranscriptStore for tests.
type transcriptMemory struct {
	sessions  map[string]TranscriptSession
	records   map[string][]TranscriptRecord
	lastModel string
}

func newTranscriptMemory() *transcriptMemory {
	return &transcriptMemory{sessions: make(map[string]TranscriptSession), records: make(map[string][]TranscriptRecord)}
}

func (m *transcriptMemory) CreateSession(_ context.Context, session TranscriptSession) error {
	if _, exists := m.sessions[session.SessionID]; !exists {
		m.sessions[session.SessionID] = session
	}
	return nil
}

func (m *transcriptMemory) SetModel(_ context.Context, sessionID, model string) error {
	m.lastModel = model
	return nil
}

func (m *transcriptMemory) Append(_ context.Context, sessionID string, records []TranscriptRecord) error {
	m.records[sessionID] = append(m.records[sessionID], records...)
	return nil
}

func (m *transcriptMemory) Load(_ context.Context, sessionID string) ([]TranscriptRecord, error) {
	out := make([]TranscriptRecord, len(m.records[sessionID]))
	copy(out, m.records[sessionID])
	return out, nil
}

func (m *transcriptMemory) Exists(_ context.Context, sessionID string) (bool, error) {
	_, ok := m.sessions[sessionID]
	return ok, nil
}

// modelStub resolves both models onto one endpoint.
type modelStub struct{ endpoint ModelEndpoint }

func (s modelStub) ListModels(context.Context) ([]agents.ModelInfo, error) {
	return []agents.ModelInfo{{ID: "1", Name: "test-model", Enabled: true}}, nil
}

func (s modelStub) ResolveModel(context.Context, string) (ModelEndpoint, error) {
	return s.endpoint, nil
}

func TestFullTurnRunsToolsPersistsAndPublishes(t *testing.T) {
	workDir := t.TempDir()
	responses := newTranscriptMemory()
	step := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		step++
		if step == 1 {
			// Step one: a write_file tool call.
			sse(w, sseToolCall())
			return
		}
		// Step two: the final message.
		sse(w, sseFinal())
	}))
	defer server.Close()

	adapter := New(Options{
		TranscriptStore: responses,
		ModelResolver:   modelStub{endpoint: ModelEndpoint{BaseURL: server.URL, ModelID: "test-model"}},
		PermissionMode:  func() string { return PermissionModeAuto },
		Now:             func() time.Time { return time.Unix(0, 0).UTC() },
	})
	inst, err := adapter.StartInstance(context.Background(), agents.StartInstanceRequest{Directory: workDir})
	if err != nil {
		t.Fatalf("StartInstance: %v", err)
	}
	ref, err := adapter.CreateSession(context.Background(), inst.ID, agents.CreateSessionOptions{Title: "会话"})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	stream, err := adapter.Subscribe(context.Background(), inst.ID)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	if err := adapter.SendMessage(context.Background(), *ref, agents.SendMessageRequest{Text: "写个文件"}); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	var types []agents.EventType
	var finalText string
	var turnEnded bool
	deadline := time.After(5 * time.Second)
	collecting := true
	for collecting {
		select {
		case event, ok := <-stream:
			if !ok {
				collecting = false
				break
			}
			if err := event.Validate(); err != nil {
				t.Fatalf("invalid event: %v", err)
			}
			types = append(types, event.Type)
			if event.Type == agents.EventMessage && event.Message != nil && event.Message.Final && event.Message.Role == "assistant" {
				finalText = event.Message.Text
			}
			if event.Type == agents.EventTurn && event.Turn != nil && event.Turn.Phase == agents.TurnEnded {
				turnEnded = true
				if event.Turn.Outcome != agents.TurnCompleted {
					t.Fatalf("turn outcome = %q", event.Turn.Outcome)
				}
				collecting = false
			}
		case <-deadline:
			collecting = false
		}
	}
	for _, want := range []agents.EventType{agents.EventMessage, agents.EventTurn} {
		found := false
		for _, got := range types {
			if got == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("event stream misses %v; got %v", want, types)
		}
	}
	for _, got := range types {
		if got == agents.EventToolCall || got == agents.EventToolResult {
			t.Fatalf("ordinary tool event reached the stream; got %v", types)
		}
	}
	if !turnEnded || finalText != "完成" {
		t.Fatalf("turn = %v, final = %q, types %v", turnEnded, finalText, types)
	}

	written, err := os.ReadFile(filepath.Join(workDir, "notes.txt"))
	if err != nil || string(written) != "hello" {
		t.Fatalf("tool did not write the file: (%q, %v)", written, err)
	}

	// The stored transcript has user, assistant text, tool call, tool result and
	// the final assistant message.
	records, err := responses.Load(context.Background(), ref.SessionID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	kinds := make([]TranscriptKind, 0, len(records))
	for _, record := range records {
		kinds = append(kinds, record.Kind)
	}
	for _, want := range []TranscriptKind{TranscriptUser, TranscriptToolCall, TranscriptToolResult, TranscriptAssistant} {
		found := false
		for _, got := range kinds {
			if got == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("transcript kinds %v miss %v", kinds, want)
		}
	}
	// The tool call record keeps the full arguments.
	for _, record := range records {
		if record.Kind == TranscriptToolCall {
			var payload toolCallRecord
			if err := json.Unmarshal(record.Payload, &payload); err != nil {
				t.Fatalf("tool call payload: %v", err)
			}
			if payload.Name != "write_file" || strings.Contains(string(payload.Input), "_truncated") {
				t.Fatalf("tool call payload = %s/%s", payload.Name, payload.Input)
			}
		}
	}

	// The stored tool result is the raw output the model is fed back, not a
	// summary, and it is what a later context rebuild reads.
	var storedToolOutput string
	records, err = adapter.store.Load(context.Background(), ref.SessionID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	for _, record := range records {
		if record.Kind != TranscriptToolResult {
			continue
		}
		var payload toolResultRecord
		if err := json.Unmarshal(record.Payload, &payload); err == nil {
			storedToolOutput = payload.Output
		}
	}
	if storedToolOutput == "" {
		t.Fatal("no tool result was stored")
	}
	if !strings.Contains(storedToolOutput, "Successfully wrote") {
		t.Fatalf("tool output = %q, want the raw write_file output", storedToolOutput)
	}
}

func TestConfirmModeBlocksUntilPermissionReply(t *testing.T) {
	workDir := t.TempDir()
	responses := newTranscriptMemory()
	first := true
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if first {
			first = false
			// The model asks write_file immediately.
			sse(w, sseToolCall())
			return
		}
		sse(w, sseFinal())
	}))
	defer server.Close()

	adapter := New(Options{
		TranscriptStore: responses,
		ModelResolver:   modelStub{endpoint: ModelEndpoint{BaseURL: server.URL, ModelID: "m"}},
		PermissionMode:  func() string { return PermissionModeConfirm },
		Now:             func() time.Time { return time.Unix(0, 0).UTC() },
	})
	inst, err := adapter.StartInstance(context.Background(), agents.StartInstanceRequest{Directory: workDir})
	if err != nil {
		t.Fatalf("StartInstance: %v", err)
	}
	ref, err := adapter.CreateSession(context.Background(), inst.ID, agents.CreateSessionOptions{})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	stream, err := adapter.Subscribe(context.Background(), inst.ID)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	if err := adapter.SendMessage(context.Background(), *ref, agents.SendMessageRequest{Text: "写"}); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	// Wait for the interaction, list it, and reject it once: the tool does not
	// run and the recorded tool result says the user refused.
	var interaction agents.InteractionPayload
	for event := range stream {
		if event.Type == agents.EventInteraction && event.Interaction != nil && event.Interaction.Phase == agents.InteractionRequested {
			interaction = *event.Interaction
			break
		}
		if event.Type == agents.EventTurn && event.Turn.Phase == agents.TurnEnded {
			t.Fatal("turn ended without an interaction")
		}
	}
	if interaction.Permission == nil || interaction.Permission.Action != "write_file" {
		t.Fatalf("interaction = %+v", interaction)
	}
	pending, err := adapter.ListPermissions(context.Background(), *ref)
	if err != nil || len(pending) != 1 {
		t.Fatalf("ListPermissions = (%v, %v)", pending, err)
	}
	if err := adapter.ReplyPermission(context.Background(), *ref, interaction.ID, "reject", ""); err != nil {
		t.Fatalf("ReplyPermission: %v", err)
	}

	events := drainUntilTurnEnd(t, stream)
	if _, exists := eventOfKind(events, agents.EventToolResult); exists {
		t.Fatal("ordinary tool result reached the stream")
	}
	if _, err := os.Stat(filepath.Join(workDir, "notes.txt")); !os.IsNotExist(err) {
		t.Fatal("rejected tool ran anyway")
	}
	records, err := adapter.store.Load(context.Background(), ref.SessionID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	var rejectionSummary string
	for _, record := range records {
		if record.Kind != TranscriptToolResult {
			continue
		}
		var payload toolResultRecord
		if json.Unmarshal(record.Payload, &payload) == nil && strings.Contains(payload.Output, "拒绝") {
			rejectionSummary = payload.Output
		}
	}
	if rejectionSummary == "" {
		t.Fatalf("the rejection never reached the model-facing output: %+v", records)
	}

	// An expired reply is a clean 409-style error.
	if err := adapter.ReplyPermission(context.Background(), *ref, interaction.ID, "once", ""); err == nil {
		t.Fatal("second reply accepted")
	}
}

func TestInterruptCancelsTheRunningTurn(t *testing.T) {
	workDir := t.TempDir()
	responses := newTranscriptMemory()
	block := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		<-block // hang until the interrupt signals the context
	}))
	// LIFO: unblock the handler before the server closes.
	defer server.Close()
	defer close(block)

	adapter := New(Options{
		TranscriptStore: responses,
		ModelResolver:   modelStub{endpoint: ModelEndpoint{BaseURL: server.URL, ModelID: "m"}},
		PermissionMode:  func() string { return PermissionModeAuto },
		Now:             func() time.Time { return time.Unix(0, 0).UTC() },
	})
	inst, err := adapter.StartInstance(context.Background(), agents.StartInstanceRequest{Directory: workDir})
	if err != nil {
		t.Fatalf("StartInstance: %v", err)
	}
	ref, err := adapter.CreateSession(context.Background(), inst.ID, agents.CreateSessionOptions{})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	stream, err := adapter.Subscribe(context.Background(), inst.ID)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	if err := adapter.SendMessage(context.Background(), *ref, agents.SendMessageRequest{Text: "挂起"}); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	// Wait for turn started, then interrupt.
	for event := range stream {
		if event.Type == agents.EventTurn && event.Turn.Phase == agents.TurnStarted {
			break
		}
	}
	if busy, err := adapter.DetectTurn(context.Background(), *ref); err != nil || !busy {
		t.Fatalf("DetectTurn = (%v, %v), want (true, nil)", busy, err)
	}
	if err := adapter.Interrupt(context.Background(), *ref); err != nil {
		t.Fatalf("Interrupt: %v", err)
	}
	for event := range stream {
		if event.Type == agents.EventTurn && event.Turn.Phase == agents.TurnEnded {
			if event.Turn.Outcome != agents.TurnCancelled {
				t.Fatalf("interrupted outcome = %q", event.Turn.Outcome)
			}
			break
		}
	}
}

func sseToolCall() map[string]any {
	return map[string]any{
		"id": "c", "object": "chat.completion.chunk", "created": 1,
		"choices": []any{map[string]any{
			"delta": map[string]any{
				"tool_calls": []any{map[string]any{
					"index": 0, "id": "call-1",
					"function": map[string]any{"name": "write_file", "arguments": `{"path":"notes.txt","content":"hello"}`},
				}},
			},
		}},
	}
}

func sseFinal() map[string]any {
	return map[string]any{
		"id": "c", "object": "chat.completion.chunk", "created": 1,
		"choices": []any{map[string]any{
			"delta": map[string]any{"content": "完成"}, "finish_reason": "stop",
		}},
	}
}

func sse(w http.ResponseWriter, chunks ...map[string]any) {
	w.Header().Set("Content-Type", "text/event-stream")
	for _, chunk := range chunks {
		encoded, _ := json.Marshal(chunk)
		_, _ = w.Write([]byte("data: " + string(encoded) + "\n\n"))
	}
	_, _ = w.Write([]byte("data: [DONE]\n\n"))
}

func eventOfKind(events []agents.NormalizedEvent, kind agents.EventType) (agents.NormalizedEvent, bool) {
	for _, event := range events {
		if event.Type == kind {
			return event, true
		}
	}
	return agents.NormalizedEvent{}, false
}

func drainUntilTurnEnd(t *testing.T, stream <-chan agents.NormalizedEvent) []agents.NormalizedEvent {
	t.Helper()
	events := []agents.NormalizedEvent{}
	deadline := time.After(5 * time.Second)
	for {
		select {
		case event, ok := <-stream:
			if !ok {
				return events
			}
			events = append(events, event)
			if event.Type == agents.EventTurn && event.Turn.Phase == agents.TurnEnded {
				return events
			}
		case <-deadline:
			return events
		}
	}
}

func TestAdapterStepLimit(t *testing.T) {
	store := newTranscriptMemory()
	resolver := modelStub{}

	// Default when unset (0) is 30
	a1 := New(Options{TranscriptStore: store, ModelResolver: resolver})
	if limit := a1.stepLimit(); limit != defaultTurnSteps {
		t.Fatalf("expected default %d, got %d", defaultTurnSteps, limit)
	}

	// Static override > 0
	a2 := New(Options{TranscriptStore: store, ModelResolver: resolver, MaxTurnSteps: 15})
	if limit := a2.stepLimit(); limit != 15 {
		t.Fatalf("expected 15, got %d", limit)
	}

	// Static override -1 (unlimited)
	a3 := New(Options{TranscriptStore: store, ModelResolver: resolver, MaxTurnSteps: -1})
	if limit := a3.stepLimit(); limit != -1 {
		t.Fatalf("expected -1, got %d", limit)
	}

	// Dynamic resolver overrides static value
	dynamicSteps := 10
	a4 := New(Options{
		TranscriptStore: store,
		ModelResolver:   resolver,
		MaxTurnSteps:    50,
		ResolveMaxTurnSteps: func() int {
			return dynamicSteps
		},
	})
	if limit := a4.stepLimit(); limit != 10 {
		t.Fatalf("expected 10, got %d", limit)
	}

	// Dynamic resolver returns -1 (unlimited)
	dynamicSteps = -1
	if limit := a4.stepLimit(); limit != -1 {
		t.Fatalf("expected -1, got %d", limit)
	}

	// Dynamic resolver returns 0 (fallback to static)
	dynamicSteps = 0
	if limit := a4.stepLimit(); limit != 50 {
		t.Fatalf("expected fallback 50, got %d", limit)
	}
}

func TestSessionPermissionModeOverridesGlobalConfirm(t *testing.T) {
	workDir := t.TempDir()
	responses := newTranscriptMemory()
	step := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		step++
		if step%2 == 1 {
			sse(w, sseToolCall())
			return
		}
		sse(w, sseFinal())
	}))
	defer server.Close()

	// Platform-wide default is confirm, which normally blocks write_file.
	adapter := New(Options{
		TranscriptStore: responses,
		ModelResolver:   modelStub{endpoint: ModelEndpoint{BaseURL: server.URL, ModelID: "m"}},
		PermissionMode:  func() string { return PermissionModeConfirm },
		Now:             func() time.Time { return time.Unix(0, 0).UTC() },
	})
	inst, err := adapter.StartInstance(context.Background(), agents.StartInstanceRequest{Directory: workDir})
	if err != nil {
		t.Fatalf("StartInstance: %v", err)
	}
	ref, err := adapter.CreateSession(context.Background(), inst.ID, agents.CreateSessionOptions{
		Title:          "BOT自动放行",
		PermissionMode: PermissionModeAuto,
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	stream, err := adapter.Subscribe(context.Background(), inst.ID)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	if err := adapter.SendMessage(context.Background(), *ref, agents.SendMessageRequest{Text: "写文件"}); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	events := drainUntilTurnEnd(t, stream)
	for _, ev := range events {
		if ev.Type == agents.EventInteraction {
			t.Fatalf("unexpected approval interaction with session PermissionMode=auto: %+v", ev.Interaction)
		}
	}
	written, err := os.ReadFile(filepath.Join(workDir, "notes.txt"))
	if err != nil || string(written) != "hello" {
		t.Fatalf("write_file did not execute under session PermissionMode=auto: (%q, %v)", written, err)
	}

	// Simulate restart: StopInstance clears in-memory sessionState, ContinueSession + SendMessage carries PermissionMode.
	_ = os.Remove(filepath.Join(workDir, "notes.txt"))
	if err := adapter.StopInstance(context.Background(), inst.ID); err != nil {
		t.Fatalf("StopInstance: %v", err)
	}
	inst2, err := adapter.StartInstance(context.Background(), agents.StartInstanceRequest{Directory: workDir})
	if err != nil {
		t.Fatalf("StartInstance 2: %v", err)
	}
	resumedRef := agents.SessionRef{InstanceID: inst2.ID, Backend: agents.BackendBuiltin, SessionID: ref.SessionID}
	if _, err := adapter.ContinueSession(context.Background(), resumedRef); err != nil {
		t.Fatalf("ContinueSession: %v", err)
	}
	stream2, err := adapter.Subscribe(context.Background(), inst2.ID)
	if err != nil {
		t.Fatalf("Subscribe 2: %v", err)
	}
	if err := adapter.SendMessage(context.Background(), resumedRef, agents.SendMessageRequest{
		Text:           "继续写文件",
		PermissionMode: PermissionModeAuto,
	}); err != nil {
		t.Fatalf("SendMessage 2: %v", err)
	}
	events2 := drainUntilTurnEnd(t, stream2)
	for _, ev := range events2 {
		if ev.Type == agents.EventInteraction {
			t.Fatalf("unexpected approval interaction after resume with SendMessage.PermissionMode=auto: %+v", ev.Interaction)
		}
	}
	written2, err := os.ReadFile(filepath.Join(workDir, "notes.txt"))
	if err != nil || string(written2) != "hello" {
		t.Fatalf("write_file did not execute after resume: (%q, %v)", written2, err)
	}
}
