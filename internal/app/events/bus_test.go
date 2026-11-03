package events

import (
	"bytes"
	"context"
	"encoding/json"
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
		done <- mgr.StreamBus(ctx, busID, &buf, nil)
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
		done <- mgr.StreamBus(ctx, busID, &buf, nil)
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
