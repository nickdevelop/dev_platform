package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestChatGroupLifecycleAndCascade(t *testing.T) {
	service, repo, _ := testService(t, nil)

	// 1. List groups initially empty
	groups, err := service.ListGroups()
	if err != nil {
		t.Fatalf("ListGroups: %v", err)
	}
	if len(groups) != 0 {
		t.Fatalf("expected 0 groups, got %d", len(groups))
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
	req = httptest.NewRequest(http.MethodPut, "/api/chat-groups/1", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("PUT /api/chat-groups/1: %d, want 200: %s", w.Code, w.Body.String())
	}

	// 4. Delete group
	req = httptest.NewRequest(http.MethodDelete, "/api/chat-groups/1", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("DELETE /api/chat-groups/1: %d, want 204", w.Code)
	}
}
