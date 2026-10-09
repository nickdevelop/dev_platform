// Package pi adapts the pi coding-agent CLI to the platform's AgentAdapter
// contract. pi is driven as a controlled child process per prompt: the adapter
// launches `pi --mode json` with exec.CommandContext (never a shell), reads the
// strict JSONL event stream from stdout, normalizes it, and fans it out to
// per-instance subscribers. Session storage stays inside pi; the adapter only
// reads pi's session files to answer discovery queries.
package pi

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"devplatform/internal/agents"
)

// maxLineBytes bounds one JSONL record on the pi protocol stream. pi's own
// docs reserve stdout for protocol records and stderr for diagnostics; a record
// beyond this bound is reported and discarded rather than buffered.
const maxLineBytes = agents.MaxRawEventBytes

// maxStderrBytes bounds how much application logging on stderr is turned into
// diagnostic events for one process. The reader keeps draining after the bound
// so pi never blocks on a full pipe, but stops emitting.
const maxStderrBytes = 64 * 1024

// parser turns pi's JSON event stream into normalized events. It is bound to
// one running prompt: the instance and session are known to the adapter before
// the process starts, so every emitted event can carry a complete envelope
// even though pi's records do not name the session themselves.
//
// The parser targets pi's `--mode json` protocol, documented as one session
// header followed by session events (agent_start, turn_start, message_*,
// tool_execution_*, compaction/retry events) terminated per invocation. It is
// stateless across records except for the session id learned from the header
// and whether the current assistant message already streamed text deltas.
type parser struct {
	instanceID string
	sessionID  string
	now        func() time.Time

	lineNumber uint64
	deltaOpen  bool
}

// newParser builds a parser for one prompt process. now stamps event times; a
// nil clock falls back to time.Now.
func newParser(instanceID, sessionID string, now func() time.Time) *parser {
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &parser{instanceID: instanceID, sessionID: sessionID, now: now}
}

// run reads stdout to EOF, emitting one or more normalized events per record.
// I/O errors are returned; protocol problems become EventError events so a
// single bad record cannot tear down the stream.
func (p *parser) run(r io.Reader, emit func(agents.NormalizedEvent)) error {
	return scanLines(r, maxLineBytes,
		func(line []byte) { p.parseLine(line, emit) },
		func() {
			emit(p.errorEvent("line_too_large",
				fmt.Sprintf("pi record exceeds the %d byte limit", maxLineBytes)))
		},
	)
}

// runStderr turns pi's application logging into bounded diagnostic events.
// stderr is never parsed as protocol data; each line is surfaced as an
// EventError with the `stderr` code, redacted and size-limited.
func (p *parser) runStderr(r io.Reader, emit func(agents.NormalizedEvent)) error {
	var written int
	return scanLines(r, maxLineBytes,
		func(line []byte) {
			if written >= maxStderrBytes {
				return
			}
			written += len(line)
			emit(p.stderrEvent(line))
		},
		func() {
			if written >= maxStderrBytes {
				return
			}
			written = maxStderrBytes
			emit(p.errorEvent("line_too_large", "pi stderr record exceeds the size limit"))
		},
	)
}

// parseLine normalizes one JSONL record. Blank lines are ignored; malformed
// JSON becomes a parse error carrying a bounded copy of the offending bytes.
func (p *parser) parseLine(line []byte, emit func(agents.NormalizedEvent)) {
	record := bytes.TrimRight(line, "\r\n")
	if len(bytes.TrimSpace(record)) == 0 {
		return
	}
	p.lineNumber++

	var event piEvent
	if err := json.Unmarshal(record, &event); err != nil {
		out := p.errorEvent("parse_error",
			fmt.Sprintf("malformed pi JSON record on line %s", strconv.FormatUint(p.lineNumber, 10)))
		out.Raw = append(json.RawMessage(nil), record...)
		emit(out)
		return
	}
	for _, normalized := range p.normalize(&event, record) {
		emit(normalized)
	}
}

