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

	"github.com/gin-gonic/gin"
)

func TestChatGroupLifecycleAndCascade(t *testing.T) {
	service, repo, _ := testService(t, nil)

	// 1. List groups: the platform seeds the fixed quick-assistant group first.
	groups, err := service.ListGroups()
	if err != nil {
		t.Fatalf("ListGroups: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("expected 1 seeded group, got %d", len(groups))
	}
	if groups[0].SystemKey != SystemKeyQuickAssistant || groups[0].Name != QuickAssistantGroupName {
		t.Fatalf("unexpected seeded group: %+v", groups[0])
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

func TestQuickAssistantGroupIsLockedAndExclusive(t *testing.T) {
	service, _, _ := testService(t, nil)

	groups, err := service.ListGroups()
	if err != nil {
		t.Fatalf("ListGroups: %v", err)
	}
	var system ChatGroup
	for _, group := range groups {
		if group.SystemKey == SystemKeyQuickAssistant {
			system = group
			break
		}
	}
	if system.ID == 0 {
		t.Fatal("seeded quick-assistant group not found")
	}

	if _, err := service.UpdateGroup(system.ID, UpdateGroupInput{Name: "改名"}); !errors.Is(err, ErrGroupLocked) {
		t.Fatalf("UpdateGroup on reserved group: got %v, want ErrGroupLocked", err)
	}
	if err := service.DeleteGroup(system.ID); !errors.Is(err, ErrGroupLocked) {
		t.Fatalf("DeleteGroup on reserved group: got %v, want ErrGroupLocked", err)
	}

	normal, err := service.CreateGroup(CreateGroupInput{Name: "普通分组"})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}

	normalChat, err := service.Create(context.Background(), CreateInput{
		GroupID: &normal.ID,
		ModelID: 1,
		Text:    "普通对话",
	})
	if err != nil {
		t.Fatalf("Create normal chat: %v", err)
	}
	if _, err := service.Update(normalChat.ID, UpdateInput{GroupID: &system.ID}); !errors.Is(err, ErrGroupLocked) {
		t.Fatalf("move into reserved group: got %v, want ErrGroupLocked", err)
	}

	quickChat, err := service.Create(context.Background(), CreateInput{
		GroupID: &system.ID,
		ModelID: 1,
		Text:    "快捷助手对话",
	})
	if err != nil {
		t.Fatalf("Create quick chat: %v", err)
	}
	if _, err := service.Update(quickChat.ID, UpdateInput{GroupID: &normal.ID}); !errors.Is(err, ErrGroupLocked) {
		t.Fatalf("move out of reserved group: got %v, want ErrGroupLocked", err)
	}
	if _, err := service.Update(quickChat.ID, UpdateInput{GroupID: &system.ID}); err != nil {
		t.Fatalf("no-op move inside reserved group: %v", err)
	}

	chats, total, err := service.ListByGroup(system.ID, 10, 0)
	if err != nil {
		t.Fatalf("ListByGroup: %v", err)
	}
	if total != 1 || len(chats) != 1 || chats[0].ID != quickChat.ID {
		t.Fatalf("ListByGroup = %+v total=%d, want only chat %d", chats, total, quickChat.ID)
	}
}

func TestChatListGroupFilterAndLockedHTTP(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service, _, _ := testService(t, nil)
	r := gin.New()
	service.RegisterRoutes(r.Group("/api"))

	groups, err := service.ListGroups()
	if err != nil {
		t.Fatalf("ListGroups: %v", err)
	}
	system := groups[0]
	if system.SystemKey != SystemKeyQuickAssistant {
		t.Fatalf("first group is not the reserved one: %+v", system)
	}

	// The reserved group rejects rename and delete with 409.
	body, _ := json.Marshal(map[string]any{"name": "cannot"})
	req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/chat-groups/%d", system.ID), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("PUT reserved group: %d, want 409: %s", w.Code, w.Body.String())
	}
	var envelope struct {
		Error string `json:"error"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &envelope)
	if envelope.Error != "group_locked" {
		t.Fatalf("error = %q, want group_locked", envelope.Error)
	}

	req = httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/chat-groups/%d", system.ID), nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("DELETE reserved group: %d, want 409", w.Code)
	}

	// A group filter returns only that group's chats.
	req = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/chats?groupId=%d&limit=10", system.ID), nil)
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
		t.Fatalf("empty reserved group returned %+v", listed)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/chats?groupId=abc", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("invalid groupId: %d, want 400", w.Code)
	}
}
