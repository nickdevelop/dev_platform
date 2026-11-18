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

func TestSendMessageTurnStartedAndContextIsolation(t *testing.T) {
	const native = "conv-user-msg"
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
        # Simulate long running prompt
        pass
'`)
	adapter := newPendingTestAdapter(t, executable, 5*time.Second)
	ctx := context.Background()
	inst, err := adapter.StartInstance(ctx, agents.StartInstanceRequest{Directory: t.TempDir()})
	if err != nil {
		t.Fatalf("StartInstance: %v", err)
	}

	subEvents, err := adapter.Subscribe(ctx, inst.ID)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	ref, err := adapter.CreateSession(ctx, inst.ID, agents.CreateSessionOptions{Title: "t"})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	// 模拟外部 HTTP 请求的 context 被 cancel
	sendCtx, cancelSend := context.WithCancel(context.Background())
	err = adapter.SendMessage(sendCtx, *ref, agents.SendMessageRequest{
		Text: "Hello User Input",
	})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	// 立即取消发送 context（模拟 HTTP 202 返回后的情况）
	cancelSend()

	// 检查事件总线中收到了 TurnStarted，且没有冗余广播 User 消息（前端已乐观呈现，避免追问时重复），
	// 并且没有因为 cancelSend 而收到 TurnCancelled
	var receivedUserMsg bool
	var receivedTurnStarted bool
	timeout := time.After(1 * time.Second)

loop:
	for {
		select {
		case ev := <-subEvents:
			if ev.Type == agents.EventMessage && ev.Message != nil && ev.Message.Role == "user" {
				receivedUserMsg = true
			}
			if ev.Type == agents.EventTurn && ev.Turn != nil && ev.Turn.Phase == agents.TurnStarted {
				receivedTurnStarted = true
			}
			if ev.Type == agents.EventTurn && ev.Turn != nil && ev.Turn.Phase == agents.TurnEnded {
				t.Fatalf("turn must not end due to external context cancel, got %+v", ev)
			}
			if receivedTurnStarted {
				// Wait a brief moment to ensure no unexpected user message arrives
				select {
				case extra := <-subEvents:
					if extra.Type == agents.EventMessage && extra.Message != nil && extra.Message.Role == "user" {
						receivedUserMsg = true
					}
				case <-time.After(100 * time.Millisecond):
				}
				break loop
			}
		case <-timeout:
			break loop
		}
	}

	if receivedUserMsg {
		t.Fatal("unexpected user message event in event stream; user messages are rendered optimistically by client")
	}
	if !receivedTurnStarted {
		t.Fatal("expected TurnStarted in event stream")
	}

	_ = adapter.StopInstance(ctx, inst.ID)
}

