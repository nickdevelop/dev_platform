package agy

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"devplatform/internal/core/agents"
	"devplatform/internal/providers/agy/acp"
)

// RawagyEvent is the legacy NDJSON event shape produced by `agy --output-format stream-json`.
type RawagyEvent struct {
	Event          string          `json:"event"`
	ConversationID string          `json:"conversation_id,omitempty"`
	Init           *RawInitPayload `json:"init,omitempty"`
	StepUpdate     *RawStepUpdate  `json:"step_update,omitempty"`
	Result         *RawResult      `json:"result,omitempty"`
}

// RawInitPayload carries initialization metadata from agy.
type RawInitPayload struct {
	Cwd            string   `json:"cwd"`
	Tools          []string `json:"tools"`
	PermissionMode string   `json:"permission_mode"`
}

// RawStepUpdate carries one step progress update from agy.
type RawStepUpdate struct {
	ConversationID string         `json:"conversation_id,omitempty"`
	StepIndex      int            `json:"step_index"`
	State          string         `json:"state"`     // "ACTIVE", "DONE", "ERROR"
	StepType       string         `json:"step_type"` // "user_input", "system_message", "agent_response", "tool"
	TextDelta      string         `json:"text_delta,omitempty"`
	ToolName       string         `json:"tool_name,omitempty"`
	ToolInfo       *RawToolInfo   `json:"tool_info,omitempty"`
	DurationSec    float64        `json:"duration_seconds,omitempty"`
	Usage          *RawTokenUsage `json:"usage,omitempty"`
}

// RawToolInfo carries details for tool calls and executions.
type RawToolInfo struct {
	Name       string          `json:"name"`
	Parameters json.RawMessage `json:"parameters,omitempty"`
	Output     string          `json:"output,omitempty"`
	Error      *RawToolError   `json:"error,omitempty"`
}

// RawToolError carries error details for failed tool executions.
type RawToolError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// RawResult carries the outcome of a turn from agy.
type RawResult struct {
	ConversationID string         `json:"conversation_id"`
	Status         string         `json:"status"` // "SUCCESS", "ERROR"
	Response       string         `json:"response,omitempty"`
	Error          string         `json:"error,omitempty"`
	DurationSec    float64        `json:"duration_seconds,omitempty"`
	NumTurns       int            `json:"num_turns,omitempty"`
	Usage          *RawTokenUsage `json:"usage,omitempty"`
	FinishedReason string         `json:"finished_reason,omitempty"`
}

// RawTokenUsage reports token usage statistics from agy.
type RawTokenUsage struct {
	InputTokens     int `json:"input_tokens"`
	OutputTokens    int `json:"output_tokens"`
	ThinkingTokens  int `json:"thinking_tokens"`
	CacheReadTokens int `json:"cache_read_tokens"`
	TotalTokens     int `json:"total_tokens"`
}

// EventNormalizer translates agy's ACP events and stream-json output lines into platform NormalizedEvents.
type EventNormalizer struct {
	instanceID string
	sessionID  string
	backend    agents.BackendID
	model      string
	now        func() time.Time

	currentTurnID   string
	turnActive      bool
	activeCalls     map[int]string // stepIndex -> toolName
	accumulatedText strings.Builder
	gate            *agents.DeliveryGate
}

// NewEventNormalizer creates a normalizer for one agy session stream.
func NewEventNormalizer(instanceID, sessionID string, now func() time.Time) *EventNormalizer {
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &EventNormalizer{
		instanceID:  instanceID,
		sessionID:   sessionID,
		backend:     agents.BackendAgy,
		now:         now,
		activeCalls: make(map[int]string),
		gate:        agents.NewDeliveryGate(),
	}
}

// SetSessionID updates the session ID if it was resolved from agy.
func (n *EventNormalizer) SetSessionID(sessionID string) {
	n.sessionID = sessionID
}

// SetModel updates the model configured for this agy session normalizer.
func (n *EventNormalizer) SetModel(model string) {
	n.model = model
}

func (n *EventNormalizer) messageModel() *agents.MessageModel {
	if n.model == "" {
		return nil
	}
	return &agents.MessageModel{
		ID:       n.model,
		Provider: string(n.backend),
	}
}

// SessionID returns the current session ID.
func (n *EventNormalizer) SessionID() string {
	return n.sessionID
}

