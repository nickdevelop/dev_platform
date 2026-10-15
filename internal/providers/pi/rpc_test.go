package pi

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"devplatform/internal/core/agents"
)

// rpcExecutable returns the test binary, which doubles as the scripted pi RPC
// stub when re-executed with PI_RPC_STUB=1.
func rpcExecutable(t *testing.T) string {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve test executable: %v", err)
	}
	return exe
}

// newRPCAdapter builds an RPC-mode adapter whose child is the RPC stub. The
// idle reaper is disabled by default so tests can inspect a live child; pass a
// mutate function to override options.
func newRPCAdapter(t *testing.T, sessionDir, scenario string, mutate ...func(*Options)) *Adapter {
	t.Helper()
	options := Options{
		Executable:     rpcExecutable(t),
		SessionDir:     sessionDir,
		Mode:           ModeRPC,
		Env:            append(os.Environ(), "PI_RPC_STUB=1", "PI_RPC_STUB_SCENARIO="+scenario),
		RPCIdleTimeout: -1,
	}
	for _, apply := range mutate {
		apply(&options)
	}
	return New(options)
}

// rpcInstance starts an instance on its own temp directory.
func rpcInstance(t *testing.T, adapter *Adapter) (*agents.Instance, string) {
	t.Helper()
	dir := t.TempDir()
	inst, err := adapter.StartInstance(context.Background(), agents.StartInstanceRequest{Directory: dir})
	if err != nil {
		t.Fatalf("StartInstance() error = %v", err)
	}
	t.Cleanup(func() { _ = adapter.StopInstance(context.Background(), inst.ID) })
	return inst, dir
}

// TestRPCSendMessageStreamsEvents is the happy path: the adapter starts one
// `pi --mode rpc` child, binds it to the platform session id, sends the prompt
// over stdin and normalizes the event stream onto the subscription.
func TestRPCSendMessageStreamsEvents(t *testing.T) {
	sessionDir := t.TempDir()
	argsLog := filepath.Join(t.TempDir(), "args.log")
	adapter := newRPCAdapter(t, sessionDir, "", func(o *Options) {
		o.Env = append(o.Env, "PI_RPC_STUB_ARGS="+argsLog)
	})
	ctx := context.Background()
	inst, dir := rpcInstance(t, adapter)
	ref, err := adapter.CreateSession(ctx, inst.ID, agents.CreateSessionOptions{})
	if err != nil {
		t.Fatal(err)
	}

	subCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	events, err := adapter.Subscribe(subCtx, inst.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.SendMessage(ctx, *ref, agents.SendMessageRequest{Text: "hello"}); err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}

	got := drainUntil(t, events, func(ev agents.NormalizedEvent) bool {
		return ev.Type == agents.EventLifecycle && ev.Lifecycle != nil && ev.Lifecycle.State == "agent_settled"
	})
	var sawDelta, sawToolCall, sawToolResult bool
	for i, ev := range got {
		if ev.InstanceID != inst.ID || ev.Backend != agents.BackendPi || ev.SessionID != ref.SessionID {
			t.Errorf("event[%d] envelope = %+v, want instance/session/pi", i, ev)
		}
		if i > 0 && ev.Sequence <= got[i-1].Sequence {
			t.Errorf("event[%d] sequence did not increase", i)
		}
		if ev.Type == agents.EventMessage && ev.Message != nil && !ev.Message.Final && ev.Message.Text == "Hi " {
			sawDelta = true
		}
		if ev.Type == agents.EventToolCall && ev.ToolCall != nil && ev.ToolCall.CallID == "call-1" {
			sawToolCall = true
		}
		if ev.Type == agents.EventToolResult && ev.ToolResult != nil && ev.ToolResult.CallID == "call-1" {
			sawToolResult = true
		}
	}
	if !sawDelta || !sawToolCall || !sawToolResult {
		t.Errorf("missed events: delta=%v toolCall=%v toolResult=%v", sawDelta, sawToolCall, sawToolResult)
	}

	rawArgs, err := os.ReadFile(argsLog)
	if err != nil {
		t.Fatalf("read args log: %v", err)
	}
	args := strings.Fields(strings.TrimSpace(string(rawArgs)))
	if !containsSequence(args, "--mode", "rpc") {
		t.Errorf("argv = %q, want --mode rpc", args)
	}
	if !containsSequence(args, "--session-id", ref.SessionID) {
		t.Errorf("argv = %q, want --session-id %s", args, ref.SessionID)
	}
	if !containsSequence(args, "--session-dir", sessionDir) {
		t.Errorf("argv = %q, want --session-dir %s", args, sessionDir)
	}
	if dir == "" {
		t.Fatal("instance directory was empty")
	}
}

