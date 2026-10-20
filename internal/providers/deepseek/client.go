package deepseek

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

const (
	defaultRPCTimeout = 30 * time.Second
	defaultMuxTimeout = 15 * time.Second
)

// RPCError represents an error returned by the dsh Typert RPC interface.
type RPCError struct {
	Code    string `json:"code,omitempty"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

func (e *RPCError) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("deepseek rpc error [%s]: %s", e.Code, e.Message)
	}
	return fmt.Sprintf("deepseek rpc error: %s", e.Message)
}

type rpcRequest struct {
	Type    string         `json:"type"`
	RPCID   string         `json:"rpcId"`
	Method  string         `json:"method"`
	Payload rpcPayloadArgs `json:"payload"`
}

type rpcPayloadArgs struct {
	Args any `json:"args"`
}

type rpcResponse struct {
	Type   string    `json:"type"`
	RPCID  string    `json:"rpcId"`
	Result rpcResult `json:"result"`
}

type rpcResult struct {
	OK    bool            `json:"ok"`
	Value json.RawMessage `json:"value,omitempty"`
	Error *RPCError       `json:"error,omitempty"`
}

// Client manages the authenticated HTTP and WebSocket connections to a running dsh process.
type Client struct {
	endpoint   string
	cookie     string
	httpClient *http.Client
	wsConn     *websocket.Conn
	wsMu       sync.Mutex

	subscribers   map[string]chan json.RawMessage
	subscribersMu sync.RWMutex

	clientID string
	done     chan struct{}
	closed   atomic.Bool
	closeOnce sync.Once
}

// ExchangeLaunchToken exchanges a dsh launch URL (http://127.0.0.1:<port>/?token=<token>)
// for an authenticated session cookie and base HTTP endpoint.
func ExchangeLaunchToken(ctx context.Context, launchURL string) (endpoint string, cookie string, err error) {
	u, err := url.Parse(launchURL)
	if err != nil {
		return "", "", fmt.Errorf("parse launch url: %w", err)
	}

	token := u.Query().Get("token")
	if token == "" {
		return "", "", errors.New("launch url missing token")
	}

	endpoint = fmt.Sprintf("%s://%s", u.Scheme, u.Host)

	// In dsh, GET /connect?token=<token>&interactions=none exchanges the token for API cookies
	connectURL := fmt.Sprintf("%s/connect?token=%s&interactions=none", endpoint, url.QueryEscape(token))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, connectURL, nil)
	if err != nil {
		return "", "", fmt.Errorf("create token exchange request: %w", err)
	}
	req.Host = u.Host

	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Timeout: 10 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		// Fallback to GET /?token=... if /connect is unavailable
		fallbackReq, err2 := http.NewRequestWithContext(ctx, http.MethodGet, launchURL, nil)
		if err2 != nil {
			return "", "", fmt.Errorf("token exchange failed: %w", err)
		}
		fallbackReq.Host = u.Host
		resp, err = client.Do(fallbackReq)
		if err != nil {
			return "", "", fmt.Errorf("token exchange failed: %w", err)
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return "", "", fmt.Errorf("token exchange rejected with HTTP %d", resp.StatusCode)
	}

	cookies := resp.Cookies()
	if len(cookies) == 0 {
		// Try raw header
		rawCookie := resp.Header.Get("Set-Cookie")
		if rawCookie == "" {
			return "", "", errors.New("token exchange response contained no Set-Cookie header")
		}
		cookie = strings.SplitN(rawCookie, ";", 2)[0]
	} else {
		var parts []string
		for _, c := range cookies {
			parts = append(parts, fmt.Sprintf("%s=%s", c.Name, c.Value))
		}
		cookie = strings.Join(parts, "; ")
	}

	return endpoint, cookie, nil
}

// NewClient creates an unauthenticated client. Call ConnectMux to initialize the WebSocket connection.
func NewClient(endpoint, cookie string) *Client {
	return &Client{
		endpoint: endpoint,
		cookie:   cookie,
		httpClient: &http.Client{
			Timeout: defaultRPCTimeout,
		},
		subscribers: make(map[string]chan json.RawMessage),
		done:        make(chan struct{}),
	}
}

// Endpoint returns the base HTTP endpoint.
func (c *Client) Endpoint() string {
	return c.endpoint
}

// ClientID returns the remote event client ID bound on this connection.
func (c *Client) ClientID() string {
	return c.clientID
}

// Call performs a unary RPC call over HTTP POST /api/<method>.
func (c *Client) Call(ctx context.Context, method string, args any, result any) error {
	rpcID := uuid.New().String()
	bodyBytes, err := json.Marshal(rpcRequest{
		Type:    "client-request",
		RPCID:   rpcID,
		Method:  method,
		Payload: rpcPayloadArgs{Args: args},
	})
	if err != nil {
		return fmt.Errorf("marshal rpc request: %w", err)
	}

	targetURL := fmt.Sprintf("%s/api/%s", c.endpoint, method)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("create rpc request: %w", err)
	}

	u, _ := url.Parse(c.endpoint)
	if u != nil && u.Host != "" {
		req.Host = u.Host
	}
	req.Header.Set("Content-Type", "application/json")
	if c.cookie != "" {
		req.Header.Set("Cookie", c.cookie)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("execute rpc %s: %w", method, err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read rpc %s response: %w", method, err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("rpc %s failed with HTTP %d: %s", method, resp.StatusCode, string(respBytes))
	}

	var res rpcResponse
	if err := json.Unmarshal(respBytes, &res); err != nil {
		return fmt.Errorf("unmarshal rpc %s response: %w", method, err)
	}

	if !res.Result.OK {
		if res.Result.Error != nil {
			return res.Result.Error
		}
		return fmt.Errorf("rpc %s failed without error payload", method)
	}

	if result != nil && len(res.Result.Value) > 0 && string(res.Result.Value) != "null" {
		if err := json.Unmarshal(res.Result.Value, result); err != nil {
			return fmt.Errorf("unmarshal rpc %s result value: %w", method, err)
		}
	}
	return nil
}

type wsClientMessage struct {
	Type     string `json:"type"`
	StreamID string `json:"streamId"`
	Endpoint string `json:"endpoint,omitempty"`
	Payload  any    `json:"payload,omitempty"`
}

type wsServerMessage struct {
	Type     string          `json:"type"`
	StreamID string          `json:"streamId"`
	Value    json.RawMessage `json:"value,omitempty"`
	Error    *RPCError       `json:"error,omitempty"`
}

// ConnectMux dials ws://<host>/api/remote.mux, starts the read loop, and binds the event stream.
func (c *Client) ConnectMux(ctx context.Context) error {
	u, err := url.Parse(c.endpoint)
	if err != nil {
		return fmt.Errorf("parse endpoint: %w", err)
	}

	scheme := "ws"
	if u.Scheme == "https" {
		scheme = "wss"
	}
	wsURL := fmt.Sprintf("%s://%s/api/remote.mux", scheme, u.Host)

	header := http.Header{}
	header.Set("Host", u.Host)
	if c.cookie != "" {
		header.Set("Cookie", c.cookie)
	}

	dialer := *websocket.DefaultDialer
	dialer.HandshakeTimeout = defaultMuxTimeout

	conn, _, err := dialer.DialContext(ctx, wsURL, header)
	if err != nil {
		return fmt.Errorf("dial remote mux websocket: %w", err)
	}
	c.wsConn = conn

	go c.readLoop()

	// Subscribe to $events to obtain ClientID and event readiness
	readyCh := make(chan string, 1)
	errCh := make(chan error, 1)

	eventsID, eventsCh, cancelEvents, err := c.Subscribe(ctx, "$events", map[string]any{})
	if err != nil {
		conn.Close()
		return fmt.Errorf("subscribe $events: %w", err)
	}

	go func() {
		for item := range eventsCh {
			var readyFrame struct {
				Type     string `json:"type"`
				ClientID string `json:"clientId"`
			}
			if err := json.Unmarshal(item, &readyFrame); err == nil && readyFrame.Type == "ready" {
				readyCh <- readyFrame.ClientID
				return
			}
		}
		errCh <- errors.New("events stream closed without ready frame")
	}()

	select {
	case id := <-readyCh:
		c.clientID = id
		// Keep events subscription active for the lifetime of this connection
		_ = eventsID
	case err := <-errCh:
		cancelEvents()
		c.Close()
		return err
	case <-ctx.Done():
		cancelEvents()
		c.Close()
		return ctx.Err()
	case <-time.After(defaultMuxTimeout):
		cancelEvents()
		c.Close()
		return errors.New("timed out waiting for dsh event readiness")
	}

	return nil
}

// Subscribe opens a remote stream over the WebSocket mux.
// It returns the streamID, a channel delivering stream items, and a cancellation function.
func (c *Client) Subscribe(ctx context.Context, endpoint string, args any) (string, <-chan json.RawMessage, func(), error) {
	if c.closed.Load() {
		return "", nil, nil, errors.New("client is closed")
	}

	streamID := newStreamID()
	ch := make(chan json.RawMessage, 200)

	c.subscribersMu.Lock()
	c.subscribers[streamID] = ch
	c.subscribersMu.Unlock()

	msg := wsClientMessage{
		Type:     "open",
		StreamID: streamID,
		Endpoint: endpoint,
		Payload:  map[string]any{"args": args},
	}

	c.wsMu.Lock()
	err := c.wsConn.WriteJSON(msg)
	c.wsMu.Unlock()

	if err != nil {
		c.subscribersMu.Lock()
		delete(c.subscribers, streamID)
		close(ch)
		c.subscribersMu.Unlock()
		return "", nil, nil, fmt.Errorf("write subscribe frame: %w", err)
	}

	cancelOnce := sync.Once{}
	cancel := func() {
		cancelOnce.Do(func() {
			c.subscribersMu.Lock()
			delete(c.subscribers, streamID)
			close(ch)
			c.subscribersMu.Unlock()

			if !c.closed.Load() && c.wsConn != nil {
				c.wsMu.Lock()
				_ = c.wsConn.WriteJSON(wsClientMessage{
					Type:     "cancel",
					StreamID: streamID,
				})
				c.wsMu.Unlock()
			}
		})
	}

	return streamID, ch, cancel, nil
}

func (c *Client) readLoop() {
	defer func() {
		c.Close()
	}()

	for {
		_, data, err := c.wsConn.ReadMessage()
		if err != nil {
			return
		}

		var frame wsServerMessage
		if err := json.Unmarshal(data, &frame); err != nil {
			continue
		}

		c.subscribersMu.RLock()
		ch, ok := c.subscribers[frame.StreamID]
		c.subscribersMu.RUnlock()

		if !ok || ch == nil {
			continue
		}

		switch frame.Type {
		case "item":
			select {
			case ch <- frame.Value:
			default:
			}
		case "end", "error":
			c.subscribersMu.Lock()
			if activeCh, exists := c.subscribers[frame.StreamID]; exists {
				delete(c.subscribers, frame.StreamID)
				close(activeCh)
			}
			c.subscribersMu.Unlock()
		}
	}
}

// Close closes the client and terminates WebSocket and pending streams.
func (c *Client) Close() error {
	c.closeOnce.Do(func() {
		c.closed.Store(true)
		close(c.done)

		c.wsMu.Lock()
		if c.wsConn != nil {
			_ = c.wsConn.Close()
		}
		c.wsMu.Unlock()

		c.subscribersMu.Lock()
		for id, ch := range c.subscribers {
			delete(c.subscribers, id)
			close(ch)
		}
		c.subscribersMu.Unlock()
	})
	return nil
}

func newStreamID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return fmt.Sprintf("stream-%x", b)
}
