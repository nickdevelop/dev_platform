package agy

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"devplatform/internal/core/agents"
)

var userRequestTagRegex = regexp.MustCompile(`(?s)<USER_REQUEST>\s*(.*?)\s*</USER_REQUEST>`)

// TranscriptStep matches one JSON object in agy's transcript.jsonl.
type TranscriptStep struct {
	StepIndex        int                 `json:"step_index"`
	Source           string              `json:"source"`
	Type             string              `json:"type"`   // "USER_INPUT", "PLANNER_RESPONSE", "GENERIC"
	Status           string              `json:"status"` // "DONE", "ERROR"
	CreatedAt        string              `json:"created_at"`
	Content          string              `json:"content"`
	Error            string              `json:"error,omitempty"`
	InputTokens      int                 `json:"input_tokens,omitempty"`
	OutputTokens     int                 `json:"output_tokens,omitempty"`
	CacheReadTokens  int                 `json:"cache_read_tokens,omitempty"`
	ToolCalls        []TranscriptToolCall `json:"tool_calls,omitempty"`
}

// TranscriptToolCall matches a tool call recorded in PLANNER_RESPONSE.
type TranscriptToolCall struct {
	Name string          `json:"name"`
	Args json.RawMessage `json:"args"`
}

// ListHistory reads and normalizes the prior events of an agy session.
func (a *Adapter) ListHistory(ctx context.Context, request agents.ListHistoryRequest) (agents.HistoryResult, error) {
	if err := ctx.Err(); err != nil {
		return agents.HistoryResult{}, err
	}
	if err := request.Validate(); err != nil {
		return agents.HistoryResult{}, err
	}

	transcriptPath := a.resolveTranscriptPath(request.SessionID)
	events, err := ParseTranscriptFile(transcriptPath, "", request.SessionID)
	if err != nil {
		if os.IsNotExist(err) {
			return agents.HistoryResult{Events: []agents.NormalizedEvent{}}, nil
		}
		return agents.HistoryResult{}, fmt.Errorf("agy: read history for session %q: %w", request.SessionID, err)
	}

	return agents.PageHistory(events, request.Limit, request.Before)
}

func (a *Adapter) resolveTranscriptPath(sessionID string) string {
	baseDir := a.sessionDir
	if baseDir == "" {
		home, err := os.UserHomeDir()
		if err == nil {
			baseDir = filepath.Join(home, ".gemini", "antigravity-cli", "brain")
		}
	}
	return filepath.Join(baseDir, sessionID, ".system_generated", "logs", "transcript.jsonl")
}

// ParseTranscriptFile parses an agy transcript.jsonl into NormalizedEvents.
func ParseTranscriptFile(filePath, instanceID, sessionID string) ([]agents.NormalizedEvent, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var events []agents.NormalizedEvent
	scanner := bufio.NewScanner(file)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 4*1024*1024)

	var lastToolCallID string
	turnIndex := 0
	turnActive := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var step TranscriptStep
		if err := json.Unmarshal([]byte(line), &step); err != nil {
			continue
		}

		ts := time.Now().UTC()
		if step.CreatedAt != "" {
			if parsed, err := time.Parse(time.RFC3339, step.CreatedAt); err == nil {
				ts = parsed
			}
		}

		switch step.Type {
		case "USER_INPUT":
			if turnActive {
				// Close previous turn before starting a new one
				events = append(events, agents.NormalizedEvent{
					InstanceID: instanceID,
					SessionID:  sessionID,
					Backend:    agents.BackendAgy,
					Timestamp:  ts,
					Type:       agents.EventTurn,
					Turn: &agents.TurnPayload{
						ID:      fmt.Sprintf("turn-%d", turnIndex),
						Phase:   agents.TurnEnded,
						Outcome: agents.TurnCompleted,
					},
				})
				turnIndex++
			}

			// Open new turn
			turnActive = true
			turnID := fmt.Sprintf("turn-%d", turnIndex)
			events = append(events, agents.NormalizedEvent{
				InstanceID: instanceID,
				SessionID:  sessionID,
				Backend:    agents.BackendAgy,
				Timestamp:  ts,
				Type:       agents.EventTurn,
				Turn: &agents.TurnPayload{
					ID:    turnID,
					Phase: agents.TurnStarted,
				},
			})

			// Extract clean user text
			userText := step.Content
			if matches := userRequestTagRegex.FindStringSubmatch(userText); len(matches) > 1 {
				userText = matches[1]
			}

			events = append(events, agents.NormalizedEvent{
				InstanceID: instanceID,
				SessionID:  sessionID,
				Backend:    agents.BackendAgy,
				Timestamp:  ts,
				Type:       agents.EventMessage,
				Message: &agents.MessagePayload{
					Role:  "user",
					Text:  userText,
					Final: true,
				},
			})

		case "PLANNER_RESPONSE":
			// If tool calls exist
			if len(step.ToolCalls) > 0 {
				for i, tc := range step.ToolCalls {
					callID := fmt.Sprintf("call-%d-%d", step.StepIndex, i)
					lastToolCallID = callID
					events = append(events, agents.NormalizedEvent{
						InstanceID: instanceID,
						SessionID:  sessionID,
						Backend:    agents.BackendAgy,
						Timestamp:  ts,
						Type:       agents.EventToolCall,
						ToolCall: &agents.ToolCallPayload{
							CallID: callID,
							Name:   tc.Name,
							Input:  tc.Args,
						},
					})
				}
			}

			// If text content exists
			if step.Content != "" {
				var usage *agents.MessageUsage
				if step.InputTokens > 0 || step.OutputTokens > 0 {
					usage = &agents.MessageUsage{
						Input:  step.InputTokens,
						Output: step.OutputTokens,
					}
				}
				events = append(events, agents.NormalizedEvent{
					InstanceID: instanceID,
					SessionID:  sessionID,
					Backend:    agents.BackendAgy,
					Timestamp:  ts,
					Type:       agents.EventMessage,
					Message: &agents.MessagePayload{
						Role:  "assistant",
						Text:  step.Content,
						Final: true,
						Usage: usage,
					},
				})
			}

		case "GENERIC":
			// Tool result
			callID := lastToolCallID
			if callID == "" {
				callID = fmt.Sprintf("call-%d", step.StepIndex)
			}
			isError := step.Status == "ERROR"
			events = append(events, agents.NormalizedEvent{
				InstanceID: instanceID,
				SessionID:  sessionID,
				Backend:    agents.BackendAgy,
				Timestamp:  ts,
				Type:       agents.EventToolResult,
				ToolResult: &agents.ToolResultPayload{
					CallID:  callID,
					Output:  step.Content,
					IsError: isError,
				},
			})
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	// Conclude trailing turn if any
	if turnActive {
		events = append(events, agents.NormalizedEvent{
			InstanceID: instanceID,
			SessionID:  sessionID,
			Backend:    agents.BackendAgy,
			Timestamp:  time.Now().UTC(),
			Type:       agents.EventTurn,
			Turn: &agents.TurnPayload{
				ID:      fmt.Sprintf("turn-%d", turnIndex),
				Phase:   agents.TurnEnded,
				Outcome: agents.TurnCompleted,
			},
		})
	}

	return events, nil
}
