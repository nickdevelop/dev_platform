package instances

import (
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

// TestReplayRingEviction verifies that once the oldest events are evicted,
// resume positions inside the retained window still replay contiguously
// while older positions are refused so the client rebuilds from history.
func TestReplayRingEviction(t *testing.T) {
	r := newReplayRing()
	total := uint64(replayBufferSize + 10)
	for seq := uint64(1); seq <= total; seq++ {
		r.add(ringEvent(seq))
	}
	// The ring kept the newest replayBufferSize events: sequences
	// total-replayBufferSize+1 .. total. One position before the oldest
	// retained event the gap starts at an evicted sequence and coverage is
	// refused; on it, the whole retained window replays.
	firstRetained := total - replayBufferSize + 1
	if _, covered := r.since(firstRetained - 2); covered {
		t.Fatalf("since(%d) reached behind the eviction point but reported coverage", firstRetained-2)
	}
	if events, covered := r.since(firstRetained - 1); !covered || len(events) != replayBufferSize {
		t.Fatalf("since(firstRetained-1) = %d events covered=%v, want %d covered", len(events), covered, replayBufferSize)
	}
	if _, covered := r.since(1); covered {
		t.Fatal("since(1) was evicted but reported coverage")
	}
	if events, covered := r.since(total); !covered || len(events) != 0 {
		t.Fatalf("since(newest) = %d events covered=%v, want empty and covered", len(events), covered)
	}
}

func seqs(events []agents.NormalizedEvent) []uint64 {
	out := make([]uint64, 0, len(events))
	for _, ev := range events {
		out = append(out, ev.Sequence)
	}
	return out
}
