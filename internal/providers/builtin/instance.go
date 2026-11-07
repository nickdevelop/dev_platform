package builtin

import (
	"context"
	"fmt"
	"sync"
	"time"

	"devplatform/internal/core/agents"
)

// instance is a lightweight embedded agent occurrence: a directory, a stream
// fan-out, per-instance sequence counters and one context that cancels every
// turn when the instance stops. There is no child process.
type instance struct {
	id        string
	directory string
	ctx       context.Context
	cancel    context.CancelFunc
	now       func() time.Time
	done      chan struct{}

	mu      sync.Mutex
	status  agents.InstanceStatus
	closed  bool
	seq     uint64
	turnSeq uint64
	subs    map[*subscriber]struct{}
}

// ensureRunning reports whether the instance can accept work.
func (inst *instance) ensureRunning() error {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	if inst.closed || inst.status != agents.InstanceRunning {
		return inst.notRunningError()
	}
	return nil
}

func (inst *instance) notRunningError() error {
	return fmt.Errorf("builtin: instance %q is not running", inst.id)
}

// nextTurn mints a turn id unique within the instance.
func (inst *instance) nextTurn() string {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	inst.turnSeq++
	return fmt.Sprintf("%s-turn-%d", inst.id, inst.turnSeq)
}

// stop closes the instance's stream and cancels the shared context, so every
// running turn unwinds to a cancelled outcome. It is idempotent.
func (inst *instance) stop() {
	inst.mu.Lock()
	if inst.closed {
		inst.mu.Unlock()
		return
	}
	inst.closed = true
	inst.status = agents.InstanceStopped
	close(inst.done)
	inst.mu.Unlock()
	inst.cancel()
}

// addSubscriber registers a subscriber unless the instance is already closed.
func (inst *instance) addSubscriber(sub *subscriber) error {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	if inst.closed {
		return inst.notRunningError()
	}
	inst.subs[sub] = struct{}{}
	return nil
}

// removeSubscriber drops a finished subscriber.
func (inst *instance) removeSubscriber(sub *subscriber) {
	inst.mu.Lock()
	delete(inst.subs, sub)
	inst.mu.Unlock()
}

// publish stamps the envelope, assigns the next per-instance sequence and
// delivers the event to every current subscriber. Delivery honors
// backpressure: a subscriber that cannot keep up blocks its own copy, which
// in turn applies backpressure to the turn loop.
func (inst *instance) publish(event agents.NormalizedEvent) {
	inst.mu.Lock()
	if inst.closed {
		inst.mu.Unlock()
		return
	}
	inst.seq++
	event.Sequence = inst.seq
	event.InstanceID = inst.id
	event.Backend = agents.BackendBuiltin
	if event.Timestamp.IsZero() {
		event.Timestamp = inst.now()
	}
	subs := make([]*subscriber, 0, len(inst.subs))
	for sub := range inst.subs {
		subs = append(subs, sub)
	}
	inst.mu.Unlock()

	for _, sub := range subs {
		select {
		case sub.stash <- event:
		case <-sub.done:
		}
	}
}

// subscriber is one consumer of an instance's stream. The same two-channel
// design pi uses: stash is written by publishers and never closed; out is
// owned and closed by forward, so a concurrent stop can never race a close
// against a send.
type subscriber struct {
	stash chan agents.NormalizedEvent
	out   chan agents.NormalizedEvent
	done  chan struct{}
	once  sync.Once
}

func newSubscriber() *subscriber {
	return &subscriber{
		stash: make(chan agents.NormalizedEvent, 64),
		out:   make(chan agents.NormalizedEvent, 64),
		done:  make(chan struct{}),
	}
}

// builtinDeliverTimeout bounds how long a torn-down subscriber waits to hand
// an already-queued terminal event to its consumer before giving up.
const builtinDeliverTimeout = 100 * time.Millisecond

// forward copies the subscriber's stash to its output and closes the output
// when the subscription ends. On teardown it flushes events already queued so
// a terminal event is delivered rather than dropped.
func (s *subscriber) forward() {
	defer close(s.out)
	for {
		select {
		case <-s.done:
			s.flush()
			return
		case event := <-s.stash:
			select {
			case s.out <- event:
			case <-s.done:
				s.deliver(event)
				s.flush()
				return
			}
		}
	}
}

// deliver sends one event to the output, giving up after a short bound.
func (s *subscriber) deliver(event agents.NormalizedEvent) {
	select {
	case s.out <- event:
	case <-time.After(builtinDeliverTimeout):
	}
}

// flush delivers the subscriber's already-queued events to its output.
func (s *subscriber) flush() {
	for {
		select {
		case event := <-s.stash:
			s.deliver(event)
		default:
			return
		}
	}
}

// watch closes the subscriber when either the caller's context ends or the
// instance stops, then unregisters it.
func (s *subscriber) watch(ctx context.Context, inst *instance) {
	select {
	case <-ctx.Done():
	case <-inst.done:
	}
	s.once.Do(func() { close(s.done) })
	inst.removeSubscriber(s)
}
