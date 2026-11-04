package events

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"sync"
	"sync/atomic"
	"time"
)

const (
	defaultBusGCTTL   = 60 * time.Second
	defaultBusBufSize = 512
	heartbeatInterval = 15 * time.Second
)

var (
	ErrInvalidRequest = errors.New("invalid_request")
	ErrBusNotFound    = errors.New("bus_not_found")
)

// ClientBus is an event pipeline for a single client (identified by UUID).
type ClientBus struct {
	id           string
	mgr          *Manager
	mu           sync.Mutex
	subs         map[string]map[string]struct{} // channel -> set of targetIDs
	out          chan EventEnvelope
	active       bool
	gcTimer      *time.Timer
	connCancel   context.CancelFunc
	droppedCount atomic.Uint64
}

// DroppedCount returns the total number of events dropped due to buffer overflow.
func (b *ClientBus) DroppedCount() uint64 {
	return b.droppedCount.Load()
}

// close tears down subscriptions and channel of the bus.
func (b *ClientBus) close() {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.gcTimer != nil {
		b.gcTimer.Stop()
		b.gcTimer = nil
	}
	if b.connCancel != nil {
		b.connCancel()
		b.connCancel = nil
	}
	b.subs = make(map[string]map[string]struct{})
}

// onConnect is invoked when an SSE connection is established.
func (b *ClientBus) onConnect(cancel context.CancelFunc) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.gcTimer != nil {
		b.gcTimer.Stop()
		b.gcTimer = nil
	}
	b.active = true
	b.connCancel = cancel
}

// onDisconnect is invoked when an SSE connection drops.
func (b *ClientBus) onDisconnect() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.active = false
	b.connCancel = nil
	if b.gcTimer != nil {
		b.gcTimer.Stop()
	}
	uuid := b.id
	mgr := b.mgr
	ttl := mgr.gcTTL
	b.gcTimer = time.AfterFunc(ttl, func() {
		mgr.RemoveBus(uuid)
	})
}

// hasSubscription reports whether the bus is listening to (channel, targetID).
func (b *ClientBus) hasSubscription(channel, targetID string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	targets, ok := b.subs[channel]
	if !ok {
		return false
	}
	if targetID == "" {
		return len(targets) > 0
	}
	_, found := targets[targetID]
	return found
}

// Manager manages all active client event buses across domains.
type Manager struct {
	mu       sync.RWMutex
	buses    map[string]*ClientBus
	handlers map[string]ChannelHandler
	gcTTL    time.Duration
	bufSize  int
}

// NewManager creates a new unified event bus manager.
func NewManager() *Manager {
	return &Manager{
		buses:    make(map[string]*ClientBus),
		handlers: make(map[string]ChannelHandler),
		gcTTL:    defaultBusGCTTL,
		bufSize:  defaultBusBufSize,
	}
}

// RegisterHandler registers a lifecycle handler for a specific channel.
func (m *Manager) RegisterHandler(channel string, handler ChannelHandler) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.handlers[channel] = handler
}

// GetOrCreate retrieves an existing ClientBus or creates a new one.
func (m *Manager) GetOrCreate(uuid string) *ClientBus {
	m.mu.Lock()
	defer m.mu.Unlock()

	bus, ok := m.buses[uuid]
	if !ok {
		bus = &ClientBus{
			id:      uuid,
			mgr:     m,
			subs:    make(map[string]map[string]struct{}),
			out:     make(chan EventEnvelope, m.bufSize),
		}
		m.buses[uuid] = bus
	}
	return bus
}

// RemoveBus closes and removes an event bus by UUID.
func (m *Manager) RemoveBus(uuid string) {
	m.mu.Lock()
	bus, ok := m.buses[uuid]
	delete(m.buses, uuid)
	m.mu.Unlock()

	if ok {
		bus.close()
	}
}

// Subscribe attaches a channel and targetID to the specified client bus.
func (m *Manager) Subscribe(ctx context.Context, uuid string, req SubscribeRequest) (*SubscribeResponse, error) {
	if uuid == "" || req.Channel == "" || req.TargetID == "" {
		return nil, ErrInvalidRequest
	}

	bus := m.GetOrCreate(uuid)

	bus.mu.Lock()
	if _, ok := bus.subs[req.Channel]; !ok {
		bus.subs[req.Channel] = make(map[string]struct{})
	}
	bus.subs[req.Channel][req.TargetID] = struct{}{}
	bus.mu.Unlock()

	m.mu.RLock()
	handler := m.handlers[req.Channel]
	m.mu.RUnlock()

	var snapshot any
	if handler != nil {
		var err error
		snapshot, err = handler.OnSubscribe(ctx, uuid, req)
		if err != nil {
			// Roll back subscription on failure
			bus.mu.Lock()
			if targets, ok := bus.subs[req.Channel]; ok {
				delete(targets, req.TargetID)
			}
			bus.mu.Unlock()
			return nil, err
		}
	}

	return &SubscribeResponse{
		Channel:  req.Channel,
		TargetID: req.TargetID,
		Snapshot: snapshot,
	}, nil
}

