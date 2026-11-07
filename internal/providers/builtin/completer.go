package builtin

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// completer is the embedded agent's OpenAI-compatible streaming client. It
// speaks the /chat/completions SSE dialect and aggregates the fragmented
// delta fields (content, reasoning_content and tool_calls) into a final
// assistant message. It is deliberately self-contained: the provider layer
// may not import the app layer's chat completer, and a ~200-line dialect
// reader is the price of the backward dependency rule.
type completer struct {
	client *http.Client
}

// chatMessage is one OpenAI chat message. Content is a JSON string, or the
// encoded parts array of a multimodal user message, or null on tool-call
// assistant messages.
type chatMessage struct {
	Role       string          `json:"role"`
	Content    json.RawMessage `json:"content"`
	ToolCalls  []wireToolCall  `json:"tool_calls,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
}

func textMessage(role, text string) chatMessage {
	encoded, _ := json.Marshal(text)
	return chatMessage{Role: role, Content: encoded}
}

// contentPart is one element of a multimodal content array.
type contentPart struct {
	Type     string       `json:"type"`
	Text     string       `json:"text,omitempty"`
	ImageURL *imageURLRef `json:"image_url,omitempty"`
}

// imageURLRef carries a base64 data URL for one image.
type imageURLRef struct {
	URL string `json:"url"`
}

// wireToolCall is a complete tool invocation on an assistant message.
type wireToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function wireFunctionCall `json:"function"`
}

// wireFunctionCall names the tool and carries its JSON argument object.
type wireFunctionCall struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// wireToolCallDelta is one streamed fragment of a tool call: arguments arrive
// split across many deltas and are joined by index.
type wireToolCallDelta struct {
	Index    int    `json:"index"`
	ID       string `json:"id,omitempty"`
	Function struct {
		Name      string `json:"name,omitempty"`
		Arguments string `json:"arguments,omitempty"`
	} `json:"function"`
}

// streamDelta is one parsed delta of an SSE choice.
type streamDelta struct {
	Text         string
	Reasoning    string
	ToolCalls    []wireToolCallDelta
	FinishReason string
}

// assistantResult is the finished reply of one completion call.
type assistantResult struct {
	Text      string
	Reasoning string
	ToolCalls []wireToolCall
	Finish    string
}

// toolSchema is one tool schema in the OpenAI wire format.
type toolSchema struct {
	Type     string           `json:"type"`
	Function toolFunctionSpec `json:"function"`
}

type toolFunctionSpec struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

// chatRequest is the wire request; note the model is required.
type chatRequest struct {
	Model     string        `json:"model"`
	Messages  []chatMessage `json:"messages"`
	Stream    bool          `json:"stream"`
	Tools     []toolSchema  `json:"tools,omitempty"`
	MaxTokens int           `json:"max_tokens,omitempty"`
}

// streamChat performs one streaming completion. Deltas are passed to onDelta
// as they arrive; the return value is the fully aggregated assistant message.
func (c *completer) streamChat(ctx context.Context, endpoint ModelEndpoint, history []chatMessage, tools []toolSchema, onDelta func(streamDelta)) (*assistantResult, error) {
	body, err := json.Marshal(chatRequest{Model: endpoint.ModelID, Messages: history, Stream: true, Tools: tools})
	if err != nil {
		return nil, fmt.Errorf("builtin: encode completion request: %w", err)
	}
	url := strings.TrimRight(endpoint.BaseURL, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("builtin: build completion request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if endpoint.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+endpoint.APIKey)
	}
	response, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("builtin: model service unreachable: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode >= 300 {
		if body, readErr := readBody(response.Body, 2048); readErr == nil {
			return nil, fmt.Errorf("builtin: model service error (http %d): %s", response.StatusCode, truncate(body, 500))
		}
		return nil, fmt.Errorf("builtin: model service error (http %d)", response.StatusCode)
	}

	reader := bufio.NewReaderSize(response.Body, 64*1024)
	result := &assistantResult{}
	toolCalls := make(map[int]*wireToolCall)

	for {
		line, readErr := readSSELine(reader)
		if readErr != nil {
			if errors.Is(readErr, io.EOF) || errors.Is(readErr, context.Canceled) || ctx.Err() != nil {
				break
			}
			return nil, fmt.Errorf("builtin: read model stream: %w", readErr)
		}
		if line == "" || !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "[DONE]" {
			break
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content          json.RawMessage     `json:"content"`
					ReasoningContent string              `json:"reasoning_content"`
					ToolCalls        []wireToolCallDelta `json:"tool_calls"`
				} `json:"delta"`
				FinishReason string `json:"finish_reason"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			// A malformed chunk is skipped, not fatal: streaming endpoints
			// occasionally emit keep-alives.
			continue
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		choice := chunk.Choices[0]
		delta := streamDelta{FinishReason: choice.FinishReason}
		if len(choice.Delta.Content) > 0 {
			if err := json.Unmarshal(choice.Delta.Content, &delta.Text); err != nil {
				// Content is either a string or null; a non-string cannot
				// become text, so decode as a free-form note.
				delta.Text = strings.TrimSpace(string(choice.Delta.Content))
			}
			result.Text += delta.Text
		}
		if choice.Delta.ReasoningContent != "" {
			delta.Reasoning = choice.Delta.ReasoningContent
			result.Reasoning += choice.Delta.ReasoningContent
		}
		for _, callDelta := range choice.Delta.ToolCalls {
			delta.ToolCalls = append(delta.ToolCalls, callDelta)
			call, ok := toolCalls[callDelta.Index]
			if !ok {
				call = &wireToolCall{Type: "function"}
				toolCalls[callDelta.Index] = call
			}
			if callDelta.ID != "" {
				call.ID = callDelta.ID
			}
			if callDelta.Function.Name != "" {
				call.Function.Name += callDelta.Function.Name
			}
			call.Function.Arguments = json.RawMessage(string(call.Function.Arguments) + callDelta.Function.Arguments)
		}
		if delta.FinishReason != "" {
			result.Finish = delta.FinishReason
		}
		if onDelta != nil && delta.Text+delta.Reasoning != "" {
			onDelta(delta)
		}
	}
	for _, call := range toolCalls {
		result.ToolCalls = append(result.ToolCalls, *call)
	}
	return result, nil
}

// readSSELine returns the next non-empty line, splitting on \r\n neighbors.
// It reports io.EOF at stream end so callers can finish aggregation.
func readSSELine(reader *bufio.Reader) (string, error) {
	line, err := reader.ReadString('\n')
	if len(line) == 0 && err != nil {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

// readBody drains up to limit bytes of a response body.
func readBody(r io.Reader, limit int64) (string, error) {
	if limit <= 0 {
		limit = 2048
	}
	data, err := io.ReadAll(io.LimitReader(r, limit))
	return string(data), err
}

// truncate bounds a string with an ellipsis.
func truncate(value string, max int) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= max {
		return strings.TrimSpace(string(runes))
	}
	return strings.TrimSpace(string(runes[:max])) + "…"
}
