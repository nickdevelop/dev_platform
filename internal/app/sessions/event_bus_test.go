package sessions

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"devplatform/internal/core/agents"
)

// TestEventBusMultiUUIDIsolation verifies that events for different sessions are isolated
// between different client bus UUIDs.
func TestEventBusMultiUUIDIsolation(t *testing.T) {
	env := newTestEnv(t)
	s1 := createNativeSession(t, env, "native-1")
	s2 := createNativeSession(t, env, "native-2")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	w1 := &recordingWriter{}
	w2 := &recordingWriter{}

	busID1 := "uuid-bus-1"
	busID2 := "uuid-bus-2"

	go func() {
		_ = env.svc.eventBus.StreamBus(ctx, busID1, w1, nil)
	}()
	go func() {
		_ = env.svc.eventBus.StreamBus(ctx, busID2, w2, nil)
	}()

	// Wait for both buses to send initial "ready" frame
	waitForFrames(t, w1, 1)
	waitForFrames(t, w2, 1)

	// Bus 1 subscribes to session 1
	snap1, err := env.svc.eventBus.SubscribeSession(ctx, busID1, env.projectID, s1.ID, 0)
	if err != nil {
		t.Fatalf("SubscribeSession s1 on bus 1: %v", err)
	}
	if snap1.SessionID != s1.ID {
		t.Fatalf("got snap1 session %q, want %q", snap1.SessionID, s1.ID)
	}

	// Bus 2 subscribes to session 2
	snap2, err := env.svc.eventBus.SubscribeSession(ctx, busID2, env.projectID, s2.ID, 0)
	if err != nil {
		t.Fatalf("SubscribeSession s2 on bus 2: %v", err)
	}
	if snap2.SessionID != s2.ID {
		t.Fatalf("got snap2 session %q, want %q", snap2.SessionID, s2.ID)
	}

	// Emit event for session 1
	env.fake.emit(t, env.instanceID, streamMessageEvent(1, "native-1", "hello session 1"))

	// Bus 1 should receive the event (ready frame + message = 2 frames)
	waitForFrames(t, w1, 2)
	frames1 := parseFrames(t, w1.Bytes())
	if len(frames1) != 2 {
		t.Fatalf("bus 1 got %d frames, want 2", len(frames1))
	}
	if frames1[1].event != "message" {
		t.Errorf("bus 1 frame 1 event = %q, want message", frames1[1].event)
	}

	// Bus 2 must NOT receive session 1's event (still only 1 frame)
	time.Sleep(50 * time.Millisecond)
	frames2 := parseFrames(t, w2.Bytes())
	if len(frames2) != 1 {
		t.Fatalf("bus 2 unexpectedly received session 1 event: %+v", frames2)
	}

	// Emit event for session 2
	env.fake.emit(t, env.instanceID, streamMessageEvent(2, "native-2", "hello session 2"))

	// Now Bus 2 should receive its event (ready frame + message = 2 frames)
	waitForFrames(t, w2, 2)
	frames2 = parseFrames(t, w2.Bytes())
	if len(frames2) != 2 {
		t.Fatalf("bus 2 got %d frames, want 2", len(frames2))
	}
	if frames2[1].event != "message" {
		t.Errorf("bus 2 frame 1 event = %q, want message", frames2[1].event)
	}
}