// StartTurn opens a turn with turnID.
func (n *EventNormalizer) StartTurn(turnID string) {
	n.currentTurnID = turnID
	n.turnActive = true
	n.accumulatedText.Reset()
}

// CurrentTurnID returns the active turn ID.
func (n *EventNormalizer) CurrentTurnID() string {
	return n.currentTurnID
}

// NormalizeACPUpdate converts an ACP sessionUpdate into platform NormalizedEvents admitted by the delivery gate.
func (n *EventNormalizer) NormalizeACPUpdate(kind string, updateJSON json.RawMessage) ([]agents.NormalizedEvent, error) {
	ts := n.now()
	var events []agents.NormalizedEvent

	switch kind {
	case "user_message_chunk":
		// user_message_chunk 是底层 ACP 进程对客户端 prompt 消息的入流回显。
		// 平台在 sendTurn 启动时已主动向总线广播了带有完整文本与图片附件的 EventMessage(role: "user")，
		// 此处作为底层入流通知静默消费，不再重复发射，避免前端出现两条重复的用户消息。
		return nil, nil

	case "agent_message_chunk":
		var chunk acp.AgentMessageChunkUpdate
		if err := json.Unmarshal(updateJSON, &chunk); err != nil {
			return nil, err
		}
		if chunk.Content.Text != "" {
			n.accumulatedText.WriteString(chunk.Content.Text)
			events = append(events, agents.NormalizedEvent{
				InstanceID: n.instanceID,
				SessionID:  n.sessionID,
				Backend:    n.backend,
				Timestamp:  ts,
				Type:       agents.EventMessage,
				Message: &agents.MessagePayload{
					Role:  "assistant",
					Text:  chunk.Content.Text,
					Final: false,
					Model: n.messageModel(),
				},
			})
		}

	case "agent_thought_chunk":
		var chunk acp.AgentThoughtChunkUpdate
		if err := json.Unmarshal(updateJSON, &chunk); err != nil {
			return nil, err
		}
		if chunk.Content.Text != "" {
			events = append(events, agents.NormalizedEvent{
				InstanceID: n.instanceID,
				SessionID:  n.sessionID,
				Backend:    n.backend,
				Timestamp:  ts,
				Type:       agents.EventThinking,
				Thinking: &agents.ThinkingPayload{
					Text:  chunk.Content.Text,
					Final: false,
				},
			})
		}

	case "tool_call":
		var tc acp.ToolCallUpdatePayload
		if err := json.Unmarshal(updateJSON, &tc); err != nil {
			return nil, err
		}
		name := tc.Name
		if name == "" {
			name = tc.Title
		}
		events = append(events, agents.NormalizedEvent{
			InstanceID: n.instanceID,
			SessionID:  n.sessionID,
			Backend:    n.backend,
			Timestamp:  ts,
			Type:       agents.EventToolCall,
			ToolCall: &agents.ToolCallPayload{
				CallID: tc.ToolCallID,
				Name:   name,
				Input:  tc.RawInput,
			},
		})

	case "tool_call_update":
		var tc acp.ToolCallUpdatePayload
		if err := json.Unmarshal(updateJSON, &tc); err != nil {
			return nil, err
		}
		// 状态守卫：仅当工具已完成 (completed) 或失败 (failed) 时才产生 ToolResult，避免中间过程产生重复事件
		if tc.Status != "completed" && tc.Status != "failed" {
			return nil, nil
		}
		output := string(tc.RawOutput)
		if (output == "" || output == "{}") && len(tc.Content) > 0 {
			output = string(tc.Content)
		}
		events = append(events, agents.NormalizedEvent{
			InstanceID: n.instanceID,
			SessionID:  n.sessionID,
			Backend:    n.backend,
			Timestamp:  ts,
			Type:       agents.EventToolResult,
			ToolResult: &agents.ToolResultPayload{
				CallID:  tc.ToolCallID,
				Output:  output,
				IsError: tc.Status == "failed",
			},
		})
	}

	return n.gate.AdmitAll(events), nil
}

