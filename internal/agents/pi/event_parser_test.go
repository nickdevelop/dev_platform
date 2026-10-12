package pi

import (
	"strings"
	"testing"
	"time"

	"devplatform/internal/agents"
)

// parserClock is a fixed clock so parser tests never depend on wall time.
func parserClock() time.Time {
	return time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
}

// collect runs the parser over r and returns the normalized events it emitted.
func collect(t *testing.T, r string) []agents.NormalizedEvent {
	t.Helper()
	p := newParser("inst-1", "sess-1", parserClock)
	var got []agents.NormalizedEvent
	if err := p.run(strings.NewReader(r), func(ev agents.NormalizedEvent) {
		got = append(got, ev)
	}); err != nil {
		t.Fatalf("parser.run() error = %v, want nil", err)
	}
	return got
}

// TestParserStreamingMessages checks the message state machine: the user
// message is emitted complete, assistant text arrives as non-final deltas and
// is closed by an empty final terminator, and the session header becomes a
// lifecycle event carrying the session id and working directory.
func TestParserStreamingMessages(t *testing.T) {
	input := strings.Join([]string{
		`{"type":"session","version":3,"id":"sess-1","cwd":"/data/projects/demo"}`,
		`{"type":"message_start","message":{"role":"user","content":"hello"}}`,
		`{"type":"message_update","usage":{},"assistantMessageEvent":{"type":"text_delta","contentIndex":0,"delta":"Hi"}}`,
		`{"type":"message_update","usage":{},"assistantMessageEvent":{"type":"text_delta","contentIndex":0,"delta":"!"}}`,
		`{"type":"message_end","message":{"role":"assistant","content":[{"type":"text","text":"Hi!"}],"stopReason":"stop"}}`,
	}, "\n") + "\n"

	got := collect(t, input)
	if len(got) != 5 {
		t.Fatalf("got %d events, want 5: %+v", len(got), got)
	}

	if got[0].Type != agents.EventLifecycle || got[0].Lifecycle == nil {
		t.Fatalf("event[0] = %+v, want lifecycle", got[0])
	}
	if got[0].Lifecycle.State != "session_started" || got[0].Lifecycle.Detail != "/data/projects/demo" {
		t.Errorf("session header lifecycle = %+v, want state session_started detail /data/projects/demo", got[0].Lifecycle)
	}
	if got[0].SessionID != "sess-1" {
		t.Errorf("session header SessionID = %q, want sess-1", got[0].SessionID)
	}

	if got[1].Type != agents.EventMessage || got[1].Message == nil || got[1].Message.Role != "user" ||
		got[1].Message.Text != "hello" || !got[1].Message.Final {
		t.Errorf("event[1] = %+v, want final user message hello", got[1])
	}

	if got[2].Type != agents.EventMessage || got[2].Message.Role != "assistant" ||
		got[2].Message.Text != "Hi" || got[2].Message.Final {
		t.Errorf("event[2] = %+v, want non-final assistant delta Hi", got[2])
	}
	if got[3].Message == nil || got[3].Message.Text != "!" || got[3].Message.Final {
		t.Errorf("event[3] = %+v, want non-final assistant delta !", got[3])
	}
	if got[4].Message == nil || got[4].Message.Role != "assistant" || !got[4].Message.Final {
		t.Errorf("event[4] = %+v, want final assistant terminator", got[4])
	}
	if got[4].Message.Text != "" {
		t.Errorf("event[4].Text = %q, want empty terminator (deltas carried the text)", got[4].Message.Text)
	}

	// Every event must carry the pi envelope so it can be validated.
	for i, ev := range got {
		if ev.Backend != agents.BackendPi {
			t.Errorf("event[%d].Backend = %q, want pi", i, ev.Backend)
		}
		if ev.InstanceID != "inst-1" {
			t.Errorf("event[%d].InstanceID = %q, want inst-1", i, ev.InstanceID)
		}
		if ev.Timestamp.IsZero() {
			t.Errorf("event[%d].Timestamp is zero", i)
		}
	}
}

// TestParserNonStreamedAssistantKeepsText pins the fallback: when a provider
// does not stream deltas, message_end still delivers the full assistant text
// as the final message.
func TestParserNonStreamedAssistantKeepsText(t *testing.T) {
	input := `{"type":"message_end","message":{"role":"assistant","content":[{"type":"text","text":"complete"}]}}` + "\n"
	got := collect(t, input)
	if len(got) != 1 {
		t.Fatalf("got %d events, want 1: %+v", len(got), got)
	}
	if got[0].Message == nil || got[0].Message.Text != "complete" || !got[0].Message.Final {
		t.Errorf("event = %+v, want final assistant message complete", got[0])
	}
}

