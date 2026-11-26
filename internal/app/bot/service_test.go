package bot

import (
	"context"
	"encoding/json"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm/logger"

	"devplatform/internal/app/chat"
	"devplatform/internal/app/database"
	"devplatform/internal/app/events"
	"devplatform/internal/app/platformtools"
	"devplatform/internal/app/projects"
	"devplatform/internal/app/prompts"
	"devplatform/internal/app/secrets"
	"devplatform/internal/app/sessions"
	"devplatform/internal/app/sessions/turns"
	"devplatform/internal/app/settings"
	"devplatform/internal/core/agents"
	"devplatform/internal/core/channels"
)

type fakeChannelAdapter struct {
	mu       sync.Mutex
	inbound  chan channels.InboundMessage
	outbound []channels.OutboundMessage
	typing   []bool
	onSent   chan struct{}
}

func newFakeChannelAdapter() *fakeChannelAdapter {
	return &fakeChannelAdapter{
		inbound: make(chan channels.InboundMessage, 8),
		onSent:  make(chan struct{}, 8),
	}
}

func (f *fakeChannelAdapter) Descriptor() channels.ChannelDescriptor {
	return channels.ChannelDescriptor{
		Type:            "fakechat",
		Label:           "Fake IM",
		SupportsQRLogin: true,
	}
}

func (f *fakeChannelAdapter) StartLogin(ctx context.Context) (*channels.LoginSession, error) {
	return &channels.LoginSession{
		QRCode: "fake-qr",
		QRURL:  "https://example.invalid/qr",
		Wait: func(ctx context.Context, onStatus func(channels.LoginEvent)) (*channels.Credentials, error) {
			if onStatus != nil {
				onStatus(channels.LoginEvent{Status: channels.LoginScanned})
			}
			creds := &channels.Credentials{BotToken: "tok-1", AccountID: "bot-1"}
			if onStatus != nil {
				onStatus(channels.LoginEvent{Status: channels.LoginConfirmed, Credentials: creds})
			}
			return creds, nil
		},
	}, nil
}

func (f *fakeChannelAdapter) Listen(ctx context.Context, creds channels.Credentials, cursor string, cb channels.ListenCallbacks) error {
	if cb.OnReady != nil {
		cb.OnReady()
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg := <-f.inbound:
			if cb.OnMessage != nil {
				_ = cb.OnMessage(ctx, msg)
			}
		}
	}
}

func (f *fakeChannelAdapter) SendMessage(ctx context.Context, creds channels.Credentials, msg channels.OutboundMessage, maxLen int) error {
	f.mu.Lock()
	f.outbound = append(f.outbound, msg)
	f.mu.Unlock()
	select {
	case f.onSent <- struct{}{}:
	default:
	}
	return nil
}

func (f *fakeChannelAdapter) SendTyping(ctx context.Context, creds channels.Credentials, userID string, typing bool) error {
	f.mu.Lock()
	f.typing = append(f.typing, typing)
	f.mu.Unlock()
	return nil
}

type fakeChatBridge struct {
	mu       sync.Mutex
	groups   []chat.ChatGroup
	chats    map[uint]chat.Chat
	notices  []string
	reply    string
	nextID   uint
	onReply  func(chatID uint, text string)
}

func (f *fakeChatBridge) ListGroups() ([]chat.ChatGroup, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.groups, nil
}

func (f *fakeChatBridge) CreateGroup(input chat.CreateGroupInput) (chat.ChatGroup, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	g := chat.ChatGroup{ID: uint(len(f.groups) + 1), Name: input.Name, ShowInFloat: input.ShowInFloat}
	f.groups = append(f.groups, g)
	return g, nil
}

func (f *fakeChatBridge) UpdateGroup(id uint, input chat.UpdateGroupInput) (chat.ChatGroup, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.groups {
		if f.groups[i].ID == id {
			f.groups[i].Name = input.Name
			f.groups[i].ShowInFloat = input.ShowInFloat
			return f.groups[i], nil
		}
	}
	return chat.ChatGroup{ID: id, Name: input.Name, ShowInFloat: input.ShowInFloat}, nil
}

func (f *fakeChatBridge) CreateSilent(groupID uint, title string, modelID uint, systemPrompt string, welcomeText string) (chat.Chat, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.chats == nil {
		f.chats = make(map[uint]chat.Chat)
	}
	f.nextID++
	c := chat.Chat{ID: f.nextID, GroupID: groupID, Title: title, ModelID: modelID}
	f.chats[c.ID] = c
	return c, nil
}

func (f *fakeChatBridge) Update(chatID uint, input chat.UpdateInput) (chat.Chat, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	c := f.chats[chatID]
	if input.Title != nil {
		c.Title = *input.Title
	}
	if input.ModelID != nil {
		c.ModelID = *input.ModelID
	}
	f.chats[chatID] = c
	return c, nil
}

func (f *fakeChatBridge) Delete(chatID uint) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.chats, chatID)
	return nil
}

func (f *fakeChatBridge) SendAndWait(ctx context.Context, chatID uint, input chat.SendInput) (chat.Message, error) {
	if f.onReply != nil {
		f.onReply(chatID, input.Text)
	}
	return chat.Message{ID: 99, ChatID: chatID, Role: chat.RoleAssistant, Text: f.reply, Status: chat.StatusComplete}, nil
}

func (f *fakeChatBridge) AppendNotice(chatID uint, text string) error {
	f.mu.Lock()
	f.notices = append(f.notices, text)
	f.mu.Unlock()
	return nil
}

func (f *fakeChatBridge) Get(id uint) (chat.Chat, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.chats[id], nil
}

