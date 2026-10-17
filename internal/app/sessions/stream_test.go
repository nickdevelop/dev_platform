package sessions

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"devplatform/internal/core/agents"
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

// waitForSubscriber blocks until the session's SSE stream has attached its
// subscriber and returns the instance's shared upstream channel. Creating a
// session starts the always-on turn tracker, which subscribes to the instance
// stream first; waiting for a subscription alone would return before the SSE
// subscriber is attached, so this waits for the second subscriber.
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
		done <- env.svc.StreamSession(ctx, env.projectID, session.ID, 0, w, nil)
	}()

	waitForSubscriber(t, env)
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
	waitForFrames(t, w, 4)

	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("StreamSession error = %v, want context.Canceled", err)
	}

	frames := parseFrames(t, w.Bytes())
	if len(frames) != 4 {
		t.Fatalf("got %d frames, want the join frame plus 3 events: %+v", len(frames), frames)
	}
	// The stream opens with the session's turn state, framed as a turn boundary.
	// That frame belongs to no position in the sequence, so it carries no id.
	if frames[0].event != string(agents.EventTurn) || frames[0].id != "" {
		t.Errorf("join frame = %+v, want an id-less turn frame", frames[0])
	}
	wantIDs := []string{"", "1", "2", "3"}
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

	if frames[1].event != string(agents.EventMessage) || frames[2].event != string(agents.EventMessage) {
		t.Errorf("message frame events = %q/%q, want message", frames[1].event, frames[2].event)
	}
	if frames[3].event != string(agents.EventUnknown) {
		t.Errorf("unknown frame event = %q, want unknown", frames[3].event)
	}
	if !strings.Contains(frames[3].data, `"backendField":"kept"`) {
		t.Errorf("unknown event raw metadata was not preserved: %s", frames[3].data)
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
		done <- env.svc.StreamSession(ctx, env.projectID, session.ID, 0, w, nil)
	}()

	waitForSubscriber(t, env)
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
	waitForFrames(t, w, 2)

	cancel()
	<-done

	frames := parseFrames(t, w.Bytes())
	if len(frames) != 2 {
		t.Fatalf("got %d frames, want the join frame and the valid unknown event: %+v", len(frames), frames)
	}
	if frames[0].event != string(agents.EventTurn) {
		t.Errorf("join frame event = %q, want turn", frames[0].event)
	}
	if frames[1].event != string(agents.EventUnknown) || !strings.Contains(frames[1].data, `"diagnostic":true`) {
		t.Errorf("valid unknown event not preserved: %+v", frames[1])
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
		done <- env.svc.StreamSession(context.Background(), env.projectID, session.ID, 0, &blockingWriter{gate: gate}, nil)
	}()

	ch := waitForSubscriber(t, env)

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
		done <- env.svc.StreamSession(context.Background(), env.projectID, session.ID, 0, &recordingWriter{}, nil)
	}()

	waitForSubscriber(t, env)
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
		done <- env.svc.StreamSession(ctx, env.projectID, session.ID, 0, &recordingWriter{}, nil)
	}()

	waitForSubscriber(t, env)
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

	err := env.svc.StreamSession(context.Background(), otherProject, session.ID, 0, &recordingWriter{}, nil)
	if !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("StreamSession cross-project error = %v, want ErrSessionNotFound", err)
	}
}

// TestSSEStreamOutlivesServerWriteTimeout verifies the SSE handler clears the
// connection write deadline, so a stream is not truncated by the server's
// absolute WriteTimeout (~60s in production).
func TestSSEStreamOutlivesServerWriteTimeout(t *testing.T) {
	gin.SetMode(gin.TestMode)
	env := newTestEnv(t)
	router := gin.New()
	env.svc.RegisterRoutes(router)

	session := createNativeSession(t, env, "native-1")

	server := httptest.NewUnstartedServer(router)
	server.Config.WriteTimeout = 100 * time.Millisecond
	server.Start()
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("%s/projects/%d/sessions/%s/events", server.URL, env.projectID, session.ID), nil)
	if err != nil {
		t.Fatalf("build SSE request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("open SSE stream: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("SSE status = %d, want 200", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("SSE Content-Type = %q, want text/event-stream", got)
	}

	waitForSubscriber(t, env)

	// Wait past the server's WriteTimeout, then emit: without a cleared write
	// deadline the flush would fail and no frame would reach the client.
	time.Sleep(250 * time.Millisecond)
	env.fake.emit(t, env.instanceID, streamMessageEvent(1, "native-1", "after-deadline"))

	frames := make(chan string, 1)
	go func() {
		reader := bufio.NewReader(resp.Body)
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			if strings.Contains(line, "after-deadline") {
				frames <- line
				return
			}
		}
	}()

	select {
	case <-frames:
	case <-time.After(3 * time.Second):
		t.Fatal("no SSE frame arrived after the server WriteTimeout; the write deadline was not cleared")
	}
}

