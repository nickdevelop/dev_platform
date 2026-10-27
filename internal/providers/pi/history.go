package pi

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"devplatform/internal/core/agents"
)

// Compile-time proof that the pi adapter can serve history reads.
var _ agents.HistoryReader = (*Adapter)(nil)

// maxHistoryRecordBytes bounds one JSONL record read for history. A tool
// result can be far larger than a session header, so the bound is generous
// compared with the discovery scan; a single record beyond it ends the read
// with the records already parsed, which is the same degradation as a
// half-written trailing line.
const maxHistoryRecordBytes = 8 * 1024 * 1024

// ListHistory reads a session's transcript from pi's own session file and
// translates it into the platform's normalized events.
//
// The translation mirrors the live stream: reasoning becomes its own event
// kind in both, so a replayed conversation looks the same as the one the user
// watched. Returned events leave InstanceID and SessionID empty — the request
// names an instance-free scope — and are marked Replayed.
func (a *Adapter) ListHistory(ctx context.Context, request agents.ListHistoryRequest) (agents.HistoryResult, error) {
	if err := ctx.Err(); err != nil {
		return agents.HistoryResult{}, err
	}
	if err := request.Validate(); err != nil {
		return agents.HistoryResult{}, err
	}
	if a.sessionDir == "" {
		return agents.HistoryResult{}, errors.New("pi: session directory is unavailable")
	}

	path, err := a.historyFile(request)
	if err != nil {
		return agents.HistoryResult{}, err
	}
	if path == "" {
		// An unknown session is nothing to replay, not a failure.
		return agents.HistoryResult{Events: []agents.NormalizedEvent{}}, nil
	}

	events, partial, err := a.readHistory(ctx, path)
	if err != nil {
		return agents.HistoryResult{}, err
	}
	result := boundHistory(events)
	// A read that stopped early dropped data just as a bounded read does, and
	// the caller must not present the remainder as the whole conversation.
	result.Truncated = result.Truncated || partial
	return result, nil
}

// historyFile returns the transcript path for the requested session, or an
// empty string when the directory holds no such transcript. It reuses the
// discovery header parse, so the working-directory check is the same one
// ListSessions applies.
func (a *Adapter) historyFile(request agents.ListHistoryRequest) (string, error) {
	dir := filepath.Join(a.sessionDir, sessionDirName(request.Directory))
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", nil
		}
		return "", fmt.Errorf("pi: read session directory %q: %w", dir, err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		ref, ok := readSessionRef(path, "", request.Directory)
		if ok && ref.SessionID == request.SessionID {
			return path, nil
		}
	}
	return "", nil
}

// historyRecord is one JSONL line, reduced to the fields history translation
// needs. Content is kept raw because pi writes it either as a JSON string or
// as a block array. Usage rides assistant messages (pi persists the
// provider-reported usage on them); standalone usage records are cost
// bookkeeping, not context, and are skipped.
type historyRecord struct {
	Type      string    `json:"type"`
	Timestamp time.Time `json:"timestamp"`
	Provider  string    `json:"provider"`
	ModelID   string    `json:"modelId"`
	Message   *struct {
		Role       string          `json:"role"`
		Content    json.RawMessage `json:"content"`
		ToolCallID string          `json:"toolCallId"`
		IsError    bool            `json:"isError"`
		Usage      *wireUsage      `json:"usage"`
	} `json:"message"`
}

// wireUsage mirrors pi's Usage: per-message provider-reported tokens with the
// prompt-cache split. Total is informational only; the platform sums the
// parts so a missing total never hides usage.
type wireUsage struct {
	Input      int `json:"input"`
	Output     int `json:"output"`
	Reasoning  int `json:"reasoning"`
	CacheRead  int `json:"cacheRead"`
	CacheWrite int `json:"cacheWrite"`
}

// usage converts wire usage to the platform shape. A zero total yields nil
// so placeholder records never become a misleading anchor.
func (u *wireUsage) usage() *agents.MessageUsage {
	if u == nil {
		return nil
	}
	usage := &agents.MessageUsage{
		Input:      u.Input,
		Output:     u.Output,
		Reasoning:  u.Reasoning,
		CacheRead:  u.CacheRead,
		CacheWrite: u.CacheWrite,
	}
	if usage.Total() == 0 {
		return nil
	}
	return usage
}

