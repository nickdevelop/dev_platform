package acp

import (
	"context"
	"encoding/json"
	"io"
	"testing"
	"time"
)

func TestClientCallSuccess(t *testing.T) {
	serverInR, serverInW := io.Pipe()
	serverOutR, serverOutW := io.Pipe()

	client := NewClient(serverInW, serverInW, nil, nil)
	go func() {
		_ = client.ReadLoop(serverOutR)
	}()
	defer client.Close()

	// Simulate server
	go func() {
		decoder := json.NewDecoder(serverInR)
		var req Request
		if err := decoder.Decode(&req); err != nil {
			return
		}
		if req.Method != MethodInitialize {
			t.Errorf("expected method %s, got %s", MethodInitialize, req.Method)
		}

		res := Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  json.RawMessage(`{"protocolVersion": 1}`),
		}
		data, _ := json.Marshal(res)
		data = append(data, '\n')
		_, _ = serverOutW.Write(data)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var result InitializeResult
	err := client.Call(ctx, MethodInitialize, InitializeParams{ProtocolVersion: 1}, &result)
	if err != nil {
		t.Fatalf("client.Call failed: %v", err)
	}

	if result.ProtocolVersion != 1 {
		t.Fatalf("expected protocolVersion 1, got %d", result.ProtocolVersion)
	}
}

func TestClientNotification(t *testing.T) {
	serverInR, serverInW := io.Pipe()
	serverOutR, serverOutW := io.Pipe()

	notifCh := make(chan string, 1)
	client := NewClient(serverInW, serverInW, nil, func(method string, params json.RawMessage) {
		if method == MethodSessionUpdate {
			notifCh <- string(params)
		}
	})
	go func() {
		_ = client.ReadLoop(serverOutR)
	}()
	defer client.Close()
	defer serverInR.Close()

	// Simulate server sending notification
	go func() {
		notif := Notification{
			JSONRPC: "2.0",
			Method:  MethodSessionUpdate,
			Params:  json.RawMessage(`{"sessionUpdate":"test"}`),
		}
		b, _ := json.Marshal(notif)
		b = append(b, '\n')
		_, _ = serverOutW.Write(b)
	}()

	select {
	case payload := <-notifCh:
		if payload != `{"sessionUpdate":"test"}` {
			t.Fatalf("unexpected notification payload: %s", payload)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for notification")
	}
}

func TestClientServerInitiatedRequest(t *testing.T) {
	serverInR, serverInW := io.Pipe()
	serverOutR, serverOutW := io.Pipe()

	client := NewClient(serverInW, serverInW, func(ctx context.Context, id any, method string, params json.RawMessage) (any, error) {
		if method == MethodRequestPermission {
			return RequestPermissionResult{
				Outcome: PermissionOutcome{
					Outcome:  "selected",
					OptionID: "allow_once",
				},
			}, nil
		}
		return nil, nil
	}, nil)

	go func() {
		_ = client.ReadLoop(serverOutR)
	}()
	defer client.Close()

	// Server sends request and reads client's response
	resultCh := make(chan string, 1)
	go func() {
		req := Request{
			JSONRPC: "2.0",
			ID:      99,
			Method:  MethodRequestPermission,
			Params:  json.RawMessage(`{}`),
		}
		b, _ := json.Marshal(req)
		b = append(b, '\n')
		_, _ = serverOutW.Write(b)

		// Read response
		decoder := json.NewDecoder(serverInR)
		var resp Response
		if err := decoder.Decode(&resp); err != nil {
			resultCh <- err.Error()
			return
		}
		resultCh <- string(resp.Result)
	}()

	select {
	case res := <-resultCh:
		var outcome RequestPermissionResult
		if err := json.Unmarshal([]byte(res), &outcome); err != nil {
			t.Fatalf("failed to unmarshal outcome: %v", err)
		}
		if outcome.Outcome.Outcome != "selected" || outcome.Outcome.OptionID != "allow_once" {
			t.Fatalf("unexpected outcome: %+v", outcome)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for server request response")
	}
}
