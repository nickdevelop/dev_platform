package chat

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

	"devplatform/internal/app/secrets"
)

// Upstream failures are classified so the service can persist a stable code
// while keeping the human-readable detail useful and redacted.
var (
	ErrUpstreamUnreachable = errors.New("chat: upstream unreachable")
	ErrUpstreamError       = errors.New("chat: upstream error")
)

// CompletionMessage is one role/content pair sent upstream.
type CompletionMessage struct {
	Role   string
	Text   string
	Images []ImageAttachment
}

// CompletionRequest is one OpenAI-compatible chat completion request.
type CompletionRequest struct {
	BaseURL  string
	APIKey   string
	Model    string
	Messages []CompletionMessage
}

// CompletionDelta is one increment reported by the upstream stream.
type CompletionDelta struct {
	Text      string
	Reasoning string
	Usage     *Usage
}

// Completer is the upstream chat transport. Tests inject a fake; production
// uses the OpenAI-compatible implementation below.
type Completer interface {
	Stream(ctx context.Context, req CompletionRequest, onDelta func(CompletionDelta) error) error
	Complete(ctx context.Context, req CompletionRequest, maxTokens int) (string, *Usage, error)
}

// OpenAICompleter calls the OpenAI-compatible /chat/completions endpoint.
type OpenAICompleter struct {
	client *http.Client
}

// NewOpenAICompleter builds the production upstream client. It deliberately
// has no overall Timeout: a long generation is bounded by the caller's
// context, while connection establishment and response headers are bounded.
func NewOpenAICompleter() *OpenAICompleter {
	transport := &http.Transport{
		DialContext:           (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		IdleConnTimeout:       90 * time.Second,
		MaxIdleConnsPerHost:   4,
	}
	return &OpenAICompleter{client: &http.Client{Transport: transport}}
}

// maxUpstreamErrorBytes bounds an upstream error excerpt before redaction.
const maxUpstreamErrorBytes = 500

// maxSSELineBytes bounds one SSE line. Model chunks are small; the limit stops
// a hostile or broken server from growing the scanner without bound.
const maxSSELineBytes = 4 << 20

type upstreamMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type upstreamContentPart struct {
	Type     string            `json:"type"`
	Text     string            `json:"text,omitempty"`
	ImageURL *upstreamImageURL `json:"image_url,omitempty"`
}

type upstreamImageURL struct {
	URL string `json:"url"`
}

type upstreamRequest struct {
	Model     string            `json:"model"`
	Messages  []upstreamMessage `json:"messages"`
	Stream    bool              `json:"stream"`
	MaxTokens int               `json:"max_tokens,omitempty"`
}

type upstreamChunk struct {
	Choices []struct {
		Delta struct {
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content"`
			Reasoning        string `json:"reasoning"`
		} `json:"delta"`
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// Stream sends one streaming completion and invokes onDelta for every chunk.
// A context cancellation is returned as ctx.Err(), which the service maps to a
// cancelled turn. Errors never contain the API key: both the exact secret and
// credential-shaped text are redacted.
func (c *OpenAICompleter) Stream(ctx context.Context, req CompletionRequest, onDelta func(CompletionDelta) error) error {
	body, err := buildRequest(req, true, 0)
	if err != nil {
		return err
	}
	response, err := c.do(ctx, req, body, true)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return upstreamStatusError(response, req.APIKey)
	}
	if onDelta == nil {
		onDelta = func(CompletionDelta) error { return nil }
	}

	scanner := bufio.NewScanner(response.Body)
	scanner.Buffer(make([]byte, 64*1024), maxSSELineBytes)
	var fallbackJSON strings.Builder
	sawSSE := false
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}
		if strings.HasPrefix(line, "event:") {
			continue
		}
		if strings.HasPrefix(line, "data:") {
			sawSSE = true
			payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if payload == "" {
				continue
			}
			if payload == "[DONE]" {
				return nil
			}
			delta, done, err := parseChunk(payload)
			if err != nil {
				return err
			}
			if delta.Text != "" || delta.Reasoning != "" || delta.Usage != nil {
				if err := onDelta(delta); err != nil {
					return err
				}
			}
			if done {
				return nil
			}
			continue
		}
		// Some compatible servers answer a stream request with a single JSON
		// document. Collect it and parse it once the body ends.
		if strings.HasPrefix(strings.TrimSpace(line), "{") {
			if fallbackJSON.Len() > 0 {
				fallbackJSON.WriteByte('\n')
			}
			fallbackJSON.WriteString(line)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("%w: %v", ErrUpstreamError, redact(err.Error(), req.APIKey))
	}
	if !sawSSE && fallbackJSON.Len() > 0 {
		delta, err := parseNonStreamDocument(fallbackJSON.String())
		if err != nil {
			return err
		}
		if delta.Text != "" || delta.Reasoning != "" || delta.Usage != nil {
			return onDelta(delta)
		}
		return nil
	}
	return nil
}

// Complete sends one non-streaming completion, used for automatic titles.
func (c *OpenAICompleter) Complete(ctx context.Context, req CompletionRequest, maxTokens int) (string, *Usage, error) {
	body, err := buildRequest(req, false, maxTokens)
	if err != nil {
		return "", nil, err
	}
	response, err := c.do(ctx, req, body, false)
	if err != nil {
		return "", nil, err
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", nil, upstreamStatusError(response, req.APIKey)
	}
	payload, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil {
		return "", nil, fmt.Errorf("%w: read response: %v", ErrUpstreamUnreachable, redact(err.Error(), req.APIKey))
	}
	delta, err := parseNonStreamDocument(string(payload))
	if err != nil {
		return "", nil, err
	}
	return delta.Text, delta.Usage, nil
}

func buildRequest(req CompletionRequest, stream bool, maxTokens int) ([]byte, error) {
	if strings.TrimSpace(req.Model) == "" {
		return nil, fmt.Errorf("%w: model is empty", ErrInvalidMessage)
	}
	messages := make([]upstreamMessage, 0, len(req.Messages))
	for _, message := range req.Messages {
		content := any(strings.TrimSpace(message.Text))
		if len(message.Images) > 0 {
			parts := make([]upstreamContentPart, 0, len(message.Images)+1)
			if strings.TrimSpace(message.Text) != "" {
				parts = append(parts, upstreamContentPart{Type: "text", Text: message.Text})
			}
			for _, image := range message.Images {
				parts = append(parts, upstreamContentPart{
					Type: "image_url",
					ImageURL: &upstreamImageURL{
						URL: "data:" + image.MediaType + ";base64," + image.Data,
					},
				})
			}
			content = parts
		}
		messages = append(messages, upstreamMessage{Role: message.Role, Content: content})
	}
	body, err := json.Marshal(upstreamRequest{
		Model:     req.Model,
		Messages:  messages,
		Stream:    stream,
		MaxTokens: maxTokens,
	})
	if err != nil {
		return nil, fmt.Errorf("chat: encode upstream request: %w", err)
	}
	return body, nil
}

func (c *OpenAICompleter) do(ctx context.Context, req CompletionRequest, body []byte, stream bool) (*http.Response, error) {
	base := strings.TrimRight(strings.TrimSpace(req.BaseURL), "/")
	parsed, err := url.Parse(base)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, fmt.Errorf("%w: invalid base url", ErrUpstreamUnreachable)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUpstreamUnreachable, redact(err.Error(), req.APIKey))
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	if stream {
		httpReq.Header.Set("Accept", "text/event-stream")
	}
	if req.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+req.APIKey)
	}
	response, err := c.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUpstreamUnreachable, redact(err.Error(), req.APIKey))
	}
	return response, nil
}

