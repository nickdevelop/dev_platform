package deepseek

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func TestExchangeLaunchToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/connect" && r.URL.Query().Get("token") == "secret123" {
			if r.URL.Query().Get("interactions") != "answer" {
				http.Error(w, "missing or invalid interactions param", http.StatusBadRequest)
				return
			}
			http.SetCookie(w, &http.Cookie{
				Name:  "dsh-auth-test",
				Value: "cookie-value-abc",
			})
			w.WriteHeader(http.StatusFound)
			return
		}
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	endpoint, cookie, err := ExchangeLaunchToken(ctx, server.URL+"/?token=secret123")
	if err != nil {
		t.Fatalf("ExchangeLaunchToken failed: %v", err)
	}

	if endpoint != server.URL {
		t.Errorf("endpoint = %s, want %s", endpoint, server.URL)
	}
	if !strings.Contains(cookie, "dsh-auth-test=cookie-value-abc") {
		t.Errorf("cookie = %s, want containing dsh-auth-test=cookie-value-abc", cookie)
	}
}

func TestClientCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/test/method" {
			var req rpcRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if req.Method != "test/method" {
				http.Error(w, "bad method", http.StatusBadRequest)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
				"type": "server-response",
				"rpcId": "` + req.RPCID + `",
				"result": {
					"ok": true,
					"value": {"echo": "hello"}
				}
			}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client := NewClient(server.URL, "dsh-auth=test")
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var res struct {
		Echo string `json:"echo"`
	}
	err := client.Call(ctx, "test/method", map[string]string{"msg": "hi"}, &res)
	if err != nil {
		t.Fatalf("client.Call failed: %v", err)
	}
	if res.Echo != "hello" {
		t.Errorf("echo = %s, want hello", res.Echo)
	}
}

func TestClientMuxAndSubscribe(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/remote.mux" {
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				t.Fatalf("upgrade: %v", err)
				return
			}
			defer conn.Close()

			for {
				var msg wsClientMessage
				if err := conn.ReadJSON(&msg); err != nil {
					return
				}
				if msg.Type == "open" && msg.Endpoint == "$events" {
					_ = conn.WriteJSON(wsServerMessage{
						Type:     "item",
						StreamID: msg.StreamID,
						Value:    json.RawMessage(`{"type": "ready", "clientId": "client-123"}`),
					})
				} else if msg.Type == "open" && msg.Endpoint == "session/follow" {
					_ = conn.WriteJSON(wsServerMessage{
						Type:     "item",
						StreamID: msg.StreamID,
						Value:    json.RawMessage(`{"type": "event", "data": "turn-data"}`),
					})
				}
			}
		}
	}))
	defer server.Close()

	client := NewClient(server.URL, "auth=cookie")
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := client.ConnectMux(ctx); err != nil {
		t.Fatalf("ConnectMux failed: %v", err)
	}

	if client.ClientID() != "client-123" {
		t.Errorf("clientID = %s, want client-123", client.ClientID())
	}

	streamID, ch, cancelStream, err := client.Subscribe(ctx, "session/follow", map[string]any{"address": "ses-1"})
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}
	defer cancelStream()

	select {
	case item := <-ch:
		if !strings.Contains(string(item), "turn-data") {
			t.Errorf("stream item = %s, want containing turn-data", string(item))
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for stream item")
	}

	_ = streamID
}

func TestCancelAfterStreamErrorDoesNotPanic(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/remote.mux" {
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				return
			}
			defer conn.Close()

			for {
				var msg wsClientMessage
				if err := conn.ReadJSON(&msg); err != nil {
					return
				}
				if msg.Type == "open" && msg.Endpoint == "$events" {
					_ = conn.WriteJSON(wsServerMessage{
						Type:     "item",
						StreamID: msg.StreamID,
						Value:    json.RawMessage(`{"type": "ready", "clientId": "client-test"}`),
					})
				} else if msg.Type == "open" && msg.Endpoint == "test/error-stream" {
					// Simulate server sending an error frame on the stream
					_ = conn.WriteJSON(wsServerMessage{
						Type:     "error",
						StreamID: msg.StreamID,
						Error: &RPCError{
							Code:    "test/failed",
							Message: "stream failed",
						},
					})
				}
			}
		}
	}))
	defer server.Close()

	client := NewClient(server.URL, "auth=cookie")
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := client.ConnectMux(ctx); err != nil {
		t.Fatalf("ConnectMux failed: %v", err)
	}

	_, ch, cancelStream, err := client.Subscribe(ctx, "test/error-stream", map[string]any{})
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}

	// Wait for ch to close due to error frame
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("expected channel to close on error")
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for stream channel closure")
	}

	// Calling cancelStream() after stream is closed must not panic!
	cancelStream()
}
