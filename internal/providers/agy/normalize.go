package agy

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"devplatform/internal/core/agents"
)

// RawagyEvent is the top-level NDJSON event shape produced by `agy --output-format stream-json`.
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
	ConversationID string        `json:"conversation_id,omitempty"`
	StepIndex      int           `json:"step_index"`
	State          string        `json:"state"`     // "ACTIVE", "DONE", "ERROR"
	StepType       string        `json:"step_type"` // "user_input", "system_message", "agent_response", "tool"
	TextDelta      string        `json:"text_delta,omitempty"`
	ToolName       string        `json:"tool_name,omitempty"`
	ToolInfo       *RawToolInfo  `json:"tool_info,omitempty"`
	DurationSec    float64       `json:"duration_seconds,omitempty"`
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
	ConversationID string        `json:"conversation_id"`
	Status         string        `json:"status"` // "SUCCESS", "ERROR"
	Response       string        `json:"response,omitempty"`
	Error          string        `json:"error,omitempty"`
	DurationSec    float64       `json:"duration_seconds,omitempty"`
	NumTurns       int           `json:"num_turns,omitempty"`
	Usage          *RawTokenUsage `json:"usage,omitempty"`
	FinishedReason string        `json:"finished_reason,omitempty"`
}

// RawTokenUsage reports token usage statistics from agy.
type RawTokenUsage struct {
	InputTokens      int `json:"input_tokens"`
	OutputTokens     int `json:"output_tokens"`
	ThinkingTokens   int `json:"thinking_tokens"`
	CacheReadTokens  int `json:"cache_read_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// EventNormalizer translates agy's stream-json output lines into platform NormalizedEvents.
type EventNormalizer struct {
	instanceID string
	sessionID  string
	backend    agents.BackendID
	now        func() time.Time

	currentTurnID string
	turnActive    bool
	activeCalls   map[int]string // stepIndex -> toolName
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
	}
}

// SetSessionID updates the session ID if it was resolved from agy's init event.
func (n *EventNormalizer) SetSessionID(sessionID string) {
	n.sessionID = sessionID
}

// SessionID returns the current session ID.
func (n *EventNormalizer) SessionID() string {
	return n.sessionID
}

// Normalize parses one raw JSON line from agy stdout and returns zero or more NormalizedEvents.
func (n *EventNormalizer) Normalize(line []byte) ([]agents.NormalizedEvent, error) {
	trimmed := strings.TrimSpace(string(line))
	if trimmed == "" {
		return nil, nil
	}

	var raw RawagyEvent
	if err := json.Unmarshal([]byte(trimmed), &raw); err != nil {
		// Emit unknown event retaining original body.
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
	// Initializing lifecycle event
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

	var events []agents.NormalizedEvent
	ts := n.now()

	// If a turn is not marked started yet, start it.
	if !n.turnActive && su.StepType != "user_input" {
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
			var inputRaw json.RawMessage
			if su.ToolInfo != nil && len(su.ToolInfo.Parameters) > 0 {
				inputRaw = su.ToolInfo.Parameters
			} else {
				inputRaw = json.RawMessage("{}")
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
					Input:  inputRaw,
				},
			})
		} else if su.State == "DONE" || su.State == "ERROR" {
			output := ""
			isErr := su.State == "ERROR"
			if su.ToolInfo != nil {
				if su.ToolInfo.Output != "" {
					output = su.ToolInfo.Output
				}
				if su.ToolInfo.Error != nil {
					isErr = true
					if output == "" {
						output = su.ToolInfo.Error.Message
					} else {
						output = fmt.Sprintf("%s\nError: %s", output, su.ToolInfo.Error.Message)
					}
				}
			}
			events = append(events, agents.NormalizedEvent{
				InstanceID: n.instanceID,
				SessionID:  n.sessionID,
				Backend:    n.backend,
				Timestamp:  ts,
				Type:       agents.EventToolResult,
				ToolResult: &agents.ToolResultPayload{
					CallID:  callID,
					Output:  output,
					IsError: isErr,
				},
			})
			delete(n.activeCalls, su.StepIndex)
		}

	case "user_input", "system_message":
		// Informational or internal to agy.
	}

	return events, nil
}

func (n *EventNormalizer) normalizeResult(raw *RawagyEvent) ([]agents.NormalizedEvent, error) {
	res := raw.Result
	var events []agents.NormalizedEvent
	ts := n.now()

	var usage *agents.MessageUsage
	if res != nil && res.Usage != nil {
		usage = &agents.MessageUsage{
			Input:     res.Usage.InputTokens,
			Output:    res.Usage.OutputTokens,
			Reasoning: res.Usage.ThinkingTokens,
			CacheRead: res.Usage.CacheReadTokens,
		}
	}

	// Emit final assistant message frame if we had an active turn
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
		},
	})

	// Conclude turn
	turnID := n.currentTurnID
	if turnID == "" {
		turnID = "turn-final"
	}

	outcome := agents.TurnCompleted
	if res != nil && res.Status == "ERROR" {
		outcome = agents.TurnFailed
		if res.Error != "" {
			events = append(events, agents.NormalizedEvent{
				InstanceID: n.instanceID,
				SessionID:  n.sessionID,
				Backend:    n.backend,
				Timestamp:  ts,
				Type:       agents.EventError,
				Error: &agents.ErrorPayload{
					Code:    "turn_error",
					Message: res.Error,
				},
			})
		}
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
	n.currentTurnID = ""
	return events, nil
}
