package chat

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"devplatform/internal/app/events"
)

func TestChatRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	completer := &fakeCompleter{
		stream: func(ctx context.Context, req CompletionRequest, onDelta func(CompletionDelta) error) error {
			return onDelta(CompletionDelta{Text: "answer"})
		},
	}
	service, repo, _ := testService(t, completer)
	router := gin.New()
	service.RegisterRoutes(router)

	// Create with a first message.
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/chats", strings.NewReader(`{"modelId":1,"text":"hello"}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var created Chat
	if err := json.Unmarshal(recorder.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	if created.ID == 0 || created.Title != "hello" {
		t.Fatalf("created = %+v", created)
	}
	waitIdle(t, repo, created.ID)

	// List.
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/chats", nil))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"chats"`) {
		t.Fatalf("list status = %d body = %s", recorder.Code, recorder.Body.String())
	}

	// Get and rename.
	path := "/chats/" + strconv.FormatUint(uint64(created.ID), 10)
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("get status = %d", recorder.Code)
	}
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPut, path, strings.NewReader(`{"title":"renamed"}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("update status = %d body = %s", recorder.Code, recorder.Body.String())
	}

	// Send another message and stop it.
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, path+"/messages", strings.NewReader(`{"text":"again"}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("send status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, path+"/stop", nil))
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("stop status = %d", recorder.Code)
	}
	waitIdle(t, repo, created.ID)

	// Regenerate.
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, path+"/regenerate", nil))
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("regenerate status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	waitIdle(t, repo, created.ID)

	// Edit the first user message.
	messages, _ := repo.ListMessages(created.ID)
	userID := uint(0)
	for _, message := range messages {
		if message.Role == RoleUser {
			userID = message.ID
			break
		}
	}
	if userID == 0 {
		t.Fatal("no user message to edit")
	}
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, path+"/messages/"+strconv.FormatUint(uint64(userID), 10)+"/edit", strings.NewReader(`{"text":"edited"}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("edit status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	waitIdle(t, repo, created.ID)

	// Validation and deletion.
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/chats", strings.NewReader(`{"modelId":1}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "invalid_message") {
		t.Fatalf("empty create = %d %s", recorder.Code, recorder.Body.String())
	}
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodDelete, path, nil))
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d", recorder.Code)
	}
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("get after delete = %d", recorder.Code)
	}
}

func TestChatBusRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	completer := &fakeCompleter{}
	service, _, _ := testService(t, completer)
	eventsMgr := events.NewManager()
	eventsService := events.NewService(eventsMgr)
	eventsMgr.RegisterHandler(events.ChannelChat, service)
	service.SetEventsManager(eventsMgr)

	router := gin.New()
	service.RegisterRoutes(router)
	eventsService.RegisterRoutes(router)

	// Create a chat first
	chat, err := service.Create(context.Background(), CreateInput{
		ModelID: 1,
		Text:    "test chat bus",
	})
	if err != nil {
		t.Fatalf("create chat: %v", err)
	}

	busUUID := "bus-handlers-test-uuid"

	// 1. POST /events/bus/:uuid/subscriptions
	subPayload := `{"channel":"chat","targetId":"` + strconv.FormatUint(uint64(chat.ID), 10) + `"}`
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/events/bus/"+busUUID+"/subscriptions", strings.NewReader(subPayload))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("subscribe code = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var resp struct {
		Channel  string    `json:"channel"`
		TargetID string    `json:"targetId"`
		Snapshot *Snapshot `json:"snapshot"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode snapshot: %v", err)
	}
	if resp.Snapshot == nil || resp.Snapshot.Conversation.ID != chat.ID {
		t.Errorf("got snapshot chat ID %v, want %d", resp.Snapshot, chat.ID)
	}

	// 2. GET /events/bus/:uuid/subscriptions
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/events/bus/"+busUUID+"/subscriptions", nil)
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("list subs code = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var listResp struct {
		Subscriptions map[string][]string `json:"subscriptions"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("decode list subs: %v", err)
	}
	chatSubs := listResp.Subscriptions["chat"]
	if len(chatSubs) != 1 || chatSubs[0] != strconv.FormatUint(uint64(chat.ID), 10) {
		t.Errorf("got subscriptions %v, want [%d]", chatSubs, chat.ID)
	}

	// 3. DELETE /events/bus/:uuid/subscriptions/:channel/:targetId
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodDelete, "/events/bus/"+busUUID+"/subscriptions/chat/"+strconv.FormatUint(uint64(chat.ID), 10), nil)
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("unsubscribe code = %d, body = %s", recorder.Code, recorder.Body.String())
	}

	// Verify subscriptions empty
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/events/bus/"+busUUID+"/subscriptions", nil)
	router.ServeHTTP(recorder, request)
	var afterResp struct {
		Subscriptions map[string][]string `json:"subscriptions"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &afterResp); err != nil {
		t.Fatalf("decode list subs after delete: %v", err)
	}
	if len(afterResp.Subscriptions["chat"]) != 0 {
		t.Errorf("expected 0 subs, got %v", afterResp.Subscriptions["chat"])
	}
}
