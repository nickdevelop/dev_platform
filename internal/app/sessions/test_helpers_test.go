package sessions

import (
	"bytes"
	"strings"
	"sync"
	"testing"
	"time"

	"devplatform/internal/core/agents"
)

type recordingWriter struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (w *recordingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.Write(p)
}

func (w *recordingWriter) Bytes() []byte {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]byte(nil), w.buf.Bytes()...)
}

type sseFrame struct {
	id    string
	event string
	data  string
}

func parseFrames(t *testing.T, raw []byte) []sseFrame {
	t.Helper()

	var frames []sseFrame
	for _, block := range strings.Split(string(raw), "\n\n") {
		block = strings.TrimSpace(block)
		if block == "" || strings.HasPrefix(block, ":") {
			continue
		}
		var f sseFrame
		for _, line := range strings.Split(block, "\n") {
			switch {
			case strings.HasPrefix(line, "id: "):
				f.id = strings.TrimPrefix(line, "id: ")
			case strings.HasPrefix(line, "event: "):
				f.event = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				f.data = strings.TrimPrefix(line, "data: ")
			}
		}
		frames = append(frames, f)
	}
	return frames
}

func streamMessageEvent(seq uint64, nativeID, text string) agents.NormalizedEvent {
	return agents.NormalizedEvent{
		InstanceID: "unused",
		Backend:    agents.BackendPi,
		SessionID:  nativeID,
		Sequence:   seq,
		Timestamp:  time.Now().UTC(),
		Type:       agents.EventMessage,
		Message:    &agents.MessagePayload{Role: "assistant", Text: text, Final: true},
	}
}

func waitForFrames(t *testing.T, w *recordingWriter, count int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for len(parseFrames(t, w.Bytes())) < count {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %d SSE frames; got %d (raw: %s)",
				count, len(parseFrames(t, w.Bytes())), w.Bytes())
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func waitForSubscriber(t *testing.T, env *testEnv) chan agents.NormalizedEvent {
	t.Helper()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if env.instances.SubscriberCount(env.instanceID) >= 2 {
			env.fake.mu.Lock()
			ch := env.fake.subs[env.instanceID]
			env.fake.mu.Unlock()
			return ch
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("timed out waiting for the SSE subscriber to attach")
	return nil
}
