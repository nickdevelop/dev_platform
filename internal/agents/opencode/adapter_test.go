package opencode

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"devplatform/internal/agents"
)

// recordingLauncher is a test double for the server launcher. It hands the
// adapter a client pointed at an httptest server and records stops.
type recordingLauncher struct {
	client    *Client
	endpoint  string
	launchErr error

	mu    sync.Mutex
	stops int
}

func (l *recordingLauncher) launch(_ context.Context, _ string) (*launchResult, error) {
	if l.launchErr != nil {
		return nil, l.launchErr
	}
	return &launchResult{
		client:   l.client,
		endpoint: l.endpoint,
		stop: func(context.Context) error {
			l.mu.Lock()
			l.stops++
			l.mu.Unlock()
			return nil
		},
	}, nil
}

func (l *recordingLauncher) stopCount() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.stops
}

func queueHandler(routes map[string]http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if handler, ok := routes[r.Method+" "+r.URL.Path]; ok {
			handler(w, r)
			return
		}
		http.NotFound(w, r)
	}
}

// newAdapterWithServer builds an adapter wired to an httptest server via the
// launcher seam, so no real opencode process is ever started.
func newAdapterWithServer(t *testing.T, handler http.HandlerFunc) (*Adapter, *recordingLauncher, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client, err := NewClient(ClientOptions{BaseURL: server.URL, Username: "opencode", Password: "pw", Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	launcher := &recordingLauncher{client: client, endpoint: server.URL}
	executable := filepath.Join(t.TempDir(), "fake-opencode")
	if err := os.WriteFile(executable, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatalf("write fake executable: %v", err)
	}
	adapter := New(Options{
		Executable: executable,
		NewID:      func() (string, error) { return "inst-1", nil },
	})
	adapter.launch = launcher.launch
	return adapter, launcher, server
}

func startTestInstance(t *testing.T, adapter *Adapter) *agents.Instance {
	t.Helper()
	instance, err := adapter.StartInstance(context.Background(), agents.StartInstanceRequest{Directory: t.TempDir()})
	if err != nil {
		t.Fatalf("StartInstance: %v", err)
	}
	return instance
}

func sseEvent(id, typ, data, directory string) string {
	location := ""
	if directory != "" {
		location = fmt.Sprintf(`,"location":{"directory":%q}`, directory)
	}
	return fmt.Sprintf("data: {\"id\":%q%s,\"type\":%q,\"data\":%s}\n\n", id, location, typ, data)
}

func collectEvents(t *testing.T, ch <-chan agents.NormalizedEvent, want int) []agents.NormalizedEvent {
	t.Helper()
	events := make([]agents.NormalizedEvent, 0, want)
	timeout := time.After(3 * time.Second)
	for len(events) < want {
		select {
		case ev, ok := <-ch:
			if !ok {
				t.Fatalf("event channel closed after %d events, want %d", len(events), want)
			}
			if err := ev.Validate(); err != nil {
				t.Fatalf("invalid normalized event: %v (%+v)", err, ev)
			}
			events = append(events, ev)
		case <-timeout:
			t.Fatalf("timed out after %d events, want %d", len(events), want)
		}
	}
	return events
}

func TestAdapterCapabilitiesDeclared(t *testing.T) {
	adapter := New(Options{})
	caps := adapter.Capabilities()
	if !caps.Streaming || !caps.SessionDiscovery || !caps.SessionContinue || !caps.Stop {
		t.Fatalf("core capabilities not declared: %+v", caps)
	}
	if !caps.ModelSelection || !caps.ToolCallEvents {
		t.Fatalf("model/tool capabilities not declared: %+v", caps)
	}
	if caps.ToolConfirmation {
		t.Fatalf("ToolConfirmation must stay false: the adapter cannot reply to permission requests: %+v", caps)
	}
}

func TestAdapterProbe(t *testing.T) {
	ctx := context.Background()
	if err := New(Options{Executable: "definitely-not-a-real-opencode-binary-xyz"}).Probe(ctx); err == nil {
		t.Fatal("expected probe failure for a missing executable")
	}

	executable := filepath.Join(t.TempDir(), "fake-opencode")
	if err := os.WriteFile(executable, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write fake executable: %v", err)
	}
	if err := New(Options{Executable: executable}).Probe(ctx); err != nil {
		t.Fatalf("probe existing executable: %v", err)
	}

	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err := New(Options{Executable: executable}).Probe(canceled); !errors.Is(err, context.Canceled) {
		t.Fatalf("probe canceled context = %v", err)
	}
}

func TestAdapterStartAndStop(t *testing.T) {
	adapter, launcher, _ := newAdapterWithServer(t, func(w http.ResponseWriter, r *http.Request) {})

	instance, err := adapter.StartInstance(context.Background(), agents.StartInstanceRequest{Directory: t.TempDir()})
	if err != nil {
		t.Fatalf("StartInstance: %v", err)
	}
	if instance.ID == "" || instance.Backend != agents.BackendOpenCodeV2 {
		t.Fatalf("unexpected instance: %+v", instance)
	}
	if instance.Status != agents.InstanceRunning {
		t.Fatalf("status = %q, want running", instance.Status)
	}
	if instance.Endpoint == "" {
		t.Fatal("server backend must report an endpoint")
	}
	if err := instance.Validate(); err != nil {
		t.Fatalf("instance does not validate: %v", err)
	}

	if err := adapter.StopInstance(context.Background(), instance.ID); err != nil {
		t.Fatalf("StopInstance: %v", err)
	}
	if got := launcher.stopCount(); got != 1 {
		t.Fatalf("stop called %d times, want 1", got)
	}
	// Idempotent: a second stop and an unknown id both succeed without
	// touching the backend again.
	if err := adapter.StopInstance(context.Background(), instance.ID); err != nil {
		t.Fatalf("second StopInstance: %v", err)
	}
	if err := adapter.StopInstance(context.Background(), "missing"); err != nil {
		t.Fatalf("unknown StopInstance: %v", err)
	}
	if got := launcher.stopCount(); got != 1 {
		t.Fatalf("stop called %d times after idempotent stops, want 1", got)
	}
}

func TestAdapterStartValidations(t *testing.T) {
	adapter, launcher, _ := newAdapterWithServer(t, func(w http.ResponseWriter, r *http.Request) {})

	if _, err := adapter.StartInstance(context.Background(), agents.StartInstanceRequest{}); err == nil {
		t.Fatal("expected error for empty directory")
	}
	if _, err := adapter.StartInstance(context.Background(), agents.StartInstanceRequest{Directory: filepath.Join(t.TempDir(), "missing")}); err == nil {
		t.Fatal("expected error for missing directory")
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := adapter.StartInstance(canceled, agents.StartInstanceRequest{Directory: t.TempDir()}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled StartInstance = %v", err)
	}
	if got := launcher.stopCount(); got != 0 {
		t.Fatalf("launcher stopped %d times during validation failures", got)
	}
}

func TestAdapterStartPropagatesLaunchFailure(t *testing.T) {
	adapter, launcher, _ := newAdapterWithServer(t, func(w http.ResponseWriter, r *http.Request) {})
	launcher.launchErr = &APIError{StatusCode: http.StatusUnauthorized, Code: "UnauthorizedError", Message: "Authentication required"}

	if _, err := adapter.StartInstance(context.Background(), agents.StartInstanceRequest{Directory: t.TempDir()}); err == nil {
		t.Fatal("expected launch failure to propagate")
	}
}

func TestAdapterListSessionsMapsNativeSessions(t *testing.T) {
	adapter, _, _ := newAdapterWithServer(t, queueHandler(map[string]http.HandlerFunc{
		"GET /api/session": func(w http.ResponseWriter, r *http.Request) {
			if got := r.URL.Query().Get("directory"); got == "" {
				t.Errorf("directory query missing")
			}
			io.WriteString(w, `{"data":[
				{"id":"ses_a","projectID":"prj","title":"Alpha","time":{"created":1,"updated":1},"location":{"directory":"/x"}},
				{"id":"ses_b","projectID":"prj","time":{"created":2,"updated":2},"location":{"directory":"/x"}}
			],"cursor":{}}`)
		},
	}))
	instance := startTestInstance(t, adapter)

	refs, err := adapter.ListSessions(context.Background(), instance.ID)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(refs) != 2 {
		t.Fatalf("got %d refs, want 2", len(refs))
	}
	if refs[0].SessionID != "ses_a" || refs[0].Title != "Alpha" || refs[0].InstanceID != instance.ID || refs[0].Backend != agents.BackendOpenCodeV2 {
		t.Fatalf("unexpected ref: %+v", refs[0])
	}
	if err := refs[0].Validate(); err != nil {
		t.Fatalf("ref does not validate: %v", err)
	}
}

func TestAdapterCreateSession(t *testing.T) {
	var body map[string]any
	adapter, _, _ := newAdapterWithServer(t, queueHandler(map[string]http.HandlerFunc{
		"POST /api/session": func(w http.ResponseWriter, r *http.Request) {
			json.NewDecoder(r.Body).Decode(&body)
			io.WriteString(w, `{"data":{"id":"ses_new","projectID":"prj","title":"Named","time":{"created":1,"updated":1},"location":{"directory":"/x"}}}`)
		},
	}))
	instance := startTestInstance(t, adapter)

	ref, err := adapter.CreateSession(context.Background(), instance.ID, agents.CreateSessionOptions{Title: "Named", Model: "prov/model"})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if ref.SessionID != "ses_new" || ref.Title != "Named" || ref.Backend != agents.BackendOpenCodeV2 {
		t.Fatalf("unexpected ref: %+v", ref)
	}
	if body["title"] != "Named" {
		t.Errorf("title sent = %v", body["title"])
	}
	model, _ := body["model"].(map[string]any)
	if model["providerID"] != "prov" || model["id"] != "model" {
		t.Errorf("model sent = %v", body["model"])
	}
	location, _ := body["location"].(map[string]any)
	if location["directory"] != instance.Directory {
		t.Errorf("location sent = %v, want %q", body["location"], instance.Directory)
	}

	if _, err := adapter.CreateSession(context.Background(), "missing", agents.CreateSessionOptions{}); err == nil {
		t.Fatal("expected error for unknown instance")
	}
	if _, err := adapter.CreateSession(context.Background(), instance.ID, agents.CreateSessionOptions{Model: "not-a-provider-model"}); err == nil {
		t.Fatal("expected error for unparseable model")
	}
}

func TestAdapterContinueSession(t *testing.T) {
	adapter, _, _ := newAdapterWithServer(t, queueHandler(map[string]http.HandlerFunc{
		"GET /api/session/ses_old": func(w http.ResponseWriter, r *http.Request) {
			io.WriteString(w, `{"data":{"id":"ses_old","projectID":"prj","title":"Fresh","time":{"created":1,"updated":2},"location":{"directory":"/x"}}}`)
		},
		"GET /api/session/ses_gone": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			io.WriteString(w, `{"_tag":"SessionNotFoundError","message":"no such session"}`)
		},
	}))
	instance := startTestInstance(t, adapter)

	ref, err := adapter.ContinueSession(context.Background(), agents.SessionRef{
		InstanceID: instance.ID, Backend: agents.BackendOpenCodeV2, SessionID: "ses_old",
	})
	if err != nil {
		t.Fatalf("ContinueSession: %v", err)
	}
	if ref.Title != "Fresh" {
		t.Fatalf("title = %q, want Fresh", ref.Title)
	}

	_, err = adapter.ContinueSession(context.Background(), agents.SessionRef{
		InstanceID: instance.ID, Backend: agents.BackendOpenCodeV2, SessionID: "ses_gone",
	})
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusNotFound {
		t.Fatalf("missing-session error = %v", err)
	}

	if _, err := adapter.ContinueSession(context.Background(), agents.SessionRef{InstanceID: instance.ID, Backend: agents.BackendPi, SessionID: "ses_old"}); err == nil {
		t.Fatal("expected error for wrong backend")
	}
	if _, err := adapter.ContinueSession(context.Background(), agents.SessionRef{InstanceID: instance.ID, Backend: agents.BackendOpenCodeV2}); err == nil {
		t.Fatal("expected error for empty session id")
	}
}

func TestAdapterSendMessagePromptsAndSwitchesModel(t *testing.T) {
	var mu sync.Mutex
	var paths []string
	var promptText string
	adapter, _, _ := newAdapterWithServer(t, queueHandler(map[string]http.HandlerFunc{
		"POST /api/session/ses_1/model": func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			paths = append(paths, r.URL.Path)
			mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
		},
		"POST /api/session/ses_1/prompt": func(w http.ResponseWriter, r *http.Request) {
			var body struct {
				Text string `json:"text"`
			}
			json.NewDecoder(r.Body).Decode(&body)
			mu.Lock()
			paths = append(paths, r.URL.Path)
			promptText = body.Text
			mu.Unlock()
			io.WriteString(w, `{"data":{"id":"msg_1","sessionID":"ses_1"}}`)
		},
	}))
	instance := startTestInstance(t, adapter)

	ref := agents.SessionRef{InstanceID: instance.ID, Backend: agents.BackendOpenCodeV2, SessionID: "ses_1"}
	if err := adapter.SendMessage(context.Background(), ref, agents.SendMessageRequest{Text: "hi there", Model: "prov/model#fast"}); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if promptText != "hi there" {
		t.Fatalf("prompt text = %q", promptText)
	}
	if len(paths) != 2 || !strings.HasSuffix(paths[0], "/model") || !strings.HasSuffix(paths[1], "/prompt") {
		t.Fatalf("request order = %v, want model then prompt", paths)
	}
}

func TestAdapterSendMessageValidation(t *testing.T) {
	adapter, _, _ := newAdapterWithServer(t, func(w http.ResponseWriter, r *http.Request) {})
	instance := startTestInstance(t, adapter)
	ref := agents.SessionRef{InstanceID: instance.ID, Backend: agents.BackendOpenCodeV2, SessionID: "ses_1"}

	if err := adapter.SendMessage(context.Background(), ref, agents.SendMessageRequest{Text: "   "}); err == nil {
		t.Fatal("expected error for empty text")
	}
	if err := adapter.SendMessage(context.Background(), agents.SessionRef{InstanceID: instance.ID, Backend: agents.BackendPi, SessionID: "ses_1"}, agents.SendMessageRequest{Text: "x"}); err == nil {
		t.Fatal("expected error for wrong backend")
	}
	if err := adapter.SendMessage(context.Background(), agents.SessionRef{InstanceID: "missing", Backend: agents.BackendOpenCodeV2, SessionID: "ses_1"}, agents.SendMessageRequest{Text: "x"}); err == nil {
		t.Fatal("expected error for unknown instance")
	}
	if err := adapter.SendMessage(context.Background(), ref, agents.SendMessageRequest{Text: "x", Model: "nope"}); err == nil {
		t.Fatal("expected error for unparseable model")
	}
}

func TestAdapterSubscribeMapsNativeEvents(t *testing.T) {
	events := strings.Join([]string{
		sseEvent("evt_1", "session.text.delta", `{"sessionID":"ses_1","assistantMessageID":"msg_1","ordinal":1,"delta":"Hel"}`, "/x"),
		sseEvent("evt_2", "session.text.delta", `{"sessionID":"ses_1","assistantMessageID":"msg_1","ordinal":1,"delta":"lo"}`, "/x"),
		sseEvent("evt_3", "session.text.ended", `{"sessionID":"ses_1","assistantMessageID":"msg_1","ordinal":1,"text":"Hello"}`, "/x"),
		sseEvent("evt_4", "session.text.started", `{"sessionID":"ses_1","assistantMessageID":"msg_2","ordinal":2}`, "/x"),
		sseEvent("evt_5", "session.text.ended", `{"sessionID":"ses_1","assistantMessageID":"msg_2","ordinal":2,"text":"complete"}`, "/x"),
		sseEvent("evt_6", "session.tool.input.started", `{"sessionID":"ses_1","assistantMessageID":"msg_2","id":"tool_1","name":"bash"}`, "/x"),
		sseEvent("evt_7", "session.tool.called", `{"sessionID":"ses_1","assistantMessageID":"msg_2","id":"tool_1","input":{"cmd":"ls"},"executed":true}`, "/x"),
		sseEvent("evt_8", "session.tool.success", `{"sessionID":"ses_1","assistantMessageID":"msg_2","id":"tool_1","content":[{"type":"text","text":"ok"}],"executed":true}`, "/x"),
		sseEvent("evt_9", "session.execution.failed", `{"sessionID":"ses_1","error":{"type":"provider.internal","message":"boom","status":502}}`, ""),
		sseEvent("evt_10", "session.mystery", `{"sessionID":"ses_1","foo":"bar"}`, "/x"),
		sseEvent("evt_11", "session.text.delta", `{"sessionID":"ses_other","delta":"nope"}`, "/other"),
	}, "")

	adapter, _, _ := newAdapterWithServer(t, queueHandler(map[string]http.HandlerFunc{
		"POST /api/session": func(w http.ResponseWriter, r *http.Request) {
			io.WriteString(w, `{"data":{"id":"ses_1","projectID":"prj","title":"Live","time":{"created":1,"updated":1}}}`)
		},
		"GET /api/event": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("content-type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			io.WriteString(w, events)
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			<-r.Context().Done()
		},
	}))
	instance := startTestInstance(t, adapter)
	if _, err := adapter.CreateSession(context.Background(), instance.ID, agents.CreateSessionOptions{}); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, err := adapter.Subscribe(ctx, instance.ID)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer adapter.StopInstance(context.Background(), instance.ID)

	got := collectEvents(t, ch, 9)

	// 1-3: a streamed assistant message: deltas then an empty final terminator.
	if got[0].Type != agents.EventMessage || got[0].Message.Text != "Hel" || got[0].Message.Final {
		t.Fatalf("event 0 = %+v", got[0])
	}
	if got[1].Type != agents.EventMessage || got[1].Message.Text != "lo" || got[1].Message.Final {
		t.Fatalf("event 1 = %+v", got[1])
	}
	if got[2].Type != agents.EventMessage || !got[2].Message.Final || got[2].Message.Text != "" {
		t.Fatalf("event 2 = %+v", got[2])
	}
	// 4: text.started is lifecycle.
	if got[3].Type != agents.EventLifecycle || got[3].Lifecycle == nil {
		t.Fatalf("event 3 = %+v", got[3])
	}
	// 5: a non-streamed message keeps its full text.
	if got[4].Type != agents.EventMessage || !got[4].Message.Final || got[4].Message.Text != "complete" {
		t.Fatalf("event 4 = %+v", got[4])
	}
	// 6-7: tool call carries the name seen earlier and the argument JSON.
	if got[5].Type != agents.EventToolCall || got[5].ToolCall.CallID != "tool_1" || got[5].ToolCall.Name != "bash" {
		t.Fatalf("event 5 = %+v", got[5])
	}
	if string(got[5].ToolCall.Input) != `{"cmd":"ls"}` {
		t.Fatalf("tool input = %s", got[5].ToolCall.Input)
	}
	if got[6].Type != agents.EventToolResult || got[6].ToolResult.CallID != "tool_1" || got[6].ToolResult.Output != "ok" || got[6].ToolResult.IsError {
		t.Fatalf("event 6 = %+v", got[6])
	}
	if got[7].Type != agents.EventError || got[7].Error.Code != "execution_failed" || !strings.Contains(got[7].Error.Message, "boom") {
		t.Fatalf("event 7 = %+v", got[7])
	}
	if got[8].Type != agents.EventUnknown || len(got[8].Raw) == 0 {
		t.Fatalf("event 8 = %+v", got[8])
	}

	// Envelope: platform backend, this instance, increasing sequence, stamps.
	for i, ev := range got {
		if ev.Backend != agents.BackendOpenCodeV2 || ev.InstanceID != instance.ID {
			t.Fatalf("event %d envelope = %+v", i, ev)
		}
		if ev.Sequence != uint64(i+1) {
			t.Fatalf("event %d sequence = %d, want %d", i, ev.Sequence, i+1)
		}
		if ev.Timestamp.IsZero() {
			t.Fatalf("event %d has no timestamp", i)
		}
		if ev.SessionID != "ses_1" {
			t.Fatalf("event %d session = %q, want ses_1", i, ev.SessionID)
		}
	}
}

func TestAdapterSubscribeIsolatesInstances(t *testing.T) {
	blocking := queueHandler(map[string]http.HandlerFunc{
		"GET /api/event": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("content-type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			<-r.Context().Done()
		},
	})
	silentAdapter, _, _ := newAdapterWithServer(t, blocking)
	silentInstance := startTestInstance(t, silentAdapter)

	noisy := strings.Join([]string{
		sseEvent("evt_1", "session.text.delta", `{"sessionID":"ses_1","delta":"noise"}`, "/x"),
	}, "")
	noisyAdapter, _, _ := newAdapterWithServer(t, queueHandler(map[string]http.HandlerFunc{
		"GET /api/event": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("content-type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			io.WriteString(w, noisy)
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			<-r.Context().Done()
		},
	}))
	noisyInstance := startTestInstance(t, noisyAdapter)

	silentCh, err := silentAdapter.Subscribe(context.Background(), silentInstance.ID)
	if err != nil {
		t.Fatalf("Subscribe silent: %v", err)
	}
	defer silentAdapter.StopInstance(context.Background(), silentInstance.ID)
	noisyCh, err := noisyAdapter.Subscribe(context.Background(), noisyInstance.ID)
	if err != nil {
		t.Fatalf("Subscribe noisy: %v", err)
	}
	defer noisyAdapter.StopInstance(context.Background(), noisyInstance.ID)

	// The noisy instance must not leak into the silent one. The noisy event is
	// filtered because the session was never registered on it, so neither
	// channel should deliver anything.
	select {
	case ev := <-silentCh:
		t.Fatalf("silent instance received %+v", ev)
	case ev := <-noisyCh:
		t.Fatalf("unregistered session event unexpectedly delivered: %+v", ev)
	case <-time.After(300 * time.Millisecond):
	}
}

func TestAdapterStreamDisconnectEmitsError(t *testing.T) {
	events := sseEvent("evt_1", "session.text.delta", `{"sessionID":"ses_1","delta":"bye"}`, "/x")
	adapter, _, _ := newAdapterWithServer(t, queueHandler(map[string]http.HandlerFunc{
		"POST /api/session": func(w http.ResponseWriter, r *http.Request) {
			io.WriteString(w, `{"data":{"id":"ses_1","projectID":"prj","time":{"created":1,"updated":1}}}`)
		},
		"GET /api/event": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("content-type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			io.WriteString(w, events)
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			// Disconnect mid-stream.
		},
	}))
	instance := startTestInstance(t, adapter)
	if _, err := adapter.CreateSession(context.Background(), instance.ID, agents.CreateSessionOptions{}); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, err := adapter.Subscribe(ctx, instance.ID)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer adapter.StopInstance(context.Background(), instance.ID)

	sawMessage := false
	timeout := time.After(3 * time.Second)
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				t.Fatal("channel closed before the stream error")
			}
			if ev.Type == agents.EventMessage {
				sawMessage = true
			}
			if ev.Type == agents.EventError {
				if ev.Error.Code != "stream_error" {
					t.Fatalf("error code = %q, want stream_error", ev.Error.Code)
				}
				if !sawMessage {
					t.Fatal("stream error arrived before the message")
				}
				return
			}
		case <-timeout:
			t.Fatal("timed out waiting for the stream error")
		}
	}
}

func TestAdapterSubscribeUnknownInstance(t *testing.T) {
	adapter := New(Options{Executable: "opencode"})
	if _, err := adapter.Subscribe(context.Background(), "missing"); err == nil {
		t.Fatal("expected error for unknown instance")
	}
}

func TestRegisterInstallsDefaultAdapter(t *testing.T) {
	registry := agents.NewRegistry()
	if err := Register(registry); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if _, ok := registry.Get(agents.BackendOpenCodeV2); !ok {
		t.Fatal("opencode adapter was not registered")
	}
	if err := Register(nil); err == nil {
		t.Fatal("expected error for nil registry")
	}
}

func TestParseModelRef(t *testing.T) {
	model, err := parseModelRef("prov/model#fast")
	if err != nil {
		t.Fatalf("parseModelRef: %v", err)
	}
	if model.ProviderID != "prov" || model.ID != "model" || model.Variant != "fast" {
		t.Fatalf("unexpected model: %+v", model)
	}
	if _, err := parseModelRef("model-only"); err == nil {
		t.Fatal("expected error for missing provider")
	}
	if _, err := parseModelRef(""); err == nil {
		t.Fatal("expected error for empty model")
	}
}
