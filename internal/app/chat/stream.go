package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
)

// heartbeatInterval is the SSE keep-alive cadence.
const heartbeatInterval = 15 * time.Second

// subscriberBuffer bounds one SSE subscriber. A slow client is dropped rather
// than letting a stalled connection block the generation or other clients;
// EventSource reconnects and receives a fresh snapshot.
const subscriberBuffer = 64

type runState struct {
	chatID      uint
	assistantID uint
	text        strings.Builder
	reasoning   strings.Builder
	usage       *Usage
	cancel      context.CancelFunc
	done        chan struct{}
}

// runView is a read-only copy of an active run, taken under the chat lock.
type runView struct {
	AssistantID uint
	Text        string
	Reasoning   string
	Usage       *Usage
}

func (r *runState) view() *runView {
	if r == nil {
		return nil
	}
	return &runView{
		AssistantID: r.assistantID,
		Text:        r.text.String(),
		Reasoning:   r.reasoning.String(),
		Usage:       r.usage,
	}
}

type chatState struct {
	mu          sync.Mutex
	run         *runState
	subscribers map[chan Event]struct{}
}

type runManager struct {
	mu          sync.Mutex
	states      map[uint]*chatState
	onBroadcast func(Event)
}

func newRunManager() *runManager {
	return &runManager{states: make(map[uint]*chatState)}
}

func (m *runManager) stateLocked(chatID uint) *chatState {
	state, ok := m.states[chatID]
	if !ok {
		state = &chatState{subscribers: make(map[chan Event]struct{})}
		m.states[chatID] = state
	}
	return state
}

func (m *runManager) state(chatID uint) *chatState {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.stateLocked(chatID)
}

func (m *runManager) tryStart(run *runState) error {
	state := m.state(run.chatID)
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.run != nil {
		return ErrChatBusy
	}
	state.run = run
	return nil
}

func (m *runManager) setAssistantID(run *runState, id uint) {
	state := m.state(run.chatID)
	state.mu.Lock()
	if state.run == run {
		run.assistantID = id
	}
	state.mu.Unlock()
}

func (m *runManager) running(chatID uint) bool {
	state := m.state(chatID)
	state.mu.Lock()
	defer state.mu.Unlock()
	return state.run != nil
}

// emitDelta appends an increment and publishes it atomically, so a subscriber
// taking a snapshot between the two operations can never see the text twice or
// miss it entirely.
func (m *runManager) emitDelta(run *runState, text, reasoning string) {
	if text == "" && reasoning == "" {
		return
	}
	state := m.state(run.chatID)
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.run != run {
		return
	}
	run.text.WriteString(text)
	run.reasoning.WriteString(reasoning)
	event := Event{
		ChatID:    run.chatID,
		Timestamp: time.Now().UTC(),
		Type:      EventDelta,
		Delta:     &Delta{MessageID: run.assistantID, Text: text, Reasoning: reasoning},
	}
	m.broadcastLocked(state, event)
}

func (m *runManager) setUsage(run *runState, usage *Usage) {
	if usage == nil {
		return
	}
	state := m.state(run.chatID)
	state.mu.Lock()
	if state.run == run {
		run.usage = usage
	}
	state.mu.Unlock()
}

func (m *runManager) view(run *runState) *runView {
	if run == nil {
		return nil
	}
	state := m.state(run.chatID)
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.run != run {
		return nil
	}
	return run.view()
}

func (m *runManager) currentRun(chatID uint) *runState {
	state := m.state(chatID)
	state.mu.Lock()
	defer state.mu.Unlock()
	return state.run
}

// abort clears a run that never produced a persisted assistant message. It is
// used when the database transaction that creates the exchange fails.
func (m *runManager) abort(run *runState) {
	state := m.state(run.chatID)
	state.mu.Lock()
	if state.run == run {
		state.run = nil
	}
	close(run.done)
	state.mu.Unlock()
}

func (m *runManager) broadcast(chatID uint, event Event) {
	state := m.state(chatID)
	state.mu.Lock()
	defer state.mu.Unlock()
	m.broadcastLocked(state, event)
}

// finishAndPublish clears the active run before broadcasting its final events.
// A subscriber that joins after this point reads the final database state and
// running=false; one that joined earlier receives the events in channel order.
func (m *runManager) finishAndPublish(run *runState, events ...Event) {
	state := m.state(run.chatID)
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.run == run {
		state.run = nil
	}
	close(run.done)
	for _, event := range events {
		m.broadcastLocked(state, event)
	}
}