// EndTurn produces the closing EventMessage (Final: true) and EventTurn (TurnEnded).
func (n *EventNormalizer) EndTurn(outcome agents.TurnOutcome, usage *acp.UsageStats) []agents.NormalizedEvent {
	ts := n.now()
	var events []agents.NormalizedEvent

	var msgUsage *agents.MessageUsage
	if usage != nil {
		msgUsage = &agents.MessageUsage{
			Input:     usage.InputTokens,
			Output:    usage.OutputTokens,
			CacheRead: usage.CachedReadTokens,
		}
	}

	// 1. Closing message chunk (携带完整的累积文本快照用于客户端收敛自愈)
	finalText := n.accumulatedText.String()
	events = append(events, agents.NormalizedEvent{
		InstanceID: n.instanceID,
		SessionID:  n.sessionID,
		Backend:    n.backend,
		Timestamp:  ts,
		Type:       agents.EventMessage,
		Message: &agents.MessagePayload{
			Role:  "assistant",
			Text:  finalText,
			Final: true,
			Usage: msgUsage,
			Model: n.messageModel(),
		},
	})

	// 2. Conclude turn
	events = append(events, agents.NormalizedEvent{
		InstanceID: n.instanceID,
		SessionID:  n.sessionID,
		Backend:    n.backend,
		Timestamp:  ts,
		Type:       agents.EventTurn,
		Turn: &agents.TurnPayload{
			ID:      n.currentTurnID,
			Phase:   agents.TurnEnded,
			Outcome: outcome,
		},
	})

	n.turnActive = false
	n.accumulatedText.Reset()
	return n.gate.AdmitAll(events)
}

// Normalize parses one raw JSON line from agy stdout and returns the events the
// delivery gate admits from it. (Retained for backwards compatibility)
func (n *EventNormalizer) Normalize(line []byte) ([]agents.NormalizedEvent, error) {
	events, err := n.normalize(line)
	return n.gate.AdmitAll(events), err
}

func (n *EventNormalizer) normalize(line []byte) ([]agents.NormalizedEvent, error) {
	trimmed := strings.TrimSpace(string(line))
	if trimmed == "" {
		return nil, nil
	}

	var raw RawagyEvent
	if err := json.Unmarshal([]byte(trimmed), &raw); err != nil {
		return []agents.NormalizedEvent{
			{
				InstanceID: n.instanceID,
				SessionID:  n.sessionID,
				Backend:    n.backend,
				Timestamp:  n.now(),
				Type:       agents.EventUnknown,
				Raw:        json.RawMessage(trimmed),
			},
		}, nil
	}

	if raw.ConversationID != "" && n.sessionID == "" {
		n.sessionID = raw.ConversationID
	}

	switch raw.Event {
	case "init":
		return n.normalizeInit(&raw)
	case "step_update":
		return n.normalizeStepUpdate(&raw)
	case "result":
		return n.normalizeResult(&raw)
	default:
		return []agents.NormalizedEvent{
			{
				InstanceID: n.instanceID,
				SessionID:  n.sessionID,
				Backend:    n.backend,
				Timestamp:  n.now(),
				Type:       agents.EventUnknown,
				Raw:        json.RawMessage(trimmed),
			},
		}, nil
	}
}

func (n *EventNormalizer) normalizeInit(raw *RawagyEvent) ([]agents.NormalizedEvent, error) {
	if raw.ConversationID != "" && n.sessionID == "" {
		n.sessionID = raw.ConversationID
	}
	detail := "agy session initialized"
	if raw.Init != nil && raw.Init.PermissionMode != "" {
		detail = fmt.Sprintf("agy initialized (mode: %s)", raw.Init.PermissionMode)
	}
	return []agents.NormalizedEvent{
		{
			InstanceID: n.instanceID,
			SessionID:  n.sessionID,
			Backend:    n.backend,
			Timestamp:  n.now(),
			Type:       agents.EventLifecycle,
			Lifecycle: &agents.LifecycleEvent{
				State:  "initialized",
				Detail: detail,
			},
		},
	}, nil
}

