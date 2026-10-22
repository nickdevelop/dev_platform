package chat

import (
	"errors"
	"testing"
	"time"
)

func makeChat(title string) (Chat, Message, Message) {
	now := time.Now().UTC()
	return Chat{Title: title, ModelID: 1, ModelName: "Test Model", CreatedAt: now, UpdatedAt: now},
		Message{Role: RoleUser, Text: "hello", Status: StatusComplete, CreatedAt: now, UpdatedAt: now},
		Message{Role: RoleAssistant, Status: StatusPending, ModelID: 1, ModelName: "Test Model", CreatedAt: now, UpdatedAt: now}
}

func TestRepositoryCRUDAndTruncation(t *testing.T) {
	db := openChatTestDB(t)
	repo := NewGORMRepository(db)

	chat, user, assistant := makeChat("first")
	if err := repo.CreateChat(&chat, &user, &assistant); err != nil {
		t.Fatalf("CreateChat: %v", err)
	}
	if chat.ID == 0 || user.ID == 0 || assistant.ID == 0 {
		t.Fatalf("ids were not assigned: chat=%d user=%d assistant=%d", chat.ID, user.ID, assistant.ID)
	}

	loaded, err := repo.GetChat(chat.ID)
	if err != nil || loaded.Title != "first" {
		t.Fatalf("GetChat = %+v, %v", loaded, err)
	}
	messages, err := repo.ListMessages(chat.ID)
	if err != nil || len(messages) != 2 {
		t.Fatalf("ListMessages = %+v, %v", messages, err)
	}

	assistant.Text = "hi"
	assistant.Status = StatusComplete
	if err := repo.UpdateMessage(&assistant); err != nil {
		t.Fatalf("UpdateMessage: %v", err)
	}

	// Append a second exchange.
	secondUser := Message{Role: RoleUser, Text: "again", Status: StatusComplete}
	secondAssistant := Message{Role: RoleAssistant, Status: StatusPending, ModelID: 1, ModelName: "Test Model"}
	if err := repo.AppendExchange(&chat, &secondUser, &secondAssistant); err != nil {
		t.Fatalf("AppendExchange: %v", err)
	}
	messages, _ = repo.ListMessages(chat.ID)
	if len(messages) != 4 {
		t.Fatalf("messages after append = %d, want 4", len(messages))
	}

	// Editing the first user message truncates everything from it onward.
	editedUser := Message{Role: RoleUser, Text: "edited", Status: StatusComplete}
	editedAssistant := Message{Role: RoleAssistant, Status: StatusPending, ModelID: 1, ModelName: "Test Model"}
	if err := repo.ReplaceFromMessage(&chat, user.ID, &editedUser, &editedAssistant); err != nil {
		t.Fatalf("ReplaceFromMessage: %v", err)
	}
	messages, _ = repo.ListMessages(chat.ID)
	if len(messages) != 2 || messages[0].Text != "edited" {
		t.Fatalf("messages after edit = %+v", messages)
	}

	// Regenerating after the edited user message replaces the assistant.
	regen := Message{Role: RoleAssistant, Status: StatusPending, ModelID: 1, ModelName: "Test Model"}
	if err := repo.Regenerate(&chat, editedUser.ID, &regen); err != nil {
		t.Fatalf("Regenerate: %v", err)
	}
	messages, _ = repo.ListMessages(chat.ID)
	if len(messages) != 2 || messages[1].ID != regen.ID {
		t.Fatalf("messages after regenerate = %+v", messages)
	}

	// Recent messages report truncation without dropping the newest ones.
	recent, truncated, err := repo.ListRecentMessages(chat.ID, 1)
	if err != nil || !truncated || len(recent) != 1 {
		t.Fatalf("ListRecentMessages = %+v truncated=%v err=%v", recent, truncated, err)
	}

	if err := repo.DeleteChat(chat.ID); err != nil {
		t.Fatalf("DeleteChat: %v", err)
	}
	if _, err := repo.GetChat(chat.ID); !errors.Is(err, ErrChatNotFound) {
		t.Fatalf("GetChat after delete = %v, want ErrChatNotFound", err)
	}
	messages, _ = repo.ListMessages(chat.ID)
	if len(messages) != 0 {
		t.Fatalf("messages after delete = %+v", messages)
	}
}

func TestRepositoryImagesAndPendingRecovery(t *testing.T) {
	db := openChatTestDB(t)
	repo := NewGORMRepository(db)
	chat, user, assistant := makeChat("images")
	user.Images = []ImageAttachment{{ID: "img-1", Name: "a.png", MediaType: "image/png", Data: "aGVsbG8=", Size: 5}}
	if err := repo.CreateChat(&chat, &user, &assistant); err != nil {
		t.Fatalf("CreateChat: %v", err)
	}
	messages, _ := repo.ListMessages(chat.ID)
	if len(messages[0].Images) != 1 || messages[0].Images[0].Name != "a.png" {
		t.Fatalf("images did not round trip: %+v", messages[0].Images)
	}

	if err := repo.MarkPendingFailed("generation_interrupted", "interrupted"); err != nil {
		t.Fatalf("MarkPendingFailed: %v", err)
	}
	messages, _ = repo.ListMessages(chat.ID)
	if messages[1].Status != StatusFailed || messages[1].ErrorCode != "generation_interrupted" {
		t.Fatalf("assistant after recovery = %+v", messages[1])
	}
}