// Unsubscribe removes a subscription from a client bus.
func (m *Manager) Unsubscribe(ctx context.Context, uuid string, channel string, targetID string) error {
	if uuid == "" || channel == "" || targetID == "" {
		return ErrInvalidRequest
	}

	m.mu.RLock()
	bus, ok := m.buses[uuid]
	handler := m.handlers[channel]
	m.mu.RUnlock()

	if !ok {
		return nil
	}

	bus.mu.Lock()
	if targets, exists := bus.subs[channel]; exists {
		delete(targets, targetID)
		if len(targets) == 0 {
			delete(bus.subs, channel)
		}
	}
	bus.mu.Unlock()

	if handler != nil {
		_ = handler.OnUnsubscribe(ctx, uuid, channel, targetID)
	}

	return nil
}

// ListSubscriptions returns all channels and their subscribed targetIDs for a bus.
func (m *Manager) ListSubscriptions(uuid string) (map[string][]string, error) {
	m.mu.RLock()
	bus, ok := m.buses[uuid]
	m.mu.RUnlock()

	if !ok {
		return map[string][]string{}, nil
	}

	bus.mu.Lock()
	defer bus.mu.Unlock()

	res := make(map[string][]string, len(bus.subs))
	for ch, targets := range bus.subs {
		items := make([]string, 0, len(targets))
		for tid := range targets {
			items = append(items, tid)
		}
		res[ch] = items
	}
	return res, nil
}

// isCriticalEventType identifies events critical to state machine integrity and human interactions.
func isCriticalEventType(eventType string) bool {
	switch eventType {
	case "ready", "turn_started", "turn_ended", "interaction_requested", "interaction_resolved",
		"session_lifecycle", "status_change", "task_finished", "flow_completed", "error", "stream_error":
		return true
	default:
		return false
	}
}

// enqueue attempts to deliver an envelope to the client bus output channel.
// Non-critical events are dropped if the channel is full.
// Critical events wait briefly (up to 50ms) for space before dropping, and drops are logged and counted.
func (b *ClientBus) enqueue(env EventEnvelope) {
	select {
	case b.out <- env:
		return
	default:
	}

	// Buffer is currently full.
	if isCriticalEventType(env.EventType) {
		timer := time.NewTimer(50 * time.Millisecond)
		defer timer.Stop()
		select {
		case b.out <- env:
			return
		case <-timer.C:
		}
	}

	// Increment dropped counter and log warning.
	count := b.droppedCount.Add(1)
	if count == 1 || count%50 == 0 {
		log.Printf("[EventBus] bus %s buffer full (%d), dropped %s event on channel %s/%s (total dropped: %d)",
			b.id, cap(b.out), env.EventType, env.Channel, env.TargetID, count)
	}
}

// PushToBus direct-delivers an event to a specific client bus.
func (m *Manager) PushToBus(uuid string, env EventEnvelope) {
	m.mu.RLock()
	bus, ok := m.buses[uuid]
	m.mu.RUnlock()
	if !ok {
		return
	}
	if env.Timestamp == 0 {
		env.Timestamp = NowMillis()
	}
	bus.enqueue(env)
}

// Broadcast sends an event to all client buses that subscribed to (channel, targetID).
func (m *Manager) Broadcast(env EventEnvelope) {
	if env.Timestamp == 0 {
		env.Timestamp = NowMillis()
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, bus := range m.buses {
		if bus.hasSubscription(env.Channel, env.TargetID) {
			bus.enqueue(env)
		}
	}
}

// BroadcastPayload is a convenience wrapper to serialize payload and broadcast.
func (m *Manager) BroadcastPayload(channel, targetID, eventType string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	m.Broadcast(EventEnvelope{
		Channel:   channel,
		TargetID:  targetID,
		EventType: eventType,
		Timestamp: NowMillis(),
		Payload:   data,
	})
	return nil
}

// StreamBus streams events for a client bus over an SSE connection.
func (m *Manager) StreamBus(ctx context.Context, uuid string, w io.Writer, flush func()) error {
	bus := m.GetOrCreate(uuid)

	connCtx, connCancel := context.WithCancel(ctx)
	defer connCancel()

	bus.onConnect(connCancel)
	defer bus.onDisconnect()

	// Initial handshake frame to indicate connection readiness
	readyEnvelope := EventEnvelope{
		BusID:     uuid,
		Channel:   ChannelSystem,
		EventType: "ready",
		Timestamp: NowMillis(),
	}
	readyBytes, _ := json.Marshal(readyEnvelope)
	if _, err := fmt.Fprintf(w, "event: ready\ndata: %s\n\n", string(readyBytes)); err != nil {
		return err
	}
	if flush != nil {
		flush()
	}

	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-connCtx.Done():
			return nil
		case <-ticker.C:
			if _, err := fmt.Fprintf(w, ":keep-alive\n\n"); err != nil {
				return err
			}
			if flush != nil {
				flush()
			}
		case env, ok := <-bus.out:
			if !ok {
				return nil
			}
			env.BusID = uuid
			wireBytes, err := json.Marshal(env)
			if err != nil {
				continue
			}

			// We emit with eventType so EventSource listeners can filter, or generic message listener
			if env.ID > 0 {
				if _, err := fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", env.ID, env.EventType, string(wireBytes)); err != nil {
					return err
				}
			} else {
				if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", env.EventType, string(wireBytes)); err != nil {
					return err
				}
			}
			if flush != nil {
				flush()
			}
		}
	}
}
