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

// sseTwoReads streams one assistant message carrying two tool calls, the shape
// a model emits when it reads two files in one step.
func sseTwoReads() map[string]any {
	return map[string]any{
		"id": "c", "object": "chat.completion.chunk", "created": 1,
		"choices": []any{map[string]any{
			"delta": map[string]any{
				"tool_calls": []any{
					map[string]any{"index": 0, "id": "call-1", "function": map[string]any{"name": "read_file", "arguments": `{"path":"a.txt"}`}},
					map[string]any{"index": 1, "id": "call-2", "function": map[string]any{"name": "read_file", "arguments": `{"path":"b.txt"}`}},
				},
			},
		}},
	}
}

// sseOneReadWithoutID streams one tool call with no id, which some
// OpenAI-compatible gateways do.
func sseOneReadWithoutID() map[string]any {
	return map[string]any{
		"id": "c", "object": "chat.completion.chunk", "created": 1,
		"choices": []any{map[string]any{
			"delta": map[string]any{
				"tool_calls": []any{
					map[string]any{"index": 0, "function": map[string]any{"name": "read_file", "arguments": `{"path":"a.txt"}`}},
				},
			},
		}},
	}
}

type orderHarness struct {
	adapter *Adapter
	ref     agents.SessionRef
	stream  <-chan agents.NormalizedEvent
}

func newOrderHarness(t *testing.T, server *httptest.Server, workDir string, mode string) orderHarness {
	t.Helper()
	adapter := New(Options{
		TranscriptStore: newTranscriptMemory(),
		ModelResolver:   modelStub{endpoint: ModelEndpoint{BaseURL: server.URL, ModelID: "m"}},
		PermissionMode:  func() string { return mode },
		Now:             func() time.Time { return time.Unix(0, 0).UTC() },
	})
	inst, err := adapter.StartInstance(context.Background(), agents.StartInstanceRequest{Directory: workDir})
	if err != nil {
		t.Fatalf("StartInstance: %v", err)
	}
	ref, err := adapter.CreateSession(context.Background(), inst.ID, agents.CreateSessionOptions{Title: "t"})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	stream, err := adapter.Subscribe(context.Background(), inst.ID)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	return orderHarness{adapter: adapter, ref: *ref, stream: stream}
}

// toolFrames flattens the tool events of a stream into "call:<id>" and
// "result:<id>" labels, in arrival order.
func toolFrames(events []agents.NormalizedEvent) []string {
	frames := make([]string, 0, len(events))
	for _, event := range events {
		switch event.Type {
		case agents.EventToolCall:
			frames = append(frames, "call:"+event.ToolCall.CallID)
		case agents.EventToolResult:
			frames = append(frames, "result:"+event.ToolResult.CallID)
		}
	}
	return frames
}

// storedToolFrames flattens a session's stored tool records into the same
// labels, in transcript order. The transcript is what the model context is
// rebuilt from, so call/result ordering and pairing are asserted there.
func storedToolFrames(t *testing.T, adapter *Adapter, sessionID string) []string {
	t.Helper()
	records, err := adapter.store.Load(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	var frames []string
	for _, record := range records {
		switch record.Kind {
		case TranscriptToolCall:
			var payload toolCallRecord
			if err := json.Unmarshal(record.Payload, &payload); err == nil {
				frames = append(frames, "call:"+payload.CallID)
			}
		case TranscriptToolResult:
			var payload toolResultRecord
			if err := json.Unmarshal(record.Payload, &payload); err == nil {
				frames = append(frames, "result:"+payload.CallID)
			}
		}
	}
	return frames
}

func TestMultipleToolCallsKeepAdjacentOrder(t *testing.T) {
	workDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(workDir, "a.txt"), []byte("AAA"), 0o644)
	_ = os.WriteFile(filepath.Join(workDir, "b.txt"), []byte("BBB"), 0o644)
	step := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		step++
		if step == 1 {
			sse(w, sseTwoReads())
			return
		}
		sse(w, sseFinal())
	}))
	defer server.Close()

	h := newOrderHarness(t, server, workDir, PermissionModeAuto)
	if err := h.adapter.SendMessage(context.Background(), h.ref, agents.SendMessageRequest{Text: "读两个文件"}); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	events := drainUntilTurnEnd(t, h.stream)
	if live := toolFrames(events); len(live) != 0 {
		t.Fatalf("ordinary tool frames reached the stream: %v", live)
	}
	got := storedToolFrames(t, h.adapter, h.ref.SessionID)
	// Each announced call must be answered before the next one is announced:
	// otherwise the model context carries a call with no result.
	wantCalls := map[string]bool{"call-1": true, "call-2": true}
	if len(got) != 4 {
		t.Fatalf("tool frames = %v, want 2 calls and 2 results", got)
	}
	seen := map[string]bool{}
	for i, frame := range got {
		kind, id, ok := strings.Cut(frame, ":")
		if !ok || !wantCalls[id] {
			t.Fatalf("tool frames = %v, unexpected %q", got, frame)
		}
		if seen[id] && kind == "call" {
			t.Fatalf("tool frames = %v, call %q announced twice", got, id)
		}
		if i%2 == 0 {
			if kind != "call" {
				t.Fatalf("tool frames = %v, want a call at position %d", got, i)
			}
			seen[id] = true
			continue
		}
		if kind != "result" {
			t.Fatalf("tool frames = %v, want a result at position %d", got, i)
		}
		callID := strings.TrimPrefix(got[i-1], "call:")
		if callID != id {
			t.Fatalf("tool frames = %v, result %q answers %q", got, id, callID)
		}
	}

	history, err := h.adapter.ListHistory(context.Background(), agents.ListHistoryRequest{Directory: workDir, SessionID: h.ref.SessionID})
	if err != nil {
		t.Fatalf("ListHistory: %v", err)
	}
	if replayed := toolFrames(history.Events); len(replayed) != 0 {
		t.Fatalf("history tool frames = %v, want none", replayed)
	}
}

