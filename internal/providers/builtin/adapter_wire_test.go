package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"devplatform/internal/core/agents"
)

// capturedRequests records the request bodies one mock model server received,
// in order.
type capturedRequests struct {
	mu   sync.Mutex
	list []string
}

func (c *capturedRequests) add(body []byte) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.list = append(c.list, string(body))
	return len(c.list)
}

func (c *capturedRequests) all() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.list...)
}

// objectToolArguments reports the first tool-call argument field that is not a
// JSON string. OpenAI-compatible services (DeepSeek among them) answer the
// object form with 400 "Invalid input: expected string, received object".
func objectToolArguments(body []byte) string {
	var payload struct {
		Messages []struct {
			ToolCalls []struct {
				Function struct {
					Arguments json.RawMessage `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "body"
	}
	for index, message := range payload.Messages {
		for callIndex, call := range message.ToolCalls {
			arguments := strings.TrimSpace(string(call.Function.Arguments))
			if arguments != "" && !strings.HasPrefix(arguments, `"`) {
				return fmt.Sprintf("messages.%d.tool_calls.%d.function.arguments", index, callIndex)
			}
		}
	}
	return ""
}

// messageRoles lists the role sequence of one request body.
func messageRoles(body []byte) []string {
	var payload struct {
		Messages []struct {
			Role string `json:"role"`
		} `json:"messages"`
	}
	_ = json.Unmarshal(body, &payload)
	roles := make([]string, 0, len(payload.Messages))
	for _, message := range payload.Messages {
		roles = append(roles, message.Role)
	}
	return roles
}

// strictToolServer replays one tool call followed by a final message, and
// rejects every request that breaks the OpenAI tool-call argument shape.
func strictToolServer(requests *capturedRequests) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		step := requests.add(body)
		if param := objectToolArguments(body); param != "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprintf(w, `{"error":{"message":"Invalid input: expected string, received object","type":"invalid_request_error","param":%q}}`, param)
			return
		}
		if step == 1 {
			sse(w, sseToolCall())
			return
		}
		sse(w, sseFinal())
	}))
}

// runBuiltinTurn sends one message through a builtin adapter against endpoint
// and returns the events up to the turn boundary.
func runBuiltinTurn(t *testing.T, endpointURL string) []agents.NormalizedEvent {
	t.Helper()
	adapter := New(Options{
		TranscriptStore: newTranscriptMemory(),
		ModelResolver:   modelStub{endpoint: ModelEndpoint{BaseURL: endpointURL, ModelID: "deepseek-chat"}},
		PermissionMode:  func() string { return PermissionModeAuto },
		Now:             func() time.Time { return time.Unix(0, 0).UTC() },
	})
	runtime, err := adapter.StartInstance(context.Background(), agents.StartInstanceRequest{Directory: t.TempDir()})
	if err != nil {
		t.Fatalf("StartInstance: %v", err)
	}
	ref, err := adapter.CreateSession(context.Background(), runtime.ID, agents.CreateSessionOptions{Title: "会话"})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	stream, err := adapter.Subscribe(context.Background(), runtime.ID)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	if err := adapter.SendMessage(context.Background(), *ref, agents.SendMessageRequest{Text: "写个文件"}); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	return drainUntilTurnEnd(t, stream)
}

// turnError returns the turn's failure text, or an empty string.
func turnError(events []agents.NormalizedEvent) string {
	if event, ok := eventOfKind(events, agents.EventError); ok && event.Error != nil {
		return event.Error.Message
	}
	return ""
}

// TestToolCallArgumentsAreJSONStrings pins the OpenAI wire rule DeepSeek
// enforces: tool_calls[].function.arguments is a JSON-encoded string, not an
// object.
func TestToolCallArgumentsAreJSONStrings(t *testing.T) {
	requests := &capturedRequests{}
	server := strictToolServer(requests)
	defer server.Close()

	events := runBuiltinTurn(t, server.URL)
	if failure := turnError(events); failure != "" {
		t.Fatalf("回合失败: %s", failure)
	}

	bodies := requests.all()
	if len(bodies) != 2 {
		t.Fatalf("请求数 = %d, want 2", len(bodies))
	}
	for index, body := range bodies {
		if param := objectToolArguments([]byte(body)); param != "" {
			t.Fatalf("请求 #%d 的 %s 不是字符串", index+1, param)
		}
	}
	if want := `"arguments":"{\"path\":\"notes.txt\",\"content\":\"hello\"}"`; !strings.Contains(bodies[1], want) {
		t.Fatalf("第二段请求缺少字符串形式的参数: %s", bodies[1])
	}
}

