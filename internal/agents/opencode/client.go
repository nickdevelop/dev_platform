// Package opencode adapts the opencode v2 HTTP API to the platform's
// AgentAdapter contract. opencode is driven as a long-lived server process:
// the adapter launches `opencode serve` on a loopback port, speaks its typed
// HTTP API over a bounded client, and fans the server's SSE event stream out as
// normalized per-instance events. Agent-native session history stays inside
// opencode; the platform keeps only the id mapping.
package opencode

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"devplatform/internal/agents"
)

const (
	// defaultUsername is the opencode server's basic-auth user. The password is
	// generated per instance and never logged.
	defaultUsername = "opencode"
	// defaultRequestTimeout bounds every non-streaming API call when the
	// caller's context carries no deadline of its own.
	defaultRequestTimeout = 30 * time.Second
	// maxResponseBytes bounds one non-streaming API response body.
	maxResponseBytes = 4 * 1024 * 1024
	// maxEventBytes bounds one SSE event payload, matching the normalized
	// event contract's raw-blob bound.
	maxEventBytes = agents.MaxRawEventBytes
)

// Client is a typed client for the opencode v2 HTTP API. Every call carries the
// instance's basic-auth credentials and honors the caller's context. It is safe
// for concurrent use.
type Client struct {
	baseURL  string
	username string
	password string
	http     *http.Client
	timeout  time.Duration
}

// ClientOptions configures a Client. BaseURL is required; Username defaults to
// "opencode". HTTPClient and Timeout are optional seams for tests and tuning.
type ClientOptions struct {
	BaseURL    string
	Username   string
	Password   string
	Timeout    time.Duration
	HTTPClient *http.Client
}

// NewClient validates options and builds a client. A custom HTTPClient is used
// as-is; otherwise a client with bounded dial/handshake/header timeouts and no
// whole-request timeout (so the event stream stays open) is installed.
func NewClient(options ClientOptions) (*Client, error) {
	if strings.TrimSpace(options.BaseURL) == "" {
		return nil, errors.New("opencode: base URL is required")
	}
	parsed, err := url.Parse(options.BaseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("opencode: invalid base URL %q", options.BaseURL)
	}

	client := &Client{
		baseURL:  strings.TrimRight(options.BaseURL, "/"),
		username: options.Username,
		password: options.Password,
		http:     options.HTTPClient,
		timeout:  options.Timeout,
	}
	if client.username == "" {
		client.username = defaultUsername
	}
	if client.timeout <= 0 {
		client.timeout = defaultRequestTimeout
	}
	if client.http == nil {
		client.http = newHTTPClient()
	}
	return client, nil
}

// newHTTPClient builds the default transport. It deliberately sets no
// http.Client.Timeout: the event stream is long-lived and is bounded by its
// context and the transport's dial/header timeouts instead.
func newHTTPClient() *http.Client {
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          16,
		MaxIdleConnsPerHost:   4,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 20 * time.Second,
		ExpectContinueTimeout: time.Second,
	}
	return &http.Client{Transport: transport}
}

// ServerInfo mirrors GET /api/info.
type ServerInfo struct {
	Version string      `json:"version"`
	PID     int         `json:"pid"`
	URLs    []string    `json:"urls"`
	Paths   ServerPaths `json:"paths"`
}

// ServerPaths mirrors the server's path inventory.
type ServerPaths struct {
	Tmp string `json:"tmp"`
}

// LocationRef mirrors Location.PublicRef: the working directory a session or
// event belongs to.
type LocationRef struct {
	Directory string `json:"directory"`
}

// ModelRef mirrors Model.Ref: the provider-scoped model identity the v2 API
// expects for session creation and switching.
type ModelRef struct {
	ID         string `json:"id"`
	ProviderID string `json:"providerID"`
	Variant    string `json:"variant,omitempty"`
}

// SessionTime mirrors the session's lifecycle timestamps (unix milliseconds).
type SessionTime struct {
	Created int64 `json:"created"`
	Updated int64 `json:"updated"`
}

