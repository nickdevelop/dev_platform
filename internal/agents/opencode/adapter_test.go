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
	// done, when set, stands in for the server process's exit channel so a
	// test can simulate the process going away.
	done chan struct{}

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
		done:     l.done,
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
	server := httptest.NewServer(withDefaultInfo(handler))
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
		// Isolate from the machine-wide service registration: tests must
		// never adopt the real desktop/sidecar service running on the host.
		ServiceFile: filepath.Join(t.TempDir(), "service.json"),
	})
	adapter.launch = launcher.launch
	// The service is a third party: the platform only ever binds one it found
	// already running, so the test states that directly instead of asking the
	// platform to boot the server (which it no longer does).
	adapter.shared = &sharedServer{client: client, endpoint: server.URL}
	return adapter, launcher, server
}

// withDefaultInfo answers GET /api/info when the test's own handler does not,
// so an adapter bound to the test server passes the readiness probe without
// every test restating the handshake.
func withDefaultInfo(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/info" {
			recorded := httptest.NewRecorder()
			handler(recorded, r)
			if recorded.Code == http.StatusNotFound {
				writeInfo(w, "2.0.24")
				return
			}
			for key, values := range recorded.Header() {
				for _, value := range values {
					w.Header().Add(key, value)
				}
			}
			w.WriteHeader(recorded.Code)
			_, _ = w.Write(recorded.Body.Bytes())
			return
		}
		handler(w, r)
	}
}

// infoHandler serves a healthy /api/info and nothing else.
func infoHandler(version string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/info" {
			writeInfo(w, version)
			return
		}
		http.NotFound(w, r)
	}
}

