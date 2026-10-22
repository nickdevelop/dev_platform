package chat

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestChatEventStreamSendsSnapshotAndDeltas(t *testing.T) {
	gin.SetMode(gin.TestMode)
	started := make(chan struct{})
	release := make(chan struct{})
	completer := &fakeCompleter{
		stream: func(ctx context.Context, req CompletionRequest, onDelta func(CompletionDelta) error) error {
			if err := onDelta(CompletionDelta{Text: "part"}); err != nil {
				return err
			}
			close(started)
			select {
			case <-release:
			case <-ctx.Done():
				return ctx.Err()
			}
			return onDelta(CompletionDelta{Text: "-end"})
		},
	}
	service, repo, _ := testService(t, completer)
	router := gin.New()
	service.RegisterRoutes(router)
	server := httptest.NewServer(router)
	defer server.Close()

	created := postChat(t, server.URL, `{"modelId":1,"text":"hello"}`)
	<-started

	response, err := http.Get(fmt.Sprintf("%s/chats/%d/events", server.URL, created.ID))
	if err != nil {
		t.Fatalf("GET events: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("events status = %d", response.StatusCode)
	}
	reader := bufio.NewReader(response.Body)
	events := readEvents(t, reader, 1)
	if events[0] != "snapshot" {
		t.Fatalf("first event = %q, want snapshot", events[0])
	}

	close(release)
	// Drain until the turn ends. The stream may already have buffered the
	// delta produced after release.
	deadline := time.After(3 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("turn ended event not observed")
		default:
		}
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("read event: %v", err)
		}
		if strings.HasPrefix(line, "event: turn") {
			break
		}
	}
	waitIdle(t, repo, created.ID)
	messages, _ := repo.ListMessages(created.ID)
	if len(messages) != 2 || messages[1].Text != "part-end" {
		t.Fatalf("messages = %+v", messages)
	}
}

func postChat(t *testing.T, base, body string) Chat {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, base+"/chats", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	payload, _ := io.ReadAll(response.Body)
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d body = %s", response.StatusCode, payload)
	}
	var chat Chat
	if err := json.Unmarshal(payload, &chat); err != nil {
		t.Fatalf("decode chat: %v", err)
	}
	return chat
}

func readEvents(t *testing.T, reader *bufio.Reader, count int) []string {
	t.Helper()
	var events []string
	for len(events) < count {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("read event: %v", err)
		}
		if strings.HasPrefix(line, "event: ") {
			events = append(events, strings.TrimSpace(strings.TrimPrefix(line, "event: ")))
		}
	}
	return events
}
