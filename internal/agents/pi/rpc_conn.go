package pi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"devplatform/internal/agents"
)

// rpcConn carries the command/response and event traffic of one RPC child. It
// owns the write side (one JSONL record per command, serialized) and turns the
// read side into normalized events, correlating response records with their
// pending command by id.
//
// Framing is strict JSONL: records are split on LF only, and a record is never
// split on Unicode line/paragraph separators (scanLines reads bytes, so those
// stay inside JSON strings, which the protocol requires).
type rpcConn struct {
	parser *parser
	emit   func(agents.NormalizedEvent)
	writer io.Writer

	// extensionUI is consulted for extension_ui_request records. When it
	// returns false (or is nil) the request is not handled here; dialog
	// methods are auto-cancelled so pi never blocks on a client that cannot
	// answer.
	extensionUI func(*rpcExtensionUIRequest) bool

	writeMu  sync.Mutex
	writeErr error

	mu      sync.Mutex
	pending map[string]*rpcPending
	nextID  uint64
	closed  bool
}

// rpcPending is one command awaiting its response.
type rpcPending struct {
	command string
	ch      chan rpcResponse
}

// errRPCClosed reports a command issued after the child's stdout ended, or
// one that was outstanding when it did.
var errRPCClosed = errors.New("pi: rpc child is no longer running")

// newRPCConn builds a connection around a child's stdin writer. The parser is
// bound to the child's known instance and session, so session events carry a
// complete envelope even though RPC emits no session header.
func newRPCConn(writer io.Writer, parser *parser, emit func(agents.NormalizedEvent)) *rpcConn {
	return &rpcConn{
		parser:  parser,
		emit:    emit,
		writer:  writer,
		pending: make(map[string]*rpcPending),
	}
}

// readLoop reads stdout records until EOF or an I/O error and returns it.
// Malformed or oversized records become error events and do not end the loop,
// matching the one-shot JSON reader's resilience.
func (c *rpcConn) readLoop(r io.Reader) error {
	return scanLines(r, maxStreamRecordBytes,
		func(line []byte) { c.dispatch(line) },
		func() {
			c.emit(c.parser.errorEvent("line_too_large",
				fmt.Sprintf("pi record exceeds the %d byte limit", maxStreamRecordBytes)))
		},
	)
}

// dispatch classifies one stdout record. Response records go to the pending
// command table, extension UI records to the approval subprotocol, and
// everything else is a session event the shared parser normalizes.
func (c *rpcConn) dispatch(line []byte) {
	record := trimRecord(line)
	if len(record) == 0 {
		return
	}
	var envelope struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(record, &envelope); err != nil {
		// Reuse the parser's malformed-record handling so line numbering and
		// the preserved raw bytes match the JSON-mode path.
		c.parser.parseLine(record, c.emit)
		return
	}
	switch envelope.Type {
	case "response":
		c.deliverResponse(record)
	case "extension_ui_request":
		c.deliverExtensionUI(record)
	default:
		c.parser.parseLine(record, c.emit)
	}
}

// deliverResponse hands a response to its waiting command. A response without
// an id (pi emits one for a malformed command) becomes a parse error event
// instead of being silently dropped.
func (c *rpcConn) deliverResponse(record []byte) {
	var response rpcResponse
	if err := json.Unmarshal(record, &response); err != nil {
		c.emit(c.parser.errorEvent("parse_error", "malformed pi response record"))
		return
	}
	if response.ID == "" {
		if response.Command == "parse" && !response.Success {
			c.emit(c.parser.errorEvent("rpc_parse", redactSecrets(firstNonEmpty(response.Error, "pi could not parse the command"))))
		}
		return
	}
	c.mu.Lock()
	pending := c.pending[response.ID]
	delete(c.pending, response.ID)
	c.mu.Unlock()
	if pending != nil {
		pending.ch <- response
	}
}

