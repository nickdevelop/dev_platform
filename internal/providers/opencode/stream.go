package opencode

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"devplatform/internal/core/agents"
)

// subscriber is one consumer of an instance's stream. stash is written by
// publishers and never closed; out is owned and closed by forward.
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

// drainTimeout bounds how long forward waits to flush an already-queued event
// to a consumer at teardown, so a terminal event is not lost while a gone
// consumer can never block the goroutine forever.
const drainTimeout = 100 * time.Millisecond

// forward copies the subscriber's stash to its output and closes the output
// when the subscription ends. On teardown it flushes events already queued
// (for example a terminal `failed` lifecycle published immediately before the
// instance was torn down) so the consumer sees them before the channel closes.
func (s *subscriber) forward() {
	defer close(s.out)
	for {
		select {
		case event := <-s.stash:
			select {
			case s.out <- event:
			case <-s.done:
				// The subscription ended, but this event was already read
				// from the stash: deliver it (bounded) before flushing the
				// rest, so a terminal event is never dropped.
				s.deliver(event)
				s.flush()
				return
			}
		case <-s.done:
			s.flush()
			return
		}
	}
}

// deliver sends one event to the output, giving up after drainTimeout so a
// gone consumer cannot wedge the goroutine.
func (s *subscriber) deliver(event agents.NormalizedEvent) {
	select {
	case s.out <- event:
	case <-time.After(drainTimeout):
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

// addSubscriber registers a subscriber unless the instance is already closed.
func (inst *instance) addSubscriber(sub *subscriber) error {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	if inst.closed {
		return fmt.Errorf("opencode: instance %q is not running", inst.id)
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

// ensureStream starts the instance's single backend event reader on first use.
func (inst *instance) ensureStream() {
	inst.mu.Lock()
	if inst.streamStarted || inst.closed {
		inst.mu.Unlock()
		return
	}
	inst.streamStarted = true
	inst.mu.Unlock()
	go inst.runStream()
}

// publish stamps the envelope, assigns the next per-instance sequence and
// delivers the event to every current subscriber.
func (inst *instance) publish(event agents.NormalizedEvent) {
	inst.mu.Lock()
	if inst.closed {
		inst.mu.Unlock()
		return
	}
	inst.seq++
	event.Sequence = inst.seq
	event.InstanceID = inst.id
	event.Backend = agents.BackendOpenCodeV2
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

// runStream reads the server's SSE stream, reconnects with backoff after a
// drop, and reports each error without affecting any other instance. The
// backoff is reset only when a connection actually delivered an event, so a
// server that accepts and immediately closes the stream backs off up to the
// cap instead of hammering it at the minimum delay. A malformed payload is
// reported with the malformed_event code so the UI can tell a bad stream from
// a dead server.
func (inst *instance) runStream() {
	backoff := streamRetryMin
	for {
		if !inst.active() {
			return
		}
		events, err := inst.consume()
		if !inst.active() {
			return
		}
		if err != nil {
			code := "stream_error"
			if errors.Is(err, errMalformedEvent) {
				code = "malformed_event"
			}
			inst.publishStreamError(code, err)
		}
		if events > 0 {
			// A connection that delivered events was healthy; reset so a
			// long-lived stream that later drops reconnects promptly.
			backoff = streamRetryMin
		}
		if !inst.pause(backoff) {
			return
		}
		backoff = nextBackoff(backoff)
	}
}

// consume opens one event-stream connection, guards it with an idle watchdog,
// and drains it. It returns the number of events read and the terminating
// error. A silent-but-open stream is canceled by the watchdog and reported as
// an error so runStream reconnects instead of blocking forever.
func (inst *instance) consume() (int, error) {
	attemptCtx, cancel := context.WithCancel(inst.ctx)
	defer cancel()

	activity := make(chan struct{}, 1)
	watchdogDone := make(chan struct{})
	// inflight reports whether a normalized event is currently wedged inside
	// inst.publish waiting for a slow subscriber. That is downstream
	// backpressure, not a silent server: cancelling on it would force a
	// reconnect whose gap the platform cannot replay (events that never
	// reached the instance ring), so the watchdog only fires when the pump is
	// genuinely blocked on reading.
	inflight := new(atomic.Bool)
	if inst.streamIdle > 0 {
		go func() {
			defer close(watchdogDone)
			timer := time.NewTimer(inst.streamIdle)
			defer timer.Stop()
			for {
				select {
				case <-attemptCtx.Done():
					return
				case <-activity:
					if !timer.Stop() {
						select {
						case <-timer.C:
						default:
						}
					}
					timer.Reset(inst.streamIdle)
				case <-timer.C:
					if inflight.Load() {
						// A publish is blocked on a slow subscriber. Stay
						// passive and re-arm: the stream is alive, only the
						// consumer is slow.
						timer.Reset(inst.streamIdle)
						continue
					}
					// No data within the idle window: force the blocked
					// read to unblock so the stream reconnects.
					cancel()
					return
				}
			}
		}()
	} else {
		close(watchdogDone)
	}

	stream, err := inst.client.Events(attemptCtx)
	if err != nil {
		cancel()
		<-watchdogDone
		return 0, err
	}
	// Every line from the server — heartbeat comments included — resets the
	// idle watchdog. opencode sends ": heartbeat" every few seconds while a
	// turn is quiet, so only decoded events would let a healthy stream look
	// silent and be wrongly canceled.
	stream.SetActivity(func() {
		select {
		case activity <- struct{}{}:
		default:
		}
	})
	events, err := inst.pump(stream, inflight)
	_ = stream.Close()
	cancel()
	<-watchdogDone
	return events, err
}

// pump reads one stream to completion, normalizing and publishing each event.
// It returns the number of events read so the caller can tell a healthy
// connection from one that closed without ever producing data.
func (inst *instance) pump(stream *EventStream, inflight *atomic.Bool) (int, error) {
	events := 0
	for {
		event, err := stream.Next()
		if err != nil {
			return events, err
		}
		events++
		for _, normalized := range inst.normalize(event) {
			// Latch while a publish is in progress so the idle watchdog can
			// tell downstream backpressure from a silent server.
			inflight.Store(true)
			inst.publish(normalized)
			inflight.Store(false)
		}
	}
}

// pause sleeps for the backoff through the test seam when set, otherwise using
// the instance's cancellation-aware wait. It reports false when the instance
// ended.
func (inst *instance) pause(d time.Duration) bool {
	if inst.sleep != nil {
		return inst.sleep(d)
	}
	return inst.wait(d)
}

// wait sleeps for the backoff or returns false when the instance ended.
func (inst *instance) wait(d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-inst.done:
		return false
	case <-inst.ctx.Done():
		return false
	}
}

// nextBackoff doubles the delay up to the cap.
func nextBackoff(current time.Duration) time.Duration {
	next := current * 2
	if next > streamRetryMax {
		next = streamRetryMax
	}
	return next
}

// publishStreamError surfaces an instance-level stream failure with the given
// diagnostic code. The contract requires a session id on error events, so
// when a session is known the error is attached to it; otherwise it is
// reported as an instance-scoped lifecycle event, which is valid without a
// session id.
func (inst *instance) publishStreamError(code string, err error) {
	message := truncate(redactSecrets(err.Error()))
	inst.mu.Lock()
	sessionID := ""
	for id := range inst.sessions {
		sessionID = id
		break
	}
	inst.mu.Unlock()

	if sessionID != "" {
		inst.publish(inst.errorEvent(sessionID, code, message))
		return
	}
	event := inst.event("")
	event.Type = agents.EventLifecycle
	event.Lifecycle = &agents.LifecycleEvent{State: code, Detail: message}
	inst.publish(event)
}
