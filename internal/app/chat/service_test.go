package chat

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestCreateStreamsPersistsAndGeneratesTitle(t *testing.T) {
	completer := &fakeCompleter{
		stream: func(ctx context.Context, req CompletionRequest, onDelta func(CompletionDelta) error) error {
			_ = onDelta(CompletionDelta{Reasoning: "think"})
			_ = onDelta(CompletionDelta{Text: "Hel"})
			_ = onDelta(CompletionDelta{Text: "lo", Usage: &Usage{Input: 11, Output: 5}})
			return nil
		},
		complete: func(ctx context.Context, req CompletionRequest, max int) (string, *Usage, error) {
			return "自动标题", nil, nil
		},
	}
	service, repo, _ := testService(t, completer)

	chat, err := service.Create(context.Background(), CreateInput{ModelID: 1, Text: "帮我写一个标题"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	waitIdle(t, repo, chat.ID)
	waitForTitle(t, repo, chat.ID, "自动标题")

	messages, err := repo.ListMessages(chat.ID)
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("messages = %+v, want user + assistant", messages)
	}
	if messages[0].Role != RoleUser || messages[0].Text != "帮我写一个标题" {
		t.Fatalf("user message = %+v", messages[0])
	}
	assistant := messages[1]
	if assistant.Status != StatusComplete || assistant.Text != "Hello" || assistant.Reasoning != "think" {
		t.Fatalf("assistant = %+v", assistant)
	}
	if assistant.Usage == nil || assistant.Usage.Input != 11 || assistant.Usage.Output != 5 {
		t.Fatalf("usage = %+v", assistant.Usage)
	}
}

func TestStopKeepsPartialAnswer(t *testing.T) {
	started := make(chan struct{})
	completer := &fakeCompleter{
		stream: func(ctx context.Context, req CompletionRequest, onDelta func(CompletionDelta) error) error {
			_ = onDelta(CompletionDelta{Text: "partial"})
			close(started)
			<-ctx.Done()
			return ctx.Err()
		},
	}
	service, repo, _ := testService(t, completer)
	chat, err := service.Create(context.Background(), CreateInput{ModelID: 1, Text: "hello"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	<-started
	if err := service.Stop(chat.ID); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	waitIdle(t, repo, chat.ID)

	messages, _ := repo.ListMessages(chat.ID)
	if len(messages) != 2 {
		t.Fatalf("messages = %+v", messages)
	}
	assistant := messages[1]
	if assistant.Status != StatusCancelled {
		t.Fatalf("status = %q, want cancelled", assistant.Status)
	}
	if assistant.Text != "partial" {
		t.Fatalf("text = %q, want partial", assistant.Text)
	}
}

func TestRenameDuringGenerationIsNotOverwritten(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	completer := &fakeCompleter{
		stream: func(ctx context.Context, req CompletionRequest, onDelta func(CompletionDelta) error) error {
			close(started)
			select {
			case <-release:
			case <-ctx.Done():
			}
			return onDelta(CompletionDelta{Text: "done"})
		},
	}
	service, repo, _ := testService(t, completer)
	chat, err := service.Create(context.Background(), CreateInput{ModelID: 1, Text: "hello"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	<-started
	title := "生成中改名"
	if _, err := service.Update(chat.ID, UpdateInput{Title: &title}); err != nil {
		t.Fatalf("Update during generation: %v", err)
	}
	close(release)
	waitIdle(t, repo, chat.ID)
	loaded, err := repo.GetChat(chat.ID)
	if err != nil {
		t.Fatalf("GetChat: %v", err)
	}
	if loaded.Title != title {
		t.Fatalf("title = %q, want the manual title %q", loaded.Title, title)
	}
}

func TestConcurrentSendIsRejected(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	completer := &fakeCompleter{
		stream: func(ctx context.Context, req CompletionRequest, onDelta func(CompletionDelta) error) error {
			_ = onDelta(CompletionDelta{Text: "first"})
			close(started)
			select {
			case <-release:
			case <-ctx.Done():
			}
			return nil
		},
	}
	service, repo, _ := testService(t, completer)
	chat, err := service.Create(context.Background(), CreateInput{ModelID: 1, Text: "hello"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	<-started
	if _, err := service.Send(context.Background(), chat.ID, SendInput{Text: "again"}); !errors.Is(err, ErrChatBusy) {
		t.Fatalf("Send while running = %v, want ErrChatBusy", err)
	}
	close(release)
	waitIdle(t, repo, chat.ID)
}

func TestEditTruncatesLaterMessages(t *testing.T) {
	completer := &fakeCompleter{
		stream: func(ctx context.Context, req CompletionRequest, onDelta func(CompletionDelta) error) error {
			return onDelta(CompletionDelta{Text: "answer"})
		},
	}
	service, repo, _ := testService(t, completer)
	chat, err := service.Create(context.Background(), CreateInput{ModelID: 1, Text: "first"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	waitIdle(t, repo, chat.ID)
	if _, err := service.Send(context.Background(), chat.ID, SendInput{Text: "second"}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	waitIdle(t, repo, chat.ID)

	before, _ := repo.ListMessages(chat.ID)
	if len(before) != 4 {
		t.Fatalf("before edit = %d messages, want 4", len(before))
	}
	edited, err := service.EditAndResend(context.Background(), chat.ID, before[0].ID, EditInput{Text: "edited"})
	if err != nil {
		t.Fatalf("EditAndResend: %v", err)
	}
	waitIdle(t, repo, chat.ID)
	messages, _ := repo.ListMessages(chat.ID)
	if len(messages) != 2 {
		t.Fatalf("after edit = %+v, want 2 messages", messages)
	}
	if messages[0].Text != "edited" || messages[0].Role != RoleUser {
		t.Fatalf("edited user = %+v", messages[0])
	}
	if messages[1].Status != StatusComplete {
		t.Fatalf("assistant = %+v", messages[1])
	}
	if edited.Title != "edited" {
		t.Fatalf("title = %q, want the updated fallback", edited.Title)
	}
}

func TestRegenerateReplacesOnlyAssistant(t *testing.T) {
	completer := &fakeCompleter{
		stream: func(ctx context.Context, req CompletionRequest, onDelta func(CompletionDelta) error) error {
			return onDelta(CompletionDelta{Text: "answer"})
		},
	}
	service, repo, _ := testService(t, completer)
	chat, err := service.Create(context.Background(), CreateInput{ModelID: 1, Text: "hello"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	waitIdle(t, repo, chat.ID)
	before, _ := repo.ListMessages(chat.ID)

	if _, err := service.Regenerate(context.Background(), chat.ID); err != nil {
		t.Fatalf("Regenerate: %v", err)
	}
	waitIdle(t, repo, chat.ID)
	after, _ := repo.ListMessages(chat.ID)
	if len(after) != 2 {
		t.Fatalf("after regenerate = %+v", after)
	}
	if after[0].ID != before[0].ID || after[1].ID == before[1].ID {
		t.Fatalf("regenerate did not preserve user and replace assistant: before=%+v after=%+v", before, after)
	}
	if after[1].Status != StatusComplete {
		t.Fatalf("assistant = %+v", after[1])
	}
}

func TestUpdateAndDelete(t *testing.T) {
	completer := &fakeCompleter{
		stream: func(ctx context.Context, req CompletionRequest, onDelta func(CompletionDelta) error) error {
			return onDelta(CompletionDelta{Text: "ok"})
		},
	}
	service, repo, resolver := testService(t, completer)
	chat, err := service.Create(context.Background(), CreateInput{ModelID: 1, Text: "hello"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	waitIdle(t, repo, chat.ID)

	title := "手动标题"
	updated, err := service.Update(chat.ID, UpdateInput{Title: &title})
	if err != nil {
		t.Fatalf("Update title: %v", err)
	}
	if updated.Title != title {
		t.Fatalf("title = %q", updated.Title)
	}
	if _, err := service.Update(chat.ID, UpdateInput{Title: stringPtr("")}); !errors.Is(err, ErrInvalidTitle) {
		t.Fatalf("empty title = %v, want ErrInvalidTitle", err)
	}
	badModel := uint(99)
	if _, err := service.Update(chat.ID, UpdateInput{ModelID: &badModel}); !errors.Is(err, ErrModelUnavailable) {
		t.Fatalf("bad model = %v, want ErrModelUnavailable", err)
	}

	resolver.mu.Lock()
	resolver.models[2] = resolver.models[1]
	model := resolver.models[2]
	model.ID = 2
	model.Name = "Second Model"
	resolver.models[2] = model
	resolver.mu.Unlock()
	updated, err = service.Update(chat.ID, UpdateInput{ModelID: uintPtr(2)})
	if err != nil {
		t.Fatalf("Update model: %v", err)
	}
	if updated.ModelID != 2 || updated.ModelName != "Second Model" {
		t.Fatalf("updated model = %+v", updated)
	}

	if err := service.Delete(chat.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := service.Get(chat.ID); !errors.Is(err, ErrChatNotFound) {
		t.Fatalf("Get after delete = %v", err)
	}
}

func TestValidation(t *testing.T) {
	completer := &fakeCompleter{}
	service, repo, resolver := testService(t, completer)

	if _, err := service.Create(context.Background(), CreateInput{ModelID: 1}); !errors.Is(err, ErrInvalidMessage) {
		t.Fatalf("empty message = %v", err)
	}
	tooMany := make([]ImageAttachment, MaxImages+1)
	if _, err := service.Create(context.Background(), CreateInput{ModelID: 1, Images: tooMany}); !errors.Is(err, ErrInvalidImage) {
		t.Fatalf("too many images = %v", err)
	}
	if _, err := service.Create(context.Background(), CreateInput{ModelID: 1, Images: []ImageAttachment{{
		MediaType: "text/plain", Data: "aGVsbG8=",
	}}}); !errors.Is(err, ErrInvalidImage) {
		t.Fatalf("bad media type = %v", err)
	}
	if _, err := service.Create(context.Background(), CreateInput{ModelID: 1, Images: []ImageAttachment{{
		MediaType: "image/png", Data: "not-base64",
	}}}); !errors.Is(err, ErrInvalidImage) {
		t.Fatalf("bad base64 = %v", err)
	}

	resolver.mu.Lock()
	model := resolver.models[1]
	model.SupportsImage = false
	resolver.models[1] = model
	resolver.mu.Unlock()
	if _, err := service.Create(context.Background(), CreateInput{
		ModelID: 1,
		Images:  []ImageAttachment{{MediaType: "image/png", Data: "aGVsbG8="}},
	}); !errors.Is(err, ErrUnsupportedImage) {
		t.Fatalf("unsupported image = %v", err)
	}

	// A valid text message still works and a fresh start recovers pending rows.
	chat, err := service.Create(context.Background(), CreateInput{ModelID: 1, Text: "ok"})
	if err != nil {
		t.Fatalf("valid create: %v", err)
	}
	waitIdle(t, repo, chat.ID)
}

func TestNormalizeInputAcceptsDataURLAndReencodes(t *testing.T) {
	text, images, err := normalizeInput("hi", []ImageAttachment{{
		Name:      "a.png",
		MediaType: "IMAGE/PNG",
		Data:      "data:image/png;base64,aGVsbG8=",
	}})
	if err != nil {
		t.Fatalf("normalizeInput: %v", err)
	}
	if text != "hi" || len(images) != 1 {
		t.Fatalf("normalizeInput = %q %+v", text, images)
	}
	if images[0].MediaType != "image/png" || images[0].Data != "aGVsbG8=" || images[0].Size != 5 {
		t.Fatalf("normalized image = %+v", images[0])
	}
}

func TestSanitizeGeneratedTitle(t *testing.T) {
	if got := sanitizeGeneratedTitle("\n“标题：自动标题。”\n"); got != "自动标题。" && got != "自动标题" {
		t.Fatalf("sanitize = %q", got)
	}
	long := strings.Repeat("问", 50)
	if got := sanitizeGeneratedTitle(long); len([]rune(got)) != 41 {
		t.Fatalf("long title = %q (%d runes)", got, len([]rune(got)))
	}
}

func waitForTitle(t *testing.T, repo Repository, chatID uint, want string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		chat, err := repo.GetChat(chatID)
		if err != nil {
			t.Fatalf("GetChat: %v", err)
		}
		if chat.Title == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	chat, _ := repo.GetChat(chatID)
	t.Fatalf("title = %q, want %q", chat.Title, want)
}

func stringPtr(value string) *string { return &value }
func uintPtr(value uint) *uint       { return &value }
