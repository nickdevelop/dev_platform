package deepseek

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"devplatform/internal/core/agents"
)

// SessionEventNormalizer translates dsh session/follow frames and $events into platform NormalizedEvents.
type SessionEventNormalizer struct {
	instanceID           string
	sessionID            string
	currentTurnID        string
	currentContextWindow *int
	currentModel         *agents.MessageModel
	sequence             uint64
}

// NewSessionEventNormalizer creates a normalizer for a given instance and session.
func NewSessionEventNormalizer(instanceID, sessionID string) *SessionEventNormalizer {
	return &SessionEventNormalizer{
		instanceID: instanceID,
		sessionID:  sessionID,
	}
}

// SetCurrentModel configures the active model for subsequent events.
func (n *SessionEventNormalizer) SetCurrentModel(m *agents.MessageModel) {
	if m == nil {
		n.currentModel = nil
		return
	}
	copy := *m
	n.currentModel = &copy
}

// CurrentModel returns the currently recorded model, or nil when none is known.
func (n *SessionEventNormalizer) CurrentModel() *agents.MessageModel {
	if n.currentModel == nil {
		return nil
	}
	copy := *n.currentModel
	return &copy
}

// NormalizeFrame parses one incoming JSON frame from session/follow into zero or more NormalizedEvents.
func (n *SessionEventNormalizer) NormalizeFrame(data []byte) []agents.NormalizedEvent {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil
	}

	frameType, _ := raw["type"].(string)
	now := time.Now().UTC()

	var events []agents.NormalizedEvent

	switch frameType {
	case "snapshot":
		// Handle opening snapshot projections: contextPressure and tokenUsage
		if projMap, ok := raw["projections"].(map[string]any); ok {
			values, _ := projMap["values"].(map[string]any)
			if values != nil {
				if cp, ok := values["contextPressure"].(map[string]any); ok {
					if cw := intValue(cp["contextWindow"]); cw > 0 {
						n.currentContextWindow = &cw
					}
				}
				var usage *agents.MessageUsage
				if tu, ok := values["tokenUsage"].(map[string]any); ok {
					u := &agents.MessageUsage{
						Input:      intValue(tu["uncachedInputTokens"]),
						Output:     intValue(tu["outputTokens"]),
						CacheRead:  intValue(tu["cacheReadTokens"]),
						CacheWrite: intValue(tu["cacheWriteTokens"]),
					}
					if u.Total() > 0 {
						usage = u
					}
				} else if cp, ok := values["contextPressure"].(map[string]any); ok {
					pt := intValue(cp["pressureTokens"])
					if pt > 0 {
						usage = &agents.MessageUsage{Input: pt}
					}
				}
				if n.currentContextWindow != nil || usage != nil {
					events = append(events, n.newEvent(now, agents.EventLifecycle, &agents.NormalizedEvent{
						Lifecycle: &agents.LifecycleEvent{
							State:        "context_anchor",
							Usage:        usage,
							ContextLimit: n.currentContextWindow,
						},
					}))
				}
			}
		}

	case "assistant-stream":
		// Handle streaming text chunks and reasoning deltas
		frame, ok := raw["frame"].(map[string]any)
		if !ok {
			return nil
		}
		fType, _ := frame["type"].(string)
		if fType == "chunk" {
			chunk, ok := frame["chunk"].(map[string]any)
			if !ok {
				return nil
			}
			chunkType, _ := chunk["type"].(string)
			text, _ := chunk["text"].(string)

			if chunkType == "text-delta" && text != "" {
				events = append(events, n.newEvent(now, agents.EventMessage, &agents.NormalizedEvent{
					Message: &agents.MessagePayload{
						Role:  "assistant",
						Text:  text,
						Final: false,
						Model: n.CurrentModel(),
					},
				}))
			} else if chunkType == "reasoning-delta" && text != "" {
				events = append(events, n.newEvent(now, agents.EventThinking, &agents.NormalizedEvent{
					Thinking: &agents.ThinkingPayload{
						Text:  text,
						Final: false,
					},
				}))
			}
		}

	case "event":
		// Handle structured session log events
		ev, ok := raw["event"].(map[string]any)
		if !ok {
			return nil
		}
		evType, _ := ev["type"].(string)
		evData, _ := ev["data"].(map[string]any)
		if evData == nil {
			evData = map[string]any{}
		}

		switch evType {
		case "turn/start":
			turnNum := 0
			if tn, ok := evData["turn"].(float64); ok {
				turnNum = int(tn)
			}
			n.currentTurnID = fmt.Sprintf("turn-%d", turnNum)
			events = append(events, n.newEvent(now, agents.EventTurn, &agents.NormalizedEvent{
				Turn: &agents.TurnPayload{
					ID:    n.currentTurnID,
					Phase: agents.TurnStarted,
				},
			}))

		case "user/message":
			// Only real user input (source.kind == "user") is admitted as a chat message.
			// System-injected contexts (e.g. source.kind == "runtime-context", "skill-catalog",
			// "time-context") are LLM prompt contexts and must not appear in the visible chat transcript.
			if src, ok := evData["source"].(map[string]any); ok {
				if kind, _ := src["kind"].(string); kind != "" && kind != "user" {
					break
				}
			}
			content := extractText(evData["content"])
			if content != "" {
				events = append(events, n.newEvent(now, agents.EventMessage, &agents.NormalizedEvent{
					Message: &agents.MessagePayload{
						Role:  "user",
						Text:  content,
						Final: true,
					},
				}))
			}

		case "request/context":
			if cw := intValue(evData["contextWindow"]); cw > 0 {
				n.currentContextWindow = &cw
				events = append(events, n.newEvent(now, agents.EventLifecycle, &agents.NormalizedEvent{
					Lifecycle: &agents.LifecycleEvent{
						State:        "context_window",
						ContextLimit: &cw,
					},
				}))
			}
			model, _ := evData["model"].(string)
			provider, _ := evData["provider"].(string)
			variant, _ := evData["variant"].(string)
			if variant == "" {
				variant, _ = evData["reasoningEffort"].(string)
			}
			if model != "" {
				modelID := model
				if provider != "" && !strings.HasPrefix(modelID, provider+"/") {
					modelID = provider + "/" + model
				}
				n.currentModel = &agents.MessageModel{
					ID:       modelID,
					Provider: provider,
					Variant:  variant,
				}
			}

		case "assistant/message", "assistant/attempt":
			// 1. Token usage report
			var usage *agents.MessageUsage
			if usageMap, ok := evData["usage"].(map[string]any); ok {
				inputTokens := intValue(usageMap["inputTokens"])
				outputTokens := intValue(usageMap["outputTokens"])
				cacheRead := intValue(usageMap["cacheReadTokens"])
				cacheWrite := intValue(usageMap["cacheWriteTokens"])
				reasoningTokens := intValue(usageMap["reasoningTokens"])

				usage = &agents.MessageUsage{
					Input:      inputTokens,
					Output:     outputTokens,
					Reasoning:  reasoningTokens,
					CacheRead:  cacheRead,
					CacheWrite: cacheWrite,
				}
			}

			msgModel := n.CurrentModel()
			if m, ok := evData["model"].(string); ok && m != "" {
				p, _ := evData["provider"].(string)
				mID := m
				if p != "" && !strings.HasPrefix(mID, p+"/") {
					mID = p + "/" + m
				}
				v, _ := evData["variant"].(string)
				if v == "" {
					v, _ = evData["reasoningEffort"].(string)
				}
				msgModel = &agents.MessageModel{
					ID:       mID,
					Provider: p,
					Variant:  v,
				}
			}

			// 2. Content blocks (reasoning and text)
			if msg, ok := evData["message"].(map[string]any); ok && msg != nil {
				if contentArr, ok := msg["content"].([]any); ok {
					for _, block := range contentArr {
						if bMap, ok := block.(map[string]any); ok {
							bType, _ := bMap["type"].(string)
							bText, _ := bMap["text"].(string)
							if bType == "reasoning" && bText != "" {
								events = append(events, n.newEvent(now, agents.EventThinking, &agents.NormalizedEvent{
									Thinking: &agents.ThinkingPayload{
										Text:  bText,
										Final: true,
									},
								}))
							} else if (bType == "text" || bType == "") && bText != "" {
								events = append(events, n.newEvent(now, agents.EventMessage, &agents.NormalizedEvent{
									Message: &agents.MessagePayload{
										Role:  "assistant",
										Text:  bText,
										Final: true,
										Usage: usage,
										Model: msgModel,
									},
								}))
							}
						}
					}
				} else if strContent, ok := msg["content"].(string); ok && strContent != "" {
					events = append(events, n.newEvent(now, agents.EventMessage, &agents.NormalizedEvent{
						Message: &agents.MessagePayload{
							Role:  "assistant",
							Text:  strContent,
							Final: true,
							Usage: usage,
							Model: msgModel,
						},
					}))
				}
			}

			if usage != nil {
				events = append(events, n.newEvent(now, agents.EventLifecycle, &agents.NormalizedEvent{
					Lifecycle: &agents.LifecycleEvent{
						State:        "step_finished",
						Usage:        usage,
						ContextLimit: n.currentContextWindow,
					},
				}))
			}

		case "tool/call":
			callID, _ := evData["callId"].(string)
			name, _ := evData["name"].(string)
			inputBytes, _ := json.Marshal(evData["arguments"])
			if len(inputBytes) > agents.MaxRawEventBytes {
				inputBytes = inputBytes[:agents.MaxRawEventBytes]
			}

			events = append(events, n.newEvent(now, agents.EventToolCall, &agents.NormalizedEvent{
				ToolCall: &agents.ToolCallPayload{
					CallID: callID,
					Name:   name,
					Input:  inputBytes,
				},
			}))

		case "tool/result":
			msg, _ := evData["message"].(map[string]any)
			if msg != nil {
				toolCallID, _ := msg["toolCallId"].(string)
				isErr, _ := msg["isError"].(bool)
				content := extractText(msg["content"])

				events = append(events, n.newEvent(now, agents.EventToolResult, &agents.NormalizedEvent{
					ToolResult: &agents.ToolResultPayload{
						CallID:  toolCallID,
						Output:  content,
						IsError: isErr,
					},
				}))
			}

		case "tool/ptc-dispatch-start":
			subCallID, _ := evData["subCallId"].(string)
			name, _ := evData["name"].(string)
			inputBytes, _ := json.Marshal(evData["arguments"])
			events = append(events, n.newEvent(now, agents.EventToolCall, &agents.NormalizedEvent{
				ToolCall: &agents.ToolCallPayload{
					CallID: subCallID,
					Name:   name,
					Input:  inputBytes,
				},
			}))

		case "tool/ptc-dispatch":
			subCallID, _ := evData["subCallId"].(string)
			isErr, _ := evData["isError"].(bool)
			content := extractText(evData["content"])
			events = append(events, n.newEvent(now, agents.EventToolResult, &agents.NormalizedEvent{
				ToolResult: &agents.ToolResultPayload{
					CallID:  subCallID,
					Output:  content,
					IsError: isErr,
				},
			}))

		case "turn/end":
			turnID := n.currentTurnID
			if turnID == "" {
				turnID = "turn-0"
			}

			outcome := agents.TurnCompleted
			var failureMessage string

			if reason, ok := evData["reason"].(map[string]any); ok {
				kind, _ := reason["kind"].(string)
				switch kind {
				case "aborted", "cancelled":
					outcome = agents.TurnCancelled
				case "error", "blocked":
					outcome = agents.TurnFailed
					if errObj, ok := reason["error"].(map[string]any); ok {
						failureMessage, _ = errObj["message"].(string)
					}
					if failureMessage == "" {
						failureMessage = "DeepSeek turn ended with error"
					}
				}
			}

			events = append(events, n.newEvent(now, agents.EventTurn, &agents.NormalizedEvent{
				Turn: &agents.TurnPayload{
					ID:      turnID,
					Phase:   agents.TurnEnded,
					Outcome: outcome,
				},
			}))

			if outcome == agents.TurnFailed {
				events = append(events, n.newEvent(now, agents.EventError, &agents.NormalizedEvent{
					Error: &agents.ErrorPayload{
						Message: failureMessage,
					},
				}))
			}

			n.currentTurnID = ""
		}
	}

	return events
}