func writeInfo(w http.ResponseWriter, version string) {
	w.Header().Set("content-type", "application/json")
	_, _ = fmt.Fprintf(w, `{"version":%q,"pid":%d,"urls":[],"paths":{"tmp":"/tmp"}}`, version, os.Getpid())
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
	if !caps.ModelSelection || !caps.ToolCallEvents || !caps.Interrupt {
		t.Fatalf("model/tool/interrupt capabilities not declared: %+v", caps)
	}
	// Permission/form replies are served by the opencode-only PermissionReplier
	// extension (AgentAdapter stays frozen for pi), so confirmation must be
	// declared.
	if !caps.ToolConfirmation {
		t.Fatalf("ToolConfirmation must be declared: the adapter replies via PermissionReplier: %+v", caps)
	}
	// The adapter reads the server's native message log, so history must be
	// declared or the platform answers 409 and never asks for it.
	if !caps.SessionHistory {
		t.Fatalf("session history not declared: %+v", caps)
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
	// Stopping the platform's own instance never reaches the service: it is a
	// third party that may be serving other clients.
	if got := launcher.stopCount(); got != 0 {
		t.Fatalf("stop called %d times, want 0", got)
	}
	// Idempotent: a second stop and an unknown id both succeed without
	// touching the backend again.
	if err := adapter.StopInstance(context.Background(), instance.ID); err != nil {
		t.Fatalf("second StopInstance: %v", err)
	}
	if err := adapter.StopInstance(context.Background(), "missing"); err != nil {
		t.Fatalf("unknown StopInstance: %v", err)
	}
	if got := launcher.stopCount(); got != 0 {
		t.Fatalf("stop called %d times after idempotent stops, want 0", got)
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
	// The switch took effect: live assistant events from now on are stamped
	// with the model the platform just switched to.
	inst := adapter.instances[instance.ID]
	streamed := inst.normalize(&Event{
		ID:   "evt_1",
		Type: "session.text.delta",
		Data: json.RawMessage(`{"sessionID":"ses_1","assistantMessageID":"msg_9","ordinal":1,"delta":"ok"}`),
	})
	if len(streamed) != 1 || streamed[0].Message == nil {
		t.Fatalf("normalize produced %+v, want one message", streamed)
	}
	model := streamed[0].Message.Model
	if model == nil || model.ID != "prov/model" || model.Provider != "prov" || model.Variant != "fast" {
		t.Fatalf("stamped model = %+v, want prov/model (prov, fast)", model)
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

// TestAdapterSubscribeStampsSessionModel pins the realtime model stamp: the
// live stream's assistant events carry no model of their own, so the adapter
// stamps them with the session's current model — learned from the create
// response — in the platform's provider/model form.
func TestAdapterSubscribeStampsSessionModel(t *testing.T) {
	events := strings.Join([]string{
		sseEvent("evt_1", "session.text.delta", `{"sessionID":"ses_1","assistantMessageID":"msg_1","ordinal":1,"delta":"Hel"}`, "/x"),
		sseEvent("evt_2", "session.text.ended", `{"sessionID":"ses_1","assistantMessageID":"msg_1","ordinal":1,"text":"Hello"}`, "/x"),
	}, "")

	adapter, _, _ := newAdapterWithServer(t, queueHandler(map[string]http.HandlerFunc{
		"POST /api/session": func(w http.ResponseWriter, r *http.Request) {
			io.WriteString(w, `{"data":{"id":"ses_1","projectID":"prj","title":"Live","model":{"id":"model-x","providerID":"prj","variant":"fast"},"time":{"created":1,"updated":1}}}`)
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

	got := collectEvents(t, ch, 2)
	// The deltas carried the text, so the ended event closes with an empty
	// terminator.
	for i, want := range []struct {
		text    string
		final   bool
		modelID string
	}{
		{"Hel", false, "prj/model-x"},
		{"", true, "prj/model-x"},
	} {
		if got[i].Type != agents.EventMessage || got[i].Message == nil {
			t.Fatalf("event %d = %+v, want a message", i, got[i])
		}
		if got[i].Message.Text != want.text || got[i].Message.Final != want.final {
			t.Fatalf("event %d message = %+v, want text %q final %v", i, got[i].Message, want.text, want.final)
		}
		model := got[i].Message.Model
		if model == nil || model.ID != want.modelID || model.Provider != "prj" || model.Variant != "fast" {
			t.Fatalf("event %d model = %+v, want %s (prj, fast)", i, model, want.modelID)
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

// TestAdapterProcessExitMarksInstanceFailed verifies finding 2: when the
// launched server process exits on its own, the adapter emits an
// instance-scoped lifecycle event carrying the failed state (so the platform's
// applyLifecycle accepts it) and the instance stops accepting work.
func TestAdapterProcessExitMarksInstanceFailed(t *testing.T) {
	adapter, launcher, _ := newAdapterWithServer(t, queueHandler(map[string]http.HandlerFunc{
		"GET /api/event": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("content-type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			<-r.Context().Done()
		},
	}))
	launcher.done = make(chan struct{})
	// The service is a third party: what the platform can observe is that the
	// process behind the service it adopted went away.
	adapter.shared.done = launcher.done
	go adapter.watchShared(adapter.shared)
	instance := startTestInstance(t, adapter)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, err := adapter.Subscribe(ctx, instance.ID)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	// The server process exits.
	close(launcher.done)

	deadline := time.After(3 * time.Second)
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				t.Fatal("channel closed before the failed lifecycle event")
			}
			if ev.Type == agents.EventLifecycle && ev.Lifecycle != nil {
				if ev.Lifecycle.State != string(agents.InstanceFailed) {
					t.Fatalf("lifecycle state = %q, want %q", ev.Lifecycle.State, agents.InstanceFailed)
				}
				if ev.SessionID != "" {
					t.Fatalf("instance-scoped lifecycle event carries session %q, want empty", ev.SessionID)
				}
				// The instance no longer accepts work.
				if _, err := adapter.CreateSession(ctx, instance.ID, agents.CreateSessionOptions{}); err == nil {
					t.Fatal("CreateSession succeeded on an instance whose process exited")
				}
				return
			}
		case <-deadline:
			t.Fatal("timed out waiting for the failed lifecycle event")
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

func TestAdapterStreamBackoffGrowsOnImmediateDisconnect(t *testing.T) {
	adapter, _, _ := newAdapterWithServer(t, queueHandler(map[string]http.HandlerFunc{
		"GET /api/event": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("content-type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			// Return immediately: the connection is accepted and closed with
			// no event, the shape that must drive backoff growth.
		},
	}))
	instance := startTestInstance(t, adapter)

	inst := adapter.instances[instance.ID]
	var mu sync.Mutex
	var waits []time.Duration
	done := make(chan struct{})
	var once sync.Once
	inst.sleep = func(d time.Duration) bool {
		mu.Lock()
		waits = append(waits, d)
		n := len(waits)
		mu.Unlock()
		if n >= 6 {
			once.Do(func() { close(done) })
			return false
		}
		return true
	}

	if _, err := adapter.Subscribe(context.Background(), instance.ID); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer adapter.StopInstance(context.Background(), instance.ID)

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for the reconnect backoff to grow")
	}

	mu.Lock()
	got := append([]time.Duration(nil), waits...)
	mu.Unlock()

	if len(got) < 3 {
		t.Fatalf("recorded %d reconnects, want at least 3: %v", len(got), got)
	}
	if got[0] != streamRetryMin {
		t.Fatalf("first backoff = %v, want %v", got[0], streamRetryMin)
	}
	for i := 1; i < len(got); i++ {
		if got[i] <= got[i-1] {
			t.Fatalf("backoff did not grow on repeated immediate disconnects: %v", got)
		}
	}
	for _, d := range got {
		if d > streamRetryMax {
			t.Fatalf("backoff %v exceeds the cap %v: %v", d, streamRetryMax, got)
		}
	}
}

func TestAdapterStreamIdleTimeoutReconnects(t *testing.T) {
	adapter, _, _ := newAdapterWithServer(t, queueHandler(map[string]http.HandlerFunc{
		"GET /api/event": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("content-type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			// Accept and never write: only the idle watchdog can end this.
			<-r.Context().Done()
		},
	}))
	adapter.streamIdle = 50 * time.Millisecond
	instance := startTestInstance(t, adapter)

	ch, err := adapter.Subscribe(context.Background(), instance.ID)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer adapter.StopInstance(context.Background(), instance.ID)

	select {
	case ev := <-ch:
		if ev.Type != agents.EventLifecycle || ev.Lifecycle == nil || ev.Lifecycle.State != "stream_error" {
			t.Fatalf("unexpected event from a silent stream: %+v", ev)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("a silent but open stream was not bounded by the idle timeout")
	}
}

func TestAdapterStreamHeartbeatKeepsIdleWatchdogAlive(t *testing.T) {
	adapter, _, _ := newAdapterWithServer(t, queueHandler(map[string]http.HandlerFunc{
		"GET /api/event": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("content-type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			flusher, _ := w.(http.Flusher)
			if flusher != nil {
				flusher.Flush()
			}
			// opencode keeps an idle stream alive with ": heartbeat" comments
			// and sends no other event while a turn is quiet. The watchdog must
			// treat those comments as activity, not as silence.
			ticker := time.NewTicker(10 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-r.Context().Done():
					return
				case <-ticker.C:
					if _, err := io.WriteString(w, ": heartbeat\n\n"); err != nil {
						return
					}
					if flusher != nil {
						flusher.Flush()
					}
				}
			}
		},
	}))
	adapter.streamIdle = 40 * time.Millisecond
	instance := startTestInstance(t, adapter)

	ch, err := adapter.Subscribe(context.Background(), instance.ID)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer adapter.StopInstance(context.Background(), instance.ID)

	// Several idle windows pass. A heartbeat-only stream is healthy and must
	// not be torn down with a stream_error.
	select {
	case ev := <-ch:
		t.Fatalf("a heartbeat-only stream was treated as dead: %+v", ev)
	case <-time.After(300 * time.Millisecond):
	}
}

func TestAdapterLaunchDiagnosticRedactsPassword(t *testing.T) {
	outFile := filepath.Join(t.TempDir(), "password.txt")
	t.Setenv("OC_TEST_OUT", outFile)

	script := filepath.Join(t.TempDir(), "fake-opencode")
	content := "#!/bin/sh\n" +
		"printf '%s' \"$OPENCODE_SERVER_PASSWORD\" > \"$OC_TEST_OUT\"\n" +
		"echo \"server password $OPENCODE_SERVER_PASSWORD\"\n" +
		"exit 1\n"
	if err := os.WriteFile(script, []byte(content), 0o755); err != nil {
		t.Fatalf("write fake executable: %v", err)
	}

	adapter := New(Options{
		Executable:     script,
		StartupTimeout: 3 * time.Second,
		// Isolate from the machine-wide service registration.
		ServiceFile: filepath.Join(t.TempDir(), "service.json"),
	})
	// Only the settings page launches the service, so that is the code path
	// whose diagnostics must not leak the password.
	_, err := adapter.launchSharedServer(context.Background(), t.TempDir())
	if err == nil {
		t.Fatal("expected the launch to fail")
	}

	password, readErr := os.ReadFile(outFile)
	if readErr != nil || len(password) == 0 {
		t.Fatalf("fake server did not capture the password: %v (%q)", readErr, password)
	}
	if strings.Contains(err.Error(), string(password)) {
		t.Fatalf("the exact password leaked in the launch diagnostic: %v", err)
	}
	if !strings.Contains(err.Error(), "[REDACTED]") {
		t.Fatalf("expected a redaction marker in the launch diagnostic: %v", err)
	}
}

func serveMessagesFixture(t *testing.T) http.HandlerFunc {
	t.Helper()
	fixture := readMessagesFixture(t)
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write(fixture)
	}
}

// TestListHistoryMapsOpenCodeMessages pins the translation of the native
// message log: conversation blocks become message and tool events in order,
// thinking and turn markers produce nothing, and every event is marked as
// replayed and left instance-free for the caller to stamp.
func TestListHistoryMapsOpenCodeMessages(t *testing.T) {
	adapter, _, _ := newAdapterWithServer(t, queueHandler(map[string]http.HandlerFunc{
		"GET /api/session/ses_1/message": serveMessagesFixture(t),
	}))
	instance := startTestInstance(t, adapter)

	result, err := adapter.ListHistory(context.Background(), agents.ListHistoryRequest{
		Directory: instance.Directory,
		SessionID: "ses_1",
	})
	if err != nil {
		t.Fatalf("ListHistory: %v", err)
	}
	events := result.Events
	if len(events) != 7 {
		t.Fatalf("ListHistory returned %d events, want 7: %+v", len(events), events)
	}

	if events[0].Type != agents.EventMessage || events[0].Message == nil ||
		events[0].Message.Role != "user" || events[0].Message.Text != "历史里的用户消息" || !events[0].Message.Final {
		t.Errorf("events[0] = %+v, want the user message", events[0])
	}
	// The reasoning block arrives before the reply it belongs to.
	if events[1].Type != agents.EventThinking || events[1].Thinking == nil ||
		events[1].Thinking.Text != "这段推理必须被跳过" || !events[1].Thinking.Final {
		t.Errorf("events[1] = %+v, want the replayed reasoning block", events[1])
	}
	if events[2].Type != agents.EventMessage || events[2].Message == nil ||
		events[2].Message.Role != "assistant" || events[2].Message.Text != "历史里的助手回复" {
		t.Errorf("events[2] = %+v, want the assistant message", events[2])
	}
	// The replayed assistant message carries the model that produced it, in
	// the platform's provider/model form.
	if events[2].Message == nil || events[2].Message.Model == nil ||
		events[2].Message.Model.ID != "acme/acme-code" ||
		events[2].Message.Model.Provider != "acme" ||
		events[2].Message.Model.Variant != "default" {
		t.Errorf("events[2].Message.Model = %+v, want acme/acme-code (acme, default)", events[2].Message.Model)
	}
	// A user message never carries a model.
	if events[0].Message != nil && events[0].Message.Model != nil {
		t.Errorf("events[0].Message.Model = %+v, want none on a user message", events[0].Message.Model)
	}
	if events[3].Type != agents.EventToolCall || events[3].ToolCall == nil ||
		events[3].ToolCall.CallID != "call_1" || events[3].ToolCall.Name != "execute" {
		t.Fatalf("events[3] = %+v, want the first tool call", events[3])
	}
	var input map[string]string
	if err := json.Unmarshal(events[3].ToolCall.Input, &input); err != nil || input["code"] != "ls -la" {
		t.Errorf("first tool call input = %s, want the fixture state.input", events[3].ToolCall.Input)
	}
	if events[4].Type != agents.EventToolResult || events[4].ToolResult == nil ||
		events[4].ToolResult.CallID != "call_1" || events[4].ToolResult.Output != "历史里的工具输出" || events[4].ToolResult.IsError {
		t.Errorf("events[4] = %+v, want the completed tool result", events[4])
	}
	if events[5].Type != agents.EventToolCall || events[5].ToolCall == nil || events[5].ToolCall.CallID != "call_2" {
		t.Fatalf("events[5] = %+v, want the second tool call", events[5])
	}
	if events[6].Type != agents.EventToolResult || events[6].ToolResult == nil ||
		events[6].ToolResult.CallID != "call_2" || !events[6].ToolResult.IsError {
		t.Errorf("events[6] = %+v, want a failed tool result for call_2", events[6])
	}
	// A failed block without content falls back to its structured error so
	// the replay never renders an empty result.
	if events[6].ToolResult == nil || events[6].ToolResult.Output != "工具失败原因" {
		t.Errorf("events[6] = %+v, want the structured error message", events[6])
	}

	for i, event := range events {
		if !event.Replayed {
			t.Errorf("events[%d].Replayed = false, want every history event marked replayed", i)
		}
		if event.InstanceID != "" {
			t.Errorf("events[%d].InstanceID = %q, want empty: the request names no instance", i, event.InstanceID)
		}
		if event.Backend != agents.BackendOpenCodeV2 {
			t.Errorf("events[%d].Backend = %q, want opencode-v2", i, event.Backend)
		}
		if event.Sequence != uint64(i+1) {
			t.Errorf("events[%d].Sequence = %d, want %d", i, event.Sequence, i+1)
		}
	}
}

// TestListHistoryKeepsSubagentResult pins the reopen gap: a subagent tool
// block carries its report in state.content (there is no `output` field on
// the wire), and the replayed result must preserve it verbatim.
func TestListHistoryKeepsSubagentResult(t *testing.T) {
	block := ContentBlock{
		Type: "tool",
		ID:   "call_sub",
		Name: "subagent",
		State: &ToolState{
			Status:  "completed",
			Input:   json.RawMessage(`{"agent":"explore","description":"desc","prompt":"do it"}`),
			Content: json.RawMessage(`[{"type":"text","text":"<subagent sessionID=\"ses_child\" state=\"completed\">\n# report"}]`),
		},
	}
	events := normalizeHistoryMessage(Message{ID: "msg_1", Type: "assistant", Content: []ContentBlock{block}})
	if len(events) != 2 {
		t.Fatalf("normalizeHistoryMessage returned %d events, want 2", len(events))
	}
	result := events[1].ToolResult
	if result == nil || result.IsError {
		t.Fatalf("result = %+v, want a successful result", result)
	}
	if !strings.Contains(result.Output, "# report") || !strings.Contains(result.Output, "ses_child") {
		t.Errorf("result output = %q, want the subagent report preserved", result.Output)
	}
}

// TestListHistoryWithoutRunningInstance pins the actionable failure: with no
// server there is nothing to read from, and the error must say to start one
// rather than looking like a missing session.
func TestListHistoryWithoutRunningInstance(t *testing.T) {
	adapter, _, _ := newAdapterWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("no request expected without a running instance, got %s %s", r.Method, r.URL.Path)
	})

	_, err := adapter.ListHistory(context.Background(), agents.ListHistoryRequest{
		Directory: t.TempDir(),
		SessionID: "ses_1",
	})
	if err == nil {
		t.Fatal("ListHistory without a running instance succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "start") {
		t.Errorf("ListHistory error = %q, want guidance to start the backend", err)
	}
}

// TestListHistoryReadsThroughTheSharedServiceForAnyDirectory pins the
// instance-free contract that survives the machine-wide service: history is
// scoped by directory and session id, so it reads through the adopted service
// both for a directory the platform started an instance in and for one it never
// did. There is no per-instance server left to choose between — the platform
// never starts one.
func TestListHistoryReadsThroughTheSharedServiceForAnyDirectory(t *testing.T) {
	fixture := readMessagesFixture(t)
	adapter, _, _ := newAdapterWithServer(t, queueHandler(map[string]http.HandlerFunc{
		"GET /api/session/ses_1/message": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("content-type", "application/json")
			_, _ = w.Write(fixture)
		},
	}))

	dirA := t.TempDir()
	if _, err := adapter.StartInstance(context.Background(), agents.StartInstanceRequest{Directory: dirA}); err != nil {
		t.Fatalf("StartInstance: %v", err)
	}

	for _, directory := range []string{dirA, t.TempDir()} {
		result, err := adapter.ListHistory(context.Background(), agents.ListHistoryRequest{Directory: directory, SessionID: "ses_1"})
		if err != nil {
			t.Fatalf("ListHistory for %s: %v", directory, err)
		}
		if len(result.Events) == 0 {
			t.Fatalf("ListHistory for %s returned nothing through the shared service", directory)
		}
	}
}

// TestAdapterSubscribeStreamsReasoning pins that the reasoning event family
// becomes its own event kind rather than being folded into the reply: deltas
// stream non-final and the block closes with an empty final terminator, exactly
// like the text family, and the text stream is unaffected.
func TestAdapterSubscribeStreamsReasoning(t *testing.T) {
	events := strings.Join([]string{
		sseEvent("evt_1", "session.reasoning.started", `{"sessionID":"ses_1","assistantMessageID":"msg_1","ordinal":1,"reasoningID":"rs_1"}`, "/x"),
		sseEvent("evt_2", "session.reasoning.delta", `{"sessionID":"ses_1","assistantMessageID":"msg_1","reasoningID":"rs_1","delta":"先看"}`, "/x"),
		sseEvent("evt_3", "session.reasoning.delta", `{"sessionID":"ses_1","assistantMessageID":"msg_1","reasoningID":"rs_1","delta":"再看"}`, "/x"),
		sseEvent("evt_4", "session.reasoning.ended", `{"sessionID":"ses_1","assistantMessageID":"msg_1","reasoningID":"rs_1","text":"先看再看"}`, "/x"),
		sseEvent("evt_5", "session.text.delta", `{"sessionID":"ses_1","assistantMessageID":"msg_1","ordinal":1,"delta":"答案"}`, "/x"),
		sseEvent("evt_6", "session.text.ended", `{"sessionID":"ses_1","assistantMessageID":"msg_1","ordinal":1,"text":"答案"}`, "/x"),
	}, "")

	adapter, _, _ := newAdapterWithServer(t, queueHandler(map[string]http.HandlerFunc{
		"POST /api/session": func(w http.ResponseWriter, r *http.Request) {
			io.WriteString(w, `{"data":{"id":"ses_1","projectID":"prj","title":"Reasoning","time":{"created":1,"updated":1}}}`)
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
	stream, err := adapter.Subscribe(ctx, instance.ID)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer adapter.StopInstance(context.Background(), instance.ID)

	got := collectEvents(t, stream, 5)

	for i, want := range map[int]string{0: "先看", 1: "再看"} {
		event := got[i]
		if event.Type != agents.EventThinking || event.Thinking == nil ||
			event.Thinking.Text != want || event.Thinking.Final {
			t.Errorf("event[%d] = %+v, want a non-final thinking chunk %q", i, event, want)
		}
	}
	if got[2].Type != agents.EventThinking || got[2].Thinking == nil || got[2].Thinking.Text != "" || !got[2].Thinking.Final {
		t.Errorf("event[2] = %+v, want an empty final thinking terminator", got[2])
	}
	if got[3].Type != agents.EventMessage || got[3].Message == nil || got[3].Message.Text != "答案" || got[3].Message.Final {
		t.Errorf("event[3] = %+v, want the streamed text delta", got[3])
	}
	if got[4].Type != agents.EventMessage || got[4].Message == nil || got[4].Message.Text != "" || !got[4].Message.Final {
		t.Errorf("event[4] = %+v, want the text terminator", got[4])
	}
}

// TestAdapterSubscribeReportsReasoningWithoutDeltas pins the non-streaming
// shape: a single ended event with text and no preceding delta is carried
// whole rather than dropped.
func TestAdapterSubscribeReportsReasoningWithoutDeltas(t *testing.T) {
	events := sseEvent("evt_1", "session.reasoning.ended",
		`{"sessionID":"ses_1","assistantMessageID":"msg_1","reasoningID":"rs_1","text":"完整推理"}`, "/x")

	adapter, _, _ := newAdapterWithServer(t, queueHandler(map[string]http.HandlerFunc{
		"POST /api/session": func(w http.ResponseWriter, r *http.Request) {
			io.WriteString(w, `{"data":{"id":"ses_1","projectID":"prj","title":"R","time":{"created":1,"updated":1}}}`)
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
	stream, err := adapter.Subscribe(ctx, instance.ID)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer adapter.StopInstance(context.Background(), instance.ID)

	got := collectEvents(t, stream, 1)

	if got[0].Type != agents.EventThinking || got[0].Thinking == nil ||
		got[0].Thinking.Text != "完整推理" || !got[0].Thinking.Final {
		t.Errorf("event[0] = %+v, want the whole reasoning block", got[0])
	}
}

// TestExecPathPrefersResolveExecutable verifies the runtime override wins while
// a nil or empty resolver falls back to the configured executable.
func TestExecPathPrefersResolveExecutable(t *testing.T) {
	override := New(Options{Executable: "fallback", ResolveExecutable: func() string { return "/custom/opencode" }})
	if got := override.execPath(); got != "/custom/opencode" {
		t.Errorf("execPath() = %q, want /custom/opencode", got)
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
