package pi

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"devplatform/internal/agents"
)

// streamingFakePi is a stand-in for the pi CLI: it records its arguments and
// working directory, then emits a representative JSON event stream. It makes
// no model calls.
const streamingFakePi = `
printf '%s\n' "$@" > "$FAKE_PI_ARGS"
pwd > "$FAKE_PI_CWD"
cat <<'EVENTS'
{"type":"session","version":3,"id":"sess-fake","cwd":"/ignored"}
{"type":"message_start","message":{"role":"user","content":"hello"}}
{"type":"message_update","usage":{},"assistantMessageEvent":{"type":"text_delta","contentIndex":0,"delta":"Hi "}}
{"type":"message_update","usage":{},"assistantMessageEvent":{"type":"text_delta","contentIndex":0,"delta":"there"}}
{"type":"message_end","message":{"role":"assistant","content":[{"type":"text","text":"Hi there"}]}}
{"type":"tool_execution_start","toolCallId":"call-1","toolName":"read","args":{"path":"main.go"}}
{"type":"tool_execution_end","toolCallId":"call-1","toolName":"read","result":{"content":[{"type":"text","text":"package main"}]},"isError":false}
{"type":"agent_settled","aborted":false}
EVENTS
`

// sleepingFakePi writes its PID and blocks until it is killed, so stop tests
// can prove process-group cleanup.
const sleepingFakePi = `
printf '%s\n' "$$" > "$FAKE_PI_PID"
exec sleep 30
`

// writeFakePi writes an executable shell script into dir and returns its path.
func writeFakePi(t *testing.T, dir, body string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake pi executable requires a unix shell")
	}
	path := filepath.Join(dir, "fake-pi")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatalf("write fake pi: %v", err)
	}
	return path
}