func TestToolCallWithoutIDKeepsHistoryPairing(t *testing.T) {
	workDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(workDir, "a.txt"), []byte("AAA"), 0o644)
	step := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		step++
		if step == 1 {
			sse(w, sseOneReadWithoutID())
			return
		}
		sse(w, sseFinal())
	}))
	defer server.Close()

	h := newOrderHarness(t, server, workDir, PermissionModeAuto)
	if err := h.adapter.SendMessage(context.Background(), h.ref, agents.SendMessageRequest{Text: "读"}); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	drainUntilTurnEnd(t, h.stream)
	var calls, results []string
	for _, frame := range storedToolFrames(t, h.adapter, h.ref.SessionID) {
		kind, id, _ := strings.Cut(frame, ":")
		if kind == "call" {
			calls = append(calls, id)
		} else {
			results = append(results, id)
		}
	}
	if len(calls) != 1 || len(results) != 1 {
		t.Fatalf("stored calls=%v results=%v, want exactly one of each", calls, results)
	}
	if calls[0] == "" || results[0] != calls[0] {
		t.Fatalf("stored calls=%v results=%v, want one non-empty shared id", calls, results)
	}
}

func TestInterruptAnswersAnnouncedToolCall(t *testing.T) {
	workDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(workDir, "a.txt"), []byte("AAA"), 0o644)
	step := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		step++
		if step == 1 {
			// A write needs approval in confirm mode, so the call is announced
			// and the turn blocks in the gate: exactly where an interrupt lands.
			sse(w, sseToolCall())
			return
		}
		sse(w, sseFinal())
	}))
	defer server.Close()

	h := newOrderHarness(t, server, workDir, PermissionModeConfirm)
	if err := h.adapter.SendMessage(context.Background(), h.ref, agents.SendMessageRequest{Text: "写"}); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	announced := false
	deadline := time.After(5 * time.Second)
	for !announced {
		select {
		case event := <-h.stream:
			if event.Type == agents.EventInteraction && event.Interaction != nil && event.Interaction.Phase == agents.InteractionRequested {
				announced = true
			}
		case <-deadline:
			t.Fatal("tool call was never announced")
		}
	}
	if err := h.adapter.Interrupt(context.Background(), h.ref); err != nil {
		t.Fatalf("Interrupt: %v", err)
	}
	drainUntilTurnEnd(t, h.stream)
	frames := storedToolFrames(t, h.adapter, h.ref.SessionID)
	if len(frames) != 2 || !strings.HasPrefix(frames[0], "call:") ||
		frames[1] != "result:"+strings.TrimPrefix(frames[0], "call:") {
		t.Fatalf("stored tool frames = %v, want the interrupted call answered", frames)
	}
}

func TestHistoryContextKeepsToolCallsPaired(t *testing.T) {
	workDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(workDir, "a.txt"), []byte("AAA"), 0o644)
	_ = os.WriteFile(filepath.Join(workDir, "b.txt"), []byte("BBB"), 0o644)
	step := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		step++
		if step == 1 {
			sse(w, sseTwoReads())
			return
		}
		sse(w, sseFinal())
	}))
	defer server.Close()

	h := newOrderHarness(t, server, workDir, PermissionModeAuto)
	if err := h.adapter.SendMessage(context.Background(), h.ref, agents.SendMessageRequest{Text: "读两个"}); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	drainUntilTurnEnd(t, h.stream)

	records, err := h.adapter.store.Load(context.Background(), h.ref.SessionID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	messages := renderHistoryMessages(records, ModelEndpoint{ModelID: "m"})
	assertToolCallsPaired(t, messages)
}

