package opencode

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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
			Text string `json:"text"`
		}
		defer r.Body.Close()
		json.NewDecoder(r.Body).Decode(&body)
		text = body.Text
		io.WriteString(w, `{"data":{"id":"msg_1","sessionID":"ses_one"}}`)
	})

	if err := client.Prompt(context.Background(), "ses_one", "hello world"); err != nil {
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
