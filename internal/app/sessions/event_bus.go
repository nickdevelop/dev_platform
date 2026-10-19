package sessions

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"devplatform/internal/core/agents"
	"devplatform/internal/app/instances"
)

const (
	defaultBusGCTTL    = 60 * time.Second
	defaultBusBufSize  = 512
)

// SubscriptionSnapshot captures the turn state and pending interactions of a session
// at the moment it is subscribed to an event bus.
type SubscriptionSnapshot struct {
	SessionID    string              `json:"sessionId"`
	Turn         *agents.TurnPayload `json:"turn"`
	Interactions interactionSnapshot `json:"interactions"`
	Resync       bool                `json:"resync"`
}

// busSessionSub tracks one session attached to a client bus.
type busSessionSub struct {
	projectID       uint
	sessionID       string
	nativeSessionID string
	instanceID      string
	backend         agents.BackendID
}

// busInstanceSub manages the instance-level event channel shared by all sessions
// of that instance attached to the same bus.
type busInstanceSub struct {
	instanceID string
	cancel     context.CancelFunc
	sessions   map[string]*busSessionSub
}

// busEnvelope carries an SSE event to be framed and sent.
type busEnvelope struct {
	id        uint64
	eventType string
	data      []byte
}

// ClientBus is an event pipeline for a single client (identified by UUID).
type ClientBus struct {
	id         string
	mgr        *EventBusManager
	mu         sync.Mutex
	sessions   map[string]*busSessionSub  // key: sessionID
	instances  map[string]*busInstanceSub // key: instanceID
	out        chan busEnvelope
	active     bool
	gcTimer    *time.Timer
	connCancel context.CancelFunc
}

// EventBusManager manages all active client event buses.
type EventBusManager struct {
	mu      sync.RWMutex
	buses   map[string]*ClientBus
	service *Service
	gcTTL   time.Duration
	bufSize int
}

// NewEventBusManager creates a new event bus manager.
func NewEventBusManager(service *Service) *EventBusManager {
	return &EventBusManager{
		buses:   make(map[string]*ClientBus),
		service: service,
		gcTTL:   defaultBusGCTTL,
		bufSize: defaultBusBufSize,
	}
}