// deliverExtensionUI routes an extension UI record to the approval handler or
// cancels the dialog so the agent is never left waiting for a client that does
// not answer.
func (c *rpcConn) deliverExtensionUI(record []byte) {
	var request rpcExtensionUIRequest
	if err := json.Unmarshal(record, &request); err != nil {
		c.emit(c.parser.errorEvent("parse_error", "malformed pi extension UI record"))
		return
	}
	if request.ID == "" {
		return
	}
	if c.extensionUI != nil && c.extensionUI(&request) {
		return
	}
	if request.dialog() {
		// No handler: resolve the dialog with its default so pi does not
		// block. The platform never sees it because it declared no
		// confirmation capability for this adapter mode.
		_ = c.respondExtensionUI(rpcExtensionUIResponse{Type: "extension_ui_response", ID: request.ID, Cancelled: true})
	}
}

// request registers a pending command, writes it, and waits for its response,
// the caller's context, or the optional per-command timeout. A zero timeout
// means only the context bounds the wait.
func (c *rpcConn) request(ctx context.Context, command rpcCommand, timeout time.Duration) (*rpcResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	command.ID = c.newCommandID(command.Type)
	pending := &rpcPending{command: command.Type, ch: make(chan rpcResponse, 1)}

	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, errRPCClosed
	}
	c.pending[command.ID] = pending
	c.mu.Unlock()

	if err := c.writeRecord(ctx, command); err != nil {
		c.mu.Lock()
		delete(c.pending, command.ID)
		c.mu.Unlock()
		return nil, err
	}

	var timer <-chan time.Time
	if timeout > 0 {
		t := time.NewTimer(timeout)
		defer t.Stop()
		timer = t.C
	}
	select {
	case response := <-pending.ch:
		return &response, nil
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.pending, command.ID)
		c.mu.Unlock()
		return nil, ctx.Err()
	case <-timer:
		c.mu.Lock()
		delete(c.pending, command.ID)
		c.mu.Unlock()
		return nil, fmt.Errorf("pi: %s command timed out after %s", command.Type, timeout)
	}
}

// writeRecord marshals one record and writes it as a single LF-terminated
// line, serialized against other writers so interleaved commands cannot
// corrupt a record. A write error is sticky: every later write fails with it.
func (c *rpcConn) writeRecord(ctx context.Context, record any) error {
	payload, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("pi: encode rpc record: %w", err)
	}
	payload = append(payload, '\n')

	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if c.writeErr != nil {
		return c.writeErr
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := writeFull(c.writer, payload); err != nil {
		c.writeErr = err
		return fmt.Errorf("pi: write rpc record: %w", err)
	}
	return nil
}

// respondExtensionUI writes one extension UI response. It bypasses the command
// table because these records are not commands.
func (c *rpcConn) respondExtensionUI(response rpcExtensionUIResponse) error {
	return c.writeRecord(context.Background(), response)
}

// newCommandID returns a unique command id. ids are only unique within one
// connection; the prefix keeps them readable in diagnostics.
func (c *rpcConn) newCommandID(command string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.nextID++
	return fmt.Sprintf("%s-%d", command, c.nextID)
}

// close fails every pending command and marks the connection closed. It is
// idempotent. A response delivered concurrently is either seen by the waiter
// or replaced by errRPCClosed, never lost without an answer.
func (c *rpcConn) close() {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	pending := c.pending
	c.pending = make(map[string]*rpcPending)
	c.mu.Unlock()
	for _, p := range pending {
		p.ch <- rpcResponse{Command: p.command, Success: false, Error: errRPCClosed.Error()}
	}
}

// writeFull writes every byte, retrying short writes.
func writeFull(w io.Writer, payload []byte) error {
	for len(payload) > 0 {
		n, err := w.Write(payload)
		if err != nil {
			return err
		}
		if n <= 0 {
			return io.ErrShortWrite
		}
		payload = payload[n:]
	}
	return nil
}

// trimRecord strips the record terminator (and an optional preceding CR, which
// the protocol accepts on input) before classification.
func trimRecord(line []byte) []byte {
	end := len(line)
	for end > 0 && (line[end-1] == '\n' || line[end-1] == '\r') {
		end--
	}
	if end == 0 {
		return nil
	}
	return line[:end]
}