func (n *SessionEventNormalizer) newEvent(ts time.Time, t agents.EventType, tmpl *agents.NormalizedEvent) agents.NormalizedEvent {
	n.sequence++
	tmpl.Backend = agents.BackendDeepSeek
	tmpl.InstanceID = n.instanceID
	tmpl.SessionID = n.sessionID
	tmpl.Sequence = n.sequence
	tmpl.Timestamp = ts
	tmpl.Type = t
	return *tmpl
}

func extractText(content any) string {
	if s, ok := content.(string); ok {
		return s
	}
	if arr, ok := content.([]any); ok {
		var parts []string
		for _, item := range arr {
			if m, ok := item.(map[string]any); ok {
				if t, ok := m["text"].(string); ok {
					parts = append(parts, t)
				}
			}
		}
		return strings.Join(parts, "\n")
	}
	return ""
}

func intValue(v any) int {
	if f, ok := v.(float64); ok {
		return int(f)
	}
	if i, ok := v.(int); ok {
		return i
	}
	return 0
}

// NormalizeProjection converts a projection update from session/control into zero or more NormalizedEvents.
func (n *SessionEventNormalizer) NormalizeProjection(key string, val any) []agents.NormalizedEvent {
	if key != "contextPressure" {
		return nil
	}
	cp, ok := val.(map[string]any)
	if !ok || cp == nil {
		return nil
	}
	now := time.Now().UTC()
	var events []agents.NormalizedEvent
	if cw := intValue(cp["contextWindow"]); cw > 0 {
		n.currentContextWindow = &cw
	}
	pt := intValue(cp["pressureTokens"])
	var usage *agents.MessageUsage
	if pt > 0 {
		usage = &agents.MessageUsage{Input: pt}
	}
	if n.currentContextWindow != nil || usage != nil {
		events = append(events, n.newEvent(now, agents.EventLifecycle, &agents.NormalizedEvent{
			Lifecycle: &agents.LifecycleEvent{
				State:        "context_pressure",
				Usage:        usage,
				ContextLimit: n.currentContextWindow,
			},
		}))
	}
	return events
}
