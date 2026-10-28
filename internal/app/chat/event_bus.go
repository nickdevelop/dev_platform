package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"
)

const (
	defaultChatBusGCTTL   = 60 * time.Second
	defaultChatBusBufSize = 512
)

// chatBusEnvelope carries one SSE event to be formatted and sent.
type chatBusEnvelope struct {
	eventType string
	data      []byte
}

// ChatClientBus is a multiplexed event pipeline for a single client (identified by UUID).
type ChatClientBus struct {
	id         string
	mgr        *EventBusManager
	mu         sync.Mutex
	chats      map[uint]struct{} // subscribed chat IDs
	out        chan chatBusEnvelope
	active     bool
	gcTimer    *time.Timer
	connCancel context.CancelFunc
}

// close tears down subscriptions and channel of the bus.
func (b *ChatClientBus) close() {
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
	b.chats = make(map[uint]struct{})
}

// onConnect is invoked when an SSE connection is established.
func (b *ChatClientBus) onConnect(cancel context.CancelFunc) {
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
func (b *ChatClientBus) onDisconnect() {
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

// EventBusManager manages all active client event buses for the pure-chat domain.
type EventBusManager struct {
	mu      sync.RWMutex
	buses   map[string]*ChatClientBus
	service *Service
	gcTTL   time.Duration
	bufSize int
}

// NewEventBusManager creates a new pure-chat event bus manager.
func NewEventBusManager(service *Service) *EventBusManager {
	return &EventBusManager{
		buses:   make(map[string]*ChatClientBus),
		service: service,
		gcTTL:   defaultChatBusGCTTL,
		bufSize: defaultChatBusBufSize,
	}
}

// GetOrCreate retrieves an existing ChatClientBus or creates a new one.
func (m *EventBusManager) GetOrCreate(uuid string) *ChatClientBus {
	m.mu.Lock()
	defer m.mu.Unlock()

	bus, ok := m.buses[uuid]
	if !ok {
		bus = &ChatClientBus{
			id:      uuid,
			mgr:     m,
			chats:   make(map[uint]struct{}),
			out:     make(chan chatBusEnvelope, m.bufSize),
		}
		m.buses[uuid] = bus
	}
	return bus
}

// RemoveBus closes and removes an event bus by UUID.
func (m *EventBusManager) RemoveBus(uuid string) {
	m.mu.Lock()
	bus, ok := m.buses[uuid]
	delete(m.buses, uuid)
	m.mu.Unlock()

	if ok {
		bus.close()
	}
}

// SubscribeChat attaches a chat to the client bus and returns its current snapshot.
func (m *EventBusManager) SubscribeChat(ctx context.Context, uuid string, chatID uint) (*Snapshot, error) {
	snap, err := m.service.Snapshot(chatID)
	if err != nil {
		return nil, err
	}

	bus := m.GetOrCreate(uuid)
	bus.mu.Lock()
	bus.chats[chatID] = struct{}{}
	bus.mu.Unlock()

	return snap, nil
}

// UnsubscribeChat detaches a chat from the client bus.
func (m *EventBusManager) UnsubscribeChat(uuid string, chatID uint) error {
	m.mu.RLock()
	bus, ok := m.buses[uuid]
	m.mu.RUnlock()

	if !ok {
		return nil
	}

	bus.mu.Lock()
	delete(bus.chats, chatID)
	bus.mu.Unlock()

	return nil
}

// ListSubscriptions returns the IDs of chats currently subscribed to the bus.
func (m *EventBusManager) ListSubscriptions(uuid string) ([]uint, error) {
	m.mu.RLock()
	bus, ok := m.buses[uuid]
	m.mu.RUnlock()

	if !ok {
		return []uint{}, nil
	}

	bus.mu.Lock()
	defer bus.mu.Unlock()

	res := make([]uint, 0, len(bus.chats))
	for cid := range bus.chats {
		res = append(res, cid)
	}
	return res, nil
}

// Broadcast sends an event to all client buses that subscribed to the event's chat.
func (m *EventBusManager) Broadcast(event Event) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if len(m.buses) == 0 {
		return
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return
	}

	env := chatBusEnvelope{
		eventType: event.Type,
		data:      payload,
	}

	for _, bus := range m.buses {
		bus.mu.Lock()
		if _, ok := bus.chats[event.ChatID]; ok {
			select {
			case bus.out <- env:
			default:
				// If client buffer is full, drop to prevent blocking the generator
			}
		}
		bus.mu.Unlock()
	}
}

// StreamBus streams events for the client bus as Server-Sent Events.
func (m *EventBusManager) StreamBus(ctx context.Context, uuid string, w io.Writer, flush func()) error {
	bus := m.GetOrCreate(uuid)

	connCtx, connCancel := context.WithCancel(ctx)
	defer connCancel()

	bus.onConnect(connCancel)
	defer bus.onDisconnect()

	// Initial ready frame
	if _, err := io.WriteString(w, "event: ready\ndata: {}\n\n"); err != nil {
		return err
	}
	if flush != nil {
		flush()
	}

	heartbeat := time.NewTicker(heartbeatInterval)
	defer heartbeat.Stop()

	for {
		select {
		case <-connCtx.Done():
			return connCtx.Err()
		case <-heartbeat.C:
			if _, err := io.WriteString(w, ": keep-alive\n\n"); err != nil {
				return err
			}
			if flush != nil {
				flush()
			}
		case env, ok := <-bus.out:
			if !ok {
				return nil
			}
			if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", env.eventType, env.data); err != nil {
				return err
			}
			if flush != nil {
				flush()
			}
		}
	}
}
