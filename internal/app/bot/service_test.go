package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm/logger"

	"devplatform/internal/app/chat"
	"devplatform/internal/app/database"
	"devplatform/internal/app/events"
	"devplatform/internal/app/projects"
	"devplatform/internal/app/prompts"
	"devplatform/internal/app/secrets"
	"devplatform/internal/app/sessions"
	"devplatform/internal/app/sessions/turns"
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
	mu      sync.Mutex
	groups  map[uint]chat.ChatGroup
	chats   map[uint]chat.Chat
	notices []string
	reply   string
	nextID  uint
	sentTo  []uint
	onReply func(chatID uint, text string)
}

func newFakeChatBridge() *fakeChatBridge {
	return &fakeChatBridge{
		groups: map[uint]chat.ChatGroup{
			1: {ID: 1, Name: "快捷助手", DefaultModelID: 9, EnablePlatformTools: true, MaxToolSteps: 8},
		},
		chats: make(map[uint]chat.Chat),
		reply: "AI 助手已回复",
	}
}

func (f *fakeChatBridge) GetGroup(id uint) (chat.ChatGroup, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	g, ok := f.groups[id]
	if !ok {
		return chat.ChatGroup{}, chat.ErrGroupNotFound
	}
	return g, nil
}

func (f *fakeChatBridge) CreateSilentWithMeta(groupID uint, title string, modelID uint, systemPrompt string, welcomeText string, source string, botChannelID uint) (chat.Chat, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextID++
	c := chat.Chat{
		ID:           f.nextID,
		GroupID:      groupID,
		Title:        title,
		ModelID:      modelID,
		Source:       source,
		BotChannelID: botChannelID,
	}
	f.chats[c.ID] = c
	return c, nil
}

func (f *fakeChatBridge) Update(chatID uint, input chat.UpdateInput) (chat.Chat, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	c, ok := f.chats[chatID]
	if !ok {
		return chat.Chat{}, chat.ErrChatNotFound
	}
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
	f.mu.Lock()
	f.sentTo = append(f.sentTo, chatID)
	f.mu.Unlock()
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
	c, ok := f.chats[id]
	if !ok {
		return chat.Chat{}, chat.ErrChatNotFound
	}
	return c, nil
}

type fakeSessionCtrl struct {
	mu           sync.Mutex
	nextID       int
	sessions     map[string]sessions.Session
	createOpts   []sessions.CreateOptions
	continuedIDs []string
	sentTexts    []string
	history      sessions.History
}

func newFakeSessionCtrl() *fakeSessionCtrl {
	return &fakeSessionCtrl{
		sessions: make(map[string]sessions.Session),
		history: sessions.History{Events: []agents.NormalizedEvent{
			{Type: agents.EventMessage, Message: &agents.MessagePayload{Role: "assistant", Text: "任务修改已完成"}},
		}},
	}
}

func (f *fakeSessionCtrl) CreateWithOptions(ctx context.Context, opts sessions.CreateOptions) (sessions.CreateOutcome, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextID++
	id := fmt.Sprintf("sess-%d", f.nextID)
	f.createOpts = append(f.createOpts, opts)
	title := opts.Title
	if title == "" && strings.TrimSpace(opts.Message) != "" {
		title = "DerivedTitle"
	}
	sess := sessions.Session{
		ID:             id,
		ProjectID:      opts.ProjectID,
		WorktreeID:     opts.WorktreeID,
		Backend:        opts.Backend,
		Title:          title,
		Model:          opts.Model,
		Source:         opts.Source,
		BotChannelID:   opts.BotChannelID,
		PermissionMode: opts.PermissionMode,
		Status:         sessions.SessionActive,
		Turn:           turns.Idle,
	}
	f.sessions[id] = sess
	return sessions.CreateOutcome{Session: sess}, nil
}

func (f *fakeSessionCtrl) Continue(ctx context.Context, projectID uint, sessionID string) (sessions.Session, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	sess, ok := f.sessions[sessionID]
	if !ok {
		return sessions.Session{}, sessions.ErrSessionNotFound
	}
	sess.Status = sessions.SessionActive
	f.sessions[sessionID] = sess
	f.continuedIDs = append(f.continuedIDs, sessionID)
	return sess, nil
}

func (f *fakeSessionCtrl) Send(ctx context.Context, projectID uint, sessionID, text, model string, images ...[]agents.ImageAttachment) (sessions.Session, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	sess, ok := f.sessions[sessionID]
	if !ok {
		return sessions.Session{}, sessions.ErrSessionNotFound
	}
	if sess.Title == "" {
		sess.Title = "SentTitle"
	}
	f.sentTexts = append(f.sentTexts, text)
	f.sessions[sessionID] = sess
	return sess, nil
}

