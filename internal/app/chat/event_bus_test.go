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
