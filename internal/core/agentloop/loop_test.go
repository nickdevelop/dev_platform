package agentloop_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"devplatform/internal/core/agentloop"
)

type mockCompleter struct {
	steps []func(req agentloop.CompletionRequest) (agentloop.CompletionDelta, error)
	call  int
}

func (m *mockCompleter) Stream(ctx context.Context, req agentloop.CompletionRequest, onDelta func(agentloop.CompletionDelta) error) error {
	if m.call >= len(m.steps) {
		return fmt.Errorf("unexpected call %d", m.call)
	}
	delta, err := m.steps[m.call](req)
	m.call++
	if err != nil {
		return err
	}
	return onDelta(delta)
}

func TestPureConversationTurn(t *testing.T) {
	completer := &mockCompleter{
		steps: []func(req agentloop.CompletionRequest) (agentloop.CompletionDelta, error){
			func(req agentloop.CompletionRequest) (agentloop.CompletionDelta, error) {
				return agentloop.CompletionDelta{
					Text:      "你好！我是开发管家。",
					Reasoning: "用户问好，给予简短礼貌回复。",
				}, nil
			},
		},
	}

	runner := agentloop.NewRunner(completer, nil)
	outcome, err := runner.Run(context.Background(), agentloop.CompletionRequest{
		Model: "mock-model",
		Messages: []agentloop.CompletionMessage{
			{Role: agentloop.RoleUser, Text: "你好"},
		},
	}, agentloop.LoopOptions{MaxSteps: 5})

	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if outcome.FinalText != "你好！我是开发管家。" {
		t.Errorf("got %q, want %q", outcome.FinalText, "你好！我是开发管家。")
	}
	if outcome.Reasoning != "用户问好，给予简短礼貌回复。" {
		t.Errorf("got reasoning %q", outcome.Reasoning)
	}
	if outcome.StepsTaken != 0 {
		t.Errorf("expected 0 tool steps, got %d", outcome.StepsTaken)
	}
	if len(outcome.Messages) != 2 {
		t.Fatalf("expected 2 messages (user + assistant), got %d", len(outcome.Messages))
	}
}

func TestReActToolExecutionLoop(t *testing.T) {
	registry := agentloop.NewRegistry()
	registry.Register(&agentloop.FuncTool{
		Def: agentloop.ToolDefinition{
			Type: "function",
			Function: agentloop.FunctionSchema{
				Name:        "get_project_info",
				Description: "获取项目详情",
				Parameters:  json.RawMessage(`{"type":"object"}`),
			},
		},
		Fn: func(ctx context.Context, call agentloop.ToolCall) (agentloop.ToolResult, error) {
			return agentloop.ToolResult{
				Status: agentloop.ToolStatusCompleted,
				Output: `{"projectName":"dev_platform","status":"active"}`,
			}, nil
		},
	})

	completer := &mockCompleter{
		steps: []func(req agentloop.CompletionRequest) (agentloop.CompletionDelta, error){
			// Turn 1: Assistant requests tool call
			func(req agentloop.CompletionRequest) (agentloop.CompletionDelta, error) {
				return agentloop.CompletionDelta{
					ToolCalls: []agentloop.ToolCall{
						{
							ID:        "call_1",
							Name:      "get_project_info",
							Arguments: "{}",
						},
					},
				}, nil
			},
			// Turn 2: Assistant reads tool response and answers user
			func(req agentloop.CompletionRequest) (agentloop.CompletionDelta, error) {
				if len(req.Messages) < 3 {
					return agentloop.CompletionDelta{}, fmt.Errorf("missing tool message in context")
				}
				lastMsg := req.Messages[len(req.Messages)-1]
				if lastMsg.Role != agentloop.RoleTool || !strings.Contains(lastMsg.Text, "dev_platform") {
					return agentloop.CompletionDelta{}, fmt.Errorf("unexpected tool output: %+v", lastMsg)
				}
				return agentloop.CompletionDelta{
					Text: "当前项目为 dev_platform，处于活跃状态。",
				}, nil
			},
		},
	}

	runner := agentloop.NewRunner(completer, registry)
	outcome, err := runner.Run(context.Background(), agentloop.CompletionRequest{
		Model: "mock-model",
		Messages: []agentloop.CompletionMessage{
			{Role: agentloop.RoleUser, Text: "查询当前项目信息"},
		},
	}, agentloop.LoopOptions{MaxSteps: 5})

	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if outcome.StepsTaken != 1 {
		t.Errorf("expected 1 step, got %d", outcome.StepsTaken)
	}
	if !strings.Contains(outcome.FinalText, "dev_platform") {
		t.Errorf("expected final answer mentioning project, got %q", outcome.FinalText)
	}
	if len(outcome.Messages) != 4 {
		// user -> assistant(tool_call) -> tool(result) -> assistant(final)
		t.Fatalf("expected 4 transcript messages, got %d", len(outcome.Messages))
	}
}

func TestWorkflowSuspensionOnToolStatus(t *testing.T) {
	registry := agentloop.NewRegistry()
	registry.Register(&agentloop.FuncTool{
		Def: agentloop.ToolDefinition{
			Type: "function",
			Function: agentloop.FunctionSchema{
				Name: "flow_ask_user",
			},
		},
		Fn: func(ctx context.Context, call agentloop.ToolCall) (agentloop.ToolResult, error) {
			return agentloop.ToolResult{
				Status: agentloop.ToolStatusSuspended,
				Output: "已向用户发起提问，等待用户回答",
			}, nil
		},
	})

	completer := &mockCompleter{
		steps: []func(req agentloop.CompletionRequest) (agentloop.CompletionDelta, error){
			func(req agentloop.CompletionRequest) (agentloop.CompletionDelta, error) {
				return agentloop.CompletionDelta{
					ToolCalls: []agentloop.ToolCall{
						{
							ID:        "ask_1",
							Name:      "flow_ask_user",
							Arguments: `{"question":"请问您需要使用什么数据库？"}`,
						},
					},
				}, nil
			},
		},
	}

	runner := agentloop.NewRunner(completer, registry)
	outcome, err := runner.Run(context.Background(), agentloop.CompletionRequest{
		Model: "mock-model",
		Messages: []agentloop.CompletionMessage{
			{Role: agentloop.RoleUser, Text: "启动协同流程"},
		},
	}, agentloop.LoopOptions{MaxSteps: 5})

	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if !outcome.Suspended {
		t.Errorf("expected run to be suspended, got false")
	}
	if outcome.PendingCall == nil || outcome.PendingCall.Name != "flow_ask_user" {
		t.Errorf("expected pending call flow_ask_user, got %+v", outcome.PendingCall)
	}
}
