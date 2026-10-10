package sessions

import (
	"context"
	"testing"
	"time"

	"devplatform/internal/agents"
)

func lifecycleFor(instanceID, nativeID, state string) agents.NormalizedEvent {
	return agents.NormalizedEvent{
		Type:       agents.EventLifecycle,
		InstanceID: instanceID,
		SessionID:  nativeID,
		Lifecycle:  &agents.LifecycleEvent{State: state},
	}
}

func waitForTurn(t *testing.T, svc *Service, projectID uint, sessionID string, want TurnState) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		got, err := svc.Get(projectID, sessionID)
		if err != nil {
			t.Fatalf("Get(%s): %v", sessionID, err)
		}
		if got.Turn == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("session %s turn = %q, want %q", sessionID, got.Turn, want)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestTurnTracksOnlyTheSessionWhoseTurnChanged(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	env.fake.createRef = &agents.SessionRef{InstanceID: env.instanceID, Backend: agents.BackendPi, SessionID: "native-busy"}
	busy, err := env.svc.Create(ctx, env.projectID, agents.BackendPi, "busy")
	if err != nil {
		t.Fatal(err)
	}
	env.fake.createRef = &agents.SessionRef{InstanceID: env.instanceID, Backend: agents.BackendPi, SessionID: "native-other"}
	other, err := env.svc.Create(ctx, env.projectID, agents.BackendPi, "other")
	if err != nil {
		t.Fatal(err)
	}
	if busy.Turn != TurnIdle {
		t.Fatalf("new session turn = %q, want idle", busy.Turn)
	}

	env.fake.emit(t, env.instanceID, lifecycleFor(env.instanceID, busy.NativeSessionID, "execution_started"))
	waitForTurn(t, env.svc, env.projectID, busy.ID, TurnRunning)
	waitForTurn(t, env.svc, env.projectID, other.ID, TurnIdle)

	env.fake.emit(t, env.instanceID, lifecycleFor(env.instanceID, busy.NativeSessionID, "execution_succeeded"))
	waitForTurn(t, env.svc, env.projectID, busy.ID, TurnIdle)
}

func TestTurnEndsOnFailureAndProcessExit(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	env.fake.createRef = &agents.SessionRef{InstanceID: env.instanceID, Backend: agents.BackendPi, SessionID: "native-failing"}
	session, err := env.svc.Create(ctx, env.projectID, agents.BackendPi, "failing")
	if err != nil {
		t.Fatal(err)
	}

	env.fake.emit(t, env.instanceID, lifecycleFor(env.instanceID, session.NativeSessionID, "agent_start"))
	waitForTurn(t, env.svc, env.projectID, session.ID, TurnRunning)

	failed := agents.NormalizedEvent{
		Type:       agents.EventError,
		InstanceID: env.instanceID,
		SessionID:  session.NativeSessionID,
		Error:      &agents.ErrorPayload{Code: "execution_failed", Message: "boom"},
	}
	env.fake.emit(t, env.instanceID, failed)
	waitForTurn(t, env.svc, env.projectID, session.ID, TurnIdle)

	env.fake.emit(t, env.instanceID, lifecycleFor(env.instanceID, session.NativeSessionID, "agent_start"))
	waitForTurn(t, env.svc, env.projectID, session.ID, TurnRunning)

	exited := agents.NormalizedEvent{
		Type:       agents.EventError,
		InstanceID: env.instanceID,
		SessionID:  session.NativeSessionID,
		Error:      &agents.ErrorPayload{Code: "process_exit", Message: "exit status 1"},
	}
	env.fake.emit(t, env.instanceID, exited)
	waitForTurn(t, env.svc, env.projectID, session.ID, TurnIdle)
}
