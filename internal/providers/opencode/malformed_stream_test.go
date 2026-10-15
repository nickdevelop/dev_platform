package opencode

import (
	"context"
	"io"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"devplatform/internal/core/agents"
)

// TestAdapterMalformedEventIsReportedAndStreamRecovers proves a malformed SSE
// payload is surfaced as a malformed_event diagnostic and that the stream
// reconnects afterwards instead of dying: the same instance keeps delivering
// normalized events from the reconnected stream.
func TestAdapterMalformedEventIsReportedAndStreamRecovers(t *testing.T) {
	directory := t.TempDir()
	var connections atomic.Int32

	adapter, _, _ := newAdapterWithServer(t, queueHandler(map[string]http.HandlerFunc{
		"POST /api/session": func(w http.ResponseWriter, r *http.Request) {
			io.WriteString(w, `{"data":{"id":"ses_1","projectID":"prj","time":{"created":1,"updated":1}}}`)
		},
		"GET /api/event": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("content-type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			flusher := w.(http.Flusher)
			flusher.Flush()
			if connections.Add(1) == 1 {
				// First connection: one malformed payload, then hold the
				// connection open so only the decode failure can end the read.
				io.WriteString(w, "data: {\"broken\"\n\n")
				flusher.Flush()
				<-r.Context().Done()
				return
			}
			// Reconnected stream: a healthy event for the instance's directory.
			io.WriteString(w, sseEvent("evt_ok", "session.created", `{"sessionID":"ses_1","title":"Recovered"}`, directory))
			flusher.Flush()
			<-r.Context().Done()
		},
	}))
	instance := startTestInstance(t, adapter)
	if _, err := adapter.CreateSession(context.Background(), instance.ID, agents.CreateSessionOptions{}); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, err := adapter.Subscribe(ctx, instance.ID)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer adapter.StopInstance(context.Background(), instance.ID)

	sawMalformed := false
	sawRecovered := false
	timeout := time.After(5 * time.Second)
	for !(sawMalformed && sawRecovered) {
		select {
		case ev, ok := <-ch:
			if !ok {
				t.Fatalf("channel closed after malformed=%v recovered=%v", sawMalformed, sawRecovered)
			}
			switch {
			case ev.Type == agents.EventError:
				if ev.Error == nil || ev.Error.Code != "malformed_event" {
					t.Fatalf("error event = %+v, want code malformed_event", ev.Error)
				}
				if ev.Error.Message == "" {
					t.Fatal("malformed_event diagnostic carries no message")
				}
				sawMalformed = true
			case ev.Type == agents.EventLifecycle && ev.Lifecycle != nil && ev.Lifecycle.State == "created":
				sawRecovered = true
			default:
				// Ignore unrelated lifecycle noise from the reconnect.
			}
		case <-timeout:
			t.Fatalf("timed out: malformed=%v recovered=%v", sawMalformed, sawRecovered)
		}
	}

	if got := connections.Load(); got < 2 {
		t.Fatalf("stream connections = %d, want at least 2 (drop then reconnect)", got)
	}
}
