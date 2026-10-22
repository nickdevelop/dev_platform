package deepseek

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"devplatform/internal/core/agents"
)

type recordedCall struct {
	method string
	args   any
}

type interactionMock struct {
	server *httptest.Server
	client *Client
	events chan map[string]any

	mu    sync.Mutex
	calls []recordedCall
}

func newInteractionMock(t *testing.T) *interactionMock {
	t.Helper()

	mock := &interactionMock{events: make(chan map[string]any, 16)}
	mock.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/remote.mux" {
			mock.serveWS(t, w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			mock.serveRPC(w, r)
			return
		}
		http.NotFound(w, r)
	}))

	mock.client = NewClient(mock.server.URL, "dsh-auth=test")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := mock.client.ConnectMux(ctx); err != nil {
		t.Fatalf("connect mock mux: %v", err)
	}

	t.Cleanup(func() {
		mock.server.Close()
		_ = mock.client.Close()
	})
	return mock
}

func (m *interactionMock) serveWS(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	done := make(chan struct{})
	defer close(done)

	var writeMu sync.Mutex
	write := func(value any) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		return conn.WriteJSON(value)
	}

	for {
		var msg wsClientMessage
		if err := conn.ReadJSON(&msg); err != nil {
			return
		}
		if msg.Type != "open" || msg.Endpoint != "$events" {
			continue
		}
		if err := write(wsServerMessage{
			Type:     "item",
			StreamID: msg.StreamID,
			Value:    json.RawMessage(`{"type":"ready","clientId":"client-test-1"}`),
		}); err != nil {
			return
		}
		go func(streamID string) {
			for {
				select {
				case event := <-m.events:
					encoded, _ := json.Marshal(event)
					if err := write(wsServerMessage{Type: "item", StreamID: streamID, Value: encoded}); err != nil {
						return
					}
				case <-done:
					return
				}
			}
		}(msg.StreamID)
	}
}

func (m *interactionMock) serveRPC(w http.ResponseWriter, r *http.Request) {
	method := strings.TrimPrefix(r.URL.Path, "/api/")
	var request rpcRequest
	_ = json.NewDecoder(r.Body).Decode(&request)

	m.mu.Lock()
	m.calls = append(m.calls, recordedCall{method: method, args: request.Payload.Args})
	m.mu.Unlock()

	var value any = map[string]any{"ok": true}
	switch method {
	case "session/list":
		value = map[string]any{
			"items": []any{
				map[string]any{
					"sessionId":      "ses-1",
					"cwd":            "/work/proj",
					"blank":          false,
					"origin":         "root",
					"running":        false,
					"agentAvailable": true,
				},
			},
		}
	case "session/projections":
		value = map[string]any{"values": map[string]any{"title": "Session"}}
	case "session/create":
		value = map[string]any{"sessionId": "ses-1"}
	}

	resp, _ := json.Marshal(rpcResponse{
		Type:  "server-response",
		RPCID: request.RPCID,
		Result: rpcResult{
			OK:    true,
			Value: mustJSON(value),
		},
	})
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(resp)
}

func (m *interactionMock) push(event map[string]any) {
	m.events <- event
}

func (m *interactionMock) lastCall(method string) (any, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for index := len(m.calls) - 1; index >= 0; index-- {
		if m.calls[index].method == method {
			return m.calls[index].args, true
		}
	}
	return nil, false
}

func setupInteractionAdapter(t *testing.T) (*interactionMock, *Adapter, agents.SessionRef, <-chan agents.NormalizedEvent) {
	t.Helper()

	mock := newInteractionMock(t)
	adapter := New(Options{
		ProcessLauncher: func(context.Context) (*Process, error) {
			return &Process{client: mock.client, exitDone: make(chan struct{})}, nil
		},
	})

	ctx := context.Background()
	instance, err := adapter.StartInstance(ctx, agents.StartInstanceRequest{Directory: "/work/proj"})
	if err != nil {
		t.Fatalf("StartInstance: %v", err)
	}
	if _, err := adapter.ListSessions(ctx, instance.ID); err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	events, err := adapter.Subscribe(ctx, instance.ID)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	ref := agents.SessionRef{
		InstanceID: instance.ID,
		Backend:    agents.BackendDeepSeek,
		SessionID:  "ses-1",
	}
	return mock, adapter, ref, events
}

