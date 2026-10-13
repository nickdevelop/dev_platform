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
	// Model is the session's current model, when the server reports one. It is
	// the model new assistant messages will run unless a switch intervenes.
	Model *ModelRef   `json:"model,omitempty"`
	Time  SessionTime `json:"time"`
}

// CreateSessionRequest is the body of POST /api/session.
type CreateSessionRequest struct {
	Title    string       `json:"title,omitempty"`
	Model    *ModelRef    `json:"model,omitempty"`
	Location *LocationRef `json:"location,omitempty"`
}

// Message is one entry of GET /api/session/{id}/message. The endpoint mixes
// conversation entries with turn markers: Type is "user", "assistant",
// "idle" or "model-switched". A user message carries its body in Text; an
// assistant message carries blocks in Content and the model that produced it
// in Model.
type Message struct {
	ID      string         `json:"id"`
	Type    string         `json:"type"`
	Text    string         `json:"text,omitempty"`
	Content []ContentBlock `json:"content,omitempty"`
	Model   *ModelRef      `json:"model,omitempty"`
	Time    MessageTime    `json:"time"`
}

// MessageTime is a message's wall-clock stamp in milliseconds since epoch.
type MessageTime struct {
	Created int64 `json:"created"`
}

// ContentBlock is one block of an assistant message: "text", "reasoning" or
// "tool". Reasoning blocks carry no normalized payload and are skipped by the
// caller, like pi's thinking blocks.
type ContentBlock struct {
	Type  string     `json:"type"`
	Text  string     `json:"text,omitempty"`
	ID    string     `json:"id,omitempty"`
	Name  string     `json:"name,omitempty"`
	State *ToolState `json:"state,omitempty"`
}