// TestEventBusUnsubscribe verifies that unsubscribed sessions stop receiving events on the bus.
func TestEventBusUnsubscribe(t *testing.T) {
	env := newTestEnv(t)
	s1 := createNativeSession(t, env, "native-unsub")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	w := &recordingWriter{}
	busID := "uuid-unsub-bus"

	go func() {
		_ = env.svc.eventBus.StreamBus(ctx, busID, w, nil)
	}()

	waitForFrames(t, w, 1) // ready frame

	_, err := env.svc.eventBus.SubscribeSession(ctx, busID, env.projectID, s1.ID, 0)
	if err != nil {
		t.Fatalf("SubscribeSession: %v", err)
	}

	// Verify subscription exists
	subs, err := env.svc.eventBus.ListSubscriptions(busID)
	if err != nil || len(subs) != 1 || subs[0] != s1.ID {
		t.Fatalf("ListSubscriptions got %v, want [%s]", subs, s1.ID)
	}

	// Unsubscribe
	if err := env.svc.eventBus.UnsubscribeSession(busID, s1.ID); err != nil {
		t.Fatalf("UnsubscribeSession: %v", err)
	}

	subs, err = env.svc.eventBus.ListSubscriptions(busID)
	if err != nil || len(subs) != 0 {
		t.Fatalf("ListSubscriptions after unsub got %v, want empty", subs)
	}

	// Emit event for s1
	env.fake.emit(t, env.instanceID, streamMessageEvent(1, "native-unsub", "should not appear"))
	time.Sleep(50 * time.Millisecond)

	frames := parseFrames(t, w.Bytes())
	if len(frames) != 1 {
		t.Fatalf("got %d frames after unsubscribe, want 1", len(frames))
	}
}

// TestEventBusHTTPHandlers verifies the REST endpoints for event bus.
func TestEventBusHTTPHandlers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	env := newTestEnv(t)
	session := createNativeSession(t, env, "native-http")

	r := gin.New()
	api := r.Group("/api")
	env.svc.RegisterRoutes(api)

	busID := "test-http-bus-uuid"

	// 1. POST /api/events/:uuid/subscriptions
	body, _ := json.Marshal(map[string]any{
		"projectId": env.projectID,
		"sessionId": session.ID,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/events/"+busID+"/subscriptions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/events/:uuid/subscriptions returned %d: %s", rec.Code, rec.Body.String())
	}

	var snapshot SubscriptionSnapshot
	if err := json.Unmarshal(rec.Body.Bytes(), &snapshot); err != nil {
		t.Fatalf("unmarshal snapshot: %v", err)
	}
	if snapshot.SessionID != session.ID {
		t.Errorf("snapshot SessionID = %q, want %q", snapshot.SessionID, session.ID)
	}
	if snapshot.Turn == nil || snapshot.Turn.Phase != agents.TurnEnded {
		t.Errorf("snapshot Turn = %+v, want Phase=ended", snapshot.Turn)
	}

	// 2. GET /api/events/:uuid/subscriptions
	reqList := httptest.NewRequest(http.MethodGet, "/api/events/"+busID+"/subscriptions", nil)
	recList := httptest.NewRecorder()
	r.ServeHTTP(recList, reqList)

	if recList.Code != http.StatusOK {
		t.Fatalf("GET /api/events/:uuid/subscriptions returned %d: %s", recList.Code, recList.Body.String())
	}
	var listResp struct {
		Subscriptions []string `json:"subscriptions"`
	}
	if err := json.Unmarshal(recList.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("unmarshal list: %v", err)
	}
	if len(listResp.Subscriptions) != 1 || listResp.Subscriptions[0] != session.ID {
		t.Errorf("list subscriptions = %v, want [%s]", listResp.Subscriptions, session.ID)
	}

	// 3. DELETE /api/events/:uuid/subscriptions/:sessionId
	reqDel := httptest.NewRequest(http.MethodDelete, "/api/events/"+busID+"/subscriptions/"+session.ID, nil)
	recDel := httptest.NewRecorder()
	r.ServeHTTP(recDel, reqDel)

	if recDel.Code != http.StatusOK {
		t.Fatalf("DELETE returned %d: %s", recDel.Code, recDel.Body.String())
	}

	// 4. Verify list is now empty
	recListAfter := httptest.NewRecorder()
	r.ServeHTTP(recListAfter, reqList)
	_ = json.Unmarshal(recListAfter.Body.Bytes(), &listResp)
	if len(listResp.Subscriptions) != 0 {
		t.Errorf("after delete, subscriptions = %v, want []", listResp.Subscriptions)
	}
}
