package agy

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"devplatform/internal/core/agents"
)

var (
	userRequestTagRegex = regexp.MustCompile(`(?s)<USER_REQUEST>\s*(.*?)\s*</USER_REQUEST>`)
	modelSettingRegex   = regexp.MustCompile("`Model Selection` from [^ ]+ to ([^.\n\r]+)")
)

// TranscriptStep matches one JSON object in agy's transcript.jsonl.
type TranscriptStep struct {
	StepIndex        int                  `json:"step_index"`
	Source           string               `json:"source"`
	Type             string               `json:"type"`   // "USER_INPUT", "PLANNER_RESPONSE", "GENERIC"
	Status           string               `json:"status"` // "DONE", "ERROR"
	CreatedAt        string               `json:"created_at"`
	Content          string               `json:"content"`
	Model            string               `json:"model,omitempty"`
	Error            string               `json:"error,omitempty"`
	InputTokens      int                  `json:"input_tokens,omitempty"`
	OutputTokens     int                  `json:"output_tokens,omitempty"`
	CacheReadTokens  int                  `json:"cache_read_tokens,omitempty"`
	ToolCalls        []TranscriptToolCall `json:"tool_calls,omitempty"`
	TruncatedFields  []string             `json:"truncated_fields,omitempty"`
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

// fullTranscriptReader provides cursor-based on-demand retrieval of full un-truncated steps
// from transcript_full.jsonl to avoid loading massive transcripts into memory upfront.
type fullTranscriptReader struct {
	file    *os.File
	scanner *bufio.Scanner
}

func newFullTranscriptReader(fullPath string) (*fullTranscriptReader, error) {
	f, err := os.Open(fullPath)
	if err != nil {
		return nil, err
	}
	s := bufio.NewScanner(f)
	buf := make([]byte, 0, 64*1024)
	s.Buffer(buf, 16*1024*1024)
	return &fullTranscriptReader{
		file:    f,
		scanner: s,
	}, nil
}

func (r *fullTranscriptReader) Close() error {
	if r.file != nil {
		return r.file.Close()
	}
	return nil
}

// readStepByIndex advances the reader until finding the line with step_index == targetStepIndex.
func (r *fullTranscriptReader) readStepByIndex(targetStepIndex int) (*TranscriptStep, error) {
	for r.scanner.Scan() {
		line := strings.TrimSpace(r.scanner.Text())
		if line == "" {
			continue
		}
		var s TranscriptStep
		if err := json.Unmarshal([]byte(line), &s); err != nil {
			continue
		}
		if s.StepIndex == targetStepIndex {
			return &s, nil
		}
		if s.StepIndex > targetStepIndex {
			// StepIndex surpassed target, rewind to check from beginning
			if _, err := r.file.Seek(0, io.SeekStart); err != nil {
				return nil, err
			}
			r.scanner = bufio.NewScanner(r.file)
			buf := make([]byte, 0, 64*1024)
			r.scanner.Buffer(buf, 16*1024*1024)
			for r.scanner.Scan() {
				rewindLine := strings.TrimSpace(r.scanner.Text())
				if rewindLine == "" {
					continue
				}
				var rs TranscriptStep
				if err := json.Unmarshal([]byte(rewindLine), &rs); err == nil && rs.StepIndex == targetStepIndex {
					return &rs, nil
				}
			}
			return nil, io.EOF
		}
	}
	if err := r.scanner.Err(); err != nil {
		return nil, err
	}
	return nil, io.EOF
}

// ParseTranscriptFile parses an agy transcript.jsonl into NormalizedEvents, on-demand
// hydrating truncated fields from transcript_full.jsonl when available.
func ParseTranscriptFile(filePath, instanceID, sessionID string) ([]agents.NormalizedEvent, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// A step without a usable created_at is stamped with the transcript's own
	// modification time, never with the time it was read: the workbench sorts
	// sessions by last interaction, so replaying a turn must not look like a new
	// one. The same anchor closes a trailing turn.
	fallbackTime := time.Now().UTC()
	if info, statErr := file.Stat(); statErr == nil {
		fallbackTime = info.ModTime().UTC()
	}

	var events []agents.NormalizedEvent
	scanner := bufio.NewScanner(file)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 4*1024*1024)

	var lastToolCallID string
	var currentModel string
	var lastStepTime time.Time
	turnIndex := 0
	turnActive := false

	var fullReader *fullTranscriptReader
	var fullReaderAttempted bool
	fullFilePath := filepath.Join(filepath.Dir(filePath), "transcript_full.jsonl")

	defer func() {
		if fullReader != nil {
			_ = fullReader.Close()
		}
	}()

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var step TranscriptStep
		if err := json.Unmarshal([]byte(line), &step); err != nil {
			continue
		}

		// On-demand hydrate truncated fields from transcript_full.jsonl
		if len(step.TruncatedFields) > 0 {
			if !fullReaderAttempted {
				fullReaderAttempted = true
				if reader, err := newFullTranscriptReader(fullFilePath); err == nil {
					fullReader = reader
				}
			}
			if fullReader != nil {
				if fullStep, err := fullReader.readStepByIndex(step.StepIndex); err == nil && fullStep != nil {
					for _, field := range step.TruncatedFields {
						switch field {
						case "content":
							step.Content = fullStep.Content
						case "tool_calls":
							if len(fullStep.ToolCalls) > 0 {
								step.ToolCalls = fullStep.ToolCalls
							}
						}
					}
				}
			}
		}

		ts := lastStepTime
		if step.CreatedAt != "" {
			if parsed, err := time.Parse(time.RFC3339, step.CreatedAt); err == nil {
				ts = parsed
			}
		}
		if ts.IsZero() {
			ts = fallbackTime
		}
		lastStepTime = ts

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
			if m := modelSettingRegex.FindStringSubmatch(userText); len(m) > 1 {
				currentModel = strings.TrimSpace(m[1])
			}
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
				var msgModel *agents.MessageModel
				if step.Model != "" {
					msgModel = &agents.MessageModel{ID: step.Model, Provider: string(agents.BackendAgy)}
				} else if currentModel != "" {
					msgModel = &agents.MessageModel{ID: currentModel, Provider: string(agents.BackendAgy)}
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
						Model: msgModel,
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
			Timestamp:  lastStepTime,
			Type:       agents.EventTurn,
			Turn: &agents.TurnPayload{
				ID:      fmt.Sprintf("turn-%d", turnIndex),
				Phase:   agents.TurnEnded,
				Outcome: agents.TurnCompleted,
			},
		})
	}

	return agents.NewDeliveryGate().AdmitAll(events), nil
}