// TestStreamSessionResumeReplaysMissedEvents verifies a resuming client is
// caught up: events it missed while away are replayed from the instance's
// buffer before the live events, and a live event that was also buffered
// (published while the resume was in flight) is delivered exactly once.
func TestStreamSessionResumeReplaysMissedEvents(t *testing.T) {
	env := newTestEnv(t)
	session := createNativeSession(t, env, "native-1")

	// The first view sees events 1 and 2, then drops.
	ctx1, cancel1 := context.WithCancel(context.Background())
	w1 := &recordingWriter{}
	done1 := make(chan error, 1)
	go func() {
		done1 <- env.svc.StreamSession(ctx1, env.projectID, session.ID, 0, w1, nil)
	}()
	waitForSubscriber(t, env)
	env.fake.emit(t, env.instanceID, streamMessageEvent(1, "native-1", "first"))
	env.fake.emit(t, env.instanceID, streamMessageEvent(2, "native-1", "second"))
	waitForFrames(t, w1, 2)
	cancel1()
	if err := <-done1; !errors.Is(err, context.Canceled) {
		t.Fatalf("first stream error = %v, want context.Canceled", err)
	}

	// The reconnect resumes from 1: event 2 was missed while away and must
	// be replayed, then live events flow behind it without re-delivering
	// anything the replay already carried.
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	w2 := &recordingWriter{}
	done2 := make(chan error, 1)
	go func() {
		done2 <- env.svc.StreamSession(ctx2, env.projectID, session.ID, 1, w2, nil)
	}()
	env.fake.emit(t, env.instanceID, streamMessageEvent(3, "native-1", "third"))
	waitForFrames(t, w2, 2)
	cancel2()
	<-done2

	frames := parseFrames(t, w2.Bytes())
	var ids []string
	for _, frame := range frames {
		if frame.event == string(agents.EventMessage) {
			ids = append(ids, frame.id)
		}
	}
	if fmt.Sprint(ids) != "[2 3]" {
		t.Fatalf("resumed stream ids = %v, want [2 3] (missed event replayed, live event delivered once)", ids)
	}
}

// TestStreamSessionResumeWithoutCoverageResyncs verifies a resume position
// the replay buffer cannot vouch for (it is ahead of the newest buffered
// event, e.g. it comes from a stream that was recreated since) is answered
// with a resync frame instead of a misleading partial tail, and live events
// keep flowing after it.
func TestStreamSessionResumeWithoutCoverageResyncs(t *testing.T) {
	env := newTestEnv(t)
	session := createNativeSession(t, env, "native-1")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := &recordingWriter{}
	done := make(chan error, 1)
	go func() {
		done <- env.svc.StreamSession(ctx, env.projectID, session.ID, 10, w, nil)
	}()
	waitForSubscriber(t, env)
	// The instance's buffer holds nothing (or at most older events), so
	// position 10 cannot be trusted: the client must rebuild from history.
	env.fake.emit(t, env.instanceID, streamMessageEvent(11, "native-1", "eleventh"))
	waitForFrames(t, w, 3)
	cancel()
	<-done

	frames := parseFrames(t, w.Bytes())
	if len(frames) != 3 {
		t.Fatalf("got %d frames, want resync + join frame + one live event: %+v", len(frames), frames)
	}
	if frames[0].event != "resync" {
		t.Errorf("first frame event = %q, want resync", frames[0].event)
	}
	if frames[1].event != string(agents.EventTurn) || frames[1].id != "" {
		t.Errorf("second frame = %+v, want the id-less turn join frame", frames[1])
	}
	if frames[2].id != "11" || frames[2].event != string(agents.EventMessage) {
		t.Errorf("third frame = %+v, want the live message at id 11", frames[2])
	}
}

// turnBoundaryEvent builds a turn boundary for the environment's session.
func turnBoundaryEvent(phase agents.TurnPhase, outcome agents.TurnOutcome) agents.NormalizedEvent {
	return agents.NormalizedEvent{
		InstanceID: "unused",
		Backend:    agents.BackendPi,
		SessionID:  "native-1",
		Sequence:   1,
		Timestamp:  time.Now().UTC(),
		Type:       agents.EventTurn,
		Turn:       &agents.TurnPayload{ID: "turn-1", Phase: phase, Outcome: outcome},
	}
}

