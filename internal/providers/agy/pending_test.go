package agy

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"devplatform/internal/core/agents"
)

// writeFakeAgy writes a stand-in agy executable whose body runs after the
// shell starts, so tests control how the ACP server responds.
func writeFakeAgy(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "agy")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0755); err != nil {
		t.Fatalf("write fake agy: %v", err)
	}
	return path
}

func newPendingTestAdapter(t *testing.T, executable string, startTimeout time.Duration) *Adapter {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	return New(Options{
		Executable:   executable,
		StartTimeout: startTimeout,
		Now:          func() time.Time { return time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC) },
	})
}

func TestCreateSessionDoesNotWaitForAgy(t *testing.T) {
	adapter := newPendingTestAdapter(t, "/nonexistent/agy", time.Minute)
	inst, err := adapter.StartInstance(context.Background(), agents.StartInstanceRequest{Directory: t.TempDir()})
	if err != nil {
		t.Fatalf("StartInstance: %v", err)
	}

	ref, err := adapter.CreateSession(context.Background(), inst.ID, agents.CreateSessionOptions{Title: "t"})
	if err != nil {
		t.Fatalf("CreateSession must not start agy, got %v", err)
	}
	if !isPendingSession(ref.SessionID) {
		t.Fatalf("SessionID = %q, want a pending id", ref.SessionID)
	}
}

func TestSendMessageBindsPendingSession(t *testing.T) {
	const native = "conv-bound-1"
	executable := writeFakeAgy(t, `python3 -c '
import sys, json
for line in sys.stdin:
    req = json.loads(line)
    method = req.get("method")
    rid = req.get("id")
    if method == "initialize":
        print(json.dumps({"jsonrpc":"2.0","id":rid,"result":{"protocolVersion":1}}), flush=True)
    elif method == "session/new":
        print(json.dumps({"jsonrpc":"2.0","id":rid,"result":{"sessionId":"`+native+`"}}), flush=True)
    elif method == "session/prompt":
        # keep turn running
        pass
'`)
	adapter := newPendingTestAdapter(t, executable, 10*time.Second)
	ctx := context.Background()
	inst, err := adapter.StartInstance(ctx, agents.StartInstanceRequest{Directory: t.TempDir()})
	if err != nil {
		t.Fatalf("StartInstance: %v", err)
	}
	ref, err := adapter.CreateSession(ctx, inst.ID, agents.CreateSessionOptions{Title: "t"})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	pendingID := ref.SessionID

	var bound []string
	err = adapter.SendMessage(ctx, *ref, agents.SendMessageRequest{
		Text: "hello",
		OnNativeSessionBound: func(id string) error {
			bound = append(bound, id)
			return nil
		},
	})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if len(bound) != 1 || bound[0] != native {
		t.Fatalf("bound = %v, want [%s]", bound, native)
	}
	child := liveInstance(t, adapter, inst.ID).getChild(native)
	if child == nil {
		t.Fatal("child must be served under the native id")
	}
	if liveInstance(t, adapter, inst.ID).resolve(pendingID) != native {
		t.Fatalf("resolve(%q) = %q, want %q", pendingID, liveInstance(t, adapter, inst.ID).resolve(pendingID), native)
	}

	err = adapter.SendMessage(ctx, *ref, agents.SendMessageRequest{
		Text: "again",
		OnNativeSessionBound: func(id string) error {
			t.Fatalf("bind must run once, got second call with %q", id)
			return nil
		},
	})
	// The fake agy never reports a result, so the first turn is still running:
	// the same child refusing a second turn proves the send reused it.
	if !errors.Is(err, errTurnInProgress) {
		t.Fatalf("second SendMessage error = %v, want errTurnInProgress from the bound child", err)
	}
	if liveInstance(t, adapter, inst.ID).getChild(native) != child {
		t.Fatal("second turn must reuse the bound child")
	}
	if err := adapter.StopInstance(ctx, inst.ID); err != nil {
		t.Fatalf("StopInstance: %v", err)
	}
}

func TestSendMessagePendingFailsWhenInitTimesOut(t *testing.T) {
	executable := writeFakeAgy(t, "sleep 30")
	adapter := newPendingTestAdapter(t, executable, 200*time.Millisecond)
	ctx := context.Background()
	inst, err := adapter.StartInstance(ctx, agents.StartInstanceRequest{Directory: t.TempDir()})
	if err != nil {
		t.Fatalf("StartInstance: %v", err)
	}
	ref, err := adapter.CreateSession(ctx, inst.ID, agents.CreateSessionOptions{})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	err = adapter.SendMessage(ctx, *ref, agents.SendMessageRequest{
		Text: "hello",
		OnNativeSessionBound: func(string) error {
			t.Fatal("bind must not run when agy never reported a conversation")
			return nil
		},
	})
	if err == nil || (!strings.Contains(err.Error(), "initialize acp") && !strings.Contains(err.Error(), "context deadline exceeded")) {
		t.Fatalf("SendMessage error = %v, want an init timeout", err)
	}
	if liveInstance(t, adapter, inst.ID).resolve(ref.SessionID) != ref.SessionID {
		t.Fatal("a failed first turn must leave the session pending")
	}
}

func TestSendMessagePendingAbortsWhenBindFails(t *testing.T) {
	executable := writeFakeAgy(t, `python3 -c '
import sys, json
for line in sys.stdin:
    req = json.loads(line)
    method = req.get("method")
    rid = req.get("id")
    if method == "initialize":
        print(json.dumps({"jsonrpc":"2.0","id":rid,"result":{"protocolVersion":1}}), flush=True)
    elif method == "session/new":
        print(json.dumps({"jsonrpc":"2.0","id":rid,"result":{"sessionId":"conv-dup"}}), flush=True)
'`)
	adapter := newPendingTestAdapter(t, executable, 10*time.Second)
	ctx := context.Background()
	inst, err := adapter.StartInstance(ctx, agents.StartInstanceRequest{Directory: t.TempDir()})
	if err != nil {
		t.Fatalf("StartInstance: %v", err)
	}
	ref, err := adapter.CreateSession(ctx, inst.ID, agents.CreateSessionOptions{})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	bindErr := errors.New("native id already indexed")
	err = adapter.SendMessage(ctx, *ref, agents.SendMessageRequest{
		Text:                 "hello",
		OnNativeSessionBound: func(string) error { return bindErr },
	})
	if !errors.Is(err, bindErr) {
		t.Fatalf("SendMessage error = %v, want the bind error", err)
	}
	if liveInstance(t, adapter, inst.ID).getChild("conv-dup") != nil || liveInstance(t, adapter, inst.ID).resolve(ref.SessionID) != ref.SessionID {
		t.Fatal("a failed bind must not expose the child or an alias")
	}
	if err := adapter.StopInstance(ctx, inst.ID); err != nil {
		t.Fatalf("StopInstance: %v", err)
	}
}

func liveInstance(t *testing.T, a *Adapter, id string) *instance {
	t.Helper()
	inst, err := a.getInstance(id)
	if err != nil {
		t.Fatalf("getInstance: %v", err)
	}
	return inst
}
