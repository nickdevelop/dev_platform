package sessions

import (
	"context"

	"devplatform/internal/agents"
)

// TurnState reports whether a session is processing one user turn. It is
// derived from the instance's lifecycle events, never stored, so it is idle
// after a platform restart.
type TurnState string

const (
	// TurnIdle means the session is waiting for the next user message.
	TurnIdle TurnState = "idle"
	// TurnRunning means the agent is processing a turn of the session.
	TurnRunning TurnState = "running"
)

// turnFromEvent maps one normalized event to the turn state it implies. Events
// that do not change the turn report false.
func turnFromEvent(ev agents.NormalizedEvent) (TurnState, bool) {
	switch ev.Type {
	case agents.EventLifecycle:
		if ev.Lifecycle == nil {
			return "", false
		}
		switch ev.Lifecycle.State {
		case "execution_started", "agent_start":
			return TurnRunning, true
		case "execution_succeeded", "execution_interrupted", "agent_settled", "agent_end":
			return TurnIdle, true
		}
	case agents.EventError:
		if ev.Error == nil {
			return "", false
		}
		switch ev.Error.Code {
		case "execution_failed", "process_exit":
			return TurnIdle, true
		}
	}
	return "", false
}

// ensureTracker starts the instance's turn tracker unless one is running. The
// tracker subscribes synchronously, so events published after this returns are
// observed; it stays subscribed until the instance's stream closes.
func (s *Service) ensureTracker(instanceID string) {
	s.turnMu.Lock()
	if s.trackers[instanceID] {
		s.turnMu.Unlock()
		return
	}
	s.trackers[instanceID] = true
	s.turnMu.Unlock()

	events, err := s.instances.Subscribe(context.Background(), instanceID)
	if err != nil {
		s.turnMu.Lock()
		delete(s.trackers, instanceID)
		s.turnMu.Unlock()
		return
	}
	go s.trackTurns(instanceID, events)
}

// trackTurns records the turn state of each native session the instance
// reports. When the instance's stream closes, its turn records are dropped.
func (s *Service) trackTurns(instanceID string, events <-chan agents.NormalizedEvent) {
	for ev := range events {
		turn, ok := turnFromEvent(ev)
		if !ok || ev.SessionID == "" {
			continue
		}
		s.turnMu.Lock()
		if s.turns[instanceID] == nil {
			s.turns[instanceID] = make(map[string]TurnState)
		}
		s.turns[instanceID][ev.SessionID] = turn
		s.turnMu.Unlock()
	}
	s.turnMu.Lock()
	delete(s.trackers, instanceID)
	delete(s.turns, instanceID)
	s.turnMu.Unlock()
}

// turnOf returns the tracked turn state of one native session, idle when no
// event has reported otherwise.
func (s *Service) turnOf(instanceID, nativeSessionID string) TurnState {
	s.turnMu.Lock()
	defer s.turnMu.Unlock()
	if turn, ok := s.turns[instanceID][nativeSessionID]; ok {
		return turn
	}
	return TurnIdle
}
