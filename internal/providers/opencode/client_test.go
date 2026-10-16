package opencode

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"devplatform/internal/core/agents"
)

const testPassword = "s3cret-password-value"

func testClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client, err := NewClient(ClientOptions{
		BaseURL:  server.URL,
		Username: "opencode",
		Password: testPassword,
		Timeout:  5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return client
}

func TestClientInfo(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/info" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if user, pass, ok := r.BasicAuth(); !ok || user != "opencode" || pass != testPassword {
			t.Errorf("missing basic auth, got user=%q pass=%q ok=%v", user, pass, ok)
		}
		w.Header().Set("content-type", "application/json")
		io.WriteString(w, `{"version":"2.0.24","pid":42,"urls":["http://127.0.0.1:9"],"paths":{"tmp":"/tmp"}}`)
	})

	info, err := client.Info(context.Background())
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	if info.Version != "2.0.24" || info.PID != 42 || len(info.URLs) != 1 {
		t.Fatalf("unexpected info: %+v", info)
	}
}

func TestClientListSessionsMapsDirectoryQuery(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/session" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if got := r.URL.Query().Get("directory"); got != "/data/projects/demo" {
			t.Errorf("directory query = %q", got)
		}
		io.WriteString(w, `{"data":[
			{"id":"ses_one","projectID":"prj","title":"First","time":{"created":1,"updated":2},"location":{"directory":"/data/projects/demo"}},
			{"id":"ses_two","projectID":"prj","time":{"created":3,"updated":4},"location":{"directory":"/data/projects/demo"}}
		],"cursor":{}}`)
	})

	sessions, err := client.ListSessions(context.Background(), "/data/projects/demo")
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("got %d sessions, want 2", len(sessions))
	}
	if sessions[0].ID != "ses_one" || sessions[0].Title != "First" {
		t.Fatalf("unexpected first session: %+v", sessions[0])
	}
	if sessions[0].Location == nil || sessions[0].Location.Directory != "/data/projects/demo" {
		t.Fatalf("location not mapped: %+v", sessions[0].Location)
	}
}

func TestClientCreateSessionPostsOptions(t *testing.T) {
	var got map[string]any
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/session" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode body: %v", err)
		}
		w.Header().Set("content-type", "application/json")
		io.WriteString(w, `{"data":{"id":"ses_new","projectID":"prj","title":"Hello","time":{"created":1,"updated":1},"location":{"directory":"/data/projects/demo"}}}`)
	})

	info, err := client.CreateSession(context.Background(), CreateSessionRequest{
		Title:    "Hello",
		Location: &LocationRef{Directory: "/data/projects/demo"},
		Model:    &ModelRef{ProviderID: "prov", ID: "model"},
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if info.ID != "ses_new" || info.Title != "Hello" {
		t.Fatalf("unexpected session: %+v", info)
	}
	if got["title"] != "Hello" {
		t.Errorf("title sent = %v", got["title"])
	}
	loc, _ := got["location"].(map[string]any)
	if loc["directory"] != "/data/projects/demo" {
		t.Errorf("location sent = %v", got["location"])
	}
	model, _ := got["model"].(map[string]any)
	if model["providerID"] != "prov" || model["id"] != "model" {
		t.Errorf("model sent = %v", got["model"])
	}
}

func TestClientSwitchModel(t *testing.T) {
	var got map[string]any
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/session/ses_one/model" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		defer r.Body.Close()
		json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusNoContent)
	})

	err := client.SwitchModel(context.Background(), "ses_one", ModelRef{ProviderID: "prov", ID: "model", Variant: "fast"})
	if err != nil {
		t.Fatalf("SwitchModel: %v", err)
	}
	model, _ := got["model"].(map[string]any)
	if model["providerID"] != "prov" || model["id"] != "model" || model["variant"] != "fast" {
		t.Fatalf("model sent = %v", got["model"])
	}
}

