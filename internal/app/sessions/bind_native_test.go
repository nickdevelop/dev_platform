package sessions

import (
	"context"
	"testing"

	"devplatform/internal/core/agents"
)

// A session created before its first turn is indexed under a pending id; the
// first turn binds it to the native id the backend assigned.
func TestCreateBindsPendingSessionOnFirstMessage(t *testing.T) {
	env := newTestEnv(t)
	otherProject := env.registerProject(t, "beta")
	env.fake.mu.Lock()
	env.fake.createRef = &agents.SessionRef{Backend: agents.BackendPi, SessionID: "pending-1"}
	env.fake.bindTo = "native-bound"
	env.fake.mu.Unlock()

	created, err := env.svc.Create(context.Background(), otherProject, agents.BackendPi, "t", "hello", "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.MessageError != "" {
		t.Fatalf("MessageError = %q, want none", created.MessageError)
	}
	if created.Session.NativeSessionID != "native-bound" {
		t.Errorf("returned native session = %q, want native-bound", created.Session.NativeSessionID)
	}
	stored, err := env.svc.Get(otherProject, created.Session.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if stored.NativeSessionID != "native-bound" {
		t.Errorf("indexed native session = %q, want native-bound", stored.NativeSessionID)
	}
}

// A pending session created without a message is bound by the first Send.
func TestSendBindsPendingSession(t *testing.T) {
	env := newTestEnv(t)
	otherProject := env.registerProject(t, "beta")
	env.fake.mu.Lock()
	env.fake.createRef = &agents.SessionRef{Backend: agents.BackendPi, SessionID: "pending-2"}
	env.fake.mu.Unlock()

	created, err := env.svc.Create(context.Background(), otherProject, agents.BackendPi, "t", "", "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.Session.NativeSessionID != "pending-2" {
		t.Fatalf("pending native session = %q, want pending-2", created.Session.NativeSessionID)
	}

	env.fake.mu.Lock()
	env.fake.bindTo = "native-first-turn"
	env.fake.mu.Unlock()
	if _, err := env.svc.Send(context.Background(), otherProject, created.Session.ID, "hi", ""); err != nil {
		t.Fatalf("Send: %v", err)
	}

	stored, err := env.svc.Get(otherProject, created.Session.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if stored.NativeSessionID != "native-first-turn" {
		t.Errorf("indexed native session = %q, want native-first-turn", stored.NativeSessionID)
	}
}
