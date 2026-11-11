package events

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

type dummyHandler struct {
	subscribed   []SubscribeRequest
	unsubscribed []string
}

func (d *dummyHandler) OnSubscribe(ctx context.Context, busID string, req SubscribeRequest) (any, error) {
	d.subscribed = append(d.subscribed, req)
	return map[string]string{"mock": "snapshot-" + req.TargetID}, nil
}

func (d *dummyHandler) OnUnsubscribe(ctx context.Context, busID string, channel string, targetID string) error {
	d.unsubscribed = append(d.unsubscribed, channel+"/"+targetID)
	return nil
}

func TestManagerPubSubAndBroadcast(t *testing.T) {
	mgr := NewManager()
	handler := &dummyHandler{}
	mgr.RegisterHandler(ChannelTasks, handler)

	busID1 := "test-bus-1"
	busID2 := "test-bus-2"

	// 1. Subscribe bus 1 to tasks/task-100
	resp, err := mgr.Subscribe(context.Background(), busID1, SubscribeRequest{
		Channel:  ChannelTasks,
		TargetID: "task-100",
	})
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}
	if resp.Channel != ChannelTasks || resp.TargetID != "task-100" {
		t.Fatalf("unexpected SubscribeResponse: %+v", resp)
	}
	snapMap, ok := resp.Snapshot.(map[string]string)
	if !ok || snapMap["mock"] != "snapshot-task-100" {
		t.Fatalf("expected snapshot from dummyHandler, got: %+v", resp.Snapshot)
	}

	// 2. Subscribe bus 2 to chat/chat-200
	_, err = mgr.Subscribe(context.Background(), busID2, SubscribeRequest{
		Channel:  ChannelChat,
		TargetID: "chat-200",
	})
	if err != nil {
		t.Fatalf("Subscribe bus 2 failed: %v", err)
	}

	// 3. Verify ListSubscriptions
	subs1, err := mgr.ListSubscriptions(busID1)
	if err != nil || len(subs1[ChannelTasks]) != 1 || subs1[ChannelTasks][0] != "task-100" {
		t.Fatalf("ListSubscriptions bus1 got %+v", subs1)
	}

	// 4. Broadcast to tasks/task-100
	taskPayload := map[string]string{"status": "running"}
	err = mgr.BroadcastPayload(ChannelTasks, "task-100", "status_change", taskPayload)
	if err != nil {
		t.Fatalf("BroadcastPayload failed: %v", err)
	}

	bus1 := mgr.GetOrCreate(busID1)
	bus2 := mgr.GetOrCreate(busID2)

	select {
	case env := <-bus1.out:
		if env.Channel != ChannelTasks || env.TargetID != "task-100" || env.EventType != "status_change" {
			t.Fatalf("unexpected envelope on bus1: %+v", env)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for event on bus1")
	}

	// bus2 should NOT receive this event
	select {
	case env := <-bus2.out:
		t.Fatalf("bus2 received unexpected event: %+v", env)
	default:
		// expected
	}

	// 5. Unsubscribe bus1
	err = mgr.Unsubscribe(context.Background(), busID1, ChannelTasks, "task-100")
	if err != nil {
		t.Fatalf("Unsubscribe failed: %v", err)
	}
	if len(handler.unsubscribed) != 1 || handler.unsubscribed[0] != "tasks/task-100" {
		t.Fatalf("handler OnUnsubscribe not invoked properly: %+v", handler.unsubscribed)
	}

	subsAfter, _ := mgr.ListSubscriptions(busID1)
	if len(subsAfter[ChannelTasks]) != 0 {
		t.Fatalf("expected no subscriptions for tasks on bus1, got %+v", subsAfter)
	}
}

func TestManagerStreamBus(t *testing.T) {
	mgr := NewManager()
	busID := "test-bus-stream"

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var buf bytes.Buffer
	done := make(chan error, 1)

	go func() {
		done <- mgr.StreamBus(ctx, busID, &buf, nil, nil)
	}()

	// Wait for ready frame to be flushed
	time.Sleep(50 * time.Millisecond)

	// Send an event
	mgr.PushToBus(busID, EventEnvelope{
		Channel:   ChannelSessions,
		TargetID:  "s1",
		EventType: "turn",
		Payload:   json.RawMessage(`{"phase":"started"}`),
	})

	time.Sleep(50 * time.Millisecond)
	cancel()

	<-done

	output := buf.String()
	if !strings.Contains(output, "event: ready\n") {
		t.Fatalf("missing ready event in stream: %q", output)
	}
	if !strings.Contains(output, "event: turn\n") {
		t.Fatalf("missing turn event in stream: %q", output)
	}
	if !strings.Contains(output, `"targetId":"s1"`) {
		t.Fatalf("missing targetId in stream: %q", output)
	}
}