// piEvent is the union of the pi JSON-mode record shapes the adapter inspects.
// Unknown fields are ignored by encoding/json.
type piEvent struct {
	Type string `json:"type"`

	// Session header.
	ID  string `json:"id"`
	Cwd string `json:"cwd"`

	// Message records.
	Message               json.RawMessage   `json:"message"`
	AssistantMessageEvent *piAssistantEvent `json:"assistantMessageEvent"`

	// Tool execution records.
	ToolCallID    string          `json:"toolCallId"`
	ToolName      string          `json:"toolName"`
	Args          json.RawMessage `json:"args"`
	Result        json.RawMessage `json:"result"`
	PartialResult json.RawMessage `json:"partialResult"`
	IsError       bool            `json:"isError"`

	// Error records.
	Error        string `json:"error"`
	ErrorMessage string `json:"errorMessage"`
}

// piAssistantEvent is the nested provider event carried by message_update.
type piAssistantEvent struct {
	Type   string `json:"type"`
	Delta  string `json:"delta"`
	Reason string `json:"reason"`
	Error  string `json:"error"`
}

// piMessage is the subset of a pi message the adapter needs: its role and its
// content (a string for user/system messages, an array of blocks otherwise).
type piMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

// normalize maps one parsed pi record onto the normalized event union. Records
// the adapter does not model are preserved as EventUnknown (with their raw
// bytes) rather than dropped; a curated set of agent/turn lifecycle records
// becomes EventLifecycle; embedded provider sub-events the union cannot express
// are preserved as unknown alongside the typed events that pi already emits.
func (p *parser) normalize(event *piEvent, raw []byte) []agents.NormalizedEvent {
	switch event.Type {
	case "session":
		if event.ID != "" {
			p.sessionID = event.ID
		}
		out := p.event()
		out.Type = agents.EventLifecycle
		out.Lifecycle = &agents.LifecycleEvent{State: "session_started", Detail: event.Cwd}
		return []agents.NormalizedEvent{out}

	case "message_start":
		role, text := messageParts(event.Message)
		if role == "assistant" {
			p.deltaOpen = false
			return nil
		}
		if text == "" {
			return nil
		}
		out := p.event()
		out.Type = agents.EventMessage
		out.Message = &agents.MessagePayload{Role: role, Text: text, Final: true}
		return []agents.NormalizedEvent{out}

	case "message_update":
		update := event.AssistantMessageEvent
		if update != nil && update.Type == "text_delta" {
			p.deltaOpen = true
			out := p.event()
			out.Type = agents.EventMessage
			out.Message = &agents.MessagePayload{Role: "assistant", Text: update.Delta, Final: false}
			return []agents.NormalizedEvent{out}
		}
		if update != nil && update.Type == "error" {
			return []agents.NormalizedEvent{p.errorEvent("provider", redactSecrets(firstNonEmpty(update.Error, update.Reason)))}
		}
		// thinking_*, toolcall_* snapshots, start and done are either
		// redundant with the typed tool_execution_* records or have no
		// normalized payload; keep them as bounded raw metadata.
		return []agents.NormalizedEvent{p.unknown(raw)}

	case "message_end":
		role, text := messageParts(event.Message)
		if role != "assistant" {
			// User and system messages were emitted complete at
			// message_start; emitting message_end too would duplicate them.
			return nil
		}
		streamed := p.deltaOpen
		p.deltaOpen = false
		out := p.event()
		out.Type = agents.EventMessage
		out.Message = &agents.MessagePayload{Role: "assistant", Text: text, Final: true}
		if streamed {
			// Deltas already carried the text; close the message with an
			// empty terminator so a concatenating consumer does not double it.
			out.Message.Text = ""
		}
		return []agents.NormalizedEvent{out}

	case "tool_execution_start":
		out := p.event()
		out.Type = agents.EventToolCall
		out.ToolCall = &agents.ToolCallPayload{CallID: event.ToolCallID, Name: event.ToolName, Input: event.Args}
		return []agents.NormalizedEvent{out}

	case "tool_execution_update":
		out := p.event()
		out.Type = agents.EventToolResult
		out.ToolResult = &agents.ToolResultPayload{CallID: event.ToolCallID, Output: resultText(event.PartialResult), IsError: false}
		return []agents.NormalizedEvent{out}

	case "tool_execution_end":
		out := p.event()
		out.Type = agents.EventToolResult
		out.ToolResult = &agents.ToolResultPayload{CallID: event.ToolCallID, Output: resultText(event.Result), IsError: event.IsError}
		return []agents.NormalizedEvent{out}

	case "error":
		return []agents.NormalizedEvent{p.errorEvent("backend", redactSecrets(firstNonEmpty(event.Error, event.ErrorMessage)))}

	default:
		if isLifecycleType(event.Type) {
			out := p.event()
			out.Type = agents.EventLifecycle
			out.Lifecycle = &agents.LifecycleEvent{State: event.Type}
			out.Raw = append(json.RawMessage(nil), raw...)
			return []agents.NormalizedEvent{out}
		}
		return []agents.NormalizedEvent{p.unknown(raw)}
	}
}