// waitForFile polls until path exists and is non-empty.
func waitForFile(t *testing.T, path string) []byte {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(path)
		if err == nil && len(data) > 0 {
			return data
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("file %s was never written", path)
	return nil
}

func newTestAdapter(t *testing.T, executable, sessionDir string) *Adapter {
	t.Helper()
	return New(Options{Executable: executable, SessionDir: sessionDir})
}

// TestAdapterProbe checks the binary is validated without starting anything.
func TestAdapterProbe(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "does-not-exist")
	if err := newTestAdapter(t, missing, "").Probe(context.Background()); err == nil {
		t.Fatal("Probe() with a missing executable = nil, want error")
	}

	fake := writeFakePi(t, dir, streamingFakePi)
	if err := newTestAdapter(t, fake, "").Probe(context.Background()); err != nil {
		t.Fatalf("Probe() = %v, want nil", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := newTestAdapter(t, fake, "").Probe(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("Probe(canceled) = %v, want context.Canceled", err)
	}
}

// TestAdapterStartInstanceValidations checks the directory is authorized and
// real, that a usable id and running status are returned, and that stopping an
// unknown instance is an idempotent success.
func TestAdapterStartInstanceValidations(t *testing.T) {
	dir := t.TempDir()
	fake := writeFakePi(t, dir, streamingFakePi)
	adapter := newTestAdapter(t, fake, "")
	ctx := context.Background()

	if _, err := adapter.StartInstance(ctx, agents.StartInstanceRequest{}); err == nil {
		t.Error("StartInstance with no directory = nil, want error")
	}

	file := filepath.Join(dir, "a-file")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.StartInstance(ctx, agents.StartInstanceRequest{Directory: file}); err == nil {
		t.Error("StartInstance with a file directory = nil, want error")
	}

	project := filepath.Join(dir, "project")
	if err := os.Mkdir(project, 0o755); err != nil {
		t.Fatal(err)
	}
	inst, err := adapter.StartInstance(ctx, agents.StartInstanceRequest{Directory: project})
	if err != nil {
		t.Fatalf("StartInstance() error = %v", err)
	}
	if inst.ID == "" || inst.Backend != agents.BackendPi || inst.Directory != project || inst.Status != agents.InstanceRunning {
		t.Errorf("StartInstance() = %+v, want running pi instance in %s", inst, project)
	}
	if err := inst.Validate(); err != nil {
		t.Errorf("returned instance does not satisfy the contract: %v", err)
	}

	// A second instance gets its own id.
	second, err := adapter.StartInstance(ctx, agents.StartInstanceRequest{Directory: project})
	if err != nil {
		t.Fatal(err)
	}
	if second.ID == inst.ID {
		t.Errorf("two instances share the id %q", inst.ID)
	}

	if err := adapter.StopInstance(ctx, inst.ID); err != nil {
		t.Errorf("StopInstance() = %v, want nil", err)
	}
	if err := adapter.StopInstance(ctx, inst.ID); err != nil {
		t.Errorf("StopInstance() on a gone instance = %v, want nil", err)
	}
	if err := adapter.StopInstance(ctx, "never-existed"); err != nil {
		t.Errorf("StopInstance(unknown) = %v, want nil", err)
	}
}

// TestAdapterSafeArgsAndWorkingDirectory proves the process is launched with
// validated argv (never a shell string) in the instance's directory.
func TestAdapterSafeArgsAndWorkingDirectory(t *testing.T) {
	dir := t.TempDir()
	fake := writeFakePi(t, dir, streamingFakePi)
	argsFile := filepath.Join(dir, "args.txt")
	cwdFile := filepath.Join(dir, "cwd.txt")
	t.Setenv("FAKE_PI_ARGS", argsFile)
	t.Setenv("FAKE_PI_CWD", cwdFile)

	adapter := newTestAdapter(t, fake, filepath.Join(dir, "sessions"))
	ctx := context.Background()
	inst, err := adapter.StartInstance(ctx, agents.StartInstanceRequest{Directory: dir})
	if err != nil {
		t.Fatal(err)
	}
	ref, err := adapter.CreateSession(ctx, inst.ID, agents.CreateSessionOptions{Title: "Demo Session", Model: "stub/model"})
	if err != nil {
		t.Fatal(err)
	}

	// The message begins with a dash: after -- it must stay one literal argv
	// element rather than being parsed as another option.
	message := "- literally a dash"
	if err := adapter.SendMessage(ctx, *ref, agents.SendMessageRequest{Text: message}); err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}

	rawArgs := string(waitForFile(t, argsFile))
	args := strings.Split(strings.TrimRight(rawArgs, "\n"), "\n")
	if len(args) < 2 || args[0] != "--mode" || args[1] != "json" {
		t.Fatalf("argv = %q, want --mode json first", args)
	}
	if !containsSequence(args, "--session-id", ref.SessionID) {
		t.Errorf("argv = %q, want --session-id %s", args, ref.SessionID)
	}
	if !containsSequence(args, "--model", "stub/model") {
		t.Errorf("argv = %q, want --model stub/model", args)
	}
	if !containsSequence(args, "--name", "Demo Session") {
		t.Errorf("argv = %q, want --name Demo Session", args)
	}
	if !containsSequence(args, "--session-dir", filepath.Join(dir, "sessions")) {
		t.Errorf("argv = %q, want --session-dir to match the configured scan root", args)
	}
	if len(args) < 2 || args[len(args)-2] != "--" || args[len(args)-1] != message {
		t.Errorf("argv = %q, want it to end with -- %q", args, message)
	}

	gotCWD, err := filepath.EvalSymlinks(strings.TrimSpace(string(waitForFile(t, cwdFile))))
	if err != nil {
		t.Fatalf("eval cwd: %v", err)
	}
	wantCWD, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatalf("eval dir: %v", err)
	}
	if gotCWD != wantCWD {
		t.Errorf("process working directory = %q, want %q", gotCWD, wantCWD)
	}
}

