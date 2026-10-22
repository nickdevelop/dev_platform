package chat

import (
	"context"
	"testing"
	"time"
)

func TestSubscribeSnapshotIncludesPartialAnswer(t *testing.T) {
	started := make(chan struct{})
	completer := &fakeCompleter{
		stream: func(ctx context.Context, req CompletionRequest, onDelta func(CompletionDelta) error) error {
			_ = onDelta(CompletionDelta{Text: "part"})
			close(started)
			<-ctx.Done()
			return ctx.Err()
		},
	}
	service, repo, _ := testService(t, completer)
	chat, err := service.Create(context.Background(), CreateInput{ModelID: 1, Text: "hello"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	<-started

	event, ch, cancel, err := service.manager.subscribe(chat.ID, func(run *runView) (Event, error) {
		return service.snapshot(chat.ID, run)
	})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer cancel()
	if event.Type != EventSnapshot || event.Snapshot == nil {
		t.Fatalf("event = %+v", event)
	}
	if !event.Snapshot.Running {
		t.Fatal("snapshot running = false, want true")
	}
	if len(event.Snapshot.Messages) != 2 {
		t.Fatalf("messages = %+v", event.Snapshot.Messages)
	}
	if event.Snapshot.Messages[1].Text != "part" || event.Snapshot.Messages[1].Status != StatusPending {
		t.Fatalf("partial assistant = %+v", event.Snapshot.Messages[1])
	}

	if err := service.Stop(chat.ID); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	waitIdle(t, repo, chat.ID)
	deadline := time.After(2 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("no final event")
		case event := <-ch:
			if event.Type == EventMessageUpdate {
				if event.Message == nil || event.Message.Status != StatusCancelled {
					t.Fatalf("message update = %+v", event.Message)
				}
				return
			}
		}
	}
}

func TestWriteEventFrame(t *testing.T) {
	var buffer testWriter
	if err := writeEvent(&buffer, Event{ChatID: 7, Type: EventTurn, Turn: &Turn{Phase: TurnEnded, Outcome: OutcomeCompleted}}); err != nil {
		t.Fatalf("writeEvent: %v", err)
	}
	want := "event: turn\ndata: "
	if len(buffer.value) < len(want) || buffer.value[:len(want)] != want {
		t.Fatalf("frame = %q", buffer.value)
	}
}

type testWriter struct{ value string }

func (w *testWriter) Write(p []byte) (int, error) {
	w.value += string(p)
	return len(p), nil
}