// lifecycleTypes are the pi records that describe agent/turn state directly.
var lifecycleTypes = map[string]bool{
	"agent_start":   true,
	"agent_end":     true,
	"agent_settled": true,
	"turn_start":    true,
	"turn_end":      true,
}

func isLifecycleType(name string) bool { return lifecycleTypes[name] }

// event returns an envelope populated with the adapter-known identity. The
// sequence is assigned later, centrally, when the event is published.
func (p *parser) event() agents.NormalizedEvent {
	return agents.NormalizedEvent{
		InstanceID: p.instanceID,
		SessionID:  p.sessionID,
		Backend:    agents.BackendPi,
		Timestamp:  p.now(),
	}
}

// errorEvent builds a typed error event on the current envelope.
func (p *parser) errorEvent(code, message string) agents.NormalizedEvent {
	out := p.event()
	out.Type = agents.EventError
	out.Error = &agents.ErrorPayload{Code: code, Message: truncate(message)}
	return out
}

// stderrEvent builds a redacted diagnostic event for one stderr line.
func (p *parser) stderrEvent(line []byte) agents.NormalizedEvent {
	return p.errorEvent("stderr", redactSecrets(strings.TrimRight(string(line), "\r\n")))
}

// unknown preserves a record the adapter could not normalize, byte for byte.
func (p *parser) unknown(raw []byte) agents.NormalizedEvent {
	out := p.event()
	out.Type = agents.EventUnknown
	out.Raw = append(json.RawMessage(nil), raw...)
	return out
}

// messageParts extracts a message's role and display text. User and system
// content is a string; assistant content is an array of blocks.
func messageParts(raw json.RawMessage) (role, text string) {
	if len(raw) == 0 {
		return "", ""
	}
	var msg piMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		return "", ""
	}
	return msg.Role, contentText(msg.Content)
}

// contentText flattens a pi message content value into display text.
func contentText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &blocks); err == nil {
		parts := make([]string, 0, len(blocks))
		for _, block := range blocks {
			if block.Text != "" {
				parts = append(parts, block.Text)
			}
		}
		return strings.Join(parts, "\n")
	}
	return ""
}

// resultText flattens a tool result or partial result into display text.
func resultText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var envelope struct {
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(raw, &envelope); err == nil && len(envelope.Content) > 0 {
		if text := contentText(envelope.Content); text != "" {
			return text
		}
	}
	return truncate(string(raw))
}

// scanLines reads LF-delimited records from r. Unlike bufio.Scanner it never
// fails on a long record: an over-bound line is reported through onOverflow and
// its remainder discarded, so a chatty backend cannot wedge the reader or
// exhaust memory. A final record without a newline is delivered before EOF.
func scanLines(r io.Reader, maxLine int, onLine func(line []byte), onOverflow func()) error {
	reader := bufio.NewReaderSize(r, maxLine)
	for {
		line, err := reader.ReadSlice('\n')
		if err == bufio.ErrBufferFull {
			for err == bufio.ErrBufferFull {
				_, err = reader.ReadSlice('\n')
			}
			if onOverflow != nil {
				onOverflow()
			}
			if err == io.EOF {
				return nil
			}
			if err != nil {
				return err
			}
			continue
		}
		if err == io.EOF {
			if len(line) > 0 {
				onLine(line)
			}
			return nil
		}
		if err != nil {
			return err
		}
		onLine(line)
	}
}

// firstNonEmpty returns the first non-empty string.
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

// truncate bounds a string to maxStderrBytes so diagnostics cannot grow the
// event unboundedly.
func truncate(value string) string {
	if len(value) <= maxStderrBytes {
		return value
	}
	return value[:maxStderrBytes] + "…(truncated)"
}
