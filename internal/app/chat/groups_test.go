package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestChatGroupLifecycleAndCascade(t *testing.T) {
	service, repo, _ := testService(t, nil)

	// 1. List groups: the platform seeds the initial quick-assistant group as an unlocked assistant.
	groups, err := service.ListGroups()
	if err != nil {
		t.Fatalf("ListGroups: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("expected 1 seeded group, got %d", len(groups))
	}
	if groups[0].Name != QuickAssistantGroupName {
		t.Fatalf("unexpected seeded group name: %s", groups[0].Name)
	}

	// 2. Validation: empty name
	if _, err := service.CreateGroup(CreateGroupInput{Name: "   "}); !errors.Is(err, ErrInvalidGroupName) {
		t.Fatalf("expected ErrInvalidGroupName for empty name, got %v", err)
	}

	// Validation: name too long
	longName := strings.Repeat("a", MaxGroupNameRunes+1)
	if _, err := service.CreateGroup(CreateGroupInput{Name: longName}); !errors.Is(err, ErrInvalidGroupName) {
		t.Fatalf("expected ErrInvalidGroupName for too long name, got %v", err)
	}

	// 3. Create valid group
	groupA, err := service.CreateGroup(CreateGroupInput{Name: "分组 A"})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	if groupA.ID == 0 || groupA.Name != "分组 A" {
		t.Fatalf("unexpected group: %+v", groupA)
	}

	groupB, err := service.CreateGroup(CreateGroupInput{Name: "分组 B"})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}

	// 4. Update group name
	updatedB, err := service.UpdateGroup(groupB.ID, UpdateGroupInput{Name: "分组 B 重命名"})
	if err != nil {
		t.Fatalf("UpdateGroup: %v", err)
	}
	if updatedB.Name != "分组 B 重命名" {
		t.Fatalf("expected updated name, got %q", updatedB.Name)
	}

	// 5. Create chat with group
	chat1, err := service.Create(context.Background(), CreateInput{
		GroupID: &groupA.ID,
		ModelID: 1,
		Text:    "属于分组 A 的对话",
	})
	if err != nil {
		t.Fatalf("Create chat in group: %v", err)
	}
	if chat1.GroupID != groupA.ID {
		t.Fatalf("expected chat1.GroupID = %d, got %d", groupA.ID, chat1.GroupID)
	}

	// 6. Move chat1 to groupB
	updatedChat1, err := service.Update(chat1.ID, UpdateInput{
		GroupID: &groupB.ID,
	})
	if err != nil {
		t.Fatalf("Update chat groupId: %v", err)
	}
	if updatedChat1.GroupID != groupB.ID {
		t.Fatalf("expected chat1.GroupID = %d, got %d", groupB.ID, updatedChat1.GroupID)
	}

	// Move back to ungrouped (0)
	var zeroGroup uint = 0
	updatedChat1, err = service.Update(chat1.ID, UpdateInput{
		GroupID: &zeroGroup,
	})
	if err != nil {
		t.Fatalf("Update chat groupId to 0: %v", err)
	}
	if updatedChat1.GroupID != 0 {
		t.Fatalf("expected chat1.GroupID = 0, got %d", updatedChat1.GroupID)
	}

	// Create chat in groupA again
	chat2, err := service.Create(context.Background(), CreateInput{
		GroupID: &groupA.ID,
		ModelID: 1,
		Text:    "属于分组 A 的新对话",
	})
	if err != nil {
		t.Fatalf("Create chat2: %v", err)
	}

	// 7. Delete groupA cascades to chat2
	if err := service.DeleteGroup(groupA.ID); err != nil {
		t.Fatalf("DeleteGroup: %v", err)
	}

	// Verify groupA deleted
	if _, err := repo.GetGroup(groupA.ID); !errors.Is(err, ErrGroupNotFound) {
		t.Fatalf("expected ErrGroupNotFound, got %v", err)
	}

	// Verify chat2 deleted
	if _, err := repo.GetChat(chat2.ID); !errors.Is(err, ErrChatNotFound) {
		t.Fatalf("expected chat2 deleted by cascade, got %v", err)
	}

	// chat1 was moved to 0, so it should still exist
	if _, err := repo.GetChat(chat1.ID); err != nil {
		t.Fatalf("expected chat1 still exists: %v", err)
	}
}

func TestChatGroupHTTPHandlers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service, _, _ := testService(t, nil)
	r := gin.New()
	service.RegisterRoutes(r.Group("/api"))

	// 1. List groups
	req := httptest.NewRequest(http.MethodGet, "/api/chat-groups", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/chat-groups: %d, want 200", w.Code)
	}

	// 2. Create group
	body, _ := json.Marshal(map[string]any{"name": "HTTP 分组"})
	req = httptest.NewRequest(http.MethodPost, "/api/chat-groups", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("POST /api/chat-groups: %d, want 201: %s", w.Code, w.Body.String())
	}
	var created ChatGroup
	_ = json.Unmarshal(w.Body.Bytes(), &created)
	if created.ID == 0 || created.Name != "HTTP 分组" {
		t.Fatalf("unexpected created group: %+v", created)
	}

	// 3. Update group
	body, _ = json.Marshal(map[string]any{"name": "HTTP 分组修改"})
	req = httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/chat-groups/%d", created.ID), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("PUT /api/chat-groups/%d: %d, want 200: %s", created.ID, w.Code, w.Body.String())
	}

	// 4. Delete group
	req = httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/chat-groups/%d", created.ID), nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("DELETE /api/chat-groups/%d: %d, want 204", created.ID, w.Code)
	}
}