func TestInterruptReturnsTurnCancelled(t *testing.T) {
	const native = "conv-cancel"
	executable := writeFakeAgy(t, `python3 -c '
import sys, json
prompt_id = None
for line in sys.stdin:
    req = json.loads(line)
    method = req.get("method")
    rid = req.get("id")
    if method == "initialize":
        print(json.dumps({"jsonrpc":"2.0","id":rid,"result":{"protocolVersion":1}}), flush=True)
    elif method == "session/new":
        print(json.dumps({"jsonrpc":"2.0","id":rid,"result":{"sessionId":"`+native+`"}}), flush=True)
    elif method == "session/prompt":
        prompt_id = rid
    elif method == "session/cancel":
        # Reply to cancel request
        print(json.dumps({"jsonrpc":"2.0","id":rid,"result":{}}), flush=True)
        # Reply to prompt request with stopReason: cancelled
        if prompt_id:
            print(json.dumps({"jsonrpc":"2.0","id":prompt_id,"result":{"stopReason":"cancelled"}}), flush=True)
'`)
	adapter := newPendingTestAdapter(t, executable, 5*time.Second)
	ctx := context.Background()
	inst, err := adapter.StartInstance(ctx, agents.StartInstanceRequest{Directory: t.TempDir()})
	if err != nil {
		t.Fatalf("StartInstance: %v", err)
	}

	subEvents, err := adapter.Subscribe(ctx, inst.ID)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	ref, err := adapter.CreateSession(ctx, inst.ID, agents.CreateSessionOptions{Title: "t"})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	err = adapter.SendMessage(ctx, *ref, agents.SendMessageRequest{
		Text: "Run something long",
	})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	// 调用 Interrupt
	if err := adapter.Interrupt(ctx, *ref); err != nil {
		t.Fatalf("Interrupt: %v", err)
	}

	// 监听事件，必须收到 Outcome == TurnCancelled 的 TurnEnded 事件
	var turnCancelled bool
	timeout := time.After(2 * time.Second)

waitLoop:
	for {
		select {
		case ev := <-subEvents:
			if ev.Type == agents.EventTurn && ev.Turn != nil && ev.Turn.Phase == agents.TurnEnded {
				if ev.Turn.Outcome == agents.TurnCancelled {
					turnCancelled = true
					break waitLoop
				} else {
					t.Fatalf("expected TurnCancelled, got Outcome=%q", ev.Turn.Outcome)
				}
			}
		case <-timeout:
			break waitLoop
		}
	}

	if !turnCancelled {
		t.Fatal("expected TurnEnded with TurnCancelled after interrupt")
	}

	_ = adapter.StopInstance(ctx, inst.ID)
}

func TestSendMessageNoDuplicateUserMessageOnACPChunk(t *testing.T) {
	const native = "conv-dedup-user"
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
        # 模拟真实 agy 进程：收到 prompt 后，先推送 user_message_chunk，再推送 agent_message_chunk，最后回复 prompt
        user_update = {"sessionId":"`+native+`","update":{"sessionUpdate":"user_message_chunk","content":{"type":"text","text":"Hello World"}}}
        print(json.dumps({"jsonrpc":"2.0","method":"session/update","params":user_update}), flush=True)
        agent_update = {"sessionId":"`+native+`","update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"Hi!"}}}
        print(json.dumps({"jsonrpc":"2.0","method":"session/update","params":agent_update}), flush=True)
        print(json.dumps({"jsonrpc":"2.0","id":rid,"result":{"stopReason":"completed"}}), flush=True)
'`)
	adapter := newPendingTestAdapter(t, executable, 5*time.Second)
	ctx := context.Background()
	inst, err := adapter.StartInstance(ctx, agents.StartInstanceRequest{Directory: t.TempDir()})
	if err != nil {
		t.Fatalf("StartInstance: %v", err)
	}

	subEvents, err := adapter.Subscribe(ctx, inst.ID)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	ref, err := adapter.CreateSession(ctx, inst.ID, agents.CreateSessionOptions{Title: "t"})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	err = adapter.SendMessage(ctx, *ref, agents.SendMessageRequest{
		Text: "Hello World",
	})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	var userMsgCount int
	var receivedTurnEnded bool
	timeout := time.After(2 * time.Second)

loop:
	for {
		select {
		case ev := <-subEvents:
			if ev.Type == agents.EventMessage && ev.Message != nil && ev.Message.Role == "user" {
				userMsgCount++
			}
			if ev.Type == agents.EventTurn && ev.Turn != nil && ev.Turn.Phase == agents.TurnEnded {
				receivedTurnEnded = true
				break loop
			}
		case <-timeout:
			break loop
		}
	}

	if !receivedTurnEnded {
		t.Fatal("expected TurnEnded in event stream")
	}
	if userMsgCount != 0 {
		t.Fatalf("expected 0 user message events in stream (optimistically appended by client), got %d", userMsgCount)
	}

	_ = adapter.StopInstance(ctx, inst.ID)
}

func liveInstance(t *testing.T, a *Adapter, id string) *instance {
	t.Helper()
	inst, err := a.getInstance(id)
	if err != nil {
		t.Fatalf("getInstance: %v", err)
	}
	return inst
}
