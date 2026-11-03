package flow

import (
	"sync"
)

// FlowEventType defines the category of an SSE event frame in a collaborative flow.
type FlowEventType string

const (
	EventChunk   FlowEventType = "chunk"
	EventMessage FlowEventType = "message"
	EventStatus  FlowEventType = "status"
)

// FlowEvent is the unified payload pushed through SSE to subscribers of a flow run.
type FlowEvent struct {
	Type    FlowEventType `json:"type"`
	RunID   uint          `json:"runId"`
	Payload any           `json:"payload"`
}

// FlowChunkPayload carries incremental streaming tokens.
type FlowChunkPayload struct {
	MessageID  uint   `json:"messageId"`
	SenderName string `json:"senderName"`
	SenderType string `json:"senderType"`
	Chunk      string `json:"chunk"`
}

// FlowStatusPayload notifies state changes of the collaborative flow.
type FlowStatusPayload struct {
	Status      string `json:"status"`
	CurrentStep int    `json:"currentStep"`
	Error       string `json:"error,omitempty"`
}

// EventBus multiplexes flow events to subscribed listeners per FlowRun.
type EventBus struct {
	mu          sync.RWMutex
	subscribers map[uint]map[chan FlowEvent]struct{}
}

// NewEventBus creates a new FlowEventBus.
func NewEventBus() *EventBus {
	return &EventBus{
		subscribers: make(map[uint]map[chan FlowEvent]struct{}),
	}
}

// Subscribe listens to events for a specific flow run. Returns a read channel and an unsubscribe func.
func (b *EventBus) Subscribe(runID uint) (<-chan FlowEvent, func()) {
	b.mu.Lock()
	defer b.mu.Unlock()

	ch := make(chan FlowEvent, 64)
	if _, ok := b.subscribers[runID]; !ok {
		b.subscribers[runID] = make(map[chan FlowEvent]struct{})
	}
	b.subscribers[runID][ch] = struct{}{}

	unsubscribe := func() {
		b.mu.Lock()
		defer b.mu.Unlock()

		if subs, ok := b.subscribers[runID]; ok {
			delete(subs, ch)
			if len(subs) == 0 {
				delete(b.subscribers, runID)
			}
		}
		close(ch)
	}

	return ch, unsubscribe
}

// Publish broadcasts an event to all subscribers of the specified flow run.
func (b *EventBus) Publish(runID uint, event FlowEvent) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	subs, ok := b.subscribers[runID]
	if !ok || len(subs) == 0 {
		return
	}

	for ch := range subs {
		select {
		case ch <- event:
		default:
			// Non-blocking drop if consumer buffer is full
		}
	}
}