// SessionInfo mirrors the subset of Session.Info the adapter needs.
type SessionInfo struct {
	ID        string       `json:"id"`
	ParentID  string       `json:"parentID,omitempty"`
	ProjectID string       `json:"projectID"`
	Title     string       `json:"title,omitempty"`
	Location  *LocationRef `json:"location,omitempty"`
	Time      SessionTime  `json:"time"`
}

// CreateSessionRequest is the body of POST /api/session.
type CreateSessionRequest struct {
	Title    string       `json:"title,omitempty"`
	Model    *ModelRef    `json:"model,omitempty"`
	Location *LocationRef `json:"location,omitempty"`
}

// APIError reports a non-2xx response from the opencode API. Message is
// redacted before it leaves the client.
type APIError struct {
	StatusCode int
	Code       string
	Message    string
	Method     string
	Path       string
}

// Error implements the error interface.
func (e *APIError) Error() string {
	code := e.Code
	if code == "" {
		code = "error"
	}
	message := e.Message
	if message == "" {
		message = http.StatusText(e.StatusCode)
	}
	return fmt.Sprintf("opencode: %s %s: HTTP %d %s: %s", e.Method, e.Path, e.StatusCode, code, message)
}

// Info reports server identity and readiness.
func (c *Client) Info(ctx context.Context) (ServerInfo, error) {
	var info ServerInfo
	if err := c.do(ctx, http.MethodGet, "/api/info", nil, nil, &info); err != nil {
		return ServerInfo{}, err
	}
	return info, nil
}

// ListSessions returns the server's sessions for a directory.
func (c *Client) ListSessions(ctx context.Context, directory string) ([]SessionInfo, error) {
	query := url.Values{}
	if directory != "" {
		query.Set("directory", directory)
	}
	var payload struct {
		Data []SessionInfo `json:"data"`
	}
	if err := c.do(ctx, http.MethodGet, "/api/session", query, nil, &payload); err != nil {
		return nil, err
	}
	return payload.Data, nil
}

// CreateSession creates a native session.
func (c *Client) CreateSession(ctx context.Context, request CreateSessionRequest) (SessionInfo, error) {
	var payload struct {
		Data SessionInfo `json:"data"`
	}
	if err := c.do(ctx, http.MethodPost, "/api/session", nil, request, &payload); err != nil {
		return SessionInfo{}, err
	}
	return payload.Data, nil
}

// GetSession retrieves one native session.
func (c *Client) GetSession(ctx context.Context, sessionID string) (SessionInfo, error) {
	var payload struct {
		Data SessionInfo `json:"data"`
	}
	if err := c.do(ctx, http.MethodGet, sessionPath(sessionID), nil, nil, &payload); err != nil {
		return SessionInfo{}, err
	}
	return payload.Data, nil
}

// SwitchModel changes the model a session uses for subsequent turns.
func (c *Client) SwitchModel(ctx context.Context, sessionID string, model ModelRef) error {
	body := struct {
		Model ModelRef `json:"model"`
	}{Model: model}
	return c.do(ctx, http.MethodPost, sessionPath(sessionID)+"/model", nil, body, nil)
}

// Prompt admits one user message and schedules the agent loop. The reply and
// tool activity arrive on the event stream.
func (c *Client) Prompt(ctx context.Context, sessionID, text string) error {
	body := struct {
		Text string `json:"text"`
	}{Text: text}
	return c.do(ctx, http.MethodPost, sessionPath(sessionID)+"/prompt", nil, body, nil)
}

// Events opens the server's SSE event stream. The caller owns the returned
// stream and must Close it.
func (c *Client) Events(ctx context.Context) (*EventStream, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/event", nil)
	if err != nil {
		return nil, fmt.Errorf("opencode: build event request: %w", err)
	}
	request.Header.Set("accept", "text/event-stream")
	c.authorize(request)

	response, err := c.http.Do(request)
	if err != nil {
		return nil, fmt.Errorf("opencode: subscribe events: %w", &redactedError{err: err, redact: c.redact})
	}
	if response.StatusCode < 200 || response.StatusCode > 299 {
		defer response.Body.Close()
		data, _ := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes))
		return nil, c.apiError(http.MethodGet, "/api/event", response.StatusCode, data)
	}
	return newEventStream(response.Body), nil
}