func TestAssistantAttributesAndUnlocked(t *testing.T) {
	fake := &fakeCompleter{}
	service, repo, _ := testService(t, fake)

	groups, err := service.ListGroups()
	if err != nil {
		t.Fatalf("ListGroups: %v", err)
	}
	if len(groups) == 0 {
		t.Fatal("expected at least one seeded group")
	}
	seeded := groups[0]

	// 1. Fixed group is removed/unlocked: can be updated with full assistant attributes
	updated, err := service.UpdateGroup(seeded.ID, UpdateGroupInput{
		Name:           "智能编程助手",
		DefaultModelID: 1,
		SystemPrompt:   "你是一个资深架构师，请严谨回答问题。",
		Icon:           "💻",
		ShowInFloat:    true,
	})
	if err != nil {
		t.Fatalf("UpdateGroup: %v", err)
	}
	if updated.Name != "智能编程助手" || updated.DefaultModelID != 1 || updated.SystemPrompt != "你是一个资深架构师，请严谨回答问题。" || updated.Icon != "💻" || !updated.ShowInFloat {
		t.Fatalf("unexpected updated assistant: %+v", updated)
	}

	// 2. Create another assistant
	other, err := service.CreateGroup(CreateGroupInput{
		Name:           "文案小助手",
		DefaultModelID: 1,
		SystemPrompt:   "你是一个文案专家。",
		Icon:           "✍️",
		ShowInFloat:    false,
	})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}

	// 3. Create chat under assistant: system prompt must be injected into completion context, but not in visible messages
	chat, err := service.Create(context.Background(), CreateInput{
		GroupID: &updated.ID,
		ModelID: 1,
		Text:    "请写一个快速排序算法",
	})
	if err != nil {
		t.Fatalf("Create chat under assistant: %v", err)
	}
	if chat.GroupID != updated.ID {
		t.Fatalf("expected chat.GroupID = %d, got %d", updated.ID, chat.GroupID)
	}

	// Wait briefly for generation goroutine to start
	time.Sleep(50 * time.Millisecond)

	fake.mu.Lock()
	reqCount := len(fake.requests)
	var firstReq CompletionRequest
	if reqCount > 0 {
		firstReq = fake.requests[0]
	}
	fake.mu.Unlock()

	if reqCount == 0 {
		t.Fatal("expected at least 1 completion request")
	}
	if len(firstReq.Messages) < 2 {
		t.Fatalf("expected at least 2 messages (system + user), got %d", len(firstReq.Messages))
	}
	if firstReq.Messages[0].Role != "system" || firstReq.Messages[0].Text != "你是一个资深架构师，请严谨回答问题。" {
		t.Fatalf("unexpected first message in completion: %+v", firstReq.Messages[0])
	}

	// Verify visible messages in repo do NOT contain system message
	messages, err := repo.ListMessages(chat.ID)
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	for _, msg := range messages {
		if msg.Role == "system" {
			t.Fatalf("system prompt should not be visible in chat messages: %+v", msg)
		}
	}

	// 4. Move chat between assistants is allowed without locked error
	moved, err := service.Update(chat.ID, UpdateInput{GroupID: &other.ID})
	if err != nil {
		t.Fatalf("Update chat groupId between assistants: %v", err)
	}
	if moved.GroupID != other.ID {
		t.Fatalf("expected moved.GroupID = %d, got %d", other.ID, moved.GroupID)
	}

	// 5. Delete assistant is allowed
	if err := service.DeleteGroup(other.ID); err != nil {
		t.Fatalf("DeleteGroup: %v", err)
	}
}

func TestChatListGroupFilterHTTP(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service, _, _ := testService(t, nil)
	r := gin.New()
	service.RegisterRoutes(r.Group("/api"))

	groups, err := service.ListGroups()
	if err != nil {
		t.Fatalf("ListGroups: %v", err)
	}
	seeded := groups[0]

	// Updating assistant via HTTP returns 200 with new attributes
	body, _ := json.Marshal(map[string]any{
		"name":           "更新助手名称",
		"defaultModelId": 1,
		"systemPrompt":   "系统提示词",
		"icon":           "🤖",
		"showInFloat":    true,
	})
	req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/chat-groups/%d", seeded.ID), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("PUT group: %d, want 200: %s", w.Code, w.Body.String())
	}
	var resp ChatGroup
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Name != "更新助手名称" || resp.Icon != "🤖" || !resp.ShowInFloat {
		t.Fatalf("unexpected group response: %+v", resp)
	}

	// A group filter returns only that group's chats.
	req = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/chats?groupId=%d&limit=10", seeded.ID), nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/chats?groupId: %d, want 200: %s", w.Code, w.Body.String())
	}
	var listed chatListResponse
	if err := json.Unmarshal(w.Body.Bytes(), &listed); err != nil {
		t.Fatalf("decode chat list: %v", err)
	}
	if listed.Total != 0 || len(listed.Chats) != 0 {
		t.Fatalf("empty group returned %+v", listed)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/chats?groupId=abc", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("invalid groupId: %d, want 400", w.Code)
	}
}
