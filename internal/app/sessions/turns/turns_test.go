package turns

import (
	"context"
	"testing"
	"time"

	"devplatform/internal/core/agents"
)

// fakeSource hands the tracker one scripted event stream per instance.
type fakeSource struct {
	events chan agents.NormalizedEvent
}

func (f *fakeSource) Subscribe(context.Context, string) (<-chan agents.NormalizedEvent, error) {
	return f.events, nil
}

func turnEvent(sessionID string, phase agents.TurnPhase) agents.NormalizedEvent {
	return agents.NormalizedEvent{
		Type:      agents.EventTurn,
		SessionID: sessionID,
		Backend:   agents.BackendPi,
		Turn:      &agents.TurnPayload{Phase: phase},
	}
}

func TestFromEventMapsOnlyTypedTurnBoundaries(t *testing.T) {
	if state, ok := FromEvent(turnEvent("s1", agents.TurnStarted)); !ok || state != Running {
		t.Errorf("turn started = (%q, %v), want (running, true)", state, ok)
	}
	if state, ok := FromEvent(turnEvent("s1", agents.TurnEnded)); !ok || state != Idle {
		t.Errorf("turn ended = (%q, %v), want (idle, true)", state, ok)
	}
	if _, ok := FromEvent(agents.NormalizedEvent{Type: agents.EventMessage}); ok {
		t.Error("a non-turn event changed the turn state")
	}
}

func TestTrackerFollowsTurnsAndForgetsSessions(t *testing.T) {
	source := &fakeSource{events: make(chan agents.NormalizedEvent, 4)}
	tracker := NewTracker(source)

	if got := tracker.Of(agents.BackendPi, "s1"); got != Idle {
		t.Fatalf("untracked session = %q, want idle", got)
	}

	tracker.Ensure("inst")
	source.events <- turnEvent("s1", agents.TurnStarted)
	waitFor(t, func() bool { return tracker.Of(agents.BackendPi, "s1") == Running })

	tracker.Forget(agents.BackendPi, "s1")
	if got := tracker.Of(agents.BackendPi, "s1"); got != Idle {
		t.Errorf("forgotten session = %q, want idle", got)
	}

	close(source.events)
	// Once the stream closes the instance's records are dropped with it.
	waitFor(t, func() bool { return tracker.Of(agents.BackendPi, "s1") == Idle })
}

// waitFor polls until cond holds or the test times out.
func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition not reached in time")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestTrackerSetManuallyUpdatesState(t *testing.T) {
	source := &fakeSource{events: make(chan agents.NormalizedEvent, 4)}
	tracker := NewTracker(source)

	if got := tracker.Of(agents.BackendPi, "s1"); got != Idle {
		t.Fatalf("untracked session = %q, want idle", got)
	}

	tracker.Set(agents.BackendPi, "s1", Running)
	if got := tracker.Of(agents.BackendPi, "s1"); got != Running {
		t.Errorf("tracker.Of after Set = %q, want running", got)
	}

	tracker.Set(agents.BackendPi, "s1", Idle)
	if got := tracker.Of(agents.BackendPi, "s1"); got != Idle {
		t.Errorf("tracker.Of after Set idle = %q, want idle", got)
	}
}

