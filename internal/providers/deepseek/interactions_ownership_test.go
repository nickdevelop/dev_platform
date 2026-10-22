package deepseek

import (
	"context"
	"encoding/json"
	"testing"

	"devplatform/internal/core/agents"
)

// interactionOptions builds an adapter bound to one mock dsh host. Every
// instance of this adapter shares that host, mirroring production, where all
// projects run against a single dsh process.
func interactionOptions(mock *interactionMock) Options {
	return Options{
		ProcessLauncher: func(context.Context) (*Process, error) {
			return &Process{client: mock.client, exitDone: make(chan struct{})}, nil
		},
	}
}

// startSubscribed starts one platform instance for dir and subscribes to its
// normalized event stream. Subscribe lists the host-wide session roster in the
// background; callers that depend on that listing use mock.waitCalls.
func startSubscribed(t *testing.T, adapter *Adapter, dir string) (*agents.Instance, <-chan agents.NormalizedEvent) {
	t.Helper()

	ctx := context.Background()
	inst, err := adapter.StartInstance(ctx, agents.StartInstanceRequest{Directory: dir})
	if err != nil {
		t.Fatalf("StartInstance(%s): %v", dir, err)
	}
	events, err := adapter.Subscribe(ctx, inst.ID)
	if err != nil {
		t.Fatalf("Subscribe(%s): %v", dir, err)
	}
	return inst, events
}

func pushQuestion(mock *interactionMock, eventID, agentID string) {
	mock.push(map[string]any{
		"type":    "waterfall",
		"event":   "user-questions/request",
		"eventId": eventID,
		"agentId": agentID,
		"request": map[string]any{
			"questions": []any{
				map[string]any{
					"id":       "q1",
					"question": "Which option?",
					"header":   "Choice",
					"options": []any{
						map[string]any{"label": "A"},
						map[string]any{"label": "B"},
					},
				},
			},
		},
	})
}

// TestQuestionReachesMatchingInstanceRegardlessOfSubscribeOrder is the
// regression test for the shared-host ownership bug: every instance subscribes
// to the same host-wide session roster, and the last subscriber must not steal
// another directory's session. A question raised by ses-a has to reach the
// subscribers of the instance that owns /work/a even though /work/b subscribed
// last.
func TestQuestionReachesMatchingInstanceRegardlessOfSubscribeOrder(t *testing.T) {
	mock := newInteractionMock(t)
	mock.setSessions([]any{
		map[string]any{"sessionId": "ses-a", "cwd": "/work/a", "origin": "root"},
		map[string]any{"sessionId": "ses-b", "cwd": "/work/b", "origin": "root"},
	})
	adapter := New(interactionOptions(mock))

	instA, eventsA := startSubscribed(t, adapter, "/work/a")
	mock.waitCalls(t, "session/list", 1)
	_, _ = startSubscribed(t, adapter, "/work/b")
	mock.waitCalls(t, "session/list", 2)

	pushQuestion(mock, "evt-a", "ses-a")

	requested := awaitInteraction(t, eventsA, agents.InteractionRequested)
	if requested.InstanceID != instA.ID {
		t.Fatalf("event instance = %q, want %q", requested.InstanceID, instA.ID)
	}
	if requested.SessionID != "ses-a" {
		t.Fatalf("event session = %q, want ses-a", requested.SessionID)
	}
	if requested.Interaction.Kind != agents.InteractionForm || requested.Interaction.Form == nil {
		t.Fatalf("interaction = %+v, want a form", requested.Interaction)
	}
	if requested.Interaction.Form.SessionID != "ses-a" {
		t.Fatalf("form session = %q, want ses-a", requested.Interaction.Form.SessionID)
	}

	forms, err := adapter.ListForms(context.Background(), agents.SessionRef{
		InstanceID: instA.ID,
		Backend:    agents.BackendDeepSeek,
		SessionID:  "ses-a",
	})
	if err != nil || len(forms) != 1 || forms[0].ID != "evt-a" {
		t.Fatalf("ListForms = %+v, err = %v, want the pending form", forms, err)
	}

	var fields []formField
	if err := json.Unmarshal(forms[0].Fields, &fields); err != nil {
		t.Fatalf("decode fields: %v", err)
	}
	if len(fields) != 1 || fields[0].Key != "q1" {
		t.Fatalf("fields = %+v, want q1", fields)
	}
}

// TestQuestionFromSubagentReachesRootConversation verifies that a session
// listed outside the instance directory still records its native parent, so an
// interaction surfaced through a child resolves to the root session the
// workbench subscribed.
func TestQuestionFromSubagentReachesRootConversation(t *testing.T) {
	mock := newInteractionMock(t)
	mock.setSessions([]any{
		map[string]any{"sessionId": "ses-root", "cwd": "/work/a", "origin": "root"},
		map[string]any{
			"sessionId":       "ses-child",
			"cwd":             "/work/elsewhere",
			"origin":          "subagent",
			"parentSessionId": "ses-root",
		},
	})
	adapter := New(interactionOptions(mock))

	inst, events := startSubscribed(t, adapter, "/work/a")
	mock.waitCalls(t, "session/list", 1)

	pushQuestion(mock, "evt-child", "ses-child")

	requested := awaitInteraction(t, events, agents.InteractionRequested)
	if requested.InstanceID != inst.ID {
		t.Fatalf("event instance = %q, want %q", requested.InstanceID, inst.ID)
	}
	if requested.SessionID != "ses-root" {
		t.Fatalf("event session = %q, want the root session ses-root", requested.SessionID)
	}
	if requested.Interaction.Form == nil || requested.Interaction.Form.SessionID != "ses-root" {
		t.Fatalf("form = %+v, want it registered against ses-root", requested.Interaction.Form)
	}
}

// TestCreatedSessionQuestionIsDelivered covers sessions the platform creates
// itself: interaction delivery must not depend on a discovery listing having
// claimed them first.
func TestCreatedSessionQuestionIsDelivered(t *testing.T) {
	mock := newInteractionMock(t)
	mock.setSessions(nil)
	mock.setCreatedSessions("ses-created")
	adapter := New(interactionOptions(mock))

	inst, events := startSubscribed(t, adapter, "/work/a")
	mock.waitCalls(t, "session/list", 1)

	ref, err := adapter.CreateSession(context.Background(), inst.ID, agents.CreateSessionOptions{})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if ref.SessionID != "ses-created" {
		t.Fatalf("created session = %q, want ses-created", ref.SessionID)
	}

	pushQuestion(mock, "evt-created", ref.SessionID)

	requested := awaitInteraction(t, events, agents.InteractionRequested)
	if requested.InstanceID != inst.ID {
		t.Fatalf("event instance = %q, want %q", requested.InstanceID, inst.ID)
	}
	if requested.SessionID != ref.SessionID {
		t.Fatalf("event session = %q, want %q", requested.SessionID, ref.SessionID)
	}
}