func awaitInteraction(t *testing.T, events <-chan agents.NormalizedEvent, phase agents.InteractionPhase) agents.NormalizedEvent {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		select {
		case event, ok := <-events:
			if !ok {
				t.Fatal("interaction event channel closed")
			}
			if event.Type == agents.EventInteraction && event.Interaction != nil && event.Interaction.Phase == phase {
				return event
			}
		case <-deadline:
			t.Fatalf("timed out waiting for %s interaction", phase)
		}
	}
}

func TestUserQuestionInteraction(t *testing.T) {
	mock, adapter, ref, events := setupInteractionAdapter(t)

	mock.push(map[string]any{
		"type":    "waterfall",
		"event":   "user-questions/request",
		"eventId": "evt-q1",
		"agentId": "ses-1",
		"request": map[string]any{
			"questions": []any{
				map[string]any{
					"id":       "q1",
					"question": "Which option?",
					"header":   "Choice",
					"options": []any{
						map[string]any{"label": "A", "description": "first"},
						map[string]any{"label": "B"},
					},
				},
				map[string]any{
					"id":       "q2",
					"question": "Explain why",
				},
			},
		},
	})

	requested := awaitInteraction(t, events, agents.InteractionRequested)
	if requested.Interaction.Kind != agents.InteractionForm {
		t.Fatalf("kind = %s, want form", requested.Interaction.Kind)
	}
	if requested.SessionID != "ses-1" || requested.InstanceID != ref.InstanceID {
		t.Fatalf("event identity = %s/%s, want ses-1/%s", requested.InstanceID, requested.SessionID, ref.InstanceID)
	}
	form := requested.Interaction.Form
	if form == nil || form.ID != "evt-q1" || form.SessionID != "ses-1" || form.Title != "Choice" {
		t.Fatalf("form = %+v", form)
	}

	var fields []formField
	if err := json.Unmarshal(form.Fields, &fields); err != nil {
		t.Fatalf("decode fields: %v", err)
	}
	if len(fields) != 2 {
		t.Fatalf("fields = %d, want 2", len(fields))
	}
	if fields[0].Key != "q1" || fields[0].Type != "string" || len(fields[0].Options) != 2 || !fields[0].Custom {
		t.Errorf("q1 field = %+v", fields[0])
	}
	if fields[1].Key != "q2" || fields[1].Type != "string" || len(fields[1].Options) != 0 {
		t.Errorf("q2 field = %+v", fields[1])
	}

	forms, err := adapter.ListForms(context.Background(), ref)
	if err != nil || len(forms) != 1 || forms[0].ID != "evt-q1" {
		t.Fatalf("ListForms = %+v, err = %v", forms, err)
	}

	if err := adapter.ReplyForm(context.Background(), ref, "evt-q1", map[string]any{
		"q1": "B",
		"q2": "because it is safer",
	}); err != nil {
		t.Fatalf("ReplyForm: %v", err)
	}

	args, ok := mock.lastCall("$events/result")
	if !ok {
		t.Fatal("no $events/result call recorded")
	}
	var payload struct {
		ClientID string `json:"clientId"`
		EventID  string `json:"eventId"`
		Outcome  struct {
			Kind  string `json:"kind"`
			Value struct {
				Answers []struct {
					ID       string   `json:"id"`
					Selected []string `json:"selected"`
					Custom   string   `json:"custom"`
				} `json:"answers"`
			} `json:"value"`
		} `json:"outcome"`
	}
	encoded, _ := json.Marshal(args)
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatalf("decode result payload: %v", err)
	}
	if payload.ClientID != "client-test-1" || payload.EventID != "evt-q1" || payload.Outcome.Kind != "result" {
		t.Fatalf("result envelope = %+v", payload)
	}
	if len(payload.Outcome.Value.Answers) != 2 {
		t.Fatalf("answers = %d, want 2", len(payload.Outcome.Value.Answers))
	}
	if got := payload.Outcome.Value.Answers[0]; got.ID != "q1" || len(got.Selected) != 1 || got.Selected[0] != "B" || got.Custom != "" {
		t.Errorf("q1 answer = %+v", got)
	}
	if got := payload.Outcome.Value.Answers[1]; got.ID != "q2" || len(got.Selected) != 0 || got.Custom != "because it is safer" {
		t.Errorf("q2 answer = %+v", got)
	}

	awaitInteraction(t, events, agents.InteractionResolved)
	if forms, _ := adapter.ListForms(context.Background(), ref); len(forms) != 0 {
		t.Fatalf("forms after reply = %+v, want none", forms)
	}
	if err := adapter.ReplyForm(context.Background(), ref, "evt-q1", map[string]any{"q1": "A"}); !errors.Is(err, agents.ErrReplyExpired) {
		t.Fatalf("second ReplyForm error = %v, want ErrReplyExpired", err)
	}
}