// TestRPCCapabilitiesToolConfirmation pins that only RPC mode advertises tool
// confirmation.
func TestRPCCapabilitiesToolConfirmation(t *testing.T) {
	rpc := newRPCAdapter(t, "", "")
	if !rpc.Capabilities().ToolConfirmation {
		t.Error("RPC Capabilities().ToolConfirmation = false, want true")
	}
	cli := New(Options{})
	if cli.Capabilities().ToolConfirmation {
		t.Error("CLI Capabilities().ToolConfirmation = true, want false")
	}
}

// TestRPCInterruptUsesAbort proves Interrupt sends abort and leaves the child
// and instance running: a later prompt still works and settles.
func TestRPCInterruptUsesAbort(t *testing.T) {
	adapter := newRPCAdapter(t, t.TempDir(), "sleep")
	ctx := context.Background()
	inst, _ := rpcInstance(t, adapter)
	ref, err := adapter.CreateSession(ctx, inst.ID, agents.CreateSessionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	events, err := adapter.Subscribe(ctx, inst.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.SendMessage(ctx, *ref, agents.SendMessageRequest{Text: "block"}); err != nil {
		t.Fatal(err)
	}
	// Wait for the run to start so abort has something to cancel.
	drainUntil(t, events, func(ev agents.NormalizedEvent) bool {
		return ev.Type == agents.EventLifecycle && ev.Lifecycle != nil && ev.Lifecycle.State == "agent_start"
	})

	if err := adapter.Interrupt(ctx, *ref); err != nil {
		t.Fatalf("Interrupt() error = %v", err)
	}
	drainUntil(t, events, func(ev agents.NormalizedEvent) bool {
		return ev.Type == agents.EventLifecycle && ev.Lifecycle != nil &&
			ev.Lifecycle.State == "agent_settled" && ev.Lifecycle.Detail == "aborted"
	})

	// The child and instance survive: a later prompt is accepted.
	instRef, err := adapter.getInstance(inst.ID)
	if err != nil {
		t.Fatal(err)
	}
	if child := instRef.child(ref.SessionID); child == nil || child.dead.Load() {
		t.Fatal("child was killed by interrupt; want it alive")
	}
	if err := adapter.SendMessage(ctx, *ref, agents.SendMessageRequest{Text: "again"}); err != nil {
		t.Errorf("SendMessage after interrupt = %v, want nil", err)
	}
}

// TestRPCStopIsOrderly checks StopInstance reclaims a live child without
// leaving it as a zombie.
func TestRPCStopIsOrderly(t *testing.T) {
	adapter := newRPCAdapter(t, t.TempDir(), "sleep")
	ctx := context.Background()
	inst, _ := rpcInstance(t, adapter)
	ref, err := adapter.CreateSession(ctx, inst.ID, agents.CreateSessionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.SendMessage(ctx, *ref, agents.SendMessageRequest{Text: "block"}); err != nil {
		t.Fatal(err)
	}
	instRef, err := adapter.getInstance(inst.ID)
	if err != nil {
		t.Fatal(err)
	}
	child := instRef.child(ref.SessionID)
	if child == nil {
		t.Fatal("no live child after send")
	}
	pid := child.proc.pid()

	stopCtx, stopCancel := context.WithTimeout(ctx, 5*time.Second)
	defer stopCancel()
	if err := adapter.StopInstance(stopCtx, inst.ID); err != nil {
		t.Fatalf("StopInstance() error = %v", err)
	}
	waitReaped(t, pid)
}

// TestRPCIdleReaperReclaimsChild proves an idle child is reclaimed after the
// configured timeout and its process reaped.
func TestRPCIdleReaperReclaimsChild(t *testing.T) {
	adapter := newRPCAdapter(t, t.TempDir(), "", func(o *Options) {
		o.RPCIdleTimeout = 150 * time.Millisecond
		o.RPCReapInterval = 20 * time.Millisecond
	})
	ctx := context.Background()
	inst, _ := rpcInstance(t, adapter)
	ref, err := adapter.CreateSession(ctx, inst.ID, agents.CreateSessionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	events, err := adapter.Subscribe(ctx, inst.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.SendMessage(ctx, *ref, agents.SendMessageRequest{Text: "hello"}); err != nil {
		t.Fatal(err)
	}
	drainUntil(t, events, func(ev agents.NormalizedEvent) bool {
		return ev.Type == agents.EventLifecycle && ev.Lifecycle != nil && ev.Lifecycle.State == "agent_settled"
	})
	instRef, err := adapter.getInstance(inst.ID)
	if err != nil {
		t.Fatal(err)
	}
	child := instRef.child(ref.SessionID)
	if child == nil {
		t.Fatal("no live child after send")
	}
	pid := child.proc.pid()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if instRef.child(ref.SessionID) == nil {
			waitReaped(t, pid)
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("idle child was never reclaimed")
}

// TestRPCCrashRestartsSameSession proves a crashed child is replaced on the
// next send with the same platform session id, so the transcript continues.
func TestRPCCrashRestartsSameSession(t *testing.T) {
	argsLog := filepath.Join(t.TempDir(), "args.log")
	adapter := newRPCAdapter(t, t.TempDir(), "crash", func(o *Options) {
		o.Env = append(o.Env, "PI_RPC_STUB_ARGS="+argsLog)
	})
	ctx := context.Background()
	inst, _ := rpcInstance(t, adapter)
	ref, err := adapter.CreateSession(ctx, inst.ID, agents.CreateSessionOptions{})
	if err != nil {
		t.Fatal(err)
	}

	if err := adapter.SendMessage(ctx, *ref, agents.SendMessageRequest{Text: "boom"}); err == nil {
		t.Fatal("SendMessage against a crashing child = nil, want error")
	}
	if err := adapter.SendMessage(ctx, *ref, agents.SendMessageRequest{Text: "boom again"}); err == nil {
		t.Fatal("second SendMessage against a crashing child = nil, want error")
	}

	rawArgs, err := os.ReadFile(argsLog)
	if err != nil {
		t.Fatalf("read args log: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(rawArgs)), "\n")
	if len(lines) != 2 {
		t.Fatalf("launches = %d (%q), want 2 (crash then restart)", len(lines), lines)
	}
	for i, line := range lines {
		if !strings.Contains(line, "--session-id "+ref.SessionID) {
			t.Errorf("launch %d argv = %q, want --session-id %s", i, line, ref.SessionID)
		}
	}
}

// TestRPCBindSessionMismatch rejects a child that is not running the requested
// session.
func TestRPCBindSessionMismatch(t *testing.T) {
	adapter := newRPCAdapter(t, t.TempDir(), "getstate-mismatch")
	ctx := context.Background()
	inst, _ := rpcInstance(t, adapter)
	ref, err := adapter.CreateSession(ctx, inst.ID, agents.CreateSessionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.SendMessage(ctx, *ref, agents.SendMessageRequest{Text: "hello"}); err == nil {
		t.Fatal("SendMessage with a mismatched binding = nil, want error")
	}
}

// TestRPCPermissions exercises the confirm-dialog mapping: a pending
// confirmation is listed, a reply resolves it, and a stale reply is expired.
func TestRPCPermissions(t *testing.T) {
	adapter := newRPCAdapter(t, t.TempDir(), "confirm")
	ctx := context.Background()
	inst, _ := rpcInstance(t, adapter)
	ref, err := adapter.CreateSession(ctx, inst.ID, agents.CreateSessionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	events, err := adapter.Subscribe(ctx, inst.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.SendMessage(ctx, *ref, agents.SendMessageRequest{Text: "delete"}); err != nil {
		t.Fatal(err)
	}

	var pending []agents.PermissionRequest
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		pending, err = adapter.ListPermissions(ctx, *ref)
		if err != nil {
			t.Fatalf("ListPermissions() error = %v", err)
		}
		if len(pending) > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(pending) != 1 {
		t.Fatalf("pending permissions = %+v, want 1", pending)
	}
	if pending[0].Action != "Allow tool call?" || pending[0].SessionID != ref.SessionID {
		t.Errorf("permission = %+v, want action and session id", pending[0])
	}
	requestID := pending[0].ID

	if err := adapter.ReplyPermission(ctx, *ref, requestID, "once", ""); err != nil {
		t.Fatalf("ReplyPermission() error = %v", err)
	}
	drainUntil(t, events, func(ev agents.NormalizedEvent) bool {
		return ev.Type == agents.EventLifecycle && ev.Lifecycle != nil && ev.Lifecycle.State == "agent_settled"
	})
	if err := adapter.ReplyPermission(ctx, *ref, requestID, "once", ""); !errors.Is(err, agents.ErrReplyExpired) {
		t.Errorf("second ReplyPermission() = %v, want ErrReplyExpired", err)
	}
}

// TestRPCAlwaysApprovalIsRemembered checks "always" records a per-session rule
// so an identical later confirmation is auto-approved without a pending card.
func TestRPCAlwaysApprovalIsRemembered(t *testing.T) {
	adapter := newRPCAdapter(t, t.TempDir(), "confirm")
	ctx := context.Background()
	inst, _ := rpcInstance(t, adapter)
	ref, err := adapter.CreateSession(ctx, inst.ID, agents.CreateSessionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	events, err := adapter.Subscribe(ctx, inst.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.SendMessage(ctx, *ref, agents.SendMessageRequest{Text: "delete"}); err != nil {
		t.Fatal(err)
	}

	var requestID string
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && requestID == "" {
		pending, err := adapter.ListPermissions(ctx, *ref)
		if err != nil {
			t.Fatal(err)
		}
		if len(pending) > 0 {
			requestID = pending[0].ID
		} else {
			time.Sleep(10 * time.Millisecond)
		}
	}
	if requestID == "" {
		t.Fatal("confirmation never became pending")
	}
	if err := adapter.ReplyPermission(ctx, *ref, requestID, "always", ""); err != nil {
		t.Fatalf("ReplyPermission(always) error = %v", err)
	}
	drainUntil(t, events, func(ev agents.NormalizedEvent) bool {
		return ev.Type == agents.EventLifecycle && ev.Lifecycle != nil && ev.Lifecycle.State == "agent_settled"
	})

	if !adapter.sessionApproved(inst.ID, ref.SessionID, confirmKey("Allow tool call?", "bash: rm -rf /tmp/x")) {
		t.Fatal("always approval was not remembered")
	}
	// The second identical confirmation is auto-approved.
	if err := adapter.SendMessage(ctx, *ref, agents.SendMessageRequest{Text: "delete again"}); err != nil {
		t.Fatal(err)
	}
	drainUntil(t, events, func(ev agents.NormalizedEvent) bool {
		return ev.Type == agents.EventLifecycle && ev.Lifecycle != nil && ev.Lifecycle.State == "agent_settled"
	})
	pending, err := adapter.ListPermissions(ctx, *ref)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Errorf("pending permissions after always = %+v, want none", pending)
	}
}

// TestRPCFollowUpQueuesSecondPrompt proves a prompt sent while the session is
// streaming is queued rather than rejected.
func TestRPCFollowUpQueuesSecondPrompt(t *testing.T) {
	adapter := newRPCAdapter(t, t.TempDir(), "")
	ctx := context.Background()
	inst, _ := rpcInstance(t, adapter)
	ref, err := adapter.CreateSession(ctx, inst.ID, agents.CreateSessionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	events, err := adapter.Subscribe(ctx, inst.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.SendMessage(ctx, *ref, agents.SendMessageRequest{Text: "first"}); err != nil {
		t.Fatal(err)
	}
	// The first prompt response has set streaming; a second prompt must queue.
	if err := adapter.SendMessage(ctx, *ref, agents.SendMessageRequest{Text: "second"}); err != nil {
		t.Fatalf("queued SendMessage() error = %v", err)
	}
	settled := 0
	deadline := time.After(5 * time.Second)
	for settled < 2 {
		select {
		case ev, ok := <-events:
			if !ok {
				t.Fatal("subscription closed before both turns settled")
			}
			if ev.Type == agents.EventLifecycle && ev.Lifecycle != nil && ev.Lifecycle.State == "agent_settled" {
				settled++
			}
		case <-deadline:
			t.Fatalf("only %d turns settled, want 2", settled)
		}
	}
}

// TestRPCListModelsUsesLiveChild checks ListModels asks a running child with
// get_available_models instead of spawning a `--list-models` process.
func TestRPCListModelsUsesLiveChild(t *testing.T) {
	adapter := newRPCAdapter(t, t.TempDir(), "")
	ctx := context.Background()
	inst, _ := rpcInstance(t, adapter)
	ref, err := adapter.CreateSession(ctx, inst.ID, agents.CreateSessionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	events, err := adapter.Subscribe(ctx, inst.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.SendMessage(ctx, *ref, agents.SendMessageRequest{Text: "hello"}); err != nil {
		t.Fatal(err)
	}
	drainUntil(t, events, func(ev agents.NormalizedEvent) bool {
		return ev.Type == agents.EventLifecycle && ev.Lifecycle != nil && ev.Lifecycle.State == "agent_settled"
	})

	// The child is live, so it answers even though the directory means nothing
	// to pi's catalog.
	models, err := adapter.ListModels(ctx, inst.Directory)
	if err != nil {
		t.Fatalf("ListModels() error = %v", err)
	}
	if len(models) != 1 || models[0].ID != "stub/gpt-1" {
		t.Errorf("ListModels() = %+v, want stub/gpt-1 from get_available_models", models)
	}
}

// TestRPCConnMalformedRecordsContinue verifies one bad record becomes a
// parse_error event and the reader keeps going, matching the JSON-mode
// resilience contract.
func TestRPCConnMalformedRecordsContinue(t *testing.T) {
	parser := newParser("inst", "sess", nil)
	var events []agents.NormalizedEvent
	conn := newRPCConn(discardWriter{}, parser, func(event agents.NormalizedEvent) {
		events = append(events, event)
	})
	input := "not json\n" +
		"{\"type\":\"agent_settled\",\"aborted\":false}\n"
	if err := conn.readLoop(strings.NewReader(input)); err != nil {
		t.Fatalf("readLoop() error = %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("events = %d (%+v), want a parse_error then agent_settled", len(events), events)
	}
	if events[0].Type != agents.EventError || events[0].Error == nil || events[0].Error.Code != "parse_error" {
		t.Errorf("first event = %+v, want parse_error", events[0])
	}
	if len(events[0].Raw) == 0 {
		t.Error("parse_error did not preserve the offending bytes")
	}
	if events[1].Type != agents.EventLifecycle || events[1].Lifecycle.State != "agent_settled" {
		t.Errorf("second event = %+v, want agent_settled after the bad record", events[1])
	}
}

// TestRPCConnSplitsOnlyOnLF verifies the reader does not treat U+2028 as a
// record boundary, as the protocol requires.
func TestRPCConnSplitsOnlyOnLF(t *testing.T) {
	parser := newParser("inst", "sess", nil)
	var got []agents.NormalizedEvent
	conn := newRPCConn(discardWriter{}, parser, func(ev agents.NormalizedEvent) { got = append(got, ev) })

	// A user message whose text contains a Unicode line separator inside the
	// JSON string, followed by an agent_settled record.
	input := "{\"type\":\"message_start\",\"message\":{\"role\":\"user\",\"content\":\"a\u2028b\"}}\n" +
		"{\"type\":\"agent_settled\",\"aborted\":false}\n"
	if err := conn.readLoop(strings.NewReader(input)); err != nil {
		t.Fatalf("readLoop() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("events = %d (%+v), want 2 (U+2028 must stay inside the record)", len(got), got)
	}
	if got[0].Type != agents.EventMessage || got[0].Message == nil || got[0].Message.Text != "a\u2028b" {
		t.Errorf("first event = %+v, want the message with U+2028 intact", got[0])
	}
	if got[1].Type != agents.EventLifecycle || got[1].Lifecycle.State != "agent_settled" {
		t.Errorf("second event = %+v, want agent_settled", got[1])
	}
}

// TestRPCCommandResponseCorrelation checks concurrent commands are matched by
// id, not by order.
func TestRPCCommandResponseCorrelation(t *testing.T) {
	parser := newParser("inst", "sess", nil)
	connReader, responseWriter := io.Pipe()
	commandReader, connWriter := io.Pipe()
	conn := newRPCConn(connWriter, parser, func(agents.NormalizedEvent) {})
	commands := bufio.NewReader(commandReader)
	done := make(chan error, 1)
	go func() { done <- conn.readLoop(connReader) }()

	type result struct {
		command string
		err     error
	}
	results := make(chan result, 2)
	go func() {
		response, err := conn.request(context.Background(), newGetStateCommand(""), time.Second)
		results <- result{command: responseCommand(response), err: err}
	}()
	first := readStubLine(t, commands)
	go func() {
		response, err := conn.request(context.Background(), newAbortCommand(""), time.Second)
		results <- result{command: responseCommand(response), err: err}
	}()
	second := readStubLine(t, commands)

	var firstCmd, secondCmd map[string]any
	if err := json.Unmarshal(first, &firstCmd); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(second, &secondCmd); err != nil {
		t.Fatal(err)
	}
	// Answer in reverse order.
	writeStubResponse(t, responseWriter, secondCmd["id"].(string), "abort")
	writeStubResponse(t, responseWriter, firstCmd["id"].(string), "get_state")

	seen := map[string]bool{}
	for i := 0; i < 2; i++ {
		result := <-results
		if result.err != nil {
			t.Fatalf("request error = %v", result.err)
		}
		seen[result.command] = true
	}
	if !seen["abort"] || !seen["get_state"] {
		t.Errorf("correlated commands = %v, want abort and get_state", seen)
	}
	conn.close()
	_ = responseWriter.Close()
	<-done
}

// readStubLine reads one LF-terminated command from a reader.
func readStubLine(t *testing.T, reader *bufio.Reader) []byte {
	t.Helper()
	line, err := reader.ReadBytes('\n')
	if err != nil {
		t.Fatalf("read command: %v", err)
	}
	return line
}

// writeStubResponse writes one response record with the given id.
func writeStubResponse(t *testing.T, writer io.Writer, id, command string) {
	t.Helper()
	data, err := json.Marshal(map[string]any{"id": id, "type": "response", "command": command, "success": true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write(append(data, '\n')); err != nil {
		t.Fatalf("write response: %v", err)
	}
}

func responseCommand(response *rpcResponse) string {
	if response == nil {
		return ""
	}
	return response.Command
}

// discardWriter drops written bytes.
type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }

// TestRPCArgsIncludeExtensions checks extension paths (the bundled approval
// gate) are passed to every RPC child.
func TestRPCArgsIncludeExtensions(t *testing.T) {
	args := rpcArgs(rpcOptions{sessionID: "sess", extensions: []string{"/tmp/a.ts", ""}})
	if !containsSequence(args, "--extension", "/tmp/a.ts") {
		t.Errorf("rpcArgs() = %q, want --extension /tmp/a.ts", args)
	}
	if strings.Count(strings.Join(args, " "), "--extension") != 1 {
		t.Errorf("rpcArgs() = %q, want exactly one --extension (empty entries dropped)", args)
	}
	if !containsSequence(args, "--mode", "rpc") || !containsSequence(args, "--session-id", "sess") {
		t.Errorf("rpcArgs() = %q, want --mode rpc --session-id sess", args)
	}
}

// TestEnsureApprovalExtensionIdempotent checks the bundled extension is written
// once and returned on later calls without rewriting.
func TestEnsureApprovalExtensionIdempotent(t *testing.T) {
	dir := t.TempDir()
	path, err := EnsureApprovalExtension(dir)
	if err != nil {
		t.Fatalf("EnsureApprovalExtension() error = %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat extension: %v", err)
	}
	firstMod := info.ModTime()

	again, err := EnsureApprovalExtension(dir)
	if err != nil {
		t.Fatalf("second EnsureApprovalExtension() error = %v", err)
	}
	if again != path {
		t.Errorf("path = %q, want %q", again, path)
	}
	info, err = os.Stat(again)
	if err != nil {
		t.Fatalf("stat extension: %v", err)
	}
	if !info.ModTime().Equal(firstMod) {
		t.Error("extension was rewritten on the second call; want idempotent")
	}
	if _, err := EnsureApprovalExtension(""); err == nil {
		t.Error("EnsureApprovalExtension(empty) = nil, want error")
	}
}

// TestRPCFixtureHandshake replays a sanitized capture of a real
// `pi --mode rpc` handshake and happy-path turn (recorded from pi 1.1.0) to
// guard against protocol drift. Extension UI fire-and-forget records and
// command responses must not leak into the event stream.
func TestRPCFixtureHandshake(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "rpc", "session.jsonl"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	parser := newParser("inst", "fixture", nil)
	var events []agents.NormalizedEvent
	conn := newRPCConn(discardWriter{}, parser, func(event agents.NormalizedEvent) {
		events = append(events, event)
	})
	if err := conn.readLoop(strings.NewReader(string(data))); err != nil {
		t.Fatalf("readLoop() error = %v", err)
	}

	var sawUser, sawDelta, sawFinal, sawToolCall, sawToolResult, sawSettled bool
	for _, event := range events {
		if event.Type == agents.EventUnknown {
			t.Errorf("fixture produced an unknown event: %s", event.Raw)
		}
		if event.Type == agents.EventMessage && event.Message != nil {
			switch {
			case event.Message.Role == "user" && event.Message.Text == "ping":
				sawUser = true
			case !event.Message.Final && event.Message.Text == "pong":
				sawDelta = true
			case event.Message.Final && event.Message.Role == "assistant":
				sawFinal = true
			}
		}
		if event.Type == agents.EventToolCall && event.ToolCall != nil {
			sawToolCall = true
		}
		if event.Type == agents.EventToolResult && event.ToolResult != nil {
			sawToolResult = true
		}
		if event.Type == agents.EventLifecycle && event.Lifecycle != nil && event.Lifecycle.State == "agent_settled" {
			sawSettled = true
		}
	}
	if !sawUser || !sawDelta || !sawFinal || !sawToolCall || !sawToolResult || !sawSettled {
		t.Errorf("fixture events: user=%v delta=%v final=%v toolCall=%v toolResult=%v settled=%v",
			sawUser, sawDelta, sawFinal, sawToolCall, sawToolResult, sawSettled)
	}
	for _, event := range events {
		if event.Type == agents.EventUnknown && strings.Contains(string(event.Raw), "extension_ui_request") {
			t.Errorf("extension UI record leaked into the event stream: %s", event.Raw)
		}
	}
}
