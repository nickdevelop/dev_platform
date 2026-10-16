// Package turns tracks whether a native session is processing one user turn.
//
// The state is derived from the typed turn boundaries on an instance's event
// stream and is never stored: a platform restart leaves every session idle. One
// tracker per instance subscribes to the stream; it stops when the stream
// closes, and its records go with it.
package turns

import (
	"context"
	"sync"

	"devplatform/internal/core/agents"
)

// State reports whether a session is processing one user turn.
type State string

const (
	// Idle means the session is waiting for the next user message.
	Idle State = "idle"
	// Running means the agent is processing a turn of the session.
	Running State = "running"
)

// Subscriber returns an instance's normalized event stream. The instance
// lifecycle service implements it.
type Subscriber interface {
	Subscribe(ctx context.Context, instanceID string) (<-chan agents.NormalizedEvent, error)
}

// SessionKey identifies the native session whose turn is being tracked.
type SessionKey struct {
	Backend  agents.BackendID
	NativeID string
}

// Tracker records the turn state of every native session of every instance it
// watches. It is safe for concurrent use.
type Tracker struct {
	subscriber Subscriber

	// mu guards watching and states. states maps (backend, nativeSessionID)
	// to the session's current turn state; instanceKeys tracks which sessions
	// were observed on an instance so they are cleared when the instance stops.
	mu           sync.Mutex
	watching     map[string]bool
	states       map[SessionKey]State
	instanceKeys map[string]map[SessionKey]bool
}

// NewTracker builds a tracker that subscribes through the given stream source.
func NewTracker(subscriber Subscriber) *Tracker {
	return &Tracker{
		subscriber:   subscriber,
		watching:     make(map[string]bool),
		states:       make(map[SessionKey]State),
		instanceKeys: make(map[string]map[SessionKey]bool),
	}
}

// FromEvent maps one normalized event to the turn state it implies. Only the
// typed turn boundaries change the state; every other event reports false.
func FromEvent(ev agents.NormalizedEvent) (State, bool) {
	if ev.Type != agents.EventTurn || ev.Turn == nil {
		return "", false
	}
	if ev.Turn.Phase == agents.TurnStarted {
		return Running, true
	}
	return Idle, true
}

// Ensure starts the instance's tracker unless one is running. The subscription
// is made synchronously, so events published after Ensure returns are observed;
// it stays subscribed until the instance's stream closes.
func (t *Tracker) Ensure(instanceID string) {
	t.mu.Lock()
	if t.watching[instanceID] {
		t.mu.Unlock()
		return
	}
	t.watching[instanceID] = true
	t.mu.Unlock()

	events, err := t.subscriber.Subscribe(context.Background(), instanceID)
	if err != nil {
		t.mu.Lock()
		delete(t.watching, instanceID)
		t.mu.Unlock()
		return
	}
	go t.track(instanceID, events)
}

// track records the turn state of each native session the instance reports.
// When the instance's stream closes, its records are dropped.
func (t *Tracker) track(instanceID string, events <-chan agents.NormalizedEvent) {
	for ev := range events {
		state, ok := FromEvent(ev)
		if !ok || ev.SessionID == "" {
			continue
		}
		key := SessionKey{Backend: ev.Backend, NativeID: ev.SessionID}
		t.mu.Lock()
		t.states[key] = state
		if t.instanceKeys[instanceID] == nil {
			t.instanceKeys[instanceID] = make(map[SessionKey]bool)
		}
		t.instanceKeys[instanceID][key] = true
		t.mu.Unlock()
	}
	t.mu.Lock()
	delete(t.watching, instanceID)
	for k := range t.instanceKeys[instanceID] {
		delete(t.states, k)
	}
	delete(t.instanceKeys, instanceID)
	t.mu.Unlock()
}

// Of returns the tracked state of one native session, idle when no event has
// reported otherwise.
func (t *Tracker) Of(backend agents.BackendID, nativeSessionID string) State {
	t.mu.Lock()
	defer t.mu.Unlock()
	if state, ok := t.states[SessionKey{Backend: backend, NativeID: nativeSessionID}]; ok {
		return state
	}
	if state, ok := t.states[SessionKey{Backend: "", NativeID: nativeSessionID}]; ok {
		return state
	}
	return Idle
}

// Forget drops one native session's tracked state, if any. It is called when a
// session's index row is deleted so a re-indexed native session starts clean.
func (t *Tracker) Forget(backend agents.BackendID, nativeSessionID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.states, SessionKey{Backend: backend, NativeID: nativeSessionID})
	delete(t.states, SessionKey{Backend: "", NativeID: nativeSessionID})
}