// TestAdapterStreamsNormalizedEvents checks that subscription events arrive in
// order with strictly increasing per-instance sequence numbers and that the
// stream is scoped to its instance.
func TestAdapterStreamsNormalizedEvents(t *testing.T) {
	dir := t.TempDir()
	fake := writeFakePi(t, dir, streamingFakePi)
	adapter := newTestAdapter(t, fake, filepath.Join(dir, "sessions"))
	ctx := context.Background()
	inst, err := adapter.StartInstance(ctx, agents.StartInstanceRequest{Directory: dir})
	if err != nil {
		t.Fatal(err)
	}
	ref, err := adapter.CreateSession(ctx, inst.ID, agents.CreateSessionOptions{})
	if err != nil {
		t.Fatal(err)
	}

	subCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	events, err := adapter.Subscribe(subCtx, inst.ID)
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	if err := adapter.SendMessage(ctx, *ref, agents.SendMessageRequest{Text: "hello"}); err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}

	got := drainUntil(t, events, func(ev agents.NormalizedEvent) bool {
		return ev.Type == agents.EventLifecycle && ev.Lifecycle != nil && ev.Lifecycle.State == "agent_settled"
	})

	var sawDelta, sawToolCall, sawToolResult bool
	for i, ev := range got {
		if ev.InstanceID != inst.ID || ev.Backend != agents.BackendPi {
			t.Errorf("event[%d] envelope = %+v, want instance %s/pi", i, ev, inst.ID)
		}
		if i > 0 && ev.Sequence <= got[i-1].Sequence {
			t.Errorf("event[%d] sequence %d did not increase after %d", i, ev.Sequence, got[i-1].Sequence)
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
		t.Errorf("missed events: delta=%v toolCall=%v toolResult=%v in %+v", sawDelta, sawToolCall, sawToolResult, got)
	}
}

// TestAdapterStopKillsProcessAndClosesStream proves stop terminates the
// process group and closes the subscription without affecting other instances.
func TestAdapterStopKillsProcessAndClosesStream(t *testing.T) {
	dir := t.TempDir()
	fake := writeFakePi(t, dir, sleepingFakePi)
	pidFile := filepath.Join(dir, "pid.txt")
	t.Setenv("FAKE_PI_PID", pidFile)

	adapter := newTestAdapter(t, fake, "")
	ctx := context.Background()
	inst, err := adapter.StartInstance(ctx, agents.StartInstanceRequest{Directory: dir})
	if err != nil {
		t.Fatal(err)
	}
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
	pid, err := strconv.Atoi(strings.TrimSpace(string(waitForFile(t, pidFile))))
	if err != nil {
		t.Fatalf("parse pid: %v", err)
	}

	stopCtx, stopCancel := context.WithTimeout(ctx, 5*time.Second)
	defer stopCancel()
	if err := adapter.StopInstance(stopCtx, inst.ID); err != nil {
		t.Fatalf("StopInstance() error = %v", err)
	}

	select {
	case _, ok := <-events:
		if ok {
			// Any buffered events may still be delivered before the channel
			// closes; keep draining until close.
			if !drainClosed(t, events) {
				t.Fatal("subscription channel did not close after stop")
			}
		}
	case <-time.After(5 * time.Second):
		t.Fatal("subscription channel did not close after stop")
	}

	if runtime.GOOS != "windows" {
		if err := syscall.Kill(pid, 0); err == nil {
			t.Errorf("process %d still alive after stop", pid)
		}
	}

	// Sending to a stopped instance must fail, and stopping again is a no-op.
	if err := adapter.SendMessage(ctx, *ref, agents.SendMessageRequest{Text: "again"}); err == nil {
		t.Error("SendMessage to a stopped instance = nil, want error")
	}
	if err := adapter.StopInstance(ctx, inst.ID); err != nil {
		t.Errorf("second StopInstance() = %v, want nil", err)
	}
}

