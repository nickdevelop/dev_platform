package chat

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

func TestChatEventBusSubscribeAndBroadcast(t *testing.T) {
	completer := &fakeCompleter{}
	service, repo, _ := testService(t, completer)

	// Create two conversations
	chat1, err := service.Create(context.Background(), CreateInput{
		ModelID: 1,
		Text:    "Hello chat 1",
	})
	if err != nil {
		t.Fatalf("Create chat1: %v", err)
	}
	waitIdle(t, repo, chat1.ID)

	chat2, err := service.Create(context.Background(), CreateInput{
		ModelID: 1,
		Text:    "Hello chat 2",
	})
	if err != nil {
		t.Fatalf("Create chat2: %v", err)
	}
	waitIdle(t, repo, chat2.ID)

	busMgr := service.eventBus
	busUUID := "bus-test-uuid-1"

	// 1. Subscribe to chat1
	snapshot, err := busMgr.SubscribeChat(context.Background(), busUUID, chat1.ID)
	if err != nil {
		t.Fatalf("SubscribeChat: %v", err)
	}
	if snapshot.Conversation.ID != chat1.ID {
		t.Errorf("got snapshot conversation ID %d, want %d", snapshot.Conversation.ID, chat1.ID)
	}
	if len(snapshot.Messages) == 0 {
		t.Errorf("snapshot has no messages")
	}

	// Verify subscriptions list
	subs, err := busMgr.ListSubscriptions(busUUID)
	if err != nil {
		t.Fatalf("ListSubscriptions: %v", err)
	}
	if len(subs) != 1 || subs[0] != chat1.ID {
		t.Fatalf("got subs %v, want [%d]", subs, chat1.ID)
	}

	// 2. Broadcast events for chat1 and chat2
	bus := busMgr.GetOrCreate(busUUID)

	event1 := Event{
		ChatID:    chat1.ID,
		Timestamp: time.Now().UTC(),
		Type:      EventDelta,
		Delta:     &Delta{MessageID: 100, Text: "delta for chat1"},
	}
	event2 := Event{
		ChatID:    chat2.ID,
		Timestamp: time.Now().UTC(),
		Type:      EventDelta,
		Delta:     &Delta{MessageID: 200, Text: "delta for chat2"},
	}

	busMgr.Broadcast(event1)
	busMgr.Broadcast(event2)

	// We should only receive event1 because bus is only subscribed to chat1
	select {
	case env := <-bus.out:
		if env.eventType != EventDelta {
			t.Errorf("got event type %q, want %q", env.eventType, EventDelta)
		}
		if !strings.Contains(string(env.data), "delta for chat1") {
			t.Errorf("got unexpected data: %s", string(env.data))
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for event1")
	}

	// Ensure no extra event (chat2) was received
	select {
	case env := <-bus.out:
		t.Fatalf("unexpected extra event received: %s", string(env.data))
	case <-time.After(100 * time.Millisecond):
		// Expected
	}

	// 3. Unsubscribe from chat1
	if err := busMgr.UnsubscribeChat(busUUID, chat1.ID); err != nil {
		t.Fatalf("UnsubscribeChat: %v", err)
	}
	subsAfter, err := busMgr.ListSubscriptions(busUUID)
	if err != nil {
		t.Fatalf("ListSubscriptions after unsub: %v", err)
	}
	if len(subsAfter) != 0 {
		t.Errorf("got subs %v, want empty", subsAfter)
	}

	// Broadcast again, should not receive
	busMgr.Broadcast(event1)
	select {
	case env := <-bus.out:
		t.Fatalf("unexpected event after unsubscribe: %s", string(env.data))
	case <-time.After(100 * time.Millisecond):
		// Expected
	}
}

func TestChatEventBusStreamAndGC(t *testing.T) {
	completer := &fakeCompleter{}
	service, _, _ := testService(t, completer)

	busMgr := service.eventBus
	busMgr.gcTTL = 100 * time.Millisecond // Short TTL for test
	busUUID := "bus-test-uuid-gc"

	bus := busMgr.GetOrCreate(busUUID)
	if bus == nil {
		t.Fatal("expected bus")
	}

	// Stream with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	var buf bytes.Buffer
	done := make(chan error, 1)
	go func() {
		done <- busMgr.StreamBus(ctx, busUUID, &buf, nil)
	}()

	// Wait briefly for connection setup
	time.Sleep(50 * time.Millisecond)

	// Check initial ready event in stream
	if !strings.Contains(buf.String(), "event: ready\n") {
		t.Errorf("expected ready frame in stream, got %q", buf.String())
	}

	// Cancel stream (simulating client disconnect)
	cancel()
	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("StreamBus did not terminate")
	}

	// Wait for GC to kick in
	time.Sleep(200 * time.Millisecond)

	busMgr.mu.RLock()
	_, exists := busMgr.buses[busUUID]
	busMgr.mu.RUnlock()

	if exists {
		t.Errorf("expected bus %s to be garbage-collected after disconnect", busUUID)
	}
}

func TestChatEventBusConnectionDisplacement(t *testing.T) {
	completer := &fakeCompleter{}
	service, _, _ := testService(t, completer)

	busMgr := service.eventBus
	busUUID := "bus-test-uuid-displace"

	// 1. Conn 1 starts
	ctx1, cancel1 := context.WithCancel(context.Background())
	defer cancel1()

	var buf1 bytes.Buffer
	done1 := make(chan error, 1)
	go func() {
		done1 <- busMgr.StreamBus(ctx1, busUUID, &buf1, nil)
	}()

	time.Sleep(30 * time.Millisecond)

	bus := busMgr.GetOrCreate(busUUID)
	bus.mu.Lock()
	if !bus.active || bus.connID != 1 {
		bus.mu.Unlock()
		t.Fatalf("expected bus active with connID 1, got active=%v, connID=%d", bus.active, bus.connID)
	}
	bus.mu.Unlock()

	// 2. Conn 2 connects to the same busUUID
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()

	var buf2 bytes.Buffer
	done2 := make(chan error, 1)
	go func() {
		done2 <- busMgr.StreamBus(ctx2, busUUID, &buf2, nil)
	}()

	// Conn 1 should be displaced and terminated promptly
	select {
	case <-done1:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("conn 1 was not displaced promptly")
	}

	// Verify bus is still active with connID 2
	bus.mu.Lock()
	if !bus.active || bus.connID != 2 {
		bus.mu.Unlock()
		t.Fatalf("expected bus active with connID 2, got active=%v, connID=%d", bus.active, bus.connID)
	}
	bus.mu.Unlock()

	// Subscribe bus to chat ID 999
	bus.mu.Lock()
	bus.chats[999] = struct{}{}
	bus.mu.Unlock()

	// 3. Broadcast 5 events
	for i := 1; i <= 5; i++ {
		busMgr.Broadcast(Event{
			ChatID:    999,
			Timestamp: time.Now().UTC(),
			Type:      EventDelta,
			Delta:     &Delta{MessageID: uint(i), Text: "delta-text"},
		})
	}

	time.Sleep(50 * time.Millisecond)
	cancel2()
	<-done2

	// Conn 1 should not have received any delta events
	out1 := buf1.String()
	if strings.Contains(out1, "delta-text") {
		t.Fatalf("conn 1 received events after displacement: %q", out1)
	}

	// Conn 2 must have received all 5 delta events
	out2 := buf2.String()
	occurrences := strings.Count(out2, "delta-text")
	if occurrences != 5 {
		t.Fatalf("conn 2 expected 5 delta events, got %d in: %q", occurrences, out2)
	}
}