// GetOrCreate retrieves an existing ClientBus or creates a new one.
func (m *EventBusManager) GetOrCreate(uuid string) *ClientBus {
	m.mu.Lock()
	defer m.mu.Unlock()

	bus, ok := m.buses[uuid]
	if !ok {
		bus = &ClientBus{
			id:        uuid,
			mgr:       m,
			sessions:  make(map[string]*busSessionSub),
			instances: make(map[string]*busInstanceSub),
			out:       make(chan busEnvelope, m.bufSize),
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

// close tears down all subscriptions and channels of the bus.
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
	for _, instSub := range b.instances {
		if instSub.cancel != nil {
			instSub.cancel()
		}
	}
	b.instances = make(map[string]*busInstanceSub)
	b.sessions = make(map[string]*busSessionSub)
}

// onConnect is invoked when a client establishes or re-establishes an SSE connection.
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
	// Start GC timer to clean up after grace period if client does not reconnect.
	uuid := b.id
	mgr := b.mgr
	ttl := mgr.gcTTL
	b.gcTimer = time.AfterFunc(ttl, func() {
		mgr.RemoveBus(uuid)
	})
}

// SubscribeSession attaches a session to the client bus and returns its current snapshot.
func (m *EventBusManager) SubscribeSession(ctx context.Context, uuid string, projectID uint, sessionID string, lastEventID uint64) (*SubscriptionSnapshot, error) {
	row, err := m.service.loadScoped(projectID, sessionID)
	if err != nil {
		return nil, err
	}
	if row.Status == string(SessionStopped) {
		return nil, fmt.Errorf("%w: session is stopped", ErrInvalidRequest)
	}

	inst, err := m.service.scopedInstance(projectID, row.InstanceID)
	if err != nil {
		return nil, err
	}
	if !instanceRunnable(inst) {
		return nil, fmt.Errorf("%w: %s is %s", instances.ErrInstanceNotRunning, inst.ID, inst.Status)
	}
	if !m.service.capabilities(inst.Backend).Streaming {
		return nil, fmt.Errorf("%w: streaming", ErrUnsupportedCapability)
	}

	bus := m.GetOrCreate(uuid)

	bus.mu.Lock()
	defer bus.mu.Unlock()

	// 1. Check if instance is already subscribed for this bus.
	instSub, ok := bus.instances[row.InstanceID]
	if !ok {
		subCtx, cancel := context.WithCancel(context.Background())
		events, err := m.service.instances.Subscribe(subCtx, row.InstanceID)
		if err != nil {
			cancel()
			return nil, err
		}

		instSub = &busInstanceSub{
			instanceID: row.InstanceID,
			cancel:     cancel,
			sessions:   make(map[string]*busSessionSub),
		}
		bus.instances[row.InstanceID] = instSub

		// Forward instance events to the bus channel.
		go bus.forwardInstanceEvents(subCtx, row.InstanceID, events)
	}

	sub := &busSessionSub{
		projectID:       projectID,
		sessionID:       row.ID,
		nativeSessionID: row.NativeSessionID,
		instanceID:      row.InstanceID,
		backend:         agents.BackendID(row.Backend),
	}
	instSub.sessions[row.ID] = sub
	bus.sessions[row.ID] = sub

	// 2. Compute Replay or Resync if requested.
	resyncNeeded := false
	if lastEventID > 0 {
		replay, covered := m.service.instances.ReplaySince(row.InstanceID, lastEventID)
		if !covered {
			resyncNeeded = true
			// Push a resync frame for this session on the bus.
			resyncData, _ := json.Marshal(map[string]string{"sessionId": row.ID})
			select {
			case bus.out <- busEnvelope{id: 0, eventType: "resync", data: resyncData}:
			default:
			}
		} else {
			for _, ev := range replay {
				if !matchesSessionRow(sub, ev) {
					continue
				}
				if ev.SessionID != "" {
					ev.SessionID = row.ID
				}
				if err := ev.Validate(); err != nil {
					continue
				}
				payload, err := json.Marshal(ev)
				if err != nil {
					continue
				}
				select {
				case bus.out <- busEnvelope{id: ev.Sequence, eventType: string(ev.Type), data: payload}:
				default:
				}
			}
		}
	}

	// 3. Build snapshot: turn state & interactions.
	ref := agents.SessionRef{
		InstanceID: row.InstanceID,
		Backend:    agents.BackendID(row.Backend),
		SessionID:  row.NativeSessionID,
		Title:      row.Title,
	}
	m.service.alignTurnState(ctx, agents.BackendID(row.Backend), ref)

	turnPhase := agents.TurnEnded
	turnOutcome := agents.TurnCompleted
	if m.service.turnTracker.Of(agents.BackendID(row.Backend), row.NativeSessionID) == TurnRunning {
		turnPhase = agents.TurnStarted
		turnOutcome = ""
	}
	turnPayload := &agents.TurnPayload{
		ID:      turnSnapshotID,
		Phase:   turnPhase,
		Outcome: turnOutcome,
	}

	interactions := m.getInteractionsSnapshot(ctx, ref)

	return &SubscriptionSnapshot{
		SessionID:    row.ID,
		Turn:         turnPayload,
		Interactions: interactions,
		Resync:       resyncNeeded,
	}, nil
}

// matchesSessionRow reports whether an event matches a subscribed session.
func matchesSessionRow(sub *busSessionSub, ev agents.NormalizedEvent) bool {
	if isSessionStrictEvent(ev.Type) {
		return ev.SessionID == sub.nativeSessionID
	}
	return ev.SessionID == "" || ev.SessionID == sub.nativeSessionID
}

// forwardInstanceEvents continuously forwards events for an instance to the bus out channel.
func (b *ClientBus) forwardInstanceEvents(ctx context.Context, instanceID string, events <-chan agents.NormalizedEvent) {
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-events:
			if !ok {
				return
			}
			b.mu.Lock()
			instSub, ok := b.instances[instanceID]
			if !ok {
				b.mu.Unlock()
				return
			}
			// Find matching sessions within this bus.
			for _, sub := range instSub.sessions {
				if matchesSessionRow(sub, ev) {
					cloned := ev
					if cloned.SessionID != "" {
						cloned.SessionID = sub.sessionID
					}
					if err := cloned.Validate(); err != nil {
						continue
					}
					payload, err := json.Marshal(cloned)
					if err != nil {
						continue
					}
					select {
					case b.out <- busEnvelope{id: cloned.Sequence, eventType: string(cloned.Type), data: payload}:
					default:
					}
				}
			}
			b.mu.Unlock()
		}
	}
}