func (f *fakeSessionCtrl) Get(projectID uint, sessionID string) (sessions.Session, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	sess, ok := f.sessions[sessionID]
	if !ok {
		return sessions.Session{}, sessions.ErrSessionNotFound
	}
	return sess, nil
}

func (f *fakeSessionCtrl) History(ctx context.Context, projectID uint, sessionID string) (sessions.History, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.history, nil
}

type fakePromptReader struct {
	items map[uint]prompts.Prompt
}

func (f *fakePromptReader) Get(id uint) (prompts.Prompt, error) {
	if p, ok := f.items[id]; ok {
		return p, nil
	}
	return prompts.Prompt{}, prompts.ErrPromptNotFound
}

type fakeProjectChecker struct {
	items map[uint]projects.Project
}

func (f *fakeProjectChecker) Get(id uint) (projects.Project, error) {
	if p, ok := f.items[id]; ok {
		return p, nil
	}
	return projects.Project{}, projects.ErrProjectNotFound
}

func setupBotTestEnv(t *testing.T) (*Service, *fakeChannelAdapter, *fakeChatBridge, *fakeSessionCtrl) {
	t.Helper()
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

	chatBridge := newFakeChatBridge()
	sessCtrl := newFakeSessionCtrl()
	promptReader := &fakePromptReader{
		items: map[uint]prompts.Prompt{
			5: {ID: 5, Name: "后端架构师", Type: prompts.PromptTypeRole, Content: "请以严谨的架构师标准分析代码。"},
		},
	}
	projChecker := &fakeProjectChecker{
		items: map[uint]projects.Project{
			7: {ID: 7, Name: "CoreRepo"},
		},
	}

	svc := NewService(
		NewRepository(db),
		reg,
		secStore,
		chatBridge,
		sessCtrl,
		promptReader,
		projChecker,
		agents.BackendBuiltin,
		events.NewManager(),
	)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		svc.Close(ctx)
	})
	return svc, adapter, chatBridge, sessCtrl
}

func waitChannelConnected(t *testing.T, svc *Service, id uint) Channel {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		cur, _ := svc.GetChannel(id)
		if cur.Status == StatusConnected && cur.HasCredentials {
			return cur
		}
		time.Sleep(20 * time.Millisecond)
	}
	cur, _ := svc.GetChannel(id)
	t.Fatalf("expected connected channel after QR login, got %+v", cur)
	return cur
}

func waitOutbound(t *testing.T, adapter *fakeChannelAdapter) channels.OutboundMessage {
	t.Helper()
	select {
	case <-adapter.onSent:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for outbound reply")
	}
	adapter.mu.Lock()
	defer adapter.mu.Unlock()
	return adapter.outbound[len(adapter.outbound)-1]
}

