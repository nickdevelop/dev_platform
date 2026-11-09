package instances

import (
	"sync"

	"devplatform/internal/core/agents"
)

// Replay retention is bounded by two independent limits: a byte budget keeps a
// few large tool payloads from monopolizing memory, while the event cap keeps a
// stream of tiny deltas from retaining an unbounded slice. Whichever is hit
// first evicts the oldest events.
const (
	replayMaxBytes  = 16 << 20
	replayMaxEvents = 2048
)

// replayRing is a bounded ring of an instance's most recent normalized events.
// Sequences are allocated per event per instance, so the ring can answer "what
// happened after sequence N" and say whether the answer is complete or the
// requester's gap starts before the oldest event the ring still holds.
//
// Eviction is always oldest-first, so the retained events stay a contiguous
// suffix. A single event larger than the byte budget is kept alone rather than
// dropped: dropping it would create a sequence hole that `since` would report
// as covered.
type replayRing struct {
	mu    sync.Mutex
	buf   []agents.NormalizedEvent
	sizes []int
	next  int // write position; buf is full once count reaches cap
	count int // events retained, never above cap
	bytes int // approximate retained payload bytes
	// first is the sequence of the oldest retained event and last the newest
	// observed, valid while seen is true. They let since distinguish an
	// evicted gap from an id ahead of the ring (a recreated stream whose
	// sequences restarted).
	first, last uint64
	seen        bool
}

func newReplayRing() *replayRing {
	return &replayRing{
		buf:   make([]agents.NormalizedEvent, replayMaxEvents),
		sizes: make([]int, replayMaxEvents),
	}
}

// add retains ev, evicting the oldest buffered events when either bound is
// exceeded. Events must be added in sequence order.
func (r *replayRing) add(ev agents.NormalizedEvent) {
	size := eventSize(ev)
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.count == len(r.buf) {
		r.evictOldestLocked()
	}
	index := r.next
	r.buf[index] = ev
	r.sizes[index] = size
	r.next = (index + 1) % len(r.buf)
	r.count++
	r.bytes += size
	if r.count == 1 {
		r.first = ev.Sequence
	}
	r.last = ev.Sequence
	r.seen = true

	for r.bytes > replayMaxBytes && r.count > 1 {
		r.evictOldestLocked()
	}
}

// evictOldestLocked drops the oldest retained event. The caller holds r.mu.
func (r *replayRing) evictOldestLocked() {
	if r.count == 0 {
		return
	}
	start := (r.next - r.count + len(r.buf)) % len(r.buf)
	r.bytes -= r.sizes[start]
	r.buf[start] = agents.NormalizedEvent{}
	r.sizes[start] = 0
	r.count--
	if r.count > 0 {
		r.first = r.buf[(start+1)%len(r.buf)].Sequence
	} else {
		r.first = 0
	}
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

// eventSize estimates how many bytes one event keeps alive. It is a cheap
// accounting heuristic, not the exact heap size: it counts the variable-length
// payload fields that dominate retained memory and a fixed envelope overhead.
func eventSize(ev agents.NormalizedEvent) int {
	const envelopeOverhead = 192
	size := envelopeOverhead + len(ev.InstanceID) + len(ev.SessionID) + len(ev.Raw)
	if ev.Lifecycle != nil {
		size += len(ev.Lifecycle.State) + len(ev.Lifecycle.Detail)
	}
	if ev.Message != nil {
		size += len(ev.Message.Role) + len(ev.Message.Text)
		for _, image := range ev.Message.Images {
			size += len(image.ID) + len(image.Name) + len(image.MediaType) + len(image.Data) + len(image.URL)
		}
		if ev.Message.Model != nil {
			size += len(ev.Message.Model.ID) + len(ev.Message.Model.Provider) + len(ev.Message.Model.Variant)
		}
	}
	if ev.Thinking != nil {
		size += len(ev.Thinking.Text)
	}
	if ev.ToolCall != nil {
		size += len(ev.ToolCall.CallID) + len(ev.ToolCall.Name) + len(ev.ToolCall.Input)
	}
	if ev.ToolResult != nil {
		size += len(ev.ToolResult.CallID) + len(ev.ToolResult.Output) + len(ev.ToolResult.Metadata)
	}
	if ev.Error != nil {
		size += len(ev.Error.Code) + len(ev.Error.Message)
	}
	if ev.Turn != nil {
		size += len(ev.Turn.ID)
	}
	if ev.Interaction != nil {
		size += len(ev.Interaction.ID)
		if permission := ev.Interaction.Permission; permission != nil {
			size += len(permission.ID) + len(permission.SessionID) + len(permission.Action) + len(permission.Message)
			for _, resource := range permission.Resources {
				size += len(resource)
			}
		}
		if form := ev.Interaction.Form; form != nil {
			size += len(form.ID) + len(form.SessionID) + len(form.Title) + len(form.Fields)
		}
	}
	return size
}