// TestParserToolEvents checks tool execution records map onto the typed tool
// payloads.
func TestParserToolEvents(t *testing.T) {
	input := strings.Join([]string{
		`{"type":"tool_execution_start","toolCallId":"call-1","toolName":"bash","args":{"command":"ls"}}`,
		`{"type":"tool_execution_end","toolCallId":"call-1","toolName":"bash","result":{"content":[{"type":"text","text":"file.go"}]},"isError":false}`,
	}, "\n") + "\n"

	got := collect(t, input)
	if len(got) != 2 {
		t.Fatalf("got %d events, want 2: %+v", len(got), got)
	}
	if got[0].Type != agents.EventToolCall || got[0].ToolCall == nil {
		t.Fatalf("event[0] = %+v, want tool call", got[0])
	}
	if got[0].ToolCall.CallID != "call-1" || got[0].ToolCall.Name != "bash" ||
		string(got[0].ToolCall.Input) != `{"command":"ls"}` {
		t.Errorf("tool call = %+v, want call-1/bash/{\"command\":\"ls\"}", got[0].ToolCall)
	}
	if got[1].Type != agents.EventToolResult || got[1].ToolResult == nil {
		t.Fatalf("event[1] = %+v, want tool result", got[1])
	}
	if got[1].ToolResult.CallID != "call-1" || got[1].ToolResult.Output != "file.go" || got[1].ToolResult.IsError {
		t.Errorf("tool result = %+v, want call-1/file.go/ok", got[1].ToolResult)
	}
}

// TestParserMalformedAndPartialLines checks that a malformed record and a
// stream cut off mid-record both surface as parse errors instead of panicking
// or silently dropping data.
func TestParserMalformedAndPartialLines(t *testing.T) {
	input := strings.Join([]string{
		`{"type":"message_update","assistantMessageEvent":{"type":"text_delta","delta":"ok"}}`,
		`this is not json`,
		`{"type":"message_end"`,
	}, "\n")

	got := collect(t, input)
	if len(got) != 3 {
		t.Fatalf("got %d events, want 3: %+v", len(got), got)
	}
	if got[0].Type != agents.EventMessage || got[0].Message.Text != "ok" {
		t.Errorf("event[0] = %+v, want valid message delta", got[0])
	}
	for _, i := range []int{1, 2} {
		if got[i].Type != agents.EventError || got[i].Error == nil || got[i].Error.Code != "parse_error" {
			t.Errorf("event[%d] = %+v, want parse_error", i, got[i])
		}
		if len(got[i].Raw) == 0 {
			t.Errorf("event[%d] carries no raw body for diagnostics", i)
		}
	}
}

// TestParserUnknownEventPreserved pins that an unrecognized event kind keeps
// its original bytes so nothing the backend reports is dropped.
func TestParserUnknownEventPreserved(t *testing.T) {
	original := `{"type":"future_event","payload":{"a":1}}`
	got := collect(t, original+"\n")
	if len(got) != 1 {
		t.Fatalf("got %d events, want 1: %+v", len(got), got)
	}
	if got[0].Type != agents.EventUnknown {
		t.Fatalf("event type = %q, want unknown", got[0].Type)
	}
	if string(got[0].Raw) != original {
		t.Errorf("raw = %s, want %s", got[0].Raw, original)
	}
	if got[0].SessionID != "sess-1" {
		t.Errorf("SessionID = %q, want sess-1", got[0].SessionID)
	}
	if err := got[0].Validate(); err != nil {
		t.Errorf("unknown event does not satisfy the contract: %v", err)
	}
}

// TestParserProviderError checks that an in-stream provider error becomes a
// typed error event with the secret material redacted.
func TestParserProviderError(t *testing.T) {
	input := `{"type":"message_update","assistantMessageEvent":{"type":"error","reason":"error","error":"auth failed for sk-abcdef1234567890"}}` + "\n"
	got := collect(t, input)
	if len(got) != 1 {
		t.Fatalf("got %d events, want 1: %+v", len(got), got)
	}
	if got[0].Type != agents.EventError || got[0].Error == nil {
		t.Fatalf("event = %+v, want error", got[0])
	}
	if got[0].Error.Code != "provider" {
		t.Errorf("error code = %q, want provider", got[0].Error.Code)
	}
	if strings.Contains(got[0].Error.Message, "sk-abcdef1234567890") {
		t.Errorf("error message leaked the secret: %q", got[0].Error.Message)
	}
}

