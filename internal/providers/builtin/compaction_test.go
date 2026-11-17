package builtin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"devplatform/internal/core/agents"
)

func sseText(text string) map[string]any {
	return map[string]any{
		"id": "c", "object": "chat.completion.chunk", "created": 1,
		"choices": []any{map[string]any{
			"delta": map[string]any{"content": text}, "finish_reason": "stop",
		}},
	}
}

func TestCompactSession(t *testing.T) {
	workDir := t.TempDir()
	store := newTranscriptMemory()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// Mock LLM returning compaction summary
		sse(w, sseText("本次会话修改了 main.go 并通过了测试。"))
	}))
	defer server.Close()

	adapter := New(Options{
		TranscriptStore: store,
		ModelResolver:   modelStub{endpoint: ModelEndpoint{BaseURL: server.URL, ModelID: "test-model"}},
		PermissionMode:  func() string { return PermissionModeAuto },
		Now:             func() time.Time { return time.Unix(0, 0).UTC() },
	})

	// Assert interface implementation
	var _ agents.SessionCompactor = adapter

	inst, err := adapter.StartInstance(context.Background(), agents.StartInstanceRequest{Directory: workDir})
	if err != nil {
		t.Fatalf("StartInstance: %v", err)
	}
	ref, err := adapter.CreateSession(context.Background(), inst.ID, agents.CreateSessionOptions{Title: "长会话"})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	sub, err := adapter.Subscribe(context.Background(), inst.ID)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	// 1. When records are few (<=4), CompactSession returns nil without calling completion
	if err := adapter.CompactSession(context.Background(), *ref, "精简"); err != nil {
		t.Fatalf("expected nil for short session, got: %v", err)
	}

	// 2. Populate more than 4 records
	for i := 1; i <= 6; i++ {
		_ = store.Append(context.Background(), ref.SessionID, []TranscriptRecord{
			{
				Seq:       i,
				Kind:      TranscriptUser,
				Content:   "User step question",
				CreatedAt: time.Now(),
			},
		})
	}

	// 3. Compact long session
	if err := adapter.CompactSession(context.Background(), *ref, "只保留核心文件变动"); err != nil {
		t.Fatalf("CompactSession failed: %v", err)
	}

	// 4. Verify compaction message was appended to transcript
	records, err := store.Load(context.Background(), ref.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	lastRecord := records[len(records)-1]
	if lastRecord.Kind != TranscriptAssistant {
		t.Errorf("expected last record kind assistant, got %v", lastRecord.Kind)
	}
	if !strings.Contains(lastRecord.Content, "【上下文已压缩】") || !strings.Contains(lastRecord.Content, "本次会话修改了 main.go") {
		t.Errorf("unexpected compaction content: %q", lastRecord.Content)
	}

	// 5. Verify published event
	select {
	case ev := <-sub:
		if ev.Type != agents.EventMessage || ev.Message == nil || !strings.Contains(ev.Message.Text, "【上下文已压缩】") {
			t.Errorf("unexpected event: %+v", ev)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for compaction event")
	}
}