// waitForTurnState blocks until the tracker reports the wanted state for the
// environment's session.
func waitForTurnState(t *testing.T, env *testEnv, want TurnState) {
	t.Helper()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if env.svc.turnTracker.Of(agents.BackendPi, "native-1") == want {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for turn state %q", want)
}

// assertJoinTurnPhase opens a stream and checks that its first frame is the
// session's turn state: an id-less turn frame with the wanted phase.
func assertJoinTurnPhase(t *testing.T, env *testEnv, sessionID string, want agents.TurnPhase) {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := &recordingWriter{}
	done := make(chan error, 1)
	go func() {
		done <- env.svc.StreamSession(ctx, env.projectID, sessionID, 0, w, nil)
	}()
	waitForFrames(t, w, 1)
	cancel()
	<-done

	frames := parseFrames(t, w.Bytes())
	join := frames[0]
	if join.event != string(agents.EventTurn) || join.id != "" {
		t.Fatalf("join frame = %+v, want an id-less turn frame", join)
	}
	var payload struct {
		Turn *agents.TurnPayload `json:"turn"`
	}
	if err := json.Unmarshal([]byte(join.data), &payload); err != nil {
		t.Fatalf("join frame data is not JSON: %v", err)
	}
	if payload.Turn == nil || payload.Turn.Phase != want {
		t.Fatalf("join frame turn = %+v, want phase %q", payload.Turn, want)
	}
}

// TestStreamSessionOpensWithTurnState verifies the first frame of a stream
// restates the session's tracked turn state. A turn boundary is published on
// the instance stream while the agent runs, so a client that attaches after the
// turn ended never sees the closing frame; without the join frame the state it
// last read stays "running" forever and the composer is stuck on 停止.
func TestStreamSessionOpensWithTurnState(t *testing.T) {
	env := newTestEnv(t)
	session := createNativeSession(t, env, "native-1")

	// A turn under way is reported as running at the join point.
	env.fake.emit(t, env.instanceID, turnBoundaryEvent(agents.TurnStarted, ""))
	waitForTurnState(t, env, TurnRunning)
	assertJoinTurnPhase(t, env, session.ID, agents.TurnStarted)

	// Once it has ended, the join point says so even though the closing boundary
	// was published long before the client attached.
	env.fake.emit(t, env.instanceID, turnBoundaryEvent(agents.TurnEnded, agents.TurnCompleted))
	waitForTurnState(t, env, TurnIdle)
	assertJoinTurnPhase(t, env, session.ID, agents.TurnEnded)
}

// TestStreamSessionOpensWithInteractionSnapshot verifies a stream of a session
// whose backend declares tool confirmation starts with the pending set (the
// authoritative state at the join point), then carries live interaction events.
func TestStreamSessionOpensWithInteractionSnapshot(t *testing.T) {
	env := newTestEnv(t)
	session := createNativeSession(t, env, "native-1")
	env.fake.mu.Lock()
	env.fake.caps.ToolConfirmation = true
	env.fake.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := &recordingWriter{}
	done := make(chan error, 1)
	go func() {
		done <- env.svc.StreamSession(ctx, env.projectID, session.ID, 0, w, nil)
	}()

	waitForSubscriber(t, env)
	waitForBytes(t, w, `event: interactions`)
	snapshot := string(w.Bytes())
	if !strings.Contains(snapshot, `"permissions":[{"id":"per_1"`) || !strings.Contains(snapshot, `"forms":[{"id":"frm_1"`) {
		t.Fatalf("snapshot frame = %s, want the pending permission and form", snapshot)
	}
	if strings.Index(snapshot, "event: interactions") > strings.Index(snapshot, `"permissions"`) {
		t.Fatalf("snapshot payload precedes its frame: %s", snapshot)
	}

	env.fake.emit(t, env.instanceID, agents.NormalizedEvent{
		InstanceID:  env.instanceID,
		Backend:     agents.BackendPi,
		SessionID:   "native-1",
		Sequence:    1,
		Timestamp:   time.Now().UTC(),
		Type:        agents.EventInteraction,
		Interaction: &agents.InteractionPayload{ID: "per_1", Kind: agents.InteractionPermission, Phase: agents.InteractionResolved},
	})
	waitForBytes(t, w, `event: interaction`)

	cancel()
	<-done
}

// TestStreamSessionWithoutToolConfirmationSendsNoSnapshot verifies a backend that
// cannot confirm tools writes no interaction snapshot.
func TestStreamSessionWithoutToolConfirmationSendsNoSnapshot(t *testing.T) {
	env := newTestEnv(t)
	session := createNativeSession(t, env, "native-1")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := &recordingWriter{}
	done := make(chan error, 1)
	go func() {
		done <- env.svc.StreamSession(ctx, env.projectID, session.ID, 0, w, nil)
	}()

	waitForSubscriber(t, env)
	env.fake.emit(t, env.instanceID, streamMessageEvent(1, "native-1", "hello"))
	waitForFrames(t, w, 1)
	cancel()
	<-done
	if strings.Contains(string(w.Bytes()), "event: interactions") {
		t.Fatalf("stream wrote an interaction snapshot without tool confirmation: %s", w.Bytes())
	}
}

// waitForBytes polls the recorded stream until it contains want.
func waitForBytes(t *testing.T, w *recordingWriter, want string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !strings.Contains(string(w.Bytes()), want) {
		if time.Now().After(deadline) {
			t.Fatalf("stream never contained %q; got %s", want, w.Bytes())
		}
		time.Sleep(5 * time.Millisecond)
	}
}