func TestManagerGC(t *testing.T) {
	mgr := NewManager()
	mgr.gcTTL = 50 * time.Millisecond
	busID := "test-bus-gc"

	ctx, cancel := context.WithCancel(context.Background())
	var buf bytes.Buffer

	done := make(chan error, 1)
	go func() {
		done <- mgr.StreamBus(ctx, busID, &buf, nil, nil)
	}()

	time.Sleep(20 * time.Millisecond)
	// Disconnect client
	cancel()
	<-done

	// Verify bus exists immediately after disconnect
	mgr.mu.RLock()
	_, exists := mgr.buses[busID]
	mgr.mu.RUnlock()
	if !exists {
		t.Fatal("bus should exist immediately after disconnect during grace period")
	}

	// Wait past TTL
	time.Sleep(100 * time.Millisecond)

	mgr.mu.RLock()
	_, existsAfter := mgr.buses[busID]
	mgr.mu.RUnlock()
	if existsAfter {
		t.Fatalf("expected bus %s to be garbage collected", busID)
	}
}

func TestClientBusBufferOverflowAndCriticalEvents(t *testing.T) {
	mgr := NewManager()
	busID := "test-bus-overflow"
	bus := mgr.GetOrCreate(busID)

	// Subscribe to a channel
	_, err := mgr.Subscribe(context.Background(), busID, SubscribeRequest{
		Channel:  ChannelSessions,
		TargetID: "s1",
	})
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}

	// 1. Fill the buffer to capacity
	bufCap := cap(bus.out)
	for i := 0; i < bufCap; i++ {
		bus.out <- EventEnvelope{
			Channel:   ChannelSessions,
			TargetID:  "s1",
			EventType: "delta",
		}
	}

	if bus.DroppedCount() != 0 {
		t.Fatalf("expected 0 dropped before overflow, got %d", bus.DroppedCount())
	}

	// 2. Broadcast a non-critical event when buffer is full; should be dropped immediately
	mgr.Broadcast(EventEnvelope{
		Channel:   ChannelSessions,
		TargetID:  "s1",
		EventType: "delta",
	})

	if bus.DroppedCount() != 1 {
		t.Fatalf("expected 1 dropped event, got %d", bus.DroppedCount())
	}

	// 3. Test critical event: when a consumer frees space within 50ms, the critical event enters successfully
	doneCh := make(chan struct{})
	go func() {
		// Wait 15ms then drain one item
		time.Sleep(15 * time.Millisecond)
		<-bus.out
		close(doneCh)
	}()

	mgr.Broadcast(EventEnvelope{
		Channel:   ChannelSessions,
		TargetID:  "s1",
		EventType: "turn_ended",
	})

	<-doneCh

	// The turn_ended event should have entered without being dropped
	if bus.DroppedCount() != 1 {
		t.Fatalf("critical event should have enqueued, but dropped count increased to %d", bus.DroppedCount())
	}
}

