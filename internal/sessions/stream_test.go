package sessions

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"devplatform/internal/agents"
)

// recordingWriter captures SSE output. It is safe for concurrent use because
// the streaming writer and the test read it at the same time.
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

// blockingWriter blocks on the first write until its gate is closed, modelling
// a client whose socket cannot keep up.
type blockingWriter struct {
	gate chan struct{}
}

func (w *blockingWriter) Write(p []byte) (int, error) {
	<-w.gate
	return len(p), nil
}

// sseFrame is one parsed Server-Sent Event.
type sseFrame struct {
	id    string
	event string
	data  string
}

// parseFrames parses SSE frames from a byte stream.
func parseFrames(t *testing.T, raw []byte) []sseFrame {
	t.Helper()

	var frames []sseFrame
	for _, block := range strings.Split(string(raw), "\n\n") {
		if strings.TrimSpace(block) == "" {
			continue
		}
		frame := sseFrame{}
		for _, line := range strings.Split(block, "\n") {
			switch {
			case strings.HasPrefix(line, "id: "):
				frame.id = strings.TrimPrefix(line, "id: ")
			case strings.HasPrefix(line, "event: "):
				frame.event = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				frame.data = strings.TrimPrefix(line, "data: ")
			}
		}
		frames = append(frames, frame)
	}
	return frames
}