// TestTurnContextDoesNotDuplicateUserMessage pins the rebuilt context shape:
// the fresh user message is part of the request exactly once.
func TestTurnContextDoesNotDuplicateUserMessage(t *testing.T) {
	requests := &capturedRequests{}
	server := strictToolServer(requests)
	defer server.Close()

	events := runBuiltinTurn(t, server.URL)
	if failure := turnError(events); failure != "" {
		t.Fatalf("回合失败: %s", failure)
	}

	bodies := requests.all()
	if len(bodies) != 2 {
		t.Fatalf("请求数 = %d, want 2", len(bodies))
	}
	want := [][]string{
		{"system", "user"},
		{"system", "user", "assistant", "tool"},
	}
	for index, body := range bodies {
		if roles := messageRoles([]byte(body)); !slices.Equal(roles, want[index]) {
			t.Fatalf("请求 #%d 角色序列 = %v, want %v", index+1, roles, want[index])
		}
	}
}

// TestNormalizeToolArguments pins the defensive fallback: an empty or broken
// fragment still yields decodable JSON on the wire.
func TestNormalizeToolArguments(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "plain", input: `{"path":"notes.txt"}`, want: `{"path":"notes.txt"}`},
		{name: "padded", input: "  {\"path\":\"notes.txt\"}\n", want: `{"path":"notes.txt"}`},
		{name: "empty", input: "", want: "{}"},
		{name: "blank", input: "   ", want: "{}"},
		{name: "truncated", input: `{"path":`, want: "{}"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := normalizeToolArguments(test.input); got != test.want {
				t.Fatalf("normalizeToolArguments(%q) = %q, want %q", test.input, got, test.want)
			}
		})
	}
}

// TestWireToolCallMarshalsArgumentsAsString pins the outgoing JSON shape: a
// valid fragment is escaped into a string, a malformed one degrades to "{}".
func TestWireToolCallMarshalsArgumentsAsString(t *testing.T) {
	message := chatMessage{Role: "assistant", ToolCalls: []wireToolCall{{
		ID:       "call-1",
		Type:     "function",
		Function: wireFunctionCall{Name: "write_file", Arguments: `{"path":"notes.txt"}`},
	}}}
	encoded, err := json.Marshal(map[string]any{"messages": []chatMessage{message}})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if param := objectToolArguments(encoded); param != "" {
		t.Fatalf("arguments 不是字符串: %s (%s)", param, encoded)
	}
	if want := `"arguments":"{\"path\":\"notes.txt\"}"`; !strings.Contains(string(encoded), want) {
		t.Fatalf("wire = %s, want 包含 %s", encoded, want)
	}

	broken := chatMessage{Role: "assistant", ToolCalls: []wireToolCall{{
		ID:       "call-2",
		Type:     "function",
		Function: wireFunctionCall{Name: "list_directory", Arguments: `{"path":`},
	}}}
	encoded, err = json.Marshal(map[string]any{"messages": []chatMessage{broken}})
	if err != nil {
		t.Fatalf("marshal malformed: %v", err)
	}
	if !strings.Contains(string(encoded), `"arguments":"{}"`) {
		t.Fatalf("非法参数未归一: %s", encoded)
	}
}

// TestRenderHistoryMessagesEmitsStringArguments covers replay: a stored tool
// call must go back to the model with string arguments too.
func TestRenderHistoryMessagesEmitsStringArguments(t *testing.T) {
	records := []TranscriptRecord{
		{Seq: 1, Kind: TranscriptUser, Content: "列出文件"},
		{Seq: 2, Kind: TranscriptToolCall, Payload: json.RawMessage(`{"callId":"call-9","name":"list_directory","input":{"path":"."}}`)},
		{Seq: 3, Kind: TranscriptToolResult, Payload: json.RawMessage(`{"callId":"call-9","output":"ok"}`)},
	}
	messages := renderHistoryMessages(records, ModelEndpoint{})
	encoded, err := json.Marshal(map[string]any{"messages": messages})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if param := objectToolArguments(encoded); param != "" {
		t.Fatalf("回放参数不是字符串: %s (%s)", param, encoded)
	}
	if want := `"arguments":"{\"path\":\".\"}"`; !strings.Contains(string(encoded), want) {
		t.Fatalf("wire = %s, want 包含 %s", encoded, want)
	}
}

// TestBuildToolCallRecordKeepsCallIDForMalformedArguments pins the storage
// fallback: a broken fragment must not drop the call id, or the rebuilt
// context would carry an orphan tool message.
func TestBuildToolCallRecordKeepsCallIDForMalformedArguments(t *testing.T) {
	record := buildToolCallRecord(1, "list_directory", "call-9", json.RawMessage(`{"path":`), time.Unix(0, 0).UTC())
	var payload toolCallRecord
	if err := json.Unmarshal(record.Payload, &payload); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if payload.CallID != "call-9" || payload.Name != "list_directory" {
		t.Fatalf("payload = %+v", payload)
	}
	if string(payload.Input) != "{}" {
		t.Fatalf("input = %s, want {}", payload.Input)
	}
}
