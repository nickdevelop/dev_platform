package acp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
)

var (
	ErrClientClosed = errors.New("acp: client is closed")
)

// RequestHandler handles server-initiated requests (e.g. session/request_permission).
type RequestHandler func(ctx context.Context, id any, method string, params json.RawMessage) (any, error)

// NotificationHandler handles server-initiated notifications (e.g. session/update).
type NotificationHandler func(method string, params json.RawMessage)

type pendingCall struct {
	done chan *Response
}

type Client struct {
	writer io.Writer
	closer io.Closer

	writeMu sync.Mutex

	mu      sync.Mutex
	pending map[uint64]*pendingCall
	nextID  atomic.Uint64
	closed  bool

	onRequest      RequestHandler
	onNotification NotificationHandler
}

// NewClient constructs an ACP JSON-RPC 2.0 client.
func NewClient(writer io.Writer, closer io.Closer, onReq RequestHandler, onNotif NotificationHandler) *Client {
	return &Client{
		writer:         writer,
		closer:         closer,
		pending:        make(map[uint64]*pendingCall),
		onRequest:      onReq,
		onNotification: onNotif,
	}
}

// Call performs a synchronous JSON-RPC 2.0 call and unmarshals the result into resultTarget.
func (c *Client) Call(ctx context.Context, method string, params any, resultTarget any) error {
	id := c.nextID.Add(1)

	rawParams, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("acp: marshal params: %w", err)
	}

	call := &pendingCall{
		done: make(chan *Response, 1),
	}

	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return ErrClientClosed
	}
	c.pending[id] = call
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
	}()

	req := Request{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  rawParams,
	}

	reqBytes, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("acp: marshal request: %w", err)
	}

	if err := c.writeLine(reqBytes); err != nil {
		return fmt.Errorf("acp: send request: %w", err)
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case resp, ok := <-call.done:
		if !ok || resp == nil {
			return ErrClientClosed
		}
		if resp.Error != nil {
			return resp.Error
		}
		if resultTarget != nil && len(resp.Result) > 0 {
			if err := json.Unmarshal(resp.Result, resultTarget); err != nil {
				return fmt.Errorf("acp: unmarshal response result: %w", err)
			}
		}
		return nil
	}
}

// Notify sends a JSON-RPC 2.0 notification.
func (c *Client) Notify(method string, params any) error {
	rawParams, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("acp: marshal params: %w", err)
	}

	notif := Notification{
		JSONRPC: "2.0",
		Method:  method,
		Params:  rawParams,
	}

	notifBytes, err := json.Marshal(notif)
	if err != nil {
		return fmt.Errorf("acp: marshal notification: %w", err)
	}

	return c.writeLine(notifBytes)
}

// Respond responds to a server-initiated request.
func (c *Client) Respond(id any, result any, rpcErr *RPCError) error {
	var rawResult json.RawMessage
	if result != nil {
		b, err := json.Marshal(result)
		if err != nil {
			return fmt.Errorf("acp: marshal response result: %w", err)
		}
		rawResult = b
	}

	resp := Response{
		JSONRPC: "2.0",
		ID:      id,
		Result:  rawResult,
		Error:   rpcErr,
	}

	respBytes, err := json.Marshal(resp)
	if err != nil {
		return fmt.Errorf("acp: marshal response: %w", err)
	}

	return c.writeLine(respBytes)
}

func (c *Client) writeLine(data []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	c.mu.Lock()
	closed := c.closed
	c.mu.Unlock()
	if closed {
		return ErrClientClosed
	}

	data = append(data, '\n')
	_, err := c.writer.Write(data)
	return err
}

// ReadLoop reads lines from the server stdout until EOF or error.
func (c *Client) ReadLoop(reader io.Reader) error {
	scanner := bufio.NewScanner(reader)
	// Buffer up to 10MB per line for large responses or diffs
	buf := make([]byte, 64*1024)
	scanner.Buffer(buf, 10*1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		c.dispatchLine(line)
	}

	err := scanner.Err()
	c.Close()
	return err
}

func (c *Client) dispatchLine(line []byte) {
	var raw struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      any             `json:"id"`
		Method  string          `json:"method"`
		Result  json.RawMessage `json:"result,omitempty"`
		Error   *RPCError       `json:"error,omitempty"`
		Params  json.RawMessage `json:"params,omitempty"`
	}

	if err := json.Unmarshal(line, &raw); err != nil {
		return
	}

	// 1. Response to client request (has ID and (Result!=nil or Error!=nil) and Method=="")
	if raw.ID != nil && raw.Method == "" {
		c.handleResponse(raw.ID, raw.Result, raw.Error)
		return
	}

	// 2. Server request (has Method and ID)
	if raw.Method != "" && raw.ID != nil {
		if c.onRequest != nil {
			go func() {
				res, err := c.onRequest(context.Background(), raw.ID, raw.Method, raw.Params)
				if err != nil {
					_ = c.Respond(raw.ID, nil, &RPCError{Code: -32000, Message: err.Error()})
				} else {
					_ = c.Respond(raw.ID, res, nil)
				}
			}()
		}
		return
	}

	// 3. Notification (has Method, no ID)
	if raw.Method != "" && raw.ID == nil {
		if c.onNotification != nil {
			c.onNotification(raw.Method, raw.Params)
		}
	}
}

func (c *Client) handleResponse(id any, result json.RawMessage, rpcErr *RPCError) {
	var idNum uint64
	switch v := id.(type) {
	case float64:
		idNum = uint64(v)
	case int64:
		idNum = uint64(v)
	case int:
		idNum = uint64(v)
	case uint64:
		idNum = v
	default:
		return
	}

	c.mu.Lock()
	call, ok := c.pending[idNum]
	if ok {
		delete(c.pending, idNum)
	}
	c.mu.Unlock()

	if ok && call != nil {
		call.done <- &Response{
			ID:     id,
			Result: result,
			Error:  rpcErr,
		}
	}
}

// Close closes the client and drains all pending calls.
func (c *Client) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	pending := c.pending
	c.pending = make(map[uint64]*pendingCall)
	c.mu.Unlock()

	for _, call := range pending {
		close(call.done)
	}

	if c.closer != nil {
		return c.closer.Close()
	}
	return nil
}