// publish builds and broadcasts a fresh snapshot. The builder runs under the
// chat lock so a concurrent finalize cannot change the database between the
// snapshot read and the registration of the event.
func (m *runManager) publish(chatID uint, build func(*runView) (Event, error)) (Event, error) {
	state := m.state(chatID)
	state.mu.Lock()
	defer state.mu.Unlock()
	event, err := build(state.run.view())
	if err != nil {
		return Event{}, err
	}
	m.broadcastLocked(state, event)
	return event, nil
}

// subscribe registers a subscriber and returns the snapshot it must render
// first. Registration and snapshot construction share the chat lock.
func (m *runManager) subscribe(chatID uint, build func(*runView) (Event, error)) (Event, <-chan Event, func(), error) {
	state := m.state(chatID)
	state.mu.Lock()
	defer state.mu.Unlock()
	event, err := build(state.run.view())
	if err != nil {
		return Event{}, nil, nil, err
	}
	ch := make(chan Event, subscriberBuffer)
	state.subscribers[ch] = struct{}{}
	cancel := func() {
		state.mu.Lock()
		if _, ok := state.subscribers[ch]; ok {
			delete(state.subscribers, ch)
			close(ch)
		}
		state.mu.Unlock()
	}
	return event, ch, cancel, nil
}

func (m *runManager) broadcastLocked(state *chatState, event Event) {
	if m.onBroadcast != nil {
		m.onBroadcast(event)
	}
	for ch := range state.subscribers {
		select {
		case ch <- event:
		default:
			delete(state.subscribers, ch)
			close(ch)
		}
	}
}

func (m *runManager) cancel(chatID uint) bool {
	state := m.state(chatID)
	state.mu.Lock()
	run := state.run
	state.mu.Unlock()
	if run == nil {
		return false
	}
	run.cancel()
	return true
}

func (m *runManager) cancelAll() []*runState {
	m.mu.Lock()
	states := make([]*chatState, 0, len(m.states))
	for _, state := range m.states {
		states = append(states, state)
	}
	m.mu.Unlock()

	var runs []*runState
	for _, state := range states {
		state.mu.Lock()
		if state.run != nil {
			runs = append(runs, state.run)
			state.run.cancel()
		}
		state.mu.Unlock()
	}
	return runs
}

func (m *runManager) wait(run *runState, ctx context.Context) bool {
	if run == nil {
		return true
	}
	select {
	case <-run.done:
		return true
	case <-ctx.Done():
		return false
	}
}

func (m *runManager) remove(chatID uint) {
	m.mu.Lock()
	state, ok := m.states[chatID]
	if ok {
		delete(m.states, chatID)
	}
	if state != nil {
		state.mu.Lock()
		for ch := range state.subscribers {
			delete(state.subscribers, ch)
			close(ch)
		}
		state.mu.Unlock()
	}
	m.mu.Unlock()
}

// waitForRun waits for one run without holding any manager lock. The service
// uses it before deleting a chat so the generation cannot resurrect a message
// after the conversation is gone.
func (s *Service) waitForRun(run *runState, ctx context.Context) bool {
	return s.manager.wait(run, ctx)
}

// StreamChat writes the chat's SSE stream until the client disconnects, the
// chat is deleted, or the subscription is dropped as too slow.
func (s *Service) StreamChat(ctx context.Context, chatID uint, w io.Writer, flush func()) error {
	event, ch, cancel, err := s.manager.subscribe(chatID, func(run *runView) (Event, error) {
		return s.snapshot(chatID, run)
	})
	if err != nil {
		return err
	}
	defer cancel()

	if err := writeEvent(w, event); err != nil {
		return err
	}
	if flush != nil {
		flush()
	}

	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if _, err := io.WriteString(w, ": keep-alive\n\n"); err != nil {
				return err
			}
			if flush != nil {
				flush()
			}
		case event, ok := <-ch:
			if !ok {
				return nil
			}
			if err := writeEvent(w, event); err != nil {
				return err
			}
			if flush != nil {
				flush()
			}
		}
	}
}

func writeEvent(w io.Writer, event Event) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("chat: encode event: %w", err)
	}
	if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Type, payload); err != nil {
		return err
	}
	return nil
}