func TestAssistantModeChannelLifecycleLazyChatAndNewCommand(t *testing.T) {
	svc, adapter, chatBridge, _ := setupBotTestEnv(t)

	// Missing chatGroupId is rejected in assistant mode.
	if _, err := svc.CreateChannel(context.Background(), CreateChannelInput{
		Name:        "无效助手渠道",
		ChannelType: "fakechat",
		TargetMode:  TargetModeAssistant,
	}); err == nil {
		t.Fatal("expected error when chatGroupId is missing in assistant mode")
	}

	// Create assistant channel -> chatId starts 0 (lazy single-channel creation).
	ch, err := svc.CreateChannel(context.Background(), CreateChannelInput{
		Name:        "快捷助手微信",
		ChannelType: "fakechat",
		TargetMode:  TargetModeAssistant,
		ChatGroupID: 1,
	})
	if err != nil {
		t.Fatalf("CreateChannel: %v", err)
	}
	if ch.TargetMode != TargetModeAssistant || ch.ChatID != 0 || ch.ChatGroupID != 1 {
		t.Fatalf("unexpected created channel: %+v", ch)
	}

	if _, err := svc.StartQRLogin(context.Background(), ch.ID); err != nil {
		t.Fatalf("StartQRLogin: %v", err)
	}
	cur := waitChannelConnected(t, svc, ch.ID)

	rawJSON, _ := json.Marshal(cur)
	if len(cur.AccountID) == 0 || string(rawJSON) == "" {
		t.Fatalf("expected accountId in DTO: %+v", cur)
	}

	// 1. First inbound message lazily creates source="bot" chat under group 1 and replies.
	adapter.inbound <- channels.InboundMessage{
		ChannelType:  "fakechat",
		FromUserID:   "wx-user-1",
		Text:         "帮我总结一下需求",
		ContextToken: "ctx-1",
		Timestamp:    time.Now().UTC(),
	}
	reply1 := waitOutbound(t, adapter)
	if reply1.ToUserID != "wx-user-1" || reply1.ContextToken != "ctx-1" || reply1.Text != "AI 助手已回复" {
		t.Fatalf("unexpected first reply: %+v", reply1)
	}

	updated, _ := svc.GetChannel(ch.ID)
	if updated.ChatID == 0 {
		t.Fatal("expected lazy ChatID bound to channel after first inbound message")
	}
	firstChatID := updated.ChatID
	createdChat := chatBridge.chats[firstChatID]
	if createdChat.Source != chat.SourceBot || createdChat.BotChannelID != ch.ID || createdChat.GroupID != 1 {
		t.Fatalf("expected bot-tagged chat bound to channel, got %+v", createdChat)
	}

	// PolicyForChat only attaches watch callback when AutoNotify=true, without altering tools/model/prompt.
	policy, ok := svc.PolicyForChat(firstChatID)
	if !ok || policy.DisableTools || policy.Executor != nil || policy.ExtraSystemPrompt != "" || policy.WrapContext == nil {
		t.Fatalf("expected lightweight watch-only policy, got %+v (ok=%v)", policy, ok)
	}

	// 2. Send "/new" -> switches channel to a new source="bot" chat.
	adapter.inbound <- channels.InboundMessage{
		ChannelType:  "fakechat",
		FromUserID:   "wx-user-1",
		Text:         "/new",
		ContextToken: "ctx-2",
		Timestamp:    time.Now().UTC(),
	}
	reply2 := waitOutbound(t, adapter)
	if !strings.Contains(reply2.Text, "已创建并切换至新的 AI 助手对话") {
		t.Fatalf("unexpected /new reply: %+v", reply2)
	}
	afterNew, _ := svc.GetChannel(ch.ID)
	if afterNew.ChatID == 0 || afterNew.ChatID == firstChatID {
		t.Fatalf("expected new ChatID after /new, got old=%d new=%d", firstChatID, afterNew.ChatID)
	}
	secondChatID := afterNew.ChatID
	if chatBridge.chats[secondChatID].Source != chat.SourceBot || chatBridge.chats[secondChatID].BotChannelID != ch.ID {
		t.Fatalf("new chat missing bot source tag: %+v", chatBridge.chats[secondChatID])
	}

	// 3. Send "新建会话 帮我写测试" -> creates 3rd chat AND sends "帮我写测试" immediately.
	adapter.inbound <- channels.InboundMessage{
		ChannelType:  "fakechat",
		FromUserID:   "wx-user-1",
		Text:         "新建会话 帮我写测试",
		ContextToken: "ctx-3",
		Timestamp:    time.Now().UTC(),
	}
	reply3 := waitOutbound(t, adapter)
	if reply3.Text != "AI 助手已回复" {
		t.Fatalf("unexpected inline new+prompt reply: %+v", reply3)
	}
	afterInlineNew, _ := svc.GetChannel(ch.ID)
	if afterInlineNew.ChatID == secondChatID {
		t.Fatalf("expected third ChatID after '新建会话 <prompt>', still %d", afterInlineNew.ChatID)
	}
}