// sessionPath builds the session resource path with an escaped id.
func sessionPath(sessionID string) string {
	return "/api/session/" + url.PathEscape(sessionID)
}

// do performs one non-streaming request, applying the default timeout when the
// caller has no deadline, enforcing a response-size bound, and surfacing non-2xx
// responses as *APIError.
func (c *Client) do(ctx context.Context, method, path string, query url.Values, body, out any) error {
	ctx, cancel := c.withTimeout(ctx)
	defer cancel()

	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("opencode: encode %s %s request: %w", method, path, err)
		}
		reader = bytes.NewReader(encoded)
	}

	endpoint := c.baseURL + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return fmt.Errorf("opencode: build %s %s request: %w", method, path, err)
	}
	if body != nil {
		request.Header.Set("content-type", "application/json")
	}
	request.Header.Set("accept", "application/json")
	c.authorize(request)

	response, err := c.http.Do(request)
	if err != nil {
		return fmt.Errorf("opencode: %s %s: %w", method, path, &redactedError{err: err, redact: c.redact})
	}
	defer response.Body.Close()

	data, readErr := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if readErr != nil {
		return fmt.Errorf("opencode: read %s %s: %w", method, path, &redactedError{err: readErr, redact: c.redact})
	}
	if len(data) > maxResponseBytes {
		return fmt.Errorf("opencode: %s %s: response exceeds %d bytes", method, path, maxResponseBytes)
	}
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return c.apiError(method, path, response.StatusCode, data)
	}
	if out != nil && len(bytes.TrimSpace(data)) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("opencode: decode %s %s response: %w", method, path, err)
		}
	}
	return nil
}

// withTimeout applies the default request timeout only when the caller has not
// already set a deadline.
func (c *Client) withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if _, ok := ctx.Deadline(); ok {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, c.timeout)
}

// authorize attaches the instance's basic-auth credentials.
func (c *Client) authorize(request *http.Request) {
	request.SetBasicAuth(c.username, c.password)
}

// apiError turns a non-2xx body into a redacted *APIError.
func (c *Client) apiError(method, path string, status int, data []byte) error {
	var envelope struct {
		Tag     string `json:"_tag"`
		Message string `json:"message"`
	}
	_ = json.Unmarshal(data, &envelope)
	return &APIError{
		StatusCode: status,
		Code:       envelope.Tag,
		Message:    c.redact(envelope.Message),
		Method:     method,
		Path:       path,
	}
}

// redact masks the instance password and anything that looks like a credential.
func (c *Client) redact(value string) string {
	if c.password != "" {
		value = strings.ReplaceAll(value, c.password, "[REDACTED]")
	}
	return redactSecrets(value)
}

// redactedError preserves an underlying error's identity (errors.Is/As) while
// redacting its message.
type redactedError struct {
	err    error
	redact func(string) string
}

// Error implements the error interface.
func (e *redactedError) Error() string { return e.redact(e.err.Error()) }

// Unwrap exposes the original error to errors.Is/errors.As.
func (e *redactedError) Unwrap() error { return e.err }

// secretPatterns match credential shapes an agent CLI or server might echo.
var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b(?:sk|rk|pk|ghp|gho|glpat)-[A-Za-z0-9._-]{6,}`),
	regexp.MustCompile(`(?i)\bbearer\s+[A-Za-z0-9._~+/-]+=*`),
	regexp.MustCompile(`(?i)\b(api[_-]?key|access[_-]?token|auth[_-]?token|token|secret|password)\b\s*[=:]\s*[^\s"',]+`),
}