// waitForFrames blocks until the writer holds at least n complete frames.
func waitForFrames(t *testing.T, w *recordingWriter, n int) {
	t.Helper()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if len(parseFrames(t, w.Bytes())) >= n {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d SSE frames; got %q", n, w.Bytes())
}

// waitForSubscriber blocks until the fake adapter has an active subscription
// for instanceID and returns its channel.
func waitForSubscriber(t *testing.T, f *fakeAdapter, instanceID string) chan agents.NormalizedEvent {
	t.Helper()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		f.mu.Lock()
		ch := f.subs[instanceID]
		f.mu.Unlock()
		if ch != nil {
			return ch
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("timed out waiting for an instance subscription")
	return nil
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

// TestStreamSessionFramesOrderedEvents verifies the SSE framing: each event is
// an id/event/data frame, sequence order is preserved, the platform session id
// replaces the native one, and unknown payloads survive.
func TestStreamSessionFramesOrderedEvents(t *testing.T) {
	env := newTestEnv(t)
	session := createNativeSession(t, env, "native-1")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := &recordingWriter{}
	done := make(chan error, 1)
	go func() {
		done <- env.svc.StreamSession(ctx, env.projectID, session.ID, w, nil)
	}()

	waitForSubscriber(t, env.fake, env.instanceID)
	env.fake.emit(t, env.instanceID, streamMessageEvent(1, "native-1", "first"))
	env.fake.emit(t, env.instanceID, streamMessageEvent(2, "native-1", "second"))
	env.fake.emit(t, env.instanceID, agents.NormalizedEvent{
		InstanceID: "unused",
		Backend:    agents.BackendPi,
		SessionID:  "native-1",
		Sequence:   3,
		Timestamp:  time.Now().UTC(),
		Type:       agents.EventUnknown,
		Raw:        json.RawMessage(`{"backendField":"kept"}`),
	})
	waitForFrames(t, w, 3)

	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("StreamSession error = %v, want context.Canceled", err)
	}

	frames := parseFrames(t, w.Bytes())
	if len(frames) != 3 {
		t.Fatalf("got %d frames, want 3: %+v", len(frames), frames)
	}
	wantIDs := []string{"1", "2", "3"}
	for i, frame := range frames {
		if frame.id != wantIDs[i] {
			t.Errorf("frame[%d] id = %q, want %q", i, frame.id, wantIDs[i])
		}
		var payload struct {
			SessionID string          `json:"sessionId"`
			Type      string          `json:"type"`
			Raw       json.RawMessage `json:"raw"`
			Message   *struct {
				Text string `json:"text"`
			} `json:"message"`
		}
		if err := json.Unmarshal([]byte(frame.data), &payload); err != nil {
			t.Fatalf("frame[%d] data is not JSON: %v", i, err)
		}
		if payload.SessionID != session.ID {
			t.Errorf("frame[%d] sessionId = %q, want the platform id %q", i, payload.SessionID, session.ID)
		}
	}

	if frames[0].event != string(agents.EventMessage) || frames[1].event != string(agents.EventMessage) {
		t.Errorf("first two frame events = %q/%q, want message", frames[0].event, frames[1].event)
	}
	if frames[2].event != string(agents.EventUnknown) {
		t.Errorf("third frame event = %q, want unknown", frames[2].event)
	}
	if !strings.Contains(frames[2].data, `"backendField":"kept"`) {
		t.Errorf("unknown event raw metadata was not preserved: %s", frames[2].data)
	}
}

// TestStreamSessionSkipsMalformedAndPreservesUnknown verifies a malformed
// normalized event never reaches the client while a valid unknown event does.
func TestStreamSessionSkipsMalformedAndPreservesUnknown(t *testing.T) {
	env := newTestEnv(t)
	session := createNativeSession(t, env, "native-1")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := &recordingWriter{}
	done := make(chan error, 1)
	go func() {
		done <- env.svc.StreamSession(ctx, env.projectID, session.ID, w, nil)
	}()

	waitForSubscriber(t, env.fake, env.instanceID)
	// A message event with no payload is malformed and must be dropped.
	env.fake.emit(t, env.instanceID, agents.NormalizedEvent{
		InstanceID: "unused",
		Backend:    agents.BackendPi,
		SessionID:  "native-1",
		Sequence:   1,
		Timestamp:  time.Now().UTC(),
		Type:       agents.EventMessage,
	})
	// An unknown event with no raw body is malformed and must be dropped.
	env.fake.emit(t, env.instanceID, agents.NormalizedEvent{
		InstanceID: "unused",
		Backend:    agents.BackendPi,
		SessionID:  "native-1",
		Sequence:   2,
		Timestamp:  time.Now().UTC(),
		Type:       agents.EventUnknown,
	})
	// An unknown event whose raw body is not valid JSON must be dropped at
	// the frame boundary rather than tearing down the stream.
	env.fake.emit(t, env.instanceID, agents.NormalizedEvent{
		InstanceID: "unused",
		Backend:    agents.BackendPi,
		SessionID:  "native-1",
		Sequence:   3,
		Timestamp:  time.Now().UTC(),
		Type:       agents.EventUnknown,
		Raw:        json.RawMessage(`not json`),
	})
	// A valid unknown event must be delivered with its raw payload.
	env.fake.emit(t, env.instanceID, agents.NormalizedEvent{
		InstanceID: "unused",
		Backend:    agents.BackendPi,
		SessionID:  "native-1",
		Sequence:   4,
		Timestamp:  time.Now().UTC(),
		Type:       agents.EventUnknown,
		Raw:        json.RawMessage(`{"diagnostic":true}`),
	})
	waitForFrames(t, w, 1)

	cancel()
	<-done

	frames := parseFrames(t, w.Bytes())
	if len(frames) != 1 {
		t.Fatalf("got %d frames, want only the valid unknown event: %+v", len(frames), frames)
	}
	if frames[0].event != string(agents.EventUnknown) || !strings.Contains(frames[0].data, `"diagnostic":true`) {
		t.Errorf("valid unknown event not preserved: %+v", frames[0])
	}
}

// TestStreamSessionSlowClientDisconnectsWithoutStalling verifies the SSE layer
// bounds per-client buffering and disconnects a client that cannot keep up
// without stalling the shared instance event stream.
func TestStreamSessionSlowClientDisconnectsWithoutStalling(t *testing.T) {
	env := newTestEnv(t)
	env.svc.streamBuffer = 4
	env.fake.mu.Lock()
	env.fake.subBuffer = 8
	env.fake.mu.Unlock()

	session := createNativeSession(t, env, "native-1")

	gate := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- env.svc.StreamSession(context.Background(), env.projectID, session.ID, &blockingWriter{gate: gate}, nil)
	}()

	ch := waitForSubscriber(t, env.fake, env.instanceID)

	// Flood far more events than any buffer; a healthy policy keeps draining
	// even while the client is wedged, so this must not block.
	emitted := make(chan error, 1)
	go func() {
		for i := 0; i < 300; i++ {
			select {
			case ch <- streamMessageEvent(uint64(i+1), "native-1", "chunk"):
			case <-time.After(2 * time.Second):
				emitted <- errors.New("instance stream stalled behind the slow client")
				return
			}
		}
		emitted <- nil
	}()

	select {
	case err := <-emitted:
		if err != nil {
			t.Fatalf("flooding events: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("flooding events blocked; the instance stream stalled behind the slow client")
	}

	// Release the wedged writer; the stream must terminate as a slow client.
	close(gate)
	select {
	case err := <-done:
		if !errors.Is(err, errSlowClient) {
			t.Fatalf("StreamSession error = %v, want errSlowClient", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("StreamSession did not disconnect the slow client")
	}
}

// TestStreamSessionEndMarksSessionDisconnected verifies that when the
// instance's event stream ends the session is marked disconnected.
func TestStreamSessionEndMarksSessionDisconnected(t *testing.T) {
	env := newTestEnv(t)
	session := createNativeSession(t, env, "native-1")

	done := make(chan error, 1)
	go func() {
		done <- env.svc.StreamSession(context.Background(), env.projectID, session.ID, &recordingWriter{}, nil)
	}()

	waitForSubscriber(t, env.fake, env.instanceID)
	if _, err := env.instances.Stop(context.Background(), env.instanceID); err != nil {
		t.Fatalf("stop instance: %v", err)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("StreamSession error = %v, want nil on stream end", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("StreamSession did not end when the instance stream closed")
	}

	got, err := env.svc.Get(env.projectID, session.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != SessionDisconnected {
		t.Errorf("session status = %q, want %q", got.Status, SessionDisconnected)
	}
}

// TestStreamSessionClientDisconnectLeavesInstanceRunning verifies that an SSE
// client going away never stops the instance or mutates the session state.
func TestStreamSessionClientDisconnectLeavesInstanceRunning(t *testing.T) {
	env := newTestEnv(t)
	session := createNativeSession(t, env, "native-1")

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- env.svc.StreamSession(ctx, env.projectID, session.ID, &recordingWriter{}, nil)
	}()

	waitForSubscriber(t, env.fake, env.instanceID)
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("StreamSession error = %v, want context.Canceled", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("StreamSession did not return after the client disconnected")
	}

	inst, err := env.instances.Get(env.instanceID)
	if err != nil {
		t.Fatalf("Get instance: %v", err)
	}
	if inst.Status != agents.InstanceRunning {
		t.Errorf("instance status = %q, want running", inst.Status)
	}
	if got := env.fake.stopCallCount(); got != 0 {
		t.Errorf("adapter StopInstance calls = %d, want 0", got)
	}
	got, err := env.svc.Get(env.projectID, session.ID)
	if err != nil {
		t.Fatalf("Get session: %v", err)
	}
	if got.Status != SessionActive {
		t.Errorf("session status = %q, want active", got.Status)
	}
}

// TestStreamSessionRejectsCrossProjectSession verifies the SSE route validates
// project scope before streaming.
func TestStreamSessionRejectsCrossProjectSession(t *testing.T) {
	env := newTestEnv(t)
	otherProject := env.registerProject(t, "beta")
	session := createNativeSession(t, env, "native-1")

	err := env.svc.StreamSession(context.Background(), otherProject, session.ID, &recordingWriter{}, nil)
	if !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("StreamSession cross-project error = %v, want ErrSessionNotFound", err)
	}
}
