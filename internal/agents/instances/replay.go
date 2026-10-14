package instances

import (
	"sync"

	"devplatform/internal/agents"
)

// replayBufferSize bounds how many normalized events one instance's replay
// ring retains for SSE clients that reconnect with a Last-Event-ID. The ring
// holds the most recent events and drops the oldest, so a client that was
// away longer than the ring covers is resynchronized from history rather
// than handed a misleading partial tail.
const replayBufferSize = 1024

// replayRing is a fixed-capacity ring of an instance's most recent
// normalized events. Sequences are allocated per event per instance, so the
// ring can answer "what happened after sequence N" and say whether the
// answer is complete or the requester's gap starts before the oldest event
// the ring still holds.
type replayRing struct {
	mu    sync.Mutex
	buf   []agents.NormalizedEvent
	next  int    // write position; buf is full once count reaches cap
	count int    // events retained, never above cap
	// first is the sequence of the oldest retained event and last the newest
	// observed, valid while seen is true. They let since distinguish an
	// evicted gap from an id ahead of the ring (a recreated stream whose
	// sequences restarted).
	first, last uint64
	seen        bool
}

func newReplayRing() *replayRing {
	return &replayRing{buf: make([]agents.NormalizedEvent, replayBufferSize)}
}

// add retains ev, evicting the oldest buffered event when the ring is full.
// Events must be added in sequence order.
func (r *replayRing) add(ev agents.NormalizedEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.count < len(r.buf) {
		if r.count == 0 {
			r.first = ev.Sequence
		}
		r.buf[r.next] = ev
		r.count++
	} else {
		// The write position is the oldest slot: overwrite it and slide the
		// oldest-sequence marker forward by one.
		r.buf[r.next] = ev
		r.first = r.buf[(r.next+1)%len(r.buf)].Sequence
	}
	r.next = (r.next + 1) % len(r.buf)
	r.last = ev.Sequence
	r.seen = true
}

// since returns the buffered events with Sequence greater than after, oldest
// first, and reports whether the ring covers everything after it. Sequences
// increase by one, so the ring covers after when its oldest event is at most
// one past it. The coverage answer is false when nothing has been buffered
// yet, when older events were evicted, and when after is newer than the
// newest observed event (the id comes from a stream that no longer exists, so
// the client must rebuild from history).
func (r *replayRing) since(after uint64) ([]agents.NormalizedEvent, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.seen {
		return nil, false
	}
	if after+1 < r.first || after > r.last {
		return nil, false
	}
	// after is inside the covered window, so every retained event with a
	// sequence past it is contiguous and complete.
	var out []agents.NormalizedEvent
	start := (r.next - r.count + len(r.buf)) % len(r.buf)
	for i := 0; i < r.count; i++ {
		ev := r.buf[(start+i)%len(r.buf)]
		if ev.Sequence > after {
			out = append(out, ev)
		}
	}
	return out, true
}