func TestClientPromptPostsText(t *testing.T) {
	var text string
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/session/ses_one/prompt" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		var body struct {
			Text  string           `json:"text"`
			Files []FileAttachment `json:"files"`
		}
		defer r.Body.Close()
		json.NewDecoder(r.Body).Decode(&body)
		text = body.Text
		io.WriteString(w, `{"data":{"id":"msg_1","sessionID":"ses_one"}}`)
	})

	if err := client.Prompt(context.Background(), "ses_one", "hello world", nil); err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	if text != "hello world" {
		t.Fatalf("text sent = %q", text)
	}
}

func TestClientAuthAndErrorStatus(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		io.WriteString(w, `{"_tag":"UnauthorizedError","message":"Authentication required"}`)
	})

	_, err := client.Info(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error is not *APIError: %v", err)
	}
	if apiErr.StatusCode != http.StatusUnauthorized || apiErr.Code != "UnauthorizedError" {
		t.Fatalf("unexpected api error: %+v", apiErr)
	}
	if strings.Contains(err.Error(), testPassword) {
		t.Fatalf("password leaked in error: %v", err)
	}
}

func TestClientRedactsEchoedPassword(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		io.WriteString(w, `{"_tag":"ForbiddenError","message":"bad credential `+testPassword+` rejected"}`)
	})

	_, err := client.Info(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), testPassword) {
		t.Fatalf("password leaked in error: %v", err)
	}
	if !strings.Contains(err.Error(), "[REDACTED]") {
		t.Fatalf("expected redaction marker, got: %v", err)
	}
}

func TestClientTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
		io.WriteString(w, `{}`)
	}))
	t.Cleanup(server.Close)

	client, err := NewClient(ClientOptions{BaseURL: server.URL, Password: testPassword, Timeout: 30 * time.Millisecond})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	_, err = client.Info(context.Background())
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want deadline exceeded", err)
	}
}

func TestClientRespectsContextCancellation(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := client.Info(ctx)
	if err == nil {
		t.Fatal("expected cancellation error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context canceled", err)
	}
}

func TestEventStreamParsesEventsAndHeartbeats(t *testing.T) {
	content := strings.Join([]string{
		": heartbeat\n",
		"\n",
		"data: {\"id\":\"evt_1\",\"type\":\"server.connected\",\"data\":{}}\n",
		"\n",
		"data: {\"id\":\"evt_2\",\"type\":\"session.text.delta\",\"location\":{\"directory\":\"/x\"},\"data\":{\"sessionID\":\"ses_1\",\"delta\":\"hi\"}}\n",
		"\n",
	}, "")
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/event" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if got := r.Header.Get("accept"); !strings.Contains(got, "text/event-stream") {
			t.Errorf("accept = %q", got)
		}
		w.Header().Set("content-type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, content)
	})

	stream, err := client.Events(context.Background())
	if err != nil {
		t.Fatalf("Events: %v", err)
	}
	defer stream.Close()

	first, err := stream.Next()
	if err != nil {
		t.Fatalf("Next first: %v", err)
	}
	if first.Type != "server.connected" {
		t.Fatalf("first type = %q", first.Type)
	}
	second, err := stream.Next()
	if err != nil {
		t.Fatalf("Next second: %v", err)
	}
	if second.Type != "session.text.delta" || second.SessionID() != "ses_1" {
		t.Fatalf("unexpected second event: %+v", second)
	}

	if _, err := stream.Next(); !errors.Is(err, io.EOF) {
		t.Fatalf("expected EOF, got %v", err)
	}
}

