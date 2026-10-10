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

	"devplatform/internal/agents"
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
// The translation mirrors the live stream on purpose: assistant thinking is
// dropped because the live parser drops it, so a replayed conversation looks
// the same as the one the user watched. Returned events leave InstanceID and
// SessionID empty — the request names an instance-free scope — and are marked
// Replayed.
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

	events, err := a.readHistory(ctx, path)
	if err != nil {
		return agents.HistoryResult{}, err
	}
	return boundHistory(events), nil
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
// as a block array.
type historyRecord struct {
	Type      string    `json:"type"`
	Timestamp time.Time `json:"timestamp"`
	Message   *struct {
		Role       string          `json:"role"`
		Content    json.RawMessage `json:"content"`
		ToolCallID string          `json:"toolCallId"`
		IsError    bool            `json:"isError"`
	} `json:"message"`
}

// historyBlock is one content block of a message. Thinking blocks carry a
// "thinking" field that is deliberately not decoded: they translate to
// nothing.
type historyBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// readHistory streams the transcript and returns its events oldest first. A
// record that does not parse ends the read with what came before it: the last
// line of a transcript being written is routinely incomplete, and everything
// already read is still valid history.
func (a *Adapter) readHistory(ctx context.Context, path string) ([]agents.NormalizedEvent, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("pi: open transcript %q: %w", path, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), maxHistoryRecordBytes)

	events := make([]agents.NormalizedEvent, 0, 16)
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var record historyRecord
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			break
		}
		events = append(events, normalizeHistoryRecord(record)...)
	}
	return events, nil
}

// normalizeHistoryRecord translates one transcript record. A record can yield
// two events (an assistant message that both speaks and calls a tool) or none
// (the session header, model changes, system preamble, thinking).
func normalizeHistoryRecord(record historyRecord) []agents.NormalizedEvent {
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
		for _, block := range decodeHistoryContent(record.Message.Content) {
			switch block.Type {
			case "text":
				if block.Text == "" {
					continue
				}
				event := envelope
				event.Type = agents.EventMessage
				event.Message = &agents.MessagePayload{Role: record.Message.Role, Text: block.Text, Final: true}
				events = append(events, event)
			case "toolCall":
				if record.Message.Role != "assistant" {
					continue
				}
				event := envelope
				event.Type = agents.EventToolCall
				event.ToolCall = &agents.ToolCallPayload{CallID: block.ID, Name: block.Name, Input: block.Arguments}
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
		event.ToolResult = &agents.ToolResultPayload{
			CallID:  record.Message.ToolCallID,
			Output:  output.String(),
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