// ToolState is a tool block's execution state: the arguments it ran with and
// how it finished. It mirrors the v2 protocol union (streaming/running/
// completed/error): a finished block carries its result in Content (an array
// of content blocks, like the live session.tool.* events), while a failed one
// carries the failure in Error. There is no `output` field on the wire —
// reading one yields empty results after reopen.
type ToolState struct {
	Status  string           `json:"status"`
	Input   json.RawMessage  `json:"input,omitempty"`
	Content json.RawMessage  `json:"content,omitempty"`
	Error   *structuredError `json:"error,omitempty"`
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

// maxMessagePages bounds the cursor walk of a single history read: a server
// that answers every page with a fresh next cursor must not pin the read
// forever. The bound is generous against real transcripts — the normalized
// history itself is capped at agents.MaxHistoryEvents — and a read that hits
// it still returns the pages it collected.
const maxMessagePages = 64

// messagePage mirrors one page of GET /api/session/{id}/message: the
// messages in the requested order plus the opaque cursors for more pages.
type messagePage struct {
	Data   []Message `json:"data"`
	Cursor *struct {
		Previous *string `json:"previous"`
		Next     *string `json:"next"`
	} `json:"cursor"`
}

// ListMessages returns one session's native message log, oldest first. The
// server stores it on disk, so any running server answers for any session it
// knows the directory of.
//
// The endpoint defaults to newest first, so the first page asks for
// oldest-first explicitly; later pages follow cursor.next, which already
// encodes the ordering and must not be combined with order. Pages are
// followed until the server reports no next cursor, so a transcript longer
// than one page is read whole rather than silently truncated.
func (c *Client) ListMessages(ctx context.Context, sessionID string) ([]Message, error) {
	messages := make([]Message, 0, 64)
	query := url.Values{}
	query.Set("order", "asc")
	cursor := ""
	for pages := 0; ; pages++ {
		if cursor != "" {
			// A cursor carries the original request's ordering; sending
			// order alongside it is rejected by the server.
			query = url.Values{}
			query.Set("cursor", cursor)
		}
		var page messagePage
		if err := c.do(ctx, http.MethodGet, "/api/session/"+sessionID+"/message", query, nil, &page); err != nil {
			return nil, err
		}
		messages = append(messages, page.Data...)
		if page.Cursor == nil || page.Cursor.Next == nil || *page.Cursor.Next == "" {
			return messages, nil
		}
		if pages+1 >= maxMessagePages {
			return messages, nil
		}
		cursor = *page.Cursor.Next
	}
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

// ModelVariant mirrors Model.Variant: a named configuration of a model that a
// message may select with ModelRef.Variant.
type ModelVariant struct {
	ID string `json:"id"`
}

// ModelRecord mirrors Model.Info: one model the server can run. ProviderID and
// ModelID form the identity that ModelRef and the prompt route accept.
type ModelRecord struct {
	ID         string         `json:"id"`
	ModelID    string         `json:"modelID"`
	ProviderID string         `json:"providerID"`
	Name       string         `json:"name"`
	Variants   []ModelVariant `json:"variants,omitempty"`
}

// ListModels returns the models the server can run for the directory.
func (c *Client) ListModels(ctx context.Context, directory string) ([]ModelRecord, error) {
	query := url.Values{}
	if directory != "" {
		query.Set("directory", directory)
	}
	var payload struct {
		Data []ModelRecord `json:"data"`
	}
	if err := c.do(ctx, http.MethodGet, "/api/model", query, nil, &payload); err != nil {
		return nil, err
	}
	return payload.Data, nil
}

// Interrupt ends the running turn of one session. The server keeps the
// session and its other sessions.
func (c *Client) Interrupt(ctx context.Context, sessionID string) error {
	return c.do(ctx, http.MethodPost, sessionPath(sessionID)+"/interrupt", nil, nil, nil)
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

// PermissionRequest mirrors the subset of Permission.Request the platform
// needs to render an approval: identity plus human-readable context. Extra
// server fields are ignored so the client stays forward-compatible.
type PermissionRequest struct {
	ID        string   `json:"id"`
	SessionID string   `json:"sessionID"`
	Action    string   `json:"action"`
	Resources []string `json:"resources"`
	Message   string   `json:"message,omitempty"`
}

// ListPermissions returns the session's pending permission requests.
func (c *Client) ListPermissions(ctx context.Context, sessionID string) ([]PermissionRequest, error) {
	var payload struct {
		Data []PermissionRequest `json:"data"`
	}
	if err := c.do(ctx, http.MethodGet, sessionPath(sessionID)+"/permission", nil, nil, &payload); err != nil {
		return nil, err
	}
	return payload.Data, nil
}

// ReplyPermission answers a pending permission request. Decision is one of
// "once", "always" or "reject" (see Permission.Reply in the v2 protocol).
func (c *Client) ReplyPermission(ctx context.Context, sessionID, requestID, decision, message string) error {
	body := struct {
		Decision string `json:"decision"`
		Message  string `json:"message,omitempty"`
	}{Decision: decision, Message: message}
	return c.do(ctx, http.MethodPost, sessionPath(sessionID)+"/permission/"+url.PathEscape(requestID)+"/reply", nil, body, nil)
}

// FormInfo mirrors the subset of Form.Info the platform needs: identity,
// title and fields stay opaque to the backend client.
type FormInfo struct {
	ID        string          `json:"id"`
	SessionID string          `json:"sessionID"`
	Title     string          `json:"title"`
	Fields    json.RawMessage `json:"fields,omitempty"`
}

// ListForms returns the session's pending forms.
func (c *Client) ListForms(ctx context.Context, sessionID string) ([]FormInfo, error) {
	var payload struct {
		Data []FormInfo `json:"data"`
	}
	if err := c.do(ctx, http.MethodGet, sessionPath(sessionID)+"/form", nil, nil, &payload); err != nil {
		return nil, err
	}
	return payload.Data, nil
}

// ReplyForm submits an answer to a pending form.
func (c *Client) ReplyForm(ctx context.Context, sessionID, formID string, answer map[string]any) error {
	body := struct {
		Answer map[string]any `json:"answer"`
	}{Answer: answer}
	return c.do(ctx, http.MethodPost, sessionPath(sessionID)+"/form/"+url.PathEscape(formID)+"/reply", nil, body, nil)
}

// OpenRawEventStream opens the server's SSE event stream and returns the raw
// HTTP response. The caller owns the body and must close it. Unlike Events,
// no SSE parsing happens here: bytes are proxied untouched so the frontend
// can project OpenCodeEvent itself (desktop data.ts model).
func (c *Client) OpenRawEventStream(ctx context.Context) (*http.Response, error) {
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
	return response, nil
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

// redactSecrets masks the value of anything that looks like a credential. It
// delegates to the shared agents helper so every diagnostic path — the
// adapters, the collector's crash report and raw CLI output — uses one
// definition.
func redactSecrets(value string) string {
	return agents.RedactSecrets(value)
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

// errMalformedEvent marks an event payload the adapter cannot decode (invalid
// JSON, or a payload beyond the size bound). It is a stream-content failure,
// not a transport failure: the connection is dropped and reconnected, and the
// diagnostic carries the malformed_event code so the UI can distinguish a bad
// payload from a dead server.
var errMalformedEvent = errors.New("opencode: malformed event payload")

// EventStream reads SSE events from an open response body. It ignores comment
// lines and heartbeats and bounds each event payload.
type EventStream struct {
	body    io.ReadCloser
	scanner *bufio.Scanner
	closed  bool
	// onActivity, when set, fires on every line read from the server —
	// including SSE comment heartbeats, which are otherwise skipped. The
	// adapter's idle watchdog uses it to tell a live-but-quiet connection
	// (opencode sends ": heartbeat" every few seconds) from a hung one. Without
	// it, a stream that only sends heartbeats looks silent and is torn down.
	onActivity func()
}

// newEventStream wraps a response body.
func newEventStream(body io.ReadCloser) *EventStream {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), maxEventBytes)
	return &EventStream{body: body, scanner: scanner}
}

// SetActivity registers a callback invoked whenever a line arrives from the
// server, heartbeats included. It must be called before the first Next; it is
// not safe to change concurrently with reads.
func (s *EventStream) SetActivity(onActivity func()) {
	s.onActivity = onActivity
}

// Next returns the next event, io.EOF when the stream ends, or an error when
// the connection breaks or the payload exceeds the bound.
func (s *EventStream) Next() (*Event, error) {
	var data []byte
	for s.scanner.Scan() {
		if s.onActivity != nil {
			s.onActivity()
		}
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
			return nil, fmt.Errorf("%w: payload exceeds %d bytes", errMalformedEvent, maxEventBytes)
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

// decode parses one complete event payload and keeps its raw bytes. A payload
// that is not valid JSON is reported through errMalformedEvent.
func (s *EventStream) decode(data []byte) (*Event, error) {
	event := &Event{}
	if err := json.Unmarshal(data, event); err != nil {
		return nil, fmt.Errorf("%w: invalid JSON: %v", errMalformedEvent, err)
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