func TestEventStreamHeartbeatCountsAsActivity(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "text/event-stream")
		// A comment heartbeat decodes to no event but is still bytes on the
		// wire; the idle watchdog relies on seeing it.
		io.WriteString(w, ": heartbeat\n\n")
		io.WriteString(w, "data: {\"id\":\"evt_1\",\"type\":\"server.connected\",\"data\":{}}\n\n")
	})

	stream, err := client.Events(context.Background())
	if err != nil {
		t.Fatalf("Events: %v", err)
	}
	defer stream.Close()

	activity := 0
	stream.SetActivity(func() { activity++ })

	if _, err := stream.Next(); err != nil {
		t.Fatalf("Next: %v", err)
	}
	if activity == 0 {
		t.Fatal("the heartbeat was not reported as stream activity")
	}
}

func TestEventStreamDisconnectMidStream(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, "data: {\"id\":\"evt_1\",\"type\":\"server.connected\",\"data\":{}}\n\n")
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		// Returning closes the response body mid-stream.
	})

	stream, err := client.Events(context.Background())
	if err != nil {
		t.Fatalf("Events: %v", err)
	}
	defer stream.Close()

	if _, err := stream.Next(); err != nil {
		t.Fatalf("first Next: %v", err)
	}
	if _, err := stream.Next(); err == nil {
		t.Fatal("expected error after mid-stream disconnect")
	}
}

func TestEventStreamMalformedPayload(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "text/event-stream")
		io.WriteString(w, "data: this is not json\n\n")
	})
	stream, err := client.Events(context.Background())
	if err != nil {
		t.Fatalf("Events: %v", err)
	}
	defer stream.Close()
	if _, err := stream.Next(); err == nil {
		t.Fatal("expected malformed payload error")
	}
}

// TestEventStreamParsesEventPastRawBound proves a single SSE event larger than
// the raw-blob bound (64KiB) is read whole. opencode's durable events carry
// complete payloads — a tool result's whole output, a text block's full
// snapshot — so a real turn routinely produces a line past the bound, and the
// old buffer sizing aborted the read with bufio.ErrTooLong ("token too long").
func TestEventStreamParsesEventPastRawBound(t *testing.T) {
	text := strings.Repeat("x", agents.MaxRawEventBytes*2)
	body := fmt.Sprintf(`{"id":"evt_big","type":"session.text.ended","data":{"sessionID":"ses_1","text":%q}}`, text)
	if len(body) <= agents.MaxRawEventBytes {
		t.Fatalf("test body is %d bytes, want more than the raw bound %d", len(body), agents.MaxRawEventBytes)
	}
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "text/event-stream")
		io.WriteString(w, "data: "+body+"\n\n")
	})
	stream, err := client.Events(context.Background())
	if err != nil {
		t.Fatalf("Events: %v", err)
	}
	defer stream.Close()

	event, err := stream.Next()
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if event.Type != "session.text.ended" {
		t.Fatalf("event type = %q", event.Type)
	}
	if len(event.Raw()) != len(body) {
		t.Fatalf("raw length = %d, want %d", len(event.Raw()), len(body))
	}
}

func TestEventStreamNonSuccessStatus(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		io.WriteString(w, `{"_tag":"UnauthorizedError","message":"Authentication required"}`)
	})
	_, err := client.Events(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewClientValidation(t *testing.T) {
	if _, err := NewClient(ClientOptions{}); err == nil {
		t.Fatal("expected error for missing base URL")
	}
	if _, err := NewClient(ClientOptions{BaseURL: "not-a-url"}); err == nil {
		t.Fatal("expected error for invalid base URL")
	}
}

func TestClientListModels(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/model" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if got := r.URL.Query().Get("directory"); got != "/work/app" {
			t.Errorf("directory query = %q, want /work/app", got)
		}
		w.Write([]byte(`{"data":[{"id":"m1","modelID":"model","providerID":"prov","name":"Model","variants":[{"id":"fast"}]}]}`))
	})

	models, err := client.ListModels(context.Background(), "/work/app")
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(models) != 1 || models[0].ProviderID != "prov" || models[0].ModelID != "model" ||
		models[0].Name != "Model" || len(models[0].Variants) != 1 || models[0].Variants[0].ID != "fast" {
		t.Fatalf("models = %+v", models)
	}
}