func upstreamStatusError(response *http.Response, apiKey string) error {
	body, _ := io.ReadAll(io.LimitReader(response.Body, maxUpstreamErrorBytes))
	detail := strings.TrimSpace(string(body))
	if detail == "" {
		detail = http.StatusText(response.StatusCode)
	}
	return fmt.Errorf("%w: status %d: %s", ErrUpstreamError, response.StatusCode, redact(detail, apiKey))
}

func parseChunk(payload string) (CompletionDelta, bool, error) {
	var chunk upstreamChunk
	if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
		return CompletionDelta{}, false, fmt.Errorf("%w: parse stream chunk", ErrUpstreamError)
	}
	if chunk.Error != nil && chunk.Error.Message != "" {
		return CompletionDelta{}, false, fmt.Errorf("%w: %s", ErrUpstreamError, chunk.Error.Message)
	}
	delta := CompletionDelta{}
	if len(chunk.Choices) > 0 {
		delta.Text = chunk.Choices[0].Delta.Content
		delta.Reasoning = chunk.Choices[0].Delta.ReasoningContent
		if delta.Reasoning == "" {
			delta.Reasoning = chunk.Choices[0].Delta.Reasoning
		}
	}
	if chunk.Usage != nil {
		delta.Usage = &Usage{Input: chunk.Usage.PromptTokens, Output: chunk.Usage.CompletionTokens}
	}
	return delta, false, nil
}

func parseNonStreamDocument(payload string) (CompletionDelta, error) {
	var chunk upstreamChunk
	if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
		return CompletionDelta{}, fmt.Errorf("%w: parse response", ErrUpstreamError)
	}
	if chunk.Error != nil && chunk.Error.Message != "" {
		return CompletionDelta{}, fmt.Errorf("%w: %s", ErrUpstreamError, chunk.Error.Message)
	}
	delta := CompletionDelta{}
	if len(chunk.Choices) > 0 {
		delta.Text = chunk.Choices[0].Message.Content
		if delta.Text == "" {
			delta.Text = chunk.Choices[0].Delta.Content
		}
		delta.Reasoning = chunk.Choices[0].Delta.ReasoningContent
	}
	if chunk.Usage != nil {
		delta.Usage = &Usage{Input: chunk.Usage.PromptTokens, Output: chunk.Usage.CompletionTokens}
	}
	return delta, nil
}

// redact removes the exact secret and credential-shaped text from an error.
func redact(text, apiKey string) string {
	text = secrets.Redact(text, apiKey)
	text = strings.TrimSpace(text)
	if len(text) > maxUpstreamErrorBytes {
		text = text[:maxUpstreamErrorBytes]
	}
	return text
}
