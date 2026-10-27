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
	"sync"
	"time"
	"unicode/utf8"

	"devplatform/internal/core/agents"
)

// maxLineBytes bounds one JSONL record on the pi protocol stream. pi's own
// docs reserve stdout for protocol records and stderr for diagnostics; a
// record beyond the stderr bound is reported and discarded rather than
// buffered.
const maxLineBytes = agents.MaxRawEventBytes

// maxStreamRecordBytes bounds one JSONL record read from pi's stdout. It is
// deliberately far larger than MaxRawEventBytes: pi's turn_end, agent_end and
// message_end records embed complete messages and tool results, and real
// transcripts carry single records well past 64KiB (tool results with file
// contents or images were observed at 100KB+). The bound matches the history
// reader's maxHistoryRecordBytes so a record survives the live stream exactly
// when it survives the transcript replay. A record beyond it is still
// reported through line_too_large and discarded so the reader cannot wedge.
const maxStreamRecordBytes = 8 * 1024 * 1024

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
	// thinkingOpen tracks a streamed reasoning block, so a turn that ends
	// without a thinking_end still closes it instead of leaving the workbench
	// with a block that never settles.
	thinkingOpen bool
	// thinkingStreamed records whether any thinking_delta arrived for the open
	// block. A block that only reports thinking_end carries its authoritative
	// text in the end record, and that text must reach the workbench instead
	// of an empty terminator.
	thinkingStreamed bool

	// turnMu guards the open agent run. normalize runs on the read goroutine
	// while an interrupt or a process exit closes the run from another.
	turnMu      sync.Mutex
	turnID      string
	turnAborted bool
	turnSeq     uint64
	// newTurnID mints turn ids unique within the instance; a parser without
	// one numbers its own turns.
	newTurnID func() string
	toolCalls map[string]toolCallInfo
}

type toolCallInfo struct {
	name string
	args json.RawMessage
}

func (p *parser) rememberTool(callID, name string, args json.RawMessage) {
	if callID == "" {
		return
	}
	if p.toolCalls == nil {
		p.toolCalls = make(map[string]toolCallInfo)
	}
	p.toolCalls[callID] = toolCallInfo{name: name, args: args}
}

func (p *parser) forgetTool(callID string) (string, json.RawMessage) {
	if p.toolCalls == nil {
		return "", nil
	}
	info, ok := p.toolCalls[callID]
	if !ok {
		return "", nil
	}
	delete(p.toolCalls, callID)
	return info.name, info.args
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
	return scanLines(r, maxStreamRecordBytes,
		func(line []byte) { p.parseLine(line, emit) },
		func() {
			emit(p.errorEvent("line_too_large",
				fmt.Sprintf("pi record exceeds the %d byte limit", maxStreamRecordBytes)))
		},
	)
}

// benignStderrPrefixes are stderr lines pi emits that are expected platform
// behaviour, not diagnostics: a platform-created session id is unknown to pi
// until the first prompt creates it, and pi says so on stderr while doing
// exactly the right thing. Surfacing it as an error alarms the user with a
// warning the workbench cannot act on, so these lines are dropped.
var benignStderrPrefixes = []string{
	"Warning: No project session found with id",
}

