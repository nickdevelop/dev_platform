package chat

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenAICompleterStreamsReasoningAndUsage(t *testing.T) {
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer sk-test" {
			t.Errorf("authorization = %q", r.Header.Get("Authorization"))
		}
		decoded, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(decoded, &body)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"think\"}}]}\n\n")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Hel\"}}]}\n\n")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"lo\"}}],\"usage\":{\"prompt_tokens\":3,\"completion_tokens\":2}}\n\n")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	completer := NewOpenAICompleter()
	var deltas []CompletionDelta
	err := completer.Stream(context.Background(), CompletionRequest{
		BaseURL: server.URL + "/v1",
		APIKey:  "sk-test",
		Model:   "test-model",
		Messages: []CompletionMessage{{
			Role: "user",
			Text: "hi",
			Images: []ImageAttachment{{
				ID: "img-1", Name: "a.png", MediaType: "image/png", Data: "aGVsbG8=", Size: 5,
			}},
		}},
	}, func(delta CompletionDelta) error {
		deltas = append(deltas, delta)
		return nil
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if len(deltas) != 3 {
		t.Fatalf("deltas = %+v, want 3", deltas)
	}
	if deltas[0].Reasoning != "think" || deltas[1].Text != "Hel" || deltas[2].Text != "lo" {
		t.Fatalf("deltas = %+v", deltas)
	}
	if deltas[2].Usage == nil || deltas[2].Usage.Input != 3 || deltas[2].Usage.Output != 2 {
		t.Fatalf("usage = %+v", deltas[2].Usage)
	}
	if body["model"] != "test-model" || body["stream"] != true {
		t.Fatalf("request body = %+v", body)
	}
	messages, _ := body["messages"].([]any)
	content, _ := messages[0].(map[string]any)["content"].([]any)
	if len(content) != 2 {
		t.Fatalf("image message content = %+v", content)
	}
}

func TestOpenAICompleterComplete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"标题"}}],"usage":{"prompt_tokens":2,"completion_tokens":1}}`)
	}))
	defer server.Close()

	completer := NewOpenAICompleter()
	text, usage, err := completer.Complete(context.Background(), CompletionRequest{
		BaseURL:  server.URL,
		Model:    "test-model",
		Messages: []CompletionMessage{{Role: "user", Text: "hello"}},
	}, 32)
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if text != "标题" || usage == nil || usage.Input != 2 || usage.Output != 1 {
		t.Fatalf("Complete = %q %+v", text, usage)
	}
}

func TestOpenAICompleterRedactsKeyFromErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, "invalid key sk-secret")
	}))
	defer server.Close()

	completer := NewOpenAICompleter()
	_, _, err := completer.Complete(context.Background(), CompletionRequest{
		BaseURL:  server.URL,
		APIKey:   "sk-secret",
		Model:    "test-model",
		Messages: []CompletionMessage{{Role: "user", Text: "hello"}},
	}, 0)
	if err == nil || !errors.Is(err, ErrUpstreamError) {
		t.Fatalf("error = %v, want ErrUpstreamError", err)
	}
	if strings.Contains(err.Error(), "sk-secret") {
		t.Fatalf("error leaked the key: %v", err)
	}
}

func TestOpenAICompleterNonStreamFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"fallback"}}]}`)
	}))
	defer server.Close()

	completer := NewOpenAICompleter()
	var text string
	err := completer.Stream(context.Background(), CompletionRequest{
		BaseURL:  server.URL,
		Model:    "test-model",
		Messages: []CompletionMessage{{Role: "user", Text: "hello"}},
	}, func(delta CompletionDelta) error {
		text += delta.Text
		return nil
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if text != "fallback" {
		t.Fatalf("text = %q", text)
	}
}
