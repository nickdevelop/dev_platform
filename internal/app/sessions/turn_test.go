package sessions

import (
	"context"
	"testing"
	"time"

	"devplatform/internal/core/agents"
)

func lifecycleFor(instanceID, nativeID, state string) agents.NormalizedEvent {
	return agents.NormalizedEvent{
		Type:       agents.EventLifecycle,
		InstanceID: instanceID,
		SessionID:  nativeID,
		Lifecycle:  &agents.LifecycleEvent{State: state},
	}
}

func turnFor(instanceID, nativeID, id string, phase agents.TurnPhase, outcome agents.TurnOutcome) agents.NormalizedEvent {
	return agents.NormalizedEvent{
		Type:       agents.EventTurn,
		InstanceID: instanceID,
		SessionID:  nativeID,
		Turn:       &agents.TurnPayload{ID: id, Phase: phase, Outcome: outcome},
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
	busyOutcome, err := env.svc.Create(ctx, env.projectID, agents.BackendPi, "busy", "", "")
	if err != nil {
		t.Fatal(err)
	}
	busy := busyOutcome.Session
	env.fake.createRef = &agents.SessionRef{InstanceID: env.instanceID, Backend: agents.BackendPi, SessionID: "native-other"}
	otherOutcome, err := env.svc.Create(ctx, env.projectID, agents.BackendPi, "other", "", "")
	if err != nil {
		t.Fatal(err)
	}
	other := otherOutcome.Session
	if busy.Turn != TurnIdle {
		t.Fatalf("new session turn = %q, want idle", busy.Turn)
	}

	env.fake.emit(t, env.instanceID, turnFor(env.instanceID, busy.NativeSessionID, "t1", agents.TurnStarted, ""))
	waitForTurn(t, env.svc, env.projectID, busy.ID, TurnRunning)
	waitForTurn(t, env.svc, env.projectID, other.ID, TurnIdle)

	env.fake.emit(t, env.instanceID, turnFor(env.instanceID, busy.NativeSessionID, "t1", agents.TurnEnded, agents.TurnCompleted))
	waitForTurn(t, env.svc, env.projectID, busy.ID, TurnIdle)
}

func TestTurnEndsOnlyOnATypedTurnBoundary(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	env.fake.createRef = &agents.SessionRef{InstanceID: env.instanceID, Backend: agents.BackendPi, SessionID: "native-failing"}
	sessionOutcome, err := env.svc.Create(ctx, env.projectID, agents.BackendPi, "failing", "", "")
	if err != nil {
		t.Fatal(err)
	}
	session := sessionOutcome.Session

	env.fake.emit(t, env.instanceID, turnFor(env.instanceID, session.NativeSessionID, "t1", agents.TurnStarted, ""))
	waitForTurn(t, env.svc, env.projectID, session.ID, TurnRunning)

	// An error alone describes the failure but does not end the turn: the
	// adapter reports the turn boundary, and the session layer follows it.
	failed := agents.NormalizedEvent{
		Type:       agents.EventError,
		InstanceID: env.instanceID,
		SessionID:  session.NativeSessionID,
		Error:      &agents.ErrorPayload{Code: "execution_failed", Message: "boom"},
	}
	env.fake.emit(t, env.instanceID, failed)
	env.fake.emit(t, env.instanceID, turnFor(env.instanceID, session.NativeSessionID, "t1", agents.TurnEnded, agents.TurnFailed))
	waitForTurn(t, env.svc, env.projectID, session.ID, TurnIdle)

	env.fake.emit(t, env.instanceID, turnFor(env.instanceID, session.NativeSessionID, "t2", agents.TurnStarted, ""))
	waitForTurn(t, env.svc, env.projectID, session.ID, TurnRunning)
}