func TestApprovalInteraction(t *testing.T) {
	mock, adapter, ref, events := setupInteractionAdapter(t)

	mock.push(map[string]any{
		"type":    "waterfall",
		"event":   "approval/request",
		"eventId": "evt-a1",
		"agentId": "ses-1",
		"request": map[string]any{
			"toolName":      "bash",
			"callId":        "call-1",
			"displayReason": map[string]any{"zh": "需要访问网络"},
		},
	})

	requested := awaitInteraction(t, events, agents.InteractionRequested)
	if requested.Interaction.Kind != agents.InteractionPermission {
		t.Fatalf("kind = %s, want permission", requested.Interaction.Kind)
	}
	permission := requested.Interaction.Permission
	if permission == nil || permission.ID != "evt-a1" || permission.Action != "bash" || permission.Message != "需要访问网络" {
		t.Fatalf("permission = %+v", permission)
	}
	if len(permission.Resources) != 1 || permission.Resources[0] != "call-1" {
		t.Errorf("resources = %+v", permission.Resources)
	}
	permissions, err := adapter.ListPermissions(context.Background(), ref)
	if err != nil || len(permissions) != 1 || permissions[0].ID != "evt-a1" {
		t.Fatalf("ListPermissions = %+v, err = %v", permissions, err)
	}

	if err := adapter.ReplyPermission(context.Background(), ref, "evt-a1", "once", ""); err != nil {
		t.Fatalf("ReplyPermission: %v", err)
	}
	args, _ := mock.lastCall("$events/result")
	if got := resultValue(t, args); got != "allowed-once" {
		t.Fatalf("permission outcome = %#v, want allowed-once", got)
	}
	awaitInteraction(t, events, agents.InteractionResolved)

	mock.push(map[string]any{
		"type":    "waterfall",
		"event":   "approval/request",
		"eventId": "evt-a2",
		"agentId": "ses-1",
		"request": map[string]any{"toolName": "bash", "reason": "writes outside the workspace"},
	})
	awaitInteraction(t, events, agents.InteractionRequested)
	if err := adapter.ReplyPermission(context.Background(), ref, "evt-a2", "reject", ""); err != nil {
		t.Fatalf("ReplyPermission reject: %v", err)
	}
	args, _ = mock.lastCall("$events/result")
	if got := resultValue(t, args); got != "rejected" {
		t.Fatalf("permission outcome = %#v, want rejected", got)
	}
	awaitInteraction(t, events, agents.InteractionResolved)
}

func TestInteractionCancelAndStreamClose(t *testing.T) {
	mock, adapter, ref, events := setupInteractionAdapter(t)

	mock.push(map[string]any{
		"type":    "waterfall",
		"event":   "user-questions/request",
		"eventId": "evt-cancel",
		"agentId": "ses-1",
		"request": map[string]any{
			"questions": []any{map[string]any{"id": "q1", "question": "Continue?"}},
		},
	})
	awaitInteraction(t, events, agents.InteractionRequested)

	mock.push(map[string]any{"type": "cancel", "eventId": "evt-cancel"})
	awaitInteraction(t, events, agents.InteractionResolved)
	if forms, _ := adapter.ListForms(context.Background(), ref); len(forms) != 0 {
		t.Fatalf("forms after cancel = %+v, want none", forms)
	}

	mock.push(map[string]any{
		"type":    "waterfall",
		"event":   "user-questions/request",
		"eventId": "evt-close",
		"agentId": "ses-1",
		"request": map[string]any{
			"questions": []any{map[string]any{"id": "q1", "question": "Still there?"}},
		},
	})
	awaitInteraction(t, events, agents.InteractionRequested)
	adapter.handleClientClosed(mock.client)
	awaitInteraction(t, events, agents.InteractionResolved)
	if forms, _ := adapter.ListForms(context.Background(), ref); len(forms) != 0 {
		t.Fatalf("forms after stream close = %+v, want none", forms)
	}
}

func resultValue(t *testing.T, args any) any {
	t.Helper()
	encoded, _ := json.Marshal(args)
	var payload struct {
		Outcome struct {
			Value any `json:"value"`
		} `json:"outcome"`
	}
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatalf("decode result payload: %v", err)
	}
	return payload.Outcome.Value
}
