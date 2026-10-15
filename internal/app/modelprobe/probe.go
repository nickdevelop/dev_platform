// Package modelprobe calls OpenAI-compatible model services on the operator's
// behalf: listing a service's models and sending a minimal test chat
// completion. Requests carry only the operator-supplied base URL and key; the
// key is never logged, echoed, or included in an error message.
package modelprobe

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"devplatform/internal/core/agents"
)

// Probe errors. Handlers map them to API error codes.
var (
	// ErrInvalidBaseURL reports a base URL that is not an absolute http(s) URL.
	ErrInvalidBaseURL = errors.New("modelprobe: invalid base url")
	// ErrUpstreamUnreachable reports a request that could not be delivered or
	// whose response could not be read.
	ErrUpstreamUnreachable = errors.New("modelprobe: upstream unreachable")
	// ErrUpstreamError reports a non-2xx response from the service.
	ErrUpstreamError = errors.New("modelprobe: upstream error")
)

// maxResponseBytes bounds how much of an upstream response is read.
const maxResponseBytes = 1 << 20

// requestTimeout bounds a single upstream call. A local model server that has
// not answered in this long is treated as unreachable.
const requestTimeout = 15 * time.Second

// RemoteModel is one model as the service's /models endpoint reports it.
type RemoteModel struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
}

// TestResult is the outcome of a successful model test.
type TestResult struct {
	ModelID   string `json:"modelId"`
	LatencyMs int64  `json:"latencyMs"`
}

// FetchModels calls GET {baseURL}/models and returns the service's models.
func FetchModels(ctx context.Context, baseURL, apiKey string) ([]RemoteModel, error) {
	base, err := normalizeBaseURL(baseURL)
	if err != nil {
		return nil, err
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/models", nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidBaseURL, err)
	}
	authorize(request, apiKey)

	body, err := do(request)
	if err != nil {
		return nil, err
	}

	var payload struct {
		Data []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("%w: parse model list", ErrUpstreamError)
	}

	models := make([]RemoteModel, 0, len(payload.Data))
	for _, entry := range payload.Data {
		if strings.TrimSpace(entry.ID) == "" {
			continue
		}
		models = append(models, RemoteModel{ID: entry.ID, Name: entry.Name})
	}
	return models, nil
}

// TestModel sends a minimal chat completion to {baseURL}/chat/completions for
// the given model and reports how long the service took to accept it.
func TestModel(ctx context.Context, baseURL, apiKey, modelID string) (TestResult, error) {
	base, err := normalizeBaseURL(baseURL)
	if err != nil {
		return TestResult{}, err
	}

	payload, err := json.Marshal(map[string]any{
		"model":      modelID,
		"messages":   []map[string]string{{"role": "user", "content": "ping"}},
		"max_tokens": 1,
	})
	if err != nil {
		return TestResult{}, fmt.Errorf("modelprobe: encode request: %w", err)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return TestResult{}, fmt.Errorf("%w: %v", ErrInvalidBaseURL, err)
	}
	request.Header.Set("Content-Type", "application/json")
	authorize(request, apiKey)

	start := time.Now()
	if _, err := do(request); err != nil {
		return TestResult{}, err
	}
	return TestResult{ModelID: modelID, LatencyMs: time.Since(start).Milliseconds()}, nil
}

// normalizeBaseURL trims a trailing slash and requires an absolute http(s) URL.
func normalizeBaseURL(raw string) (string, error) {
	base := strings.TrimRight(strings.TrimSpace(raw), "/")
	parsed, err := url.Parse(base)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", fmt.Errorf("%w: %q", ErrInvalidBaseURL, raw)
	}
	return base, nil
}

// authorize sets the bearer token when one is present.
func authorize(request *http.Request, apiKey string) {
	if apiKey != "" {
		request.Header.Set("Authorization", "Bearer "+apiKey)
	}
}

// do performs the request with a bounded timeout and response size, mapping
// transport and status failures to the probe errors. The response body is
// redacted before any part of it reaches an error.
func do(request *http.Request) ([]byte, error) {
	client := &http.Client{Timeout: requestTimeout}
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUpstreamUnreachable, agents.RedactSecrets(err.Error()))
	}
	defer response.Body.Close()

	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("%w: read response: %v", ErrUpstreamUnreachable, agents.RedactSecrets(err.Error()))
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("%w: status %d: %s", ErrUpstreamError, response.StatusCode, agents.RedactSecrets(truncate(body, 200)))
	}
	return body, nil
}

// truncate shortens a response excerpt for an error message.
func truncate(body []byte, limit int) string {
	text := strings.TrimSpace(string(body))
	if len(text) > limit {
		return text[:limit]
	}
	return text
}