type fakeEngineProvider struct{}

func (fakeEngineProvider) Engines() (settings.EnginesSetting, error) {
	cfg := settings.DefaultEnginesSetting()
	cfg.Bot.EnabledTools = []string{"platform_list_projects"}
	return cfg, nil
}

type fakeSessionObserver struct {
	mu      sync.Mutex
	session sessions.Session
	history sessions.History
}

func (f *fakeSessionObserver) Get(projectID uint, sessionID string) (sessions.Session, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.session, nil
}

func (f *fakeSessionObserver) History(ctx context.Context, projectID uint, sessionID string) (sessions.History, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.history, nil
}

func TestBotChannelLifecycleInboundAndSessionWatch(t *testing.T) {
	dir := t.TempDir()
	db, err := database.Open(dir)
	if err != nil {
		t.Fatalf("database.Open: %v", err)
	}
	db.Logger = logger.Discard
	if err := database.Migrate(db); err != nil {
		t.Fatalf("database.Migrate: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})

	secStore, err := secrets.NewStore(filepath.Join(dir, "sec"))
	if err != nil {
		t.Fatalf("secrets.NewStore: %v", err)
	}
	adapter := newFakeChannelAdapter()
	reg := channels.NewRegistry()
	if err := reg.Register(adapter); err != nil {
		t.Fatalf("reg.Register: %v", err)
	}

	chatBridge := &fakeChatBridge{reply: "已为您创建会话 s-1"}
	projSvc := projects.NewService(db)
	promptSvc := prompts.NewService(prompts.NewRepository(db))
	toolsExec := platformtools.NewExecutor(projSvc, promptSvc)

	sessObs := &fakeSessionObserver{
		session: sessions.Session{ID: "s-1", ProjectID: 1, Status: sessions.SessionActive, Turn: turns.Running},
		history: sessions.History{Events: []agents.NormalizedEvent{
			{Type: agents.EventMessage, Message: &agents.MessagePayload{Role: "assistant", Text: "任务修改已完成"}},
		}},
	}

	svc := NewService(NewRepository(db), reg, secStore, fakeEngineProvider{}, chatBridge, toolsExec, sessObs, events.NewManager())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		svc.Close(ctx)
	})

	// 1. Create channel -> auto binds Chat
	ch, err := svc.CreateChannel(context.Background(), CreateChannelInput{
		Name:        "测试渠道",
		ChannelType: "fakechat",
	})
	if err != nil {
		t.Fatalf("CreateChannel: %v", err)
	}
	if ch.ChatID == 0 || ch.Status != StatusUnconfigured {
		t.Fatalf("unexpected created channel: %+v", ch)
	}
	if len(chatBridge.groups) != 1 || chatBridge.groups[0].ShowInFloat {
		t.Fatalf("expected bot group to stay hidden from quick assistant float bar: %+v", chatBridge.groups)
	}

	// 2. PolicyForChat strictly enforces BotConfig.EnabledTools
	policy, ok := svc.PolicyForChat(ch.ChatID)
	if !ok || policy.DisableTools || policy.Executor == nil {
		t.Fatalf("expected active bot policy for chat %d", ch.ChatID)
	}
	if defs := policy.Executor.Definitions(); len(defs) != 1 || defs[0].Function.Name != "platform_list_projects" {
		t.Fatalf("unexpected filtered policy tools: %+v", defs)
	}

	// 3. QR Login succeeds and auto-starts channel
	if _, err := svc.StartQRLogin(context.Background(), ch.ID); err != nil {
		t.Fatalf("StartQRLogin: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		cur, _ := svc.GetChannel(ch.ID)
		if cur.Status == StatusConnected && cur.HasCredentials {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	cur, _ := svc.GetChannel(ch.ID)
	if cur.Status != StatusConnected || !cur.HasCredentials {
		t.Fatalf("expected connected channel after QR login, got %+v", cur)
	}

	// 4. Inbound message from any user (no whitelist) -> Bot replies & context_token stored
	adapter.inbound <- channels.InboundMessage{
		ChannelType:  "fakechat",
		FromUserID:   "stranger@im.wechat",
		Text:         "帮我看看项目",
		ContextToken: "ctx-stranger",
		Timestamp:    time.Now().UTC(),
	}
	select {
	case <-adapter.onSent:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for outbound reply")
	}

	adapter.mu.Lock()
	if len(adapter.outbound) == 0 || adapter.outbound[0].ToUserID != "stranger@im.wechat" || adapter.outbound[0].ContextToken != "ctx-stranger" {
		t.Fatalf("unexpected outbound reply: %+v", adapter.outbound)
	}
	adapter.mu.Unlock()

	// Verify secret is never leaked in JSON DTO
	rawJSON, _ := json.Marshal(cur)
	if string(rawJSON) == "" || len(cur.AccountID) == 0 {
		t.Fatalf("expected non-empty account id in DTO: %+v", cur)
	}

	// 5. WatchSession pushes completion summary back to originating user
	svc.WatchSession(ch.ID, "stranger@im.wechat", 1, "s-1")
	time.Sleep(150 * time.Millisecond)
	sessObs.mu.Lock()
	sessObs.session.Turn = turns.Idle
	sessObs.mu.Unlock()

	select {
	case <-adapter.onSent:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for session completion notification")
	}

	adapter.mu.Lock()
	last := adapter.outbound[len(adapter.outbound)-1]
	adapter.mu.Unlock()
	if last.ContextToken != "ctx-stranger" || last.ToUserID != "stranger@im.wechat" {
		t.Fatalf("unexpected notification target: %+v", last)
	}
}