// isBenignStderr reports whether a stderr line matches an expected-notice
// prefix (after trimming surrounding whitespace and pi's color codes' absence
// is assumed: stderr is plain text when not a TTY, which is how the adapter
// runs pi).
func isBenignStderr(line string) bool {
	trimmed := strings.TrimSpace(line)
	for _, prefix := range benignStderrPrefixes {
		if strings.HasPrefix(trimmed, prefix) {
			return true
		}
	}
	return false
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
			if isBenignStderr(string(line)) {
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
// Unknown fields are ignored by encoding/json. Usage rides message_update
// records (the provider-reported tokens for the turn so far); the parser
// forwards it on final message events as the context anchor.
type piEvent struct {
	Type string `json:"type"`

	// Session header.
	ID  string `json:"id"`
	Cwd string `json:"cwd"`

	// Message records.
	Message               json.RawMessage   `json:"message"`
	AssistantMessageEvent *piAssistantEvent `json:"assistantMessageEvent"`
	Usage                 *wireUsage        `json:"usage"`

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

	// Lifecycle records. agent_settled carries aborted=true when the run was
	// aborted, which the adapter surfaces as the turn's settled detail.
	Aborted bool `json:"aborted"`
}

// piAssistantEvent is the nested provider event carried by message_update.
type piAssistantEvent struct {
	Type    string          `json:"type"`
	Delta   string          `json:"delta"`
	Content json.RawMessage `json:"content"`
	Reason  string          `json:"reason"`
	Error   string          `json:"error"`
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
		// A tool result is conversation plumbing, not speech: the workbench
		// already renders it inside its tool block from the tool_execution_*
		// records, and the history replay maps it the same way. Emitting it as
		// a message shows every tool output twice.
		if role == "toolResult" {
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
		// Reasoning streams under the same envelope as text: thinking_start
		// opens the block, thinking_delta carries it and thinking_end closes it.
		// The workbench renders it as its own kind of entry.
		if update != nil && update.Type == "thinking_start" {
			p.thinkingOpen = true
			p.thinkingStreamed = false
			return nil
		}
		if update != nil && update.Type == "thinking_delta" {
			p.thinkingOpen = true
			p.thinkingStreamed = true
			out := p.event()
			out.Type = agents.EventThinking
			out.Thinking = &agents.ThinkingPayload{Text: update.Delta, Final: false}
			return []agents.NormalizedEvent{out}
		}
		if update != nil && update.Type == "thinking_end" {
			return p.closeThinking(contentText(update.Content))
		}
		if update != nil && update.Type == "error" {
			return []agents.NormalizedEvent{p.errorEvent("provider", redactSecrets(firstNonEmpty(update.Error, update.Reason)))}
		}
		// toolcall_* snapshots, start and done are either redundant with the
		// typed tool_execution_* records or have no normalized payload; keep
		// them as bounded raw metadata.
		return []agents.NormalizedEvent{p.unknown(raw)}

	case "message_end":
		role, text := messageParts(event.Message)
		if role != "assistant" {
			// User and system messages were emitted complete at
			// message_start; emitting message_end too would duplicate them.
			return nil
		}
		// A turn that ends while reasoning is open closes the block first, so
		// the order the user sees stays reasoning-then-reply.
		events := p.closeThinking("")
		streamed := p.deltaOpen
		p.deltaOpen = false
		if !streamed && text == "" {
			// Neither streamed deltas nor display text (a thinking-and-toolCall
			// message): emitting an empty final would render a blank bubble.
			return events
		}
		out := p.event()
		out.Type = agents.EventMessage
		out.Message = &agents.MessagePayload{Role: "assistant", Text: text, Final: true}
		if streamed {
			// Deltas already carried the text; close the message with an
			// empty terminator so a concatenating consumer does not double it.
			out.Message.Text = ""
		}
		// The envelope usage is the turn's provider-reported tokens; it rides
		// the final message as the context anchor. Deltas never carry it.
		out.Message.Usage = event.Usage.usage()
		return append(events, out)

	case "tool_execution_start":
		p.rememberTool(event.ToolCallID, event.ToolName, event.Args)
		out := p.event()
		out.Type = agents.EventToolCall
		out.ToolCall = &agents.ToolCallPayload{
			CallID: event.ToolCallID,
			Name:   event.ToolName,
			Input:  agents.PruneToolInput(event.ToolName, event.Args),
		}
		return []agents.NormalizedEvent{out}

	case "tool_execution_update":
		return nil

	case "tool_execution_end":
		name, args := p.forgetTool(event.ToolCallID)
		if name == "" {
			name = event.ToolName
		}
		if len(args) == 0 {
			args = event.Args
		}
		out := p.event()
		out.Type = agents.EventToolResult
		summary := agents.SummarizeToolResult(name, args, resultText(event.Result), event.IsError)
		out.ToolResult = &agents.ToolResultPayload{CallID: event.ToolCallID, Output: summary, IsError: event.IsError}
		return []agents.NormalizedEvent{out}

	case "error":
		return []agents.NormalizedEvent{p.errorEvent("backend", redactSecrets(firstNonEmpty(event.Error, event.ErrorMessage)))}

	// An agent run is a turn. agent_end and agent_settled both close it; the
	// second finds no open run and is dropped, so a turn never ends twice.
	case "agent_start":
		return []agents.NormalizedEvent{p.beginTurn()}

	case "agent_end":
		if turn, open := p.endTurn(agents.TurnCompleted); open {
			return []agents.NormalizedEvent{turn}
		}
		return nil

	case "agent_settled":
		outcome := agents.TurnCompleted
		if event.Aborted {
			outcome = agents.TurnCancelled
		}
		if turn, open := p.endTurn(outcome); open {
			return []agents.NormalizedEvent{turn}
		}
		return nil

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

// lifecycleTypes are the pi records that describe inner steps of a run. The
// agent run itself (agent_start, agent_end, agent_settled) is a turn, so it is
// not listed here.
var lifecycleTypes = map[string]bool{
	"turn_start": true,
	"turn_end":   true,
}

// beginTurn opens an agent run and returns its started event.
func (p *parser) beginTurn() agents.NormalizedEvent {
	p.turnMu.Lock()
	defer p.turnMu.Unlock()
	p.turnID = p.nextTurnID()
	p.turnAborted = false
	return p.turnEvent(agents.TurnPayload{ID: p.turnID, Phase: agents.TurnStarted})
}

// endTurn closes the open agent run with the given outcome. It reports false
// when no run is open. A run the platform asked pi to abort ends cancelled even
// when pi reports it as completed, because the cancellation is what the user did.
func (p *parser) endTurn(outcome agents.TurnOutcome) (agents.NormalizedEvent, bool) {
	p.turnMu.Lock()
	defer p.turnMu.Unlock()
	if p.turnID == "" {
		return agents.NormalizedEvent{}, false
	}
	if p.turnAborted && outcome == agents.TurnCompleted {
		outcome = agents.TurnCancelled
	}
	id := p.turnID
	p.turnID = ""
	p.turnAborted = false
	return p.turnEvent(agents.TurnPayload{ID: id, Phase: agents.TurnEnded, Outcome: outcome}), true
}

// markAbort records that the platform asked pi to abort the open run. It does
// nothing when no run is open, so an abort sent while idle cannot cancel the
// next run.
func (p *parser) markAbort() {
	p.turnMu.Lock()
	defer p.turnMu.Unlock()
	if p.turnID != "" {
		p.turnAborted = true
	}
}

// nextTurnID mints the next turn id. The caller holds turnMu.
func (p *parser) nextTurnID() string {
	if p.newTurnID != nil {
		return p.newTurnID()
	}
	p.turnSeq++
	return fmt.Sprintf("%s-turn-%d", p.sessionID, p.turnSeq)
}

// turnEvent builds a turn boundary of the parser's session.
func (p *parser) turnEvent(payload agents.TurnPayload) agents.NormalizedEvent {
	out := p.event()
	out.Type = agents.EventTurn
	out.Turn = &payload
	return out
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
// closeThinking emits the final terminator for an open reasoning block,
// mirroring how a streamed message is closed: when deltas already carried the
// text the terminator is empty, so a concatenating consumer does not double it
// and keeps the block streaming. When no delta arrived, the authoritative
// content reported by thinking_end (pi's json.md) is emitted as the block's
// text instead of an empty terminator. It emits nothing when no reasoning was
// open, so a turn without thinking is unchanged.
func (p *parser) closeThinking(content string) []agents.NormalizedEvent {
	if !p.thinkingOpen {
		return nil
	}
	text := ""
	if !p.thinkingStreamed {
		text = content
	}
	p.thinkingOpen = false
	p.thinkingStreamed = false
	out := p.event()
	out.Type = agents.EventThinking
	out.Thinking = &agents.ThinkingPayload{Text: text, Final: true}
	return []agents.NormalizedEvent{out}
}

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
// event unboundedly. It backs off to a UTF-8 rune boundary so a multi-byte
// character is never split into invalid bytes.
func truncate(value string) string {
	if len(value) <= maxStderrBytes {
		return value
	}
	cut := maxStderrBytes
	for cut > 0 && !utf8.RuneStart(value[cut]) {
		cut--
	}
	return value[:cut] + "…(truncated)"
}
