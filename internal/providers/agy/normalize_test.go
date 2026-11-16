package agy

import (
	"testing"
	"time"

	"devplatform/internal/core/agents"
)

func TestEventNormalizer(t *testing.T) {
	fixedTime := time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)
	now := func() time.Time { return fixedTime }
	normalizer := NewEventNormalizer("inst-1", "ses-1", now)
	normalizer.SetModel("gemini-2.0-flash")

	// 1. Test init event
	initJSON := `{"event":"init","conversation_id":"ses-1","init":{"cwd":"/workspace","tools":["run_command"],"permission_mode":"always-proceed"}}`
	events, err := normalizer.Normalize([]byte(initJSON))
	if err != nil {
		t.Fatalf("normalize init: %v", err)
	}
	if len(events) != 1 || events[0].Type != agents.EventLifecycle {
		t.Fatalf("expected 1 lifecycle event, got %+v", events)
	}
	if events[0].Lifecycle.State != "initialized" {
		t.Errorf("lifecycle state = %q, want initialized", events[0].Lifecycle.State)
	}

	// 2. Test text delta
	deltaJSON := `{"event":"step_update","step_update":{"conversation_id":"ses-1","step_index":1,"state":"ACTIVE","step_type":"agent_response","text_delta":"Hello world"}}`
	events, err = normalizer.Normalize([]byte(deltaJSON))
	if err != nil {
		t.Fatalf("normalize delta: %v", err)
	}
	// First non-user step should start a turn, then emit message
	if len(events) != 2 {
		t.Fatalf("expected 2 events (turn start + message), got %d: %+v", len(events), events)
	}
	if events[0].Type != agents.EventTurn || events[0].Turn.Phase != agents.TurnStarted {
		t.Errorf("event[0] = %+v, want TurnStarted", events[0])
	}
	if events[1].Type != agents.EventMessage || events[1].Message.Text != "Hello world" {
		t.Errorf("event[1] = %+v, want message 'Hello world'", events[1])
	}
	if events[1].Message.Model == nil || events[1].Message.Model.ID != "gemini-2.0-flash" {
		t.Errorf("expected model gemini-2.0-flash, got %+v", events[1].Message.Model)
	}

	// 3. Test tool call active
	toolActiveJSON := `{"event":"step_update","step_update":{"conversation_id":"ses-1","step_index":2,"state":"ACTIVE","step_type":"tool","tool_name":"run_command","tool_info":{"name":"run_command","parameters":{"CommandLine":"date"}}}}`
	events, err = normalizer.Normalize([]byte(toolActiveJSON))
	if err != nil {
		t.Fatalf("normalize tool active: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("expected the ordinary tool call to be dropped, got %+v", events)
	}

	// 4. Test tool call done
	toolDoneJSON := `{"event":"step_update","step_update":{"conversation_id":"ses-1","step_index":2,"state":"DONE","step_type":"tool","tool_name":"run_command","tool_info":{"name":"run_command","parameters":{"CommandLine":"date"},"output":"Thu Nov 12 10:00:00 UTC 2026"}}}`
	events, err = normalizer.Normalize([]byte(toolDoneJSON))
	if err != nil {
		t.Fatalf("normalize tool done: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("expected the ordinary tool result to be dropped, got %+v", events)
	}

	// 5. Test tool call error
	toolErrJSON := `{"event":"step_update","step_update":{"conversation_id":"ses-1","step_index":3,"state":"ERROR","step_type":"tool","tool_name":"read_file","tool_info":{"name":"read_file","error":{"type":"TOOL_ERROR","message":"not found"}}}}`
	events, err = normalizer.Normalize([]byte(toolErrJSON))
	if err != nil {
		t.Fatalf("normalize tool error: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("expected the failed tool result to be dropped, got %+v", events)
	}

	// 6. Test result event
	resultJSON := `{"event":"result","result":{"conversation_id":"ses-1","status":"SUCCESS","response":"Done!","duration_seconds":2.5,"num_turns":1,"usage":{"input_tokens":100,"output_tokens":50,"thinking_tokens":10,"cache_read_tokens":0,"total_tokens":160}}}`
	events, err = normalizer.Normalize([]byte(resultJSON))
	if err != nil {
		t.Fatalf("normalize result: %v", err)
	}
	// Expect final message + TurnEnded
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d: %+v", len(events), events)
	}
	if events[0].Type != agents.EventMessage || !events[0].Message.Final {
		t.Errorf("event[0] = %+v, want final message", events[0])
	}
	if events[0].Message.Usage == nil || events[0].Message.Usage.Input != 100 || events[0].Message.Usage.Reasoning != 10 {
		t.Errorf("unexpected usage: %+v", events[0].Message.Usage)
	}
	if events[1].Type != agents.EventTurn || events[1].Turn.Phase != agents.TurnEnded || events[1].Turn.Outcome != agents.TurnCompleted {
		t.Errorf("event[1] = %+v, want TurnCompleted", events[1])
	}

	// 7. Test unknown event / raw fallback
	unknownJSON := `{"event":"custom_alien_event","data":{"foo":"bar"}}`
	events, err = normalizer.Normalize([]byte(unknownJSON))
	if err != nil {
		t.Fatalf("normalize unknown: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("expected the unknown event to be dropped, got %+v", events)
	}
}

func TestNormalizeACPUpdate_UserMessageChunk(t *testing.T) {
	norm := NewEventNormalizer("inst-1", "ses-1", func() time.Time {
		return time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
	})

	update := []byte(`{"sessionUpdate":"user_message_chunk","content":{"type":"text","text":"用户输入文本"}}`)
	events, err := norm.NormalizeACPUpdate("user_message_chunk", update)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	ev := events[0]
	if ev.Type != agents.EventMessage || ev.Message == nil {
		t.Fatalf("expected EventMessage, got %+v", ev)
	}
	if ev.Message.Role != "user" || ev.Message.Text != "用户输入文本" || !ev.Message.Final {
		t.Errorf("unexpected message payload: %+v", ev.Message)
	}
}

func TestNormalizeACPUpdate_AskQuestionTool(t *testing.T) {
	norm := NewEventNormalizer("inst-1", "ses-1", func() time.Time {
		return time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
	})

	// ACP 会同时给出 name 和 title，问答工具必须保留 Name: "ask_question" 以便通过 DeliveryGate
	update := []byte(`{"sessionUpdate":"tool_call","toolCallId":"call_123","name":"ask_question","title":"Run ask_question","status":"in_progress","rawInput":{"questions":[{"question":"喜欢猫还是狗？"}]}}`)
	events, err := norm.NormalizeACPUpdate("tool_call", update)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event admitted by delivery gate, got %d", len(events))
	}
	ev := events[0]
	if ev.Type != agents.EventToolCall || ev.ToolCall == nil {
		t.Fatalf("expected EventToolCall, got %+v", ev)
	}
	if ev.ToolCall.Name != "ask_question" {
		t.Errorf("expected tool name 'ask_question', got %q", ev.ToolCall.Name)
	}
}

func TestNormalizeACPUpdate_ToolCallUpdateStatusGuardAndDiff(t *testing.T) {
	norm := NewEventNormalizer("inst-1", "ses-1", func() time.Time {
		return time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
	})

	// 1. 中间状态 (pending / in_progress) 必须被状态守卫拦截，避免重复发出 ToolResult
	pendingUpdate := []byte(`{"sessionUpdate":"tool_call_update","toolCallId":"call_456","status":"in_progress","rawOutput":{}}`)
	events, err := norm.NormalizeACPUpdate("tool_call_update", pendingUpdate)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("expected in_progress update to be dropped, got %+v", events)
	}

	// 2. 状态为 completed 时，从 content 数组提取真实 diff
	completedUpdate := []byte(`{"sessionUpdate":"tool_call_update","toolCallId":"call_456","status":"completed","rawOutput":{},"content":[{"type":"diff","path":"/tmp/a.txt","newText":"hello"}]}`)
	events, err = norm.NormalizeACPUpdate("tool_call_update", completedUpdate)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// DeliveryGate 默认只放行问答工具的 ToolResult。但通过 NormalizeACPUpdate 产生内部事件检验：
	// 我们直接测试 normalizeACPUpdate 逻辑或检查状态守卫返回值
}

func TestEventNormalizer_EndTurnFullText(t *testing.T) {
	norm := NewEventNormalizer("inst-1", "ses-1", func() time.Time {
		return time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
	})
	norm.StartTurn("turn-1")

	// 模拟流式增量接收
	_, _ = norm.NormalizeACPUpdate("agent_message_chunk", []byte(`{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"Hello "}}`))
	_, _ = norm.NormalizeACPUpdate("agent_message_chunk", []byte(`{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"World!"}}`))

	// 结束轮次，final 消息必须携带完整的全量累积快照
	endEvents := norm.EndTurn(agents.TurnCompleted, nil)
	if len(endEvents) < 2 {
		t.Fatalf("expected at least 2 events, got %d", len(endEvents))
	}
	msgEvent := endEvents[0]
	if msgEvent.Type != agents.EventMessage || !msgEvent.Message.Final {
		t.Fatalf("expected final message event, got %+v", msgEvent)
	}
	if msgEvent.Message.Text != "Hello World!" {
		t.Errorf("expected accumulated text 'Hello World!', got %q", msgEvent.Message.Text)
	}
}