// TestAdapterCreateAndContinueSession checks adapter-assigned session ids and
// that continue returns a usable reference.
func TestAdapterCreateAndContinueSession(t *testing.T) {
	dir := t.TempDir()
	fake := writeFakePi(t, dir, streamingFakePi)
	adapter := newTestAdapter(t, fake, "")
	ctx := context.Background()
	inst, err := adapter.StartInstance(ctx, agents.StartInstanceRequest{Directory: dir})
	if err != nil {
		t.Fatal(err)
	}

	ref, err := adapter.CreateSession(ctx, inst.ID, agents.CreateSessionOptions{Title: "Refactor"})
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if ref.SessionID == "" || ref.InstanceID != inst.ID || ref.Backend != agents.BackendPi || ref.Title != "Refactor" {
		t.Fatalf("CreateSession() = %+v, want a pi ref with id, instance and title", ref)
	}
	if err := ref.Validate(); err != nil {
		t.Errorf("session ref does not satisfy the contract: %v", err)
	}

	continued, err := adapter.ContinueSession(ctx, *ref)
	if err != nil {
		t.Fatalf("ContinueSession() error = %v", err)
	}
	if continued.SessionID != ref.SessionID || continued.InstanceID != inst.ID || continued.Backend != agents.BackendPi {
		t.Errorf("ContinueSession() = %+v, want the same reference refreshed", continued)
	}
}