func (m *EventBusManager) getInteractionsSnapshot(ctx context.Context, ref agents.SessionRef) interactionSnapshot {
	snapshot := interactionSnapshot{
		Permissions: []PermissionRequest{},
		Forms:       []FormInfo{},
	}
	if !m.service.capabilities(ref.Backend).ToolConfirmation {
		return snapshot
	}
	replier, err := m.service.replierFor(ref.Backend, "tool confirmation")
	if err != nil {
		return snapshot
	}
	perms, err := replier.ListPermissions(ctx, ref)
	if err == nil && perms != nil {
		snapshot.Permissions = perms
	}
	forms, err := replier.ListForms(ctx, ref)
	if err == nil && forms != nil {
		snapshot.Forms = forms
	}
	return snapshot
}

// UnsubscribeSession detaches a session from the client bus.
func (m *EventBusManager) UnsubscribeSession(uuid string, sessionID string) error {
	m.mu.RLock()
	bus, ok := m.buses[uuid]
	m.mu.RUnlock()

	if !ok {
		return nil
	}

	bus.mu.Lock()
	defer bus.mu.Unlock()

	sub, ok := bus.sessions[sessionID]
	if !ok {
		return nil
	}
	delete(bus.sessions, sessionID)

	instSub, ok := bus.instances[sub.instanceID]
	if ok {
		delete(instSub.sessions, sessionID)
		if len(instSub.sessions) == 0 {
			if instSub.cancel != nil {
				instSub.cancel()
			}
			delete(bus.instances, sub.instanceID)
		}
	}

	return nil
}

// ListSubscriptions returns the IDs of sessions currently subscribed to the bus.
func (m *EventBusManager) ListSubscriptions(uuid string) ([]string, error) {
	m.mu.RLock()
	bus, ok := m.buses[uuid]
	m.mu.RUnlock()

	if !ok {
		return []string{}, nil
	}

	bus.mu.Lock()
	defer bus.mu.Unlock()

	res := make([]string, 0, len(bus.sessions))
	for sid := range bus.sessions {
		res = append(res, sid)
	}
	return res, nil
}

// StreamBus streams events for the client bus as Server-Sent Events.
func (m *EventBusManager) StreamBus(ctx context.Context, uuid string, w io.Writer, flush func()) error {
	bus := m.GetOrCreate(uuid)

	connCtx, connCancel := context.WithCancel(ctx)
	defer connCancel()

	bus.onConnect(connCancel)
	defer bus.onDisconnect()

	// Send initial ready frame
	if _, err := io.WriteString(w, "event: ready\ndata: {}\n\n"); err != nil {
		return err
	}
	if flush != nil {
		flush()
	}

	heartbeat := time.NewTicker(heartbeatInterval)
	defer heartbeat.Stop()

	var slow atomic.Bool

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
				if slow.Load() {
					return errSlowClient
				}
				return nil
			}
			var err error
			if env.id > 0 {
				_, err = fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", env.id, env.eventType, env.data)
			} else {
				_, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", env.eventType, env.data)
			}
			if err != nil {
				return err
			}
			if flush != nil {
				flush()
			}
		}
	}
}
