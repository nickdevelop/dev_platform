package chat

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"devplatform/internal/app/database"
	"devplatform/internal/app/modelconfig"
)

func openChatTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := database.Open(t.TempDir())
	if err != nil {
		t.Fatalf("database.Open: %v", err)
	}
	if err := database.Migrate(db); err != nil {
		t.Fatalf("database.Migrate: %v", err)
	}
	db.Logger = logger.Discard
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

type fakeResolver struct {
	mu     sync.Mutex
	models map[uint]modelconfig.ResolvedModel
	err    error
}

func newFakeResolver() *fakeResolver {
	return &fakeResolver{models: map[uint]modelconfig.ResolvedModel{
		1: {
			ID:            1,
			ServiceID:     1,
			ModelID:       "test-model",
			Name:          "Test Model",
			BaseURL:       "http://example.invalid/v1",
			APIKey:        "sk-test",
			SupportsImage: true,
			ContextTokens: 128000,
			OutputTokens:  4096,
		},
	}}
}

func (f *fakeResolver) ResolveModel(id uint) (modelconfig.ResolvedModel, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return modelconfig.ResolvedModel{}, f.err
	}
	model, ok := f.models[id]
	if !ok {
		return modelconfig.ResolvedModel{}, modelconfig.ErrModelNotFound
	}
	return model, nil
}

type fakeCompleter struct {
	mu       sync.Mutex
	stream   func(context.Context, CompletionRequest, func(CompletionDelta) error) error
	complete func(context.Context, CompletionRequest, int) (string, *Usage, error)
	requests []CompletionRequest
}

func (f *fakeCompleter) Stream(ctx context.Context, req CompletionRequest, onDelta func(CompletionDelta) error) error {
	f.mu.Lock()
	f.requests = append(f.requests, req)
	fn := f.stream
	f.mu.Unlock()
	if fn == nil {
		return onDelta(CompletionDelta{Text: "ok"})
	}
	return fn(ctx, req, onDelta)
}

func (f *fakeCompleter) Complete(ctx context.Context, req CompletionRequest, maxTokens int) (string, *Usage, error) {
	f.mu.Lock()
	f.requests = append(f.requests, req)
	fn := f.complete
	f.mu.Unlock()
	if fn == nil {
		return "", nil, errors.New("no title")
	}
	return fn(ctx, req, maxTokens)
}

func testService(t *testing.T, completer Completer) (*Service, Repository, *fakeResolver) {
	t.Helper()
	db := openChatTestDB(t)
	repo := NewGORMRepository(db)
	resolver := newFakeResolver()
	service := NewService(db, resolver, WithCompleter(completer))
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		service.Close(ctx)
	})
	return service, repo, resolver
}

func waitIdle(t *testing.T, repo Repository, chatID uint) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		chat, err := repo.GetChat(chatID)
		if err != nil {
			t.Fatalf("GetChat: %v", err)
		}
		_ = chat
		messages, err := repo.ListMessages(chatID)
		if err != nil {
			t.Fatalf("ListMessages: %v", err)
		}
		pending := false
		for _, message := range messages {
			if message.Status == StatusPending {
				pending = true
			}
		}
		if !pending {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("generation did not settle")
}