// TestAdapterListSessions discovers pi's on-disk JSONL sessions for the
// instance directory, reads their headers and titles, and excludes sessions
// that belong to another working directory.
func TestAdapterListSessions(t *testing.T) {
	dir := t.TempDir()
	sessionRoot := t.TempDir()
	fake := writeFakePi(t, dir, streamingFakePi)

	folder := filepath.Join(sessionRoot, sessionDirName(dir))
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	sessionFile := filepath.Join(folder, "2026-10-09T00-00-00-000Z_sess-1.jsonl")
	content := strings.Join([]string{
		`{"type":"session","version":3,"id":"sess-1","timestamp":"2026-10-09T00:00:00.000Z","cwd":"` + dir + `"}`,
		`{"type":"message","id":"a","parentId":null,"timestamp":"2026-10-09T00:00:01.000Z","message":{"role":"user","content":"hi","timestamp":1}}`,
		`{"type":"session_info","id":"b","parentId":"a","timestamp":"2026-10-09T00:00:02.000Z","name":"My Session"}`,
	}, "\n") + "\n"
	if err := os.WriteFile(sessionFile, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	// A stray file in another project's folder must not leak in.
	other := filepath.Join(sessionRoot, sessionDirName("/somewhere/else"))
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(other, "2026-10-09T00-00-00-000Z_sess-x.jsonl"),
		[]byte(`{"type":"session","version":3,"id":"sess-x","cwd":"/somewhere/else"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	adapter := newTestAdapter(t, fake, sessionRoot)
	ctx := context.Background()
	inst, err := adapter.StartInstance(ctx, agents.StartInstanceRequest{Directory: dir})
	if err != nil {
		t.Fatal(err)
	}
	refs, err := adapter.ListSessions(ctx, inst.ID)
	if err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}
	if len(refs) != 1 {
		t.Fatalf("ListSessions() = %+v, want exactly one session", refs)
	}
	if refs[0].SessionID != "sess-1" || refs[0].Title != "My Session" ||
		refs[0].InstanceID != inst.ID || refs[0].Backend != agents.BackendPi {
		t.Errorf("ListSessions()[0] = %+v, want sess-1/My Session", refs[0])
	}

	// A directory with no sessions on disk yields an empty, non-error list.
	emptyDir := t.TempDir()
	emptyInst, err := adapter.StartInstance(ctx, agents.StartInstanceRequest{Directory: emptyDir})
	if err != nil {
		t.Fatal(err)
	}
	refs, err = adapter.ListSessions(ctx, emptyInst.ID)
	if err != nil {
		t.Fatalf("ListSessions() for an empty project error = %v", err)
	}
	if len(refs) != 0 {
		t.Errorf("ListSessions() = %+v, want empty", refs)
	}
}

// TestAdapterSendMessageValidation checks the preconditions SendMessage
// enforces before spawning anything.
func TestAdapterSendMessageValidation(t *testing.T) {
	dir := t.TempDir()
	fake := writeFakePi(t, dir, streamingFakePi)
	adapter := newTestAdapter(t, fake, "")
	inst, err := adapter.StartInstance(context.Background(), agents.StartInstanceRequest{Directory: dir})
	if err != nil {
		t.Fatal(err)
	}
	ref, err := adapter.CreateSession(context.Background(), inst.ID, agents.CreateSessionOptions{})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := adapter.SendMessage(ctx, *ref, agents.SendMessageRequest{Text: "hi"}); !errors.Is(err, context.Canceled) {
		t.Errorf("SendMessage(canceled) = %v, want context.Canceled", err)
	}
	if err := adapter.SendMessage(context.Background(), *ref, agents.SendMessageRequest{Text: "  "}); err == nil {
		t.Error("SendMessage with empty text = nil, want error")
	}
	unknown := agents.SessionRef{InstanceID: "nope", Backend: agents.BackendPi, SessionID: "s"}
	if err := adapter.SendMessage(context.Background(), unknown, agents.SendMessageRequest{Text: "hi"}); err == nil {
		t.Error("SendMessage for an unknown instance = nil, want error")
	}
}

// TestAdapterCapabilitiesDeclared checks the adapter only advertises behaviour
// the targeted JSON mode actually provides.
func TestAdapterCapabilitiesDeclared(t *testing.T) {
	adapter := New(Options{Executable: "pi"})
	want := agents.Capabilities{
		Streaming:        true,
		SessionDiscovery: true,
		SessionContinue:  true,
		Stop:             true,
		ModelSelection:   true,
		ToolCallEvents:   true,
		ToolConfirmation: false,
		Interrupt:        true,
		// The adapter reads pi's own transcript, so history must be declared
		// or the platform answers 409 and never asks for it.
		SessionHistory: true,
	}
	if got := adapter.Capabilities(); got != want {
		t.Errorf("Capabilities() = %+v, want %+v", got, want)
	}
}

// TestRegisterInstallsDefaultAdapter checks the composition-root hook wires the
// pi backend into a registry.
func TestRegisterInstallsDefaultAdapter(t *testing.T) {
	registry := agents.NewRegistry()
	if err := Register(registry); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if _, ok := registry.Get(agents.BackendPi); !ok {
		t.Error("Register() did not install the pi adapter")
	}
	if err := Register(registry); err == nil {
		t.Error("Register() twice succeeded, want an already-registered error")
	}
	if err := Register(nil); err == nil {
		t.Error("Register(nil) succeeded, want error")
	}
}

// TestAdapterStopRacingSendReapsProcess reproduces the stop/send interleaving
// where the instance closes after the process was started but before it was
// registered: attachProcess must still reap the child, leaving no zombie.
func TestAdapterStopRacingSendReapsProcess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("process liveness check requires a unix shell")
	}
	dir := t.TempDir()
	fake := writeFakePi(t, dir, sleepingFakePi)
	pidFile := filepath.Join(dir, "pid.txt")
	t.Setenv("FAKE_PI_PID", pidFile)

	adapter := newTestAdapter(t, fake, "")
	ctx := context.Background()
	started, err := adapter.StartInstance(ctx, agents.StartInstanceRequest{Directory: dir})
	if err != nil {
		t.Fatal(err)
	}
	inst, err := adapter.getInstance(started.ID)
	if err != nil {
		t.Fatal(err)
	}

	// The process is started under the instance context, exactly as
	// SendMessage starts it, and is deliberately left unregistered.
	proc, err := startProcess(inst.ctx, processSpec{
		executable: fake,
		args:       []string{"--mode", "json", "--", "race"},
		dir:        dir,
	})
	if err != nil {
		t.Fatalf("startProcess() error = %v", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(waitForFile(t, pidFile))))
	if err != nil {
		t.Fatalf("parse pid: %v", err)
	}

	// Stop first: the snapshot cannot contain the unregistered process.
	inst.stop(ctx)

	// Now the racing send tries to attach it: it must be refused and reaped.
	if attached := inst.attachProcess(proc, "sess-race"); attached {
		t.Fatal("attachProcess() registered a process on a stopped instance")
	}
	waitReaped(t, pid)
}

// TestAdapterStopForgetsSessionMetadata proves stopping an instance drops its
// session entries (no unbounded growth) without touching other instances.
func TestAdapterStopForgetsSessionMetadata(t *testing.T) {
	dir := t.TempDir()
	fake := writeFakePi(t, dir, streamingFakePi)
	adapter := newTestAdapter(t, fake, "")
	ctx := context.Background()

	first, err := adapter.StartInstance(ctx, agents.StartInstanceRequest{Directory: dir})
	if err != nil {
		t.Fatal(err)
	}
	second, err := adapter.StartInstance(ctx, agents.StartInstanceRequest{Directory: dir})
	if err != nil {
		t.Fatal(err)
	}
	for _, instanceID := range []string{first.ID, first.ID, second.ID} {
		if _, err := adapter.CreateSession(ctx, instanceID, agents.CreateSessionOptions{}); err != nil {
			t.Fatalf("CreateSession(%s): %v", instanceID, err)
		}
	}
	if got := sessionCount(adapter); got != 3 {
		t.Fatalf("session entries = %d, want 3", got)
	}

	if err := adapter.StopInstance(ctx, first.ID); err != nil {
		t.Fatalf("StopInstance() error = %v", err)
	}
	if got := sessionCount(adapter); got != 1 {
		t.Fatalf("session entries after stop = %d, want 1 (only the other instance)", got)
	}

	// A create that races the teardown must not resurrect an entry.
	if _, err := adapter.CreateSession(ctx, first.ID, agents.CreateSessionOptions{}); err == nil {
		t.Error("CreateSession on a stopped instance = nil, want error")
	}
	if got := sessionCount(adapter); got != 1 {
		t.Errorf("session entries = %d after a post-stop create, want 1", got)
	}
}

// sessionCount reports how many session metadata entries the adapter holds.
func sessionCount(adapter *Adapter) int {
	adapter.mu.Lock()
	defer adapter.mu.Unlock()
	return len(adapter.sessions)
}

// waitReaped polls until the pid is no longer signalable, i.e. the child has
// been reaped rather than left as a zombie (a zombie still answers signal 0).
func waitReaped(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(pid, 0); err != nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("process %d was not reaped", pid)
}

// containsSequence reports whether args contains the given adjacent pair.
func containsSequence(args []string, a, b string) bool {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == a && args[i+1] == b {
			return true
		}
	}
	return false
}

// drainUntil collects events until pred is satisfied or the channel closes.
func drainUntil(t *testing.T, events <-chan agents.NormalizedEvent, pred func(agents.NormalizedEvent) bool) []agents.NormalizedEvent {
	t.Helper()
	var got []agents.NormalizedEvent
	timeout := time.After(5 * time.Second)
	for {
		select {
		case ev, ok := <-events:
			if !ok {
				t.Fatalf("subscription closed before the expected event: %+v", got)
			}
			got = append(got, ev)
			if pred(ev) {
				return got
			}
		case <-timeout:
			t.Fatalf("timed out waiting for the expected event: %+v", got)
		}
	}
}

// drainClosed drains events until the channel closes, reporting whether it did.
func drainClosed(t *testing.T, events <-chan agents.NormalizedEvent) bool {
	t.Helper()
	timeout := time.After(5 * time.Second)
	for {
		select {
		case _, ok := <-events:
			if !ok {
				return true
			}
		case <-timeout:
			return false
		}
	}
}

// TestExecPathPrefersResolveExecutable verifies the runtime override wins while
// a nil or empty resolver falls back to the configured executable.
func TestExecPathPrefersResolveExecutable(t *testing.T) {
	override := New(Options{Executable: "fallback", ResolveExecutable: func() string { return "/custom/pi" }})
	if got := override.execPath(); got != "/custom/pi" {
		t.Errorf("execPath() = %q, want /custom/pi", got)
	}

	empty := New(Options{Executable: "fallback", ResolveExecutable: func() string { return "" }})
	if got := empty.execPath(); got != "fallback" {
		t.Errorf("execPath() with empty resolver = %q, want fallback", got)
	}

	defaulted := New(Options{Executable: "fallback"})
	if got := defaulted.execPath(); got != "fallback" {
		t.Errorf("execPath() without resolver = %q, want fallback", got)
	}
}

// TestReadSessionRefPreview checks that discovery surfaces the first user
// message as a preview title without disturbing explicit naming.
func TestReadSessionRefPreview(t *testing.T) {
	dir := t.TempDir()
	header := `{"type":"session","version":3,"id":"sess-1","timestamp":"2026-10-09T00:00:00.000Z","cwd":"` + dir + `"}`

	writeSession := func(t *testing.T, lines ...string) string {
		t.Helper()
		path := filepath.Join(t.TempDir(), "sess.jsonl")
		content := append([]string{header}, lines...)
		if err := os.WriteFile(path, []byte(strings.Join(content, "\n")+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}

	t.Run("string content", func(t *testing.T) {
		path := writeSession(t,
			`{"type":"message","id":"u1","parentId":null,"message":{"role":"user","content":"部署日志系统"}}`)
		ref, ok := readSessionRef(path, "inst-1", dir)
		if !ok {
			t.Fatal("readSessionRef() = not ok")
		}
		if ref.Title != "" || ref.Preview != "部署日志系统" {
			t.Errorf("ref = %+v, want empty Title and Preview 部署日志系统", ref)
		}
	})

	t.Run("block content collapses whitespace", func(t *testing.T) {
		path := writeSession(t,
			`{"type":"message","id":"u1","parentId":null,"message":{"role":"user","content":[{"type":"text","text":"修复 登录\n\n问题"}]}}`)
		ref, ok := readSessionRef(path, "inst-1", dir)
		if !ok {
			t.Fatal("readSessionRef() = not ok")
		}
		if ref.Preview != "修复 登录 问题" {
			t.Errorf("Preview = %q, want %q", ref.Preview, "修复 登录 问题")
		}
	})

	t.Run("first empty text falls to next user message", func(t *testing.T) {
		path := writeSession(t,
			`{"type":"message","id":"u1","parentId":null,"message":{"role":"user","content":""}}`,
			`{"type":"message","id":"u2","parentId":"u1","message":{"role":"user","content":"次条消息"}}`)
		ref, ok := readSessionRef(path, "inst-1", dir)
		if !ok {
			t.Fatal("readSessionRef() = not ok")
		}
		if ref.Preview != "次条消息" {
			t.Errorf("Preview = %q, want 次条消息", ref.Preview)
		}
	})

	t.Run("non-user messages yield no preview", func(t *testing.T) {
		path := writeSession(t,
			`{"type":"message","id":"s1","parentId":null,"message":{"role":"system","content":"系统提示"}}`,
			`{"type":"message","id":"a1","parentId":"s1","message":{"role":"assistant","content":"回复"}}`)
		ref, ok := readSessionRef(path, "inst-1", dir)
		if !ok {
			t.Fatal("readSessionRef() = not ok")
		}
		if ref.Preview != "" {
			t.Errorf("Preview = %q, want empty", ref.Preview)
		}
	})

	t.Run("explicit name and preview are independent", func(t *testing.T) {
		path := writeSession(t,
			`{"type":"message","id":"u1","parentId":null,"message":{"role":"user","content":"部署日志系统"}}`,
			`{"type":"session_info","id":"b","parentId":"u1","name":"My Session"}`)
		ref, ok := readSessionRef(path, "inst-1", dir)
		if !ok {
			t.Fatal("readSessionRef() = not ok")
		}
		if ref.Title != "My Session" || ref.Preview != "部署日志系统" {
			t.Errorf("ref = %+v, want Title My Session and Preview 部署日志系统", ref)
		}
	})

	t.Run("no messages yield no preview", func(t *testing.T) {
		ref, ok := readSessionRef(writeSession(t), "inst-1", dir)
		if !ok {
			t.Fatal("readSessionRef() = not ok")
		}
		if ref.Preview != "" {
			t.Errorf("Preview = %q, want empty", ref.Preview)
		}
	})
}

// argsLoggingFakePi appends one argv line per invocation, so a test can watch
// every launch of a session, not just the first.
const argsLoggingFakePi = `
printf '%s\n' "$*" >> "$FAKE_PI_ARGS"
cat <<'EVENTS'
{"type":"session","version":3,"id":"sess-fake","cwd":"/ignored"}
{"type":"agent_settled","aborted":false}
EVENTS
`

// TestAdapterSendMessageUsesRefTitleAsName checks that a title the platform
// derived after session creation rides along as --name (pi persists it) and
// sticks in session metadata for later turns.
func TestAdapterSendMessageUsesRefTitleAsName(t *testing.T) {
	dir := t.TempDir()
	fake := writeFakePi(t, dir, argsLoggingFakePi)
	argsFile := filepath.Join(dir, "args.log")
	t.Setenv("FAKE_PI_ARGS", argsFile)

	adapter := newTestAdapter(t, fake, filepath.Join(dir, "sessions"))
	ctx := context.Background()
	inst, err := adapter.StartInstance(ctx, agents.StartInstanceRequest{Directory: dir})
	if err != nil {
		t.Fatal(err)
	}
	ref, err := adapter.CreateSession(ctx, inst.ID, agents.CreateSessionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	ref.Title = "部署日志系统"
	if err := adapter.SendMessage(ctx, *ref, agents.SendMessageRequest{Text: "部署日志系统"}); err != nil {
		t.Fatal(err)
	}
	first := waitForFile(t, argsFile)
	if !strings.Contains(string(first), "--name 部署日志系统") {
		t.Errorf("first launch argv = %q, want --name 部署日志系统", first)
	}

	// A later send without a title keeps the remembered name.
	ref.Title = ""
	if err := adapter.SendMessage(ctx, *ref, agents.SendMessageRequest{Text: "继续"}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	var second []byte
	for time.Now().Before(deadline) {
		second, _ = os.ReadFile(argsFile)
		if strings.Count(string(second), "\n") >= 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	lines := strings.Split(strings.TrimSpace(string(second)), "\n")
	if len(lines) < 2 || !strings.Contains(lines[len(lines)-1], "--name 部署日志系统") {
		t.Errorf("second launch argv = %q, want --name 部署日志系统", lines)
	}
}

// TestAdapterConcurrentSendsShareSessionTitle hammers one session with a
// titling send and untitled sends at once: the title must be remembered for
// later turns, and (under -race) the shared metadata must show no data race.
func TestAdapterConcurrentSendsShareSessionTitle(t *testing.T) {
	dir := t.TempDir()
	fake := writeFakePi(t, dir, argsLoggingFakePi)
	argsFile := filepath.Join(dir, "args.log")
	t.Setenv("FAKE_PI_ARGS", argsFile)

	adapter := newTestAdapter(t, fake, filepath.Join(dir, "sessions"))
	ctx := context.Background()
	inst, err := adapter.StartInstance(ctx, agents.StartInstanceRequest{Directory: dir})
	if err != nil {
		t.Fatal(err)
	}
	ref, err := adapter.CreateSession(ctx, inst.ID, agents.CreateSessionOptions{})
	if err != nil {
		t.Fatal(err)
	}

	for round := 0; round < 10; round++ {
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			named := *ref
			named.Title = "共享标题"
			if err := adapter.SendMessage(ctx, named, agents.SendMessageRequest{Text: "命名"}); err != nil {
				t.Errorf("named SendMessage: %v", err)
			}
		}()
		go func() {
			defer wg.Done()
			if err := adapter.SendMessage(ctx, *ref, agents.SendMessageRequest{Text: "无名"}); err != nil {
				t.Errorf("untitled SendMessage: %v", err)
			}
		}()
		wg.Wait()
	}

	// Whichever send won, the remembered title must ride on later launches.
	if err := adapter.SendMessage(ctx, *ref, agents.SendMessageRequest{Text: "收尾"}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	var log string
	for time.Now().Before(deadline) {
		data, _ := os.ReadFile(argsFile)
		log = string(data)
		if strings.Count(strings.TrimSpace(log), "\n") >= 20 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	lines := strings.Split(strings.TrimSpace(log), "\n")
	if len(lines) < 21 || !strings.Contains(lines[len(lines)-1], "--name 共享标题") {
		t.Errorf("last launch argv = %q, want --name 共享标题", lines[len(lines)-1])
	}
}