// historyBlock is one content block of a message. Reasoning blocks carry their
// text in pi's own "thinking" field, not in "text".
type historyBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	Thinking  string          `json:"thinking"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// readHistory streams the transcript and returns its events oldest first, plus
// whether the read stopped before the end of the file.
//
// A record that does not parse ends the read with what came before it. For the
// last line of a transcript still being written that is the right answer:
// nothing earlier is lost. Anywhere else it means the rest of the conversation
// is unreadable, so the read is reported as partial and the caller can say so.
func (a *Adapter) readHistory(ctx context.Context, path string) ([]agents.NormalizedEvent, bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, false, fmt.Errorf("pi: open transcript %q: %w", path, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), maxHistoryRecordBytes)

	events := make([]agents.NormalizedEvent, 0, 16)
	var currentModel *agents.MessageModel
	toolCalls := make(map[string]toolCallInfo)
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}
		var record historyRecord
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			// Scan consumes the next line to settle whether anything followed
			// the unreadable one; a clean end is not a partial read.
			more := scanner.Scan()
			return events, more || scanner.Err() != nil, nil
		}
		if record.Type == "model_change" && (record.Provider != "" || record.ModelID != "") {
			id := qualifyModelID(record.Provider, record.ModelID)
			currentModel = &agents.MessageModel{
				ID:       id,
				Provider: record.Provider,
			}
		}
		events = append(events, normalizeHistoryRecord(record, currentModel, toolCalls)...)
	}
	if err := scanner.Err(); err != nil {
		// An unreadable or oversized record ended the read before the end.
		return events, true, nil
	}
	return events, false, nil
}

// normalizeHistoryRecord translates one transcript record. A record can yield
// two events (an assistant message that both speaks and calls a tool) or none
// (the session header, model changes, system preamble, thinking).
func normalizeHistoryRecord(record historyRecord, model *agents.MessageModel, toolCalls map[string]toolCallInfo) []agents.NormalizedEvent {
	if record.Type != "message" || record.Message == nil {
		return nil
	}
	observed := record.Timestamp
	if observed.IsZero() {
		observed = time.Now().UTC()
	}
	envelope := agents.NormalizedEvent{
		Backend:   agents.BackendPi,
		Timestamp: observed,
		Replayed:  true,
	}

	switch record.Message.Role {
	case "user", "assistant":
		events := make([]agents.NormalizedEvent, 0, 2)
		usage := record.Message.Usage.usage()
		for _, block := range decodeHistoryContent(record.Message.Content) {
			switch block.Type {
			case "text":
				if block.Text == "" {
					continue
				}
				event := envelope
				event.Type = agents.EventMessage
				event.Message = &agents.MessagePayload{Role: record.Message.Role, Text: block.Text, Final: true}
				// The turn's usage belongs to the assistant message once: a
				// multi-block reply must not anchor it several times, and
				// user turns carry no usage of their own.
				if record.Message.Role == "assistant" {
					event.Message.Usage = usage
					usage = nil
					if model != nil {
						m := *model
						event.Message.Model = &m
					}
				}
				events = append(events, event)
			case "toolCall":
				if record.Message.Role != "assistant" {
					continue
				}
				if toolCalls != nil {
					toolCalls[block.ID] = toolCallInfo{name: block.Name, args: block.Arguments}
				}
				event := envelope
				event.Type = agents.EventToolCall
				event.ToolCall = &agents.ToolCallPayload{
					CallID: block.ID,
					Name:   block.Name,
					Input:  agents.PruneToolInput(block.Name, block.Arguments),
				}
				events = append(events, event)
			case "thinking":
				if record.Message.Role != "assistant" || block.Thinking == "" {
					continue
				}
				event := envelope
				event.Type = agents.EventThinking
				event.Thinking = &agents.ThinkingPayload{Text: block.Thinking, Final: true}
				events = append(events, event)
			}
		}
		return events
	case "toolResult":
		var output strings.Builder
		for _, block := range decodeHistoryContent(record.Message.Content) {
			if block.Type == "text" {
				output.WriteString(block.Text)
			}
		}
		event := envelope
		event.Type = agents.EventToolResult
		var name string
		var args json.RawMessage
		if toolCalls != nil {
			if info, ok := toolCalls[record.Message.ToolCallID]; ok {
				name = info.name
				args = info.args
			}
		}
		summary := agents.SummarizeToolResult(name, args, output.String(), record.Message.IsError)
		event.ToolResult = &agents.ToolResultPayload{
			CallID:  record.Message.ToolCallID,
			Output:  summary,
			IsError: record.Message.IsError,
		}
		return []agents.NormalizedEvent{event}
	default:
		// "system" and anything a future version adds: not conversation.
		return nil
	}
}

// decodeHistoryContent accepts both content shapes pi writes: a bare JSON
// string, or an array of typed blocks.
func decodeHistoryContent(raw json.RawMessage) []historyBlock {
	if len(raw) == 0 {
		return nil
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return []historyBlock{{Type: "text", Text: text}}
	}
	var blocks []historyBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return nil
	}
	return blocks
}

// boundHistory keeps the most recent MaxHistoryEvents and renumbers them from
// one, so an unbounded transcript is never materialized whole. It reports
// whether older events were dropped.
func boundHistory(events []agents.NormalizedEvent) agents.HistoryResult {
	truncated := len(events) > agents.MaxHistoryEvents
	if truncated {
		events = events[len(events)-agents.MaxHistoryEvents:]
	}
	for i := range events {
		events[i].Sequence = uint64(i + 1)
	}
	return agents.HistoryResult{Events: events, Truncated: truncated}
}
