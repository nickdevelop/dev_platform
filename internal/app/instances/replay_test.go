package instances

import (
	"strings"
	"testing"
	"time"

	"devplatform/internal/core/agents"
)

func ringEvent(seq uint64) agents.NormalizedEvent {
	return agents.NormalizedEvent{
		InstanceID: "inst-1",
		Backend:    agents.BackendPi,
		SessionID:  "native-1",
		Sequence:   seq,
		Timestamp:  time.Now().UTC(),
		Type:       agents.EventMessage,
		Message:    &agents.MessagePayload{Role: "assistant", Text: "chunk", Final: true},
	}
}

func ringResultEvent(seq uint64, output string) agents.NormalizedEvent {
	return agents.NormalizedEvent{
		InstanceID: "inst-1",
		Backend:    agents.BackendPi,
		SessionID:  "native-1",
		Sequence:   seq,
		Timestamp:  time.Now().UTC(),
		Type:       agents.EventToolResult,
		ToolResult: &agents.ToolResultPayload{CallID: "call-1", Output: output},
	}
}

// TestReplayRingEmpty verifies a ring that has seen nothing cannot vouch for
// any resume position: a reconnecting client must resynchronize.
func TestReplayRingEmpty(t *testing.T) {
	r := newReplayRing()
	if events, covered := r.since(0); covered || events != nil {
		t.Fatalf("since(0) on empty ring = %v/%v, want no events and no coverage", events, covered)
	}
}

// TestReplayRingWithinBuffer verifies the covered-window answers: everything
// past a position inside the window replays contiguously, a position at the
// newest event is caught up (nothing to replay), and a position ahead of the
// newest event (an id from a stream that no longer exists) is refused.
func TestReplayRingWithinBuffer(t *testing.T) {
	r := newReplayRing()
	for seq := uint64(1); seq <= 5; seq++ {
		r.add(ringEvent(seq))
	}

	events, covered := r.since(3)
	if !covered || len(events) != 2 || events[0].Sequence != 4 || events[1].Sequence != 5 {
		t.Fatalf("since(3) = %v/%v, want sequences 4,5 covered", seqs(events), covered)
	}
	// Position zero is fully covered while the ring still holds the stream's
	// very first event.
	if events, covered := r.since(0); !covered || len(events) != 5 {
		t.Fatalf("since(0) = %d events covered=%v, want all 5 covered", len(events), covered)
	}
	// A caught-up client gets an empty but complete answer, so it does not
	// resynchronize needlessly.
	if events, covered := r.since(5); !covered || len(events) != 0 {
		t.Fatalf("since(5) = %d events covered=%v, want empty and covered", len(events), covered)
	}
	if events, covered := r.since(6); covered || events != nil {
		t.Fatalf("since(6) = %v/%v, want no events and no coverage", events, covered)
	}
}

// TestReplayRingEventCap verifies the count bound: a stream of small events
// keeps the newest replayMaxEvents, and positions before that window cannot be
// covered.
func TestReplayRingEventCap(t *testing.T) {
	r := newReplayRing()
	total := uint64(replayMaxEvents + 10)
	for seq := uint64(1); seq <= total; seq++ {
		r.add(ringEvent(seq))
	}
	// The ring kept the newest replayMaxEvents events.
	firstRetained := total - replayMaxEvents + 1
	if _, covered := r.since(firstRetained - 2); covered {
		t.Fatalf("since(%d) reached behind the eviction point but reported coverage", firstRetained-2)
	}
	if events, covered := r.since(firstRetained - 1); !covered || len(events) != replayMaxEvents {
		t.Fatalf("since(firstRetained-1) = %d events covered=%v, want %d covered", len(events), covered, replayMaxEvents)
	}
	if _, covered := r.since(1); covered {
		t.Fatal("since(1) was evicted but reported coverage")
	}
	if events, covered := r.since(total); !covered || len(events) != 0 {
		t.Fatalf("since(newest) = %d events covered=%v, want empty and covered", len(events), covered)
	}
}

// TestReplayRingByteCap verifies the byte bound: large tool results evict older
// events before the count cap is reached, while coverage remains a contiguous
// suffix.
func TestReplayRingByteCap(t *testing.T) {
	r := newReplayRing()
	output := strings.Repeat("x", replayMaxBytes/4)
	for seq := uint64(1); seq <= 5; seq++ {
		r.add(ringResultEvent(seq, output))
	}
	// Each event is roughly a quarter of the budget, so at most three fit
	// after the last add.
	events, covered := r.since(2)
	if !covered || len(events) != 3 {
		t.Fatalf("since(2) = %d events covered=%v, want 3 covered", len(events), covered)
	}
	if first := events[0].Sequence; first != 3 {
		t.Fatalf("oldest retained sequence = %d, want 3", first)
	}
	if _, covered := r.since(1); covered {
		t.Fatal("since(1) was evicted by the byte budget but reported coverage")
	}
}

// TestReplayRingOversizedEvent verifies a single event larger than the byte
// budget is kept alone instead of creating a sequence hole, and is evicted when
// the next event arrives.
func TestReplayRingOversizedEvent(t *testing.T) {
	r := newReplayRing()
	r.add(ringResultEvent(1, strings.Repeat("x", replayMaxBytes*2)))
	if events, covered := r.since(0); !covered || len(events) != 1 {
		t.Fatalf("since(0) = %d events covered=%v, want the oversized event covered", len(events), covered)
	}

	r.add(ringEvent(2))
	events, covered := r.since(1)
	if !covered || len(events) != 1 || events[0].Sequence != 2 {
		t.Fatalf("since(1) = %v/%v, want only the newest event", seqs(events), covered)
	}
	if _, covered := r.since(0); covered {
		t.Fatal("since(0) reported coverage after the oversized event was evicted")
	}
}

func seqs(events []agents.NormalizedEvent) []uint64 {
	out := make([]uint64, 0, len(events))
	for _, ev := range events {
		out = append(out, ev.Sequence)
	}
	return out
}