// TestParserStderrDiagnostics checks that application logging on stderr is
// surfaced as a diagnostic error event, with secrets redacted.
func TestParserStderrDiagnostics(t *testing.T) {
	p := newParser("inst-1", "sess-1", parserClock)
	var got []agents.NormalizedEvent
	err := p.runStderr(strings.NewReader("warning: provider token=supersecretvalue expired\n"), func(ev agents.NormalizedEvent) {
		got = append(got, ev)
	})
	if err != nil {
		t.Fatalf("runStderr() error = %v, want nil", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d events, want 1: %+v", len(got), got)
	}
	if got[0].Type != agents.EventError || got[0].Error == nil || got[0].Error.Code != "stderr" {
		t.Fatalf("event = %+v, want stderr error", got[0])
	}
	if strings.Contains(got[0].Error.Message, "supersecretvalue") {
		t.Errorf("stderr diagnostic leaked the secret: %q", got[0].Error.Message)
	}
}

// TestParserEndOfStream checks the terminal behaviours: an empty stream yields
// nothing, a valid final record without a trailing newline is still parsed,
// and an oversized record becomes a bounded error without wedging the reader.
func TestParserEndOfStream(t *testing.T) {
	if got := collect(t, ""); len(got) != 0 {
		t.Errorf("empty stream produced %d events, want 0", len(got))
	}

	got := collect(t, `{"type":"message_end","message":{"role":"assistant","content":[{"type":"text","text":"tail"}]}}`)
	if len(got) != 1 || got[0].Message == nil || got[0].Message.Text != "tail" {
		t.Errorf("final record without newline = %+v, want assistant tail", got)
	}

	oversized := `{"type":"future_event","pad":"` + strings.Repeat("a", maxStreamRecordBytes+10) + `"}`
	got = collect(t, oversized+"\n"+`{"type":"message_end","message":{"role":"assistant","content":[{"type":"text","text":"after"}]}}`+"\n")
	if len(got) != 2 {
		t.Fatalf("got %d events, want 2: %+v", len(got), got)
	}
	if got[0].Type != agents.EventError || got[0].Error == nil || got[0].Error.Code != "line_too_large" {
		t.Errorf("event[0] = %+v, want line_too_large", got[0])
	}
	if got[1].Message == nil || got[1].Message.Text != "after" {
		t.Errorf("event[1] = %+v, want the record after the oversized line", got[1])
	}
}

// TestParserStreamsThinking checks that reasoning is carried as its own event
// kind: deltas arrive non-final, and the block is closed by an empty final
// terminator, exactly like a streamed assistant message.
func TestParserStreamsThinking(t *testing.T) {
	input := strings.Join([]string{
		`{"type":"session","version":3,"id":"sess-1","cwd":"/data/projects/demo"}`,
		`{"type":"message_update","usage":{},"assistantMessageEvent":{"type":"thinking_start","contentIndex":0}}`,
		`{"type":"message_update","usage":{},"assistantMessageEvent":{"type":"thinking_delta","contentIndex":0,"delta":"让我","partial":{}}}`,
		`{"type":"message_update","usage":{},"assistantMessageEvent":{"type":"thinking_delta","contentIndex":0,"delta":"想想","partial":{}}}`,
		`{"type":"message_update","usage":{},"assistantMessageEvent":{"type":"thinking_end","contentIndex":0,"content":"让我想想","partial":{}}}`,
		`{"type":"message_end","message":{"role":"assistant","content":[{"type":"thinking","thinking":"让我想想"},{"type":"text","text":"好"}],"stopReason":"stop"}}`,
	}, "\n") + "\n"

	got := collect(t, input)
	if len(got) != 5 {
		t.Fatalf("got %d events, want 5 (lifecycle, two deltas, terminator, message): %+v", len(got), got)
	}
	for i, want := range map[int]string{1: "让我", 2: "想想"} {
		event := got[i]
		if event.Type != agents.EventThinking || event.Thinking == nil {
			t.Fatalf("event[%d] = %+v, want a thinking event", i, event)
		}
		if event.Thinking.Text != want || event.Thinking.Final {
			t.Errorf("event[%d] thinking = %+v, want a non-final chunk %q", i, event.Thinking, want)
		}
		if event.SessionID != "sess-1" {
			t.Errorf("event[%d].SessionID = %q, want sess-1", i, event.SessionID)
		}
		if err := event.Validate(); err != nil {
			t.Errorf("event[%d] does not validate: %v", i, err)
		}
	}

	terminator := got[3]
	if terminator.Type != agents.EventThinking || terminator.Thinking == nil ||
		terminator.Thinking.Text != "" || !terminator.Thinking.Final {
		t.Errorf("event[3] = %+v, want an empty final thinking terminator (deltas already carried the text)", terminator)
	}

	// The assistant message must still close normally alongside the reasoning.
	if got[4].Type != agents.EventMessage || got[4].Message == nil || got[4].Message.Text != "好" {
		t.Errorf("event[4] = %+v, want the assistant message", got[4])
	}
}

// TestParserClosesDanglingThinking pins robustness: a turn that ends without a
// thinking_end must not leave a streaming block open in the workbench.
func TestParserClosesDanglingThinking(t *testing.T) {
	input := strings.Join([]string{
		`{"type":"session","version":3,"id":"sess-1","cwd":"/data/projects/demo"}`,
		`{"type":"message_update","usage":{},"assistantMessageEvent":{"type":"thinking_delta","contentIndex":0,"delta":"半截"}}`,
		`{"type":"message_end","message":{"role":"assistant","content":[{"type":"text","text":"好"}],"stopReason":"stop"}}`,
	}, "\n") + "\n"

	got := collect(t, input)
	if len(got) != 4 {
		t.Fatalf("got %d events, want 4 (lifecycle, delta, thinking terminator, message): %+v", len(got), got)
	}
	if got[2].Type != agents.EventThinking || got[2].Thinking == nil || !got[2].Thinking.Final {
		t.Errorf("event[2] = %+v, want a final thinking terminator before the message", got[2])
	}
	if got[3].Type != agents.EventMessage {
		t.Errorf("event[3] = %+v, want the assistant message", got[3])
	}
}

// TestParserKeepsLargeRecord pins the raised stdout bound: a single JSONL
// record larger than the old 64KiB bound but within the stream bound (a full
// turn_end/agent_end or tool result easily exceeds 64KiB in real sessions)
// is parsed, not discarded as line_too_large.
func TestParserKeepsLargeRecord(t *testing.T) {
	delta := strings.Repeat("x", 200000)
	input := `{"type":"message_update","usage":{},"assistantMessageEvent":{"type":"text_delta","contentIndex":0,"delta":"` + delta + `"}}` + "\n"

	got := collect(t, input)
	if len(got) != 1 {
		t.Fatalf("got %d events, want 1 (types: %v)", len(got), eventTypes(got))
	}
	if got[0].Type != agents.EventMessage || got[0].Message == nil || got[0].Message.Text != delta {
		t.Errorf("event[0] = type %v, want the 200KB text delta carried as a message", got[0].Type)
	}
}

// eventTypes names each event's type for compact failure messages.
func eventTypes(events []agents.NormalizedEvent) []string {
	out := make([]string, 0, len(events))
	for _, event := range events {
		out = append(out, string(event.Type))
	}
	return out
}

// TestParserStderrSuppressesBenignWarning pins that pi's expected first-prompt
// notice for a platform-created session id ("No project session found with
// id …; creating a new session with that id.") never reaches the workbench as
// an error, while every other stderr line still does.
func TestParserStderrSuppressesBenignWarning(t *testing.T) {
	p := newParser("inst-1", "sess-1", parserClock)
	var got []agents.NormalizedEvent
	stream := "Warning: No project session found with id '9e21336f-ed58-4631-a3ab-db3f84c7baa6'; creating a new session with that id.\n" +
		"Warning: something else is wrong\n"
	if err := p.runStderr(strings.NewReader(stream), func(ev agents.NormalizedEvent) {
		got = append(got, ev)
	}); err != nil {
		t.Fatalf("runStderr() error = %v, want nil", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d events, want 1: %+v", len(got), eventTypes(got))
	}
	if got[0].Error == nil || !strings.Contains(got[0].Error.Message, "something else") {
		t.Errorf("event = %+v, want the non-benign warning carried through", got[0])
	}
}