func TestClientInterrupt(t *testing.T) {
	var path, method string
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		path, method = r.URL.Path, r.Method
		w.WriteHeader(http.StatusNoContent)
	})

	if err := client.Interrupt(context.Background(), "ses_one"); err != nil {
		t.Fatalf("Interrupt: %v", err)
	}
	if method != http.MethodPost || path != "/api/session/ses_one/interrupt" {
		t.Fatalf("request = %s %s", method, path)
	}
}

// readMessagesFixture returns the structural transcript fixture shared by the
// client and adapter history tests.
func readMessagesFixture(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "messages.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return raw
}

// TestClientListMessagesRequestsAscendingOrder pins the ordering contract:
// the native endpoint defaults to newest first, so history must ask for
// oldest first explicitly or a replayed conversation renders reversed.
func TestClientListMessagesRequestsAscendingOrder(t *testing.T) {
	fixture := readMessagesFixture(t)
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("order"); got != "asc" {
			t.Errorf("order query = %q, want asc: the endpoint defaults to newest first", got)
		}
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write(fixture)
	})

	messages, err := client.ListMessages(context.Background(), "ses_1")
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(messages) != 5 {
		t.Fatalf("ListMessages returned %d messages, want 5", len(messages))
	}
	if messages[0].Type != "user" || messages[0].Text != "历史里的用户消息" {
		t.Errorf("messages[0] = %+v, want the oldest message first", messages[0])
	}
}

// TestClientListMessagesFollowsCursor pins that a multi-page transcript is
// read whole: the client follows cursor.next until it is empty, keeping page
// order, instead of stopping at the server's default page size.
func TestClientListMessagesFollowsCursor(t *testing.T) {
	var queries []string
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		queries = append(queries, r.URL.RawQuery)
		w.Header().Set("content-type", "application/json")
		if r.URL.Query().Get("cursor") == "" {
			io.WriteString(w, `{"data":[
				{"id":"m1","type":"user","text":"first","time":{"created":1}}
			],"cursor":{"previous":null,"next":"page2"}}`)
			return
		}
		// The cursor already encodes the ordering: the API forbids combining
		// it with order.
		if got := r.URL.Query().Get("order"); got != "" {
			t.Errorf("follow-up order query = %q, want empty when paging with a cursor", got)
		}
		io.WriteString(w, `{"data":[
			{"id":"m2","type":"user","text":"second","time":{"created":2}}
		],"cursor":{"previous":null,"next":null}}`)
	})

	messages, err := client.ListMessages(context.Background(), "ses_1")
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(queries) != 2 {
		t.Fatalf("ListMessages made %d requests, want 2 (first page + cursor follow-up)", len(queries))
	}
	if len(messages) != 2 || messages[0].Text != "first" || messages[1].Text != "second" {
		t.Fatalf("ListMessages = %+v, want both pages in order", messages)
	}
}
func TestClientListMessages(t *testing.T) {
	fixture := readMessagesFixture(t)
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/session/ses_1/message" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write(fixture)
	})

	messages, err := client.ListMessages(context.Background(), "ses_1")
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(messages) != 5 {
		t.Fatalf("ListMessages returned %d messages, want 5", len(messages))
	}
	if messages[0].Type != "user" || messages[0].Text != "历史里的用户消息" {
		t.Errorf("messages[0] = %+v, want the fixture user message", messages[0])
	}
	if messages[1].Type != "assistant" || len(messages[1].Content) != 3 {
		t.Fatalf("messages[1] = %+v, want the fixture assistant blocks", messages[1])
	}
	tool := messages[1].Content[2]
	if tool.Type != "tool" || tool.ID != "call_1" || tool.Name != "execute" || tool.State == nil ||
		tool.State.Status != "completed" || flattenContent(tool.State.Content) != "历史里的工具输出" {
		t.Errorf("assistant tool block = %+v, want the fixture tool call", tool)
	}
}