func TestClientBusConnectionDisplacement(t *testing.T) {
	mgr := NewManager()
	busID := "test-bus-displace"

	// 1. Start Conn 1
	ctx1, cancel1 := context.WithCancel(context.Background())
	defer cancel1()

	var buf1 bytes.Buffer
	done1 := make(chan error, 1)
	go func() {
		done1 <- mgr.StreamBus(ctx1, busID, &buf1, nil, nil)
	}()

	// Wait for conn 1 ready
	time.Sleep(30 * time.Millisecond)

	bus := mgr.GetOrCreate(busID)
	bus.mu.Lock()
	if !bus.active || bus.connID != 1 {
		bus.mu.Unlock()
		t.Fatalf("expected bus active with connID 1, got active=%v, connID=%d", bus.active, bus.connID)
	}
	bus.mu.Unlock()

	// 2. Start Conn 2 on the same busID (e.g. client reconnect or page refresh)
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()

	var buf2 bytes.Buffer
	done2 := make(chan error, 1)
	go func() {
		done2 <- mgr.StreamBus(ctx2, busID, &buf2, nil, nil)
	}()

	// Conn 1 should be displaced and terminated promptly without waiting for ctx1 cancel
	select {
	case err := <-done1:
		if err != nil {
			t.Fatalf("conn 1 terminated with error: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("conn 1 was not displaced/terminated promptly after conn 2 connected")
	}

	// Verify bus is still active with connID 2 despite conn 1 exiting
	bus.mu.Lock()
	if !bus.active || bus.connID != 2 {
		bus.mu.Unlock()
		t.Fatalf("expected bus active with connID 2, got active=%v, connID=%d", bus.active, bus.connID)
	}
	bus.mu.Unlock()

	// 3. Emit 20 sequential events
	for i := 1; i <= 20; i++ {
		mgr.PushToBus(busID, EventEnvelope{
			ID:        uint64(i),
			Channel:   ChannelSessions,
			TargetID:  "s1",
			EventType: "delta",
			Payload:   json.RawMessage(json.RawMessage(`{"seq":1}`)),
		})
	}

	time.Sleep(50 * time.Millisecond)
	cancel2()
	<-done2

	// Check buf1 (conn 1): should only contain the initial ready event
	out1 := buf1.String()
	if strings.Contains(out1, "event: delta\n") {
		t.Fatalf("conn 1 received delta events after being displaced: %q", out1)
	}

	// Check buf2 (conn 2): must contain all 20 sequential events with strictly no missing ids
	out2 := buf2.String()
	for i := 1; i <= 20; i++ {
		expectedID := "id: " + string(rune('0'+i/10))
		if i < 10 {
			expectedID = "id: " + string(rune('0'+i)) + "\nevent: delta\n"
		} else {
			expectedID = "id: " + string([]byte{byte('0' + i/10), byte('0' + i%10)}) + "\nevent: delta\n"
		}
		if !strings.Contains(out2, expectedID) {
			t.Fatalf("conn 2 missing sequence event id %d: %q", i, out2)
		}
	}
}

type blockingWriter struct {
	readyStarted chan struct{}
	releaseReady chan struct{}
	buf          bytes.Buffer
}

func (w *blockingWriter) Write(p []byte) (int, error) {
	if strings.Contains(string(p), "event: ready") {
		select {
		case <-w.readyStarted:
		default:
			close(w.readyStarted)
		}
		// Block here until released
		<-w.releaseReady
	}
	return w.buf.Write(p)
}

func TestClientBusDisplacementMigratesBufferedEvents(t *testing.T) {
	mgr := NewManager()
	busID := "test-bus-migrate"

	// Conn 1 uses blockingWriter: it will block during ready write,
	// so it cannot read from its out channel.
	w1 := &blockingWriter{
		readyStarted: make(chan struct{}),
		releaseReady: make(chan struct{}),
	}
	defer close(w1.releaseReady)

	ctx1, cancel1 := context.WithCancel(context.Background())
	defer cancel1()

	done1 := make(chan error, 1)
	go func() {
		done1 <- mgr.StreamBus(ctx1, busID, w1, nil, nil)
	}()

	// Wait until conn 1 reaches ready write (onConnect has executed)
	<-w1.readyStarted

	// Enqueue 3 events into the bus while conn 1 is blocked in ready write
	for i := 1; i <= 3; i++ {
		mgr.PushToBus(busID, EventEnvelope{
			ID:        uint64(i),
			Channel:   ChannelSessions,
			TargetID:  "s1",
			EventType: "delta",
		})
	}

	// Now Conn 2 connects. It should displace Conn 1 and migrate events 1..3 into its new channel.
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	var buf2 bytes.Buffer
	done2 := make(chan error, 1)
	go func() {
		done2 <- mgr.StreamBus(ctx2, busID, &buf2, nil, nil)
	}()

	time.Sleep(30 * time.Millisecond)

	// Send 2 more events after Conn 2 connected
	for i := 4; i <= 5; i++ {
		mgr.PushToBus(busID, EventEnvelope{
			ID:        uint64(i),
			Channel:   ChannelSessions,
			TargetID:  "s1",
			EventType: "delta",
		})
	}

	time.Sleep(50 * time.Millisecond)
	cancel2()
	<-done2

	out2 := buf2.String()
	for i := 1; i <= 5; i++ {
		expectedID := fmt.Sprintf("id: %d\nevent: delta\n", i)
		if !strings.Contains(out2, expectedID) {
			t.Fatalf("conn 2 missing migrated/subsequent sequence event id %d: %q", i, out2)
		}
	}
}