// redactSecrets masks the value of anything that looks like a credential. It is
// conservative so ordinary diagnostics survive intact.
func redactSecrets(value string) string {
	for _, pattern := range secretPatterns {
		value = pattern.ReplaceAllStringFunc(value, func(match string) string {
			if idx := strings.IndexAny(match, "=:"); idx >= 0 {
				return match[:idx+1] + "[REDACTED]"
			}
			if space := strings.IndexAny(match, " \t"); space >= 0 {
				return match[:space+1] + "[REDACTED]"
			}
			return "[REDACTED]"
		})
	}
	return value
}

// Event is one decoded SSE event from GET /api/event. The envelope carries the
// event id, type, optional location and durable aggregate metadata; Data holds
// the type-specific body untouched.
type Event struct {
	ID       string          `json:"id"`
	Created  int64           `json:"created,omitempty"`
	Type     string          `json:"type"`
	Location *LocationRef    `json:"location,omitempty"`
	Data     json.RawMessage `json:"data"`
	Durable  *Durable        `json:"durable,omitempty"`

	raw []byte
}

// Durable mirrors the event's durable aggregate metadata.
type Durable struct {
	AggregateID string `json:"aggregateID"`
	Sequence    uint64 `json:"seq"`
	Version     uint64 `json:"version"`
}

// SessionID extracts the session id carried in the event body, or "" when the
// event is not session-scoped.
func (e *Event) SessionID() string {
	if len(e.Data) == 0 {
		return ""
	}
	var probe struct {
		SessionID string `json:"sessionID"`
	}
	if err := json.Unmarshal(e.Data, &probe); err != nil {
		return ""
	}
	return probe.SessionID
}

// Raw returns the untouched bytes of the SSE event body.
func (e *Event) Raw() []byte { return e.raw }

// EventStream reads SSE events from an open response body. It ignores comment
// lines and heartbeats and bounds each event payload.
type EventStream struct {
	body    io.ReadCloser
	scanner *bufio.Scanner
	closed  bool
}

// newEventStream wraps a response body.
func newEventStream(body io.ReadCloser) *EventStream {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), maxEventBytes)
	return &EventStream{body: body, scanner: scanner}
}

// Next returns the next event, io.EOF when the stream ends, or an error when
// the connection breaks or the payload exceeds the bound.
func (s *EventStream) Next() (*Event, error) {
	var data []byte
	for s.scanner.Scan() {
		line := bytes.TrimRight(s.scanner.Bytes(), "\r\n")
		if len(line) == 0 {
			if len(data) > 0 {
				return s.decode(data)
			}
			continue
		}
		if line[0] == ':' {
			continue
		}
		field, value := splitSSEField(line)
		if field != "data" {
			continue
		}
		if len(data) > 0 {
			data = append(data, '\n')
		}
		data = append(data, value...)
		if len(data) > maxEventBytes {
			return nil, fmt.Errorf("opencode: event payload exceeds %d bytes", maxEventBytes)
		}
	}
	if err := s.scanner.Err(); err != nil {
		return nil, fmt.Errorf("opencode: read event stream: %w", err)
	}
	if len(data) > 0 {
		return s.decode(data)
	}
	return nil, io.EOF
}

// decode parses one complete event payload and keeps its raw bytes.
func (s *EventStream) decode(data []byte) (*Event, error) {
	event := &Event{}
	if err := json.Unmarshal(data, event); err != nil {
		return nil, fmt.Errorf("opencode: decode event: %w", err)
	}
	event.raw = append([]byte(nil), data...)
	return event, nil
}

// Close closes the response body. It is safe to call more than once.
func (s *EventStream) Close() error {
	if s.closed {
		return nil
	}
	s.closed = true
	return s.body.Close()
}

// splitSSEField splits an "field: value" line, trimming one optional space.
func splitSSEField(line []byte) (string, []byte) {
	idx := bytes.IndexByte(line, ':')
	if idx < 0 {
		return string(line), nil
	}
	field := string(line[:idx])
	value := line[idx+1:]
	if len(value) > 0 && value[0] == ' ' {
		value = value[1:]
	}
	return field, value
}
