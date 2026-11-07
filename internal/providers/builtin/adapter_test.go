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
	for _, want := range []agents.EventType{
		agents.EventMessage, agents.EventTurn, agents.EventToolCall, agents.EventToolResult,
	} {
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
	if _, exists := eventOfKind(events, agents.EventToolResult); !exists {
		t.Fatal("no tool result event after reject")
	}
	if _, err := os.Stat(filepath.Join(workDir, "notes.txt")); !os.IsNotExist(err) {
		t.Fatal("rejected tool ran anyway")
	}
	var rejectionSummary string
	for _, event := range events {
		if event.Type == agents.EventToolResult && event.ToolResult != nil {
			if strings.Contains(event.ToolResult.Output, "拒绝") || strings.Contains(event.ToolResult.Output, "拒绝") {
				rejectionSummary = event.ToolResult.Output
			}
		}
	}
	if rejectionSummary == "" {
		t.Fatalf("the rejection never reached the model-facing summary: %+v", events)
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
