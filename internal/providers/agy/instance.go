package agy

import (
	"context"
	"fmt"
	"sync"
	"time"

	"devplatform/internal/core/agents"
)

type subscriber struct {
	in   chan agents.NormalizedEvent
	out  chan agents.NormalizedEvent
	done chan struct{}
}

func newSubscriber() *subscriber {
	return &subscriber{
		in:   make(chan agents.NormalizedEvent, 128),
		out:  make(chan agents.NormalizedEvent, 128),
		done: make(chan struct{}),
	}
}

func (s *subscriber) forward() {
	defer close(s.out)
	for ev := range s.in {
		select {
		case s.out <- ev:
		case <-s.done:
			return
		}
	}
}

func (s *subscriber) watch(ctx context.Context, inst *instance) {
	select {
	case <-ctx.Done():
	case <-inst.done:
	}
	close(s.done)
	inst.removeSubscriber(s)
}

type instance struct {
	id        string
	directory string
	now       func() time.Time
	ctx       context.Context
	cancel    context.CancelFunc
	done      chan struct{}

	mu       sync.Mutex
	status   agents.InstanceStatus
	closed   bool
	seq      uint64
	subs     map[*subscriber]struct{}
	children map[string]*agyChild
}

func newInstance(id, directory string, now func() time.Time) *instance {
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &instance{
		id:        id,
		directory: directory,
		now:       now,
		ctx:       ctx,
		cancel:    cancel,
		status:    agents.InstanceRunning,
		done:      make(chan struct{}),
		subs:      make(map[*subscriber]struct{}),
		children:  make(map[string]*agyChild),
	}
}

func (inst *instance) ensureRunning() error {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	if inst.closed || inst.status != agents.InstanceRunning {
		return fmt.Errorf("agy: instance %q is not running", inst.id)
	}
	return nil
}

func (inst *instance) addSubscriber(sub *subscriber) error {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	if inst.closed {
		return fmt.Errorf("agy: instance %q is closed", inst.id)
	}
	inst.subs[sub] = struct{}{}
	return nil
}

func (inst *instance) removeSubscriber(sub *subscriber) {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	if _, ok := inst.subs[sub]; ok {
		delete(inst.subs, sub)
		close(sub.in)
	}
}

func (inst *instance) broadcast(ev agents.NormalizedEvent) {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	if inst.closed {
		return
	}
	inst.seq++
	ev.Sequence = inst.seq
	if ev.Timestamp.IsZero() {
		ev.Timestamp = inst.now()
	}
	if ev.InstanceID == "" {
		ev.InstanceID = inst.id
	}
	if ev.Backend == "" {
		ev.Backend = agents.BackendAgy
	}

	for sub := range inst.subs {
		select {
		case sub.in <- ev:
		default:
			// Dropped frame if subscriber is completely stalled
		}
	}
}

func (inst *instance) getChild(sessionID string) *agyChild {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	return inst.children[sessionID]
}

func (inst *instance) setChild(sessionID string, c *agyChild) {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	inst.children[sessionID] = c
}

func (inst *instance) removeChild(sessionID string) {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	delete(inst.children, sessionID)
}

func (inst *instance) stop() {
	inst.mu.Lock()
	if inst.closed {
		inst.mu.Unlock()
		return
	}
	inst.closed = true
	inst.status = agents.InstanceStopped
	inst.cancel()
	close(inst.done)

	// Copy children to close outside mutex
	children := make([]*agyChild, 0, len(inst.children))
	for _, c := range inst.children {
		children = append(children, c)
	}
	subs := make([]*subscriber, 0, len(inst.subs))
	for s := range inst.subs {
		subs = append(subs, s)
	}
	inst.children = make(map[string]*agyChild)
	inst.subs = make(map[*subscriber]struct{})
	inst.mu.Unlock()

	for _, c := range children {
		_ = c.close()
	}
	for _, s := range subs {
		close(s.in)
	}
}