// TestLegacyTranscriptDuplicatesReplayCleanly feeds the reader the transcript
// older versions wrote (a handoff batch plus a per-call record, and a call with
// no id at all) and requires replay and context to stay usable.
func TestLegacyTranscriptDuplicatesReplayCleanly(t *testing.T) {
	workDir := t.TempDir()
	adapter := New(Options{
		TranscriptStore: newTranscriptMemory(),
		ModelResolver:   modelStub{endpoint: ModelEndpoint{BaseURL: "http://127.0.0.1:1", ModelID: "m"}},
		PermissionMode:  func() string { return PermissionModeAuto },
	})
	inst, err := adapter.StartInstance(context.Background(), agents.StartInstanceRequest{Directory: workDir})
	if err != nil {
		t.Fatalf("StartInstance: %v", err)
	}
	ref, err := adapter.CreateSession(context.Background(), inst.ID, agents.CreateSessionOptions{Title: "t"})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	now := time.Unix(0, 0).UTC()
	legacy := []TranscriptRecord{
		buildUserRecord(1, agents.SendMessageRequest{Text: "读两个"}, now),
		buildToolCallRecord(2, "read_file", "", []byte(`{"path":"a.txt"}`), now),
		buildToolCallRecord(3, "read_file", "call-1", []byte(`{"path":"a.txt"}`), now),
		buildToolCallRecord(4, "read_file", "call-2", []byte(`{"path":"b.txt"}`), now),
		buildToolCallRecord(5, "read_file", "call-1", []byte(`{"path":"a.txt"}`), now),
		buildToolResultRecord(6, "call-1", "1: AAA", false, now),
		buildToolCallRecord(7, "read_file", "call-2", []byte(`{"path":"b.txt"}`), now),
		buildToolResultRecord(8, "call-2", "1: BBB", false, now),
	}
	if err := adapter.store.Append(context.Background(), ref.SessionID, legacy); err != nil {
		t.Fatalf("Append: %v", err)
	}

	history, err := adapter.ListHistory(context.Background(), agents.ListHistoryRequest{Directory: workDir, SessionID: ref.SessionID})
	if err != nil {
		t.Fatalf("ListHistory: %v", err)
	}
	if frames := toolFrames(history.Events); len(frames) != 0 {
		t.Fatalf("history tool frames = %v, want none", frames)
	}

	// Legacy transcripts must still rebuild a valid model context: every call
	// is paired with its result even when the duplicate records are present.
	assertToolCallsPaired(t, renderHistoryMessages(legacy, ModelEndpoint{ModelID: "m"}))
}

// assertToolCallsPaired verifies the OpenAI loop invariant: every assistant
// message carrying tool_calls is followed by exactly one tool message per call,
// each answering the same call id.
func assertToolCallsPaired(t *testing.T, messages []chatMessage) {
	t.Helper()
	for i := 0; i < len(messages); i++ {
		call := messages[i]
		if len(call.ToolCalls) == 0 {
			continue
		}
		if i+len(call.ToolCalls) >= len(messages) {
			t.Fatalf("assistant tool_calls at %d has no following tool messages: %#v", i, messages)
		}
		for offset, want := range call.ToolCalls {
			reply := messages[i+1+offset]
			if reply.Role != "tool" || reply.ToolCallID != want.ID {
				t.Fatalf("message at %d (%s %q) does not answer %q: %#v", i+1+offset, reply.Role, reply.ToolCallID, want.ID, messages)
			}
		}
		i += len(call.ToolCalls)
	}
	// No orphan tool message may precede its call.
	for i, message := range messages {
		if message.Role == "tool" {
			if i == 0 {
				t.Fatalf("tool message at 0 has no call: %#v", messages)
			}
			call := messages[i-1]
			found := len(call.ToolCalls) > 0
			for j := i - 1; j >= 0 && !found; j-- {
				if messages[j].Role == "tool" {
					continue
				}
				found = len(messages[j].ToolCalls) > 0
			}
			if !found {
				t.Fatalf("tool message at %d has no preceding call: %#v", i, messages)
			}
		}
	}
}

// sseTextThenToolCalls streams the shape of a model that explains itself and
// then asks for two tools in the same completion.
func sseTextThenToolCalls() []map[string]any {
	text := map[string]any{
		"id": "c", "object": "chat.completion.chunk", "created": 1,
		"choices": []any{map[string]any{
			"delta": map[string]any{"content": "我先读这两个文件。"},
		}},
	}
	return []map[string]any{text, sseTwoReads()}
}

func TestStreamedTextPrecedesItsToolCalls(t *testing.T) {
	workDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(workDir, "a.txt"), []byte("AAA"), 0o644)
	_ = os.WriteFile(filepath.Join(workDir, "b.txt"), []byte("BBB"), 0o644)
	step := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		step++
		if step == 1 {
			sse(w, sseTextThenToolCalls()...)
			return
		}
		sse(w, sseFinal())
	}))
	defer server.Close()

	h := newOrderHarness(t, server, workDir, PermissionModeAuto)
	if err := h.adapter.SendMessage(context.Background(), h.ref, agents.SendMessageRequest{Text: "读两个"}); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	events := drainUntilTurnEnd(t, h.stream)
	textAt, callAt := -1, -1
	for index, event := range events {
		if event.Type == agents.EventMessage && event.Message != nil && event.Message.Role == "assistant" && !event.Message.Final && textAt < 0 {
			textAt = index
		}
		if event.Type == agents.EventToolCall && callAt < 0 {
			callAt = index
		}
	}
	if textAt < 0 {
		t.Fatalf("stream lacks the assistant text chunk")
	}
	if callAt >= 0 {
		t.Fatalf("ordinary tool call reached the stream at %d", callAt)
	}
}