func (n *EventNormalizer) normalizeStepUpdate(raw *RawagyEvent) ([]agents.NormalizedEvent, error) {
	su := raw.StepUpdate
	if su == nil {
		return nil, nil
	}

	ts := n.now()
	var events []agents.NormalizedEvent

	if su.StepType != "user_input" && su.StepType != "system_message" {
		if !n.turnActive {
			n.turnActive = true
			n.currentTurnID = fmt.Sprintf("turn-%d", su.StepIndex)
			events = append(events, agents.NormalizedEvent{
				InstanceID: n.instanceID,
				SessionID:  n.sessionID,
				Backend:    n.backend,
				Timestamp:  ts,
				Type:       agents.EventTurn,
				Turn: &agents.TurnPayload{
					ID:    n.currentTurnID,
					Phase: agents.TurnStarted,
				},
			})
		}
	}

	switch su.StepType {
	case "agent_response":
		if su.TextDelta != "" {
			events = append(events, agents.NormalizedEvent{
				InstanceID: n.instanceID,
				SessionID:  n.sessionID,
				Backend:    n.backend,
				Timestamp:  ts,
				Type:       agents.EventMessage,
				Message: &agents.MessagePayload{
					Role:  "assistant",
					Text:  su.TextDelta,
					Final: false,
					Model: n.messageModel(),
				},
			})
		}

	case "tool":
		callID := fmt.Sprintf("call-%d", su.StepIndex)
		toolName := su.ToolName
		if toolName == "" && su.ToolInfo != nil {
			toolName = su.ToolInfo.Name
		}

		if su.State == "ACTIVE" {
			n.activeCalls[su.StepIndex] = toolName
			var input json.RawMessage
			if su.ToolInfo != nil && len(su.ToolInfo.Parameters) > 0 {
				input = su.ToolInfo.Parameters
			}
			events = append(events, agents.NormalizedEvent{
				InstanceID: n.instanceID,
				SessionID:  n.sessionID,
				Backend:    n.backend,
				Timestamp:  ts,
				Type:       agents.EventToolCall,
				ToolCall: &agents.ToolCallPayload{
					CallID: callID,
					Name:   toolName,
					Input:  input,
				},
			})
		} else if su.State == "DONE" || su.State == "ERROR" {
			delete(n.activeCalls, su.StepIndex)
			output := ""
			if su.ToolInfo != nil {
				if su.ToolInfo.Output != "" {
					output = su.ToolInfo.Output
				} else if su.ToolInfo.Error != nil {
					output = su.ToolInfo.Error.Message
				}
			}
			events = append(events, agents.NormalizedEvent{
				InstanceID: n.instanceID,
				SessionID:  n.sessionID,
				Backend:    n.backend,
				Timestamp:  ts,
				Type:       agents.EventToolResult,
				ToolResult: &agents.ToolResultPayload{
					CallID: callID,
					Output: output,
				},
			})
		}
	}

	return events, nil
}

func (n *EventNormalizer) normalizeResult(raw *RawagyEvent) ([]agents.NormalizedEvent, error) {
	res := raw.Result
	if res == nil {
		return nil, nil
	}

	ts := n.now()
	var events []agents.NormalizedEvent

	var usage *agents.MessageUsage
	if res.Usage != nil {
		usage = &agents.MessageUsage{
			Input:     res.Usage.InputTokens,
			Output:    res.Usage.OutputTokens,
			Reasoning: res.Usage.ThinkingTokens,
			CacheRead: res.Usage.CacheReadTokens,
		}
	}

	events = append(events, agents.NormalizedEvent{
		InstanceID: n.instanceID,
		SessionID:  n.sessionID,
		Backend:    n.backend,
		Timestamp:  ts,
		Type:       agents.EventMessage,
		Message: &agents.MessagePayload{
			Role:  "assistant",
			Text:  "",
			Final: true,
			Usage: usage,
			Model: n.messageModel(),
		},
	})

	outcome := agents.TurnCompleted
	if res.Status == "ERROR" {
		outcome = agents.TurnFailed
	} else if res.FinishedReason == "CANCELLED" {
		outcome = agents.TurnCancelled
	}

	turnID := n.currentTurnID
	if turnID == "" {
		turnID = "turn-final"
	}

	events = append(events, agents.NormalizedEvent{
		InstanceID: n.instanceID,
		SessionID:  n.sessionID,
		Backend:    n.backend,
		Timestamp:  ts,
		Type:       agents.EventTurn,
		Turn: &agents.TurnPayload{
			ID:      turnID,
			Phase:   agents.TurnEnded,
			Outcome: outcome,
		},
	})

	n.turnActive = false
	return events, nil
}