func TestAgentModeChannelAutoApproveRoleInjectionAndNewSession(t *testing.T) {
	svc, adapter, _, sessCtrl := setupBotTestEnv(t)

	// Missing defaultProjectId is rejected in agent mode.
	if _, err := svc.CreateChannel(context.Background(), CreateChannelInput{
		Name:        "无效Agent渠道",
		ChannelType: "fakechat",
		TargetMode:  TargetModeAgent,
	}); err == nil {
		t.Fatal("expected error when defaultProjectId is missing in agent mode")
	}

	ch, err := svc.CreateChannel(context.Background(), CreateChannelInput{
		Name:             "架构师研发渠道",
		ChannelType:      "fakechat",
		TargetMode:       TargetModeAgent,
		DefaultProjectID: 7,
		WorktreeID:       2,
		PromptID:         5,
		AgentModel:       "gpt-5-codex",
	})
	if err != nil {
		t.Fatalf("CreateChannel agent: %v", err)
	}
	if ch.TargetMode != TargetModeAgent || ch.DefaultBackend != string(agents.BackendBuiltin) || ch.ActiveSessionID != "" {
		t.Fatalf("unexpected agent channel DTO: %+v", ch)
	}

	if _, err := svc.StartQRLogin(context.Background(), ch.ID); err != nil {
		t.Fatalf("StartQRLogin: %v", err)
	}
	waitChannelConnected(t, svc, ch.ID)

	// 1. First inbound message lazily creates an auto-approved bot session with <Role> prefix.
	adapter.inbound <- channels.InboundMessage{
		ChannelType:  "fakechat",
		FromUserID:   "wx-dev-1",
		Text:         "排查路由并发问题",
		ContextToken: "ctx-a",
		Timestamp:    time.Now().UTC(),
	}
	reply1 := waitOutbound(t, adapter)
	if reply1.Text != "任务修改已完成" {
		t.Fatalf("unexpected first agent reply: %+v", reply1)
	}

	chAfter1, _ := svc.GetChannel(ch.ID)
	if chAfter1.ActiveSessionID != "sess-1" {
		t.Fatalf("expected ActiveSessionID=sess-1, got %q", chAfter1.ActiveSessionID)
	}
	if len(sessCtrl.createOpts) != 1 {
		t.Fatalf("expected 1 CreateWithOptions call, got %d", len(sessCtrl.createOpts))
	}
	c1 := sessCtrl.createOpts[0]
	if c1.Source != "bot" || c1.BotChannelID != ch.ID || c1.PermissionMode != "auto" || c1.Backend != agents.BackendBuiltin || c1.WorktreeID != 2 || c1.Model != "gpt-5-codex" {
		t.Fatalf("unexpected session create options: %+v", c1)
	}
	if !strings.Contains(c1.Message, `<Role name="后端架构师">`) || !strings.Contains(c1.Message, "排查路由并发问题") {
		t.Fatalf("expected <Role> wrapper in first message, got:\n%s", c1.Message)
	}

	// 2. Simulate restart disconnection, then send follow-up message -> resumes existing session without repeating <Role>.
	sessCtrl.mu.Lock()
	s1 := sessCtrl.sessions["sess-1"]
	s1.Status = sessions.SessionDisconnected
	sessCtrl.sessions["sess-1"] = s1
	sessCtrl.mu.Unlock()

	adapter.inbound <- channels.InboundMessage{
		ChannelType:  "fakechat",
		FromUserID:   "wx-dev-1",
		Text:         "补充单元测试",
		ContextToken: "ctx-b",
		Timestamp:    time.Now().UTC(),
	}
	reply2 := waitOutbound(t, adapter)
	if reply2.Text != "任务修改已完成" {
		t.Fatalf("unexpected second agent reply: %+v", reply2)
	}
	if len(sessCtrl.continuedIDs) != 1 || sessCtrl.continuedIDs[0] != "sess-1" {
		t.Fatalf("expected Continue to resume sess-1, got %+v", sessCtrl.continuedIDs)
	}
	if len(sessCtrl.sentTexts) != 1 || sessCtrl.sentTexts[0] != "补充单元测试" {
		t.Fatalf("expected raw follow-up text without duplicate <Role>, got %+v", sessCtrl.sentTexts)
	}

	// 3. Bare "new" command creates untitled sess-2; subsequent message injects <Role> cleanly.
	adapter.inbound <- channels.InboundMessage{
		ChannelType:  "fakechat",
		FromUserID:   "wx-dev-1",
		Text:         "new",
		ContextToken: "ctx-c",
		Timestamp:    time.Now().UTC(),
	}
	reply3 := waitOutbound(t, adapter)
	if !strings.Contains(reply3.Text, "sess-2") {
		t.Fatalf("expected new session notification with sess-2, got %+v", reply3)
	}
	chAfter3, _ := svc.GetChannel(ch.ID)
	if chAfter3.ActiveSessionID != "sess-2" {
		t.Fatalf("expected ActiveSessionID=sess-2, got %q", chAfter3.ActiveSessionID)
	}

	adapter.inbound <- channels.InboundMessage{
		ChannelType:  "fakechat",
		FromUserID:   "wx-dev-1",
		Text:         "开始新任务",
		ContextToken: "ctx-d",
		Timestamp:    time.Now().UTC(),
	}
	waitOutbound(t, adapter)
	if len(sessCtrl.sentTexts) != 2 || !strings.Contains(sessCtrl.sentTexts[1], `<Role name="后端架构师">`) {
		t.Fatalf("expected first real message after bare 'new' to inject <Role>, got %+v", sessCtrl.sentTexts)
	}
}
