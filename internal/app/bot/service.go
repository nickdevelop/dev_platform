package bot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"devplatform/internal/app/chat"
	"devplatform/internal/app/database"
	"devplatform/internal/app/events"
	"devplatform/internal/app/platformtools"
	"devplatform/internal/app/secrets"
	"devplatform/internal/app/sessions"
	"devplatform/internal/app/settings"
	"devplatform/internal/core/channels"
)

const (
	botGroupSystemKey   = "bot-channels"
	botGroupName        = "Bot 渠道"
	typingInterval      = 4 * time.Second
	qrLoginTimeout      = 5 * time.Minute
	turnTimeout         = 10 * time.Minute
	botEngineBasePrompt = "你是 dev_platform 平台的【Bot 执行引擎】，基于 ReAct 工具循环为微信等外部 IM 渠道提供平台操作能力。\n" +
		"运行约定：\n" +
		"1. 操作平台项目、工作树、Agent 后端、编码会话或团队协同前，先调用 platform_list_projects / platform_list_worktrees / platform_list_agent_backends / platform_list_teams 等查询工具动态获取实时元数据，不要猜测 ID。\n" +
		"2. 创建或续发会话后，明确返回关键会话 ID、项目与当前状态，异步完成时系统会自动推送结果。\n" +
		"3. 输出使用清晰简洁的纯文本条目，适配手机端阅读。"
)

// SecretStore abstracts reading and writing channel credentials.
type SecretStore interface {
	Set(ctx context.Context, id, value string) error
	Get(ctx context.Context, id string) (string, error)
	Delete(ctx context.Context, id string) error
	Exists(ctx context.Context, id string) (bool, error)
}

// EngineSettingsProvider provides BotConfig from platform settings.
type EngineSettingsProvider interface {
	Engines() (settings.EnginesSetting, error)
}

// ChatBridge abstracts the subset of chat.Service capabilities needed by Bot channels.
type ChatBridge interface {
	ListGroups() ([]chat.ChatGroup, error)
	CreateGroup(input chat.CreateGroupInput) (chat.ChatGroup, error)
	UpdateGroup(id uint, input chat.UpdateGroupInput) (chat.ChatGroup, error)
	CreateSilent(groupID uint, title string, modelID uint, systemPrompt string, welcomeText string) (chat.Chat, error)
	Update(chatID uint, input chat.UpdateInput) (chat.Chat, error)
	Delete(chatID uint) error
	SendAndWait(ctx context.Context, chatID uint, input chat.SendInput) (chat.Message, error)
	AppendNotice(chatID uint, text string) error
	Get(id uint) (chat.Chat, error)
}

type channelRuntime struct {
	cancel context.CancelFunc
	queue  chan inboundWork
}

type inboundWork struct {
	msg channels.InboundMessage
}

// Service manages external IM bot channels, QR login flows, inbound message
// routing into platform Bot chats, and asynchronous session completion watches.
type Service struct {
	repo            *Repository
	registry        *channels.Registry
	secrets         SecretStore
	settings        EngineSettingsProvider
	chats           ChatBridge
	tools           *platformtools.Executor
	sessionObserver sessions.TurnObserver
	eventsMgr       *events.Manager

	baseCtx   context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	closeOnce sync.Once

	mu           sync.Mutex
	runtimes     map[uint]*channelRuntime
	qrLogins     map[uint]context.CancelFunc
	qrStates     map[uint]QRLoginState
	watchCancels map[string]context.CancelFunc
}

// NewService constructs a Bot Service.
func NewService(
	repo *Repository,
	registry *channels.Registry,
	secretStore SecretStore,
	settingsSvc EngineSettingsProvider,
	chatSvc ChatBridge,
	toolsExec *platformtools.Executor,
	sessionObserver sessions.TurnObserver,
	eventsMgr *events.Manager,
) *Service {
	baseCtx, cancel := context.WithCancel(context.Background())
	return &Service{
		repo:            repo,
		registry:        registry,
		secrets:         secretStore,
		settings:        settingsSvc,
		chats:           chatSvc,
		tools:           toolsExec,
		sessionObserver: sessionObserver,
		eventsMgr:       eventsMgr,
		baseCtx:         baseCtx,
		cancel:          cancel,
		runtimes:        make(map[uint]*channelRuntime),
		qrLogins:        make(map[uint]context.CancelFunc),
		qrStates:        make(map[uint]QRLoginState),
		watchCancels:    make(map[string]context.CancelFunc),
	}
}

// OnSubscribe implements events.ChannelHandler for ChannelBot.
func (s *Service) OnSubscribe(ctx context.Context, busID string, req events.SubscribeRequest) (any, error) {
	list, err := s.ListChannels()
	if err != nil {
		return nil, err
	}
	return map[string]any{"channels": list}, nil
}

// OnUnsubscribe implements events.ChannelHandler for ChannelBot.
func (s *Service) OnUnsubscribe(ctx context.Context, busID string, channel string, targetID string) error {
	return nil
}

// StartAll resumes enabled channels with stored credentials and recovers any
// unfinished session completion watches from the previous process.
func (s *Service) StartAll() {
	if s.chats != nil {
		_, _ = s.ensureBotGroup()
	}
	rows, err := s.repo.ListChannels()
	if err == nil {
		for _, row := range rows {
			if row.Enabled && row.SecretRef != "" {
				if startErr := s.StartChannel(row.ID); startErr != nil {
					log.Printf("bot: auto-start channel %d (%s): %v", row.ID, row.Name, startErr)
				}
			}
		}
	}
	watches, err := s.repo.ListActiveWatches()
	if err == nil {
		for _, w := range watches {
			s.spawnWatch(w)
		}
	}
}

// Close gracefully stops all channel listeners, login pollers and session watches.
func (s *Service) Close(ctx context.Context) {
	s.closeOnce.Do(func() {
		s.cancel()
		s.mu.Lock()
		for _, rt := range s.runtimes {
			rt.cancel()
		}
		for _, cancelQR := range s.qrLogins {
			cancelQR()
		}
		for _, cancelWatch := range s.watchCancels {
			cancelWatch()
		}
		s.mu.Unlock()

		done := make(chan struct{})
		go func() {
			s.wg.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-ctx.Done():
		}
	})
}

// Descriptors returns all registered channel adapters.
func (s *Service) Descriptors() []channels.ChannelDescriptor {
	if s.registry == nil {
		return nil
	}
	return s.registry.Descriptors()
}

// ToolCatalog returns all configurable platform tools for the Bot engine.
func (s *Service) ToolCatalog() []platformtools.ToolMeta {
	if s.tools == nil {
		return nil
	}
	return s.tools.ToolCatalog()
}

// ListChannels returns all configured bot channels.
func (s *Service) ListChannels() ([]Channel, error) {
	rows, err := s.repo.ListChannels()
	if err != nil {
		return nil, err
	}
	out := make([]Channel, 0, len(rows))
	for _, row := range rows {
		out = append(out, s.toDTO(row))
	}
	return out, nil
}

// GetChannel returns one channel by ID.
func (s *Service) GetChannel(id uint) (Channel, error) {
	row, err := s.repo.GetChannel(id)
	if err != nil {
		return Channel{}, err
	}
	return s.toDTO(row), nil
}

// CreateChannel creates a new bot channel and binds a dedicated Bot Chat conversation.
func (s *Service) CreateChannel(ctx context.Context, input CreateChannelInput) (Channel, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return Channel{}, fmt.Errorf("%w: name is required", ErrInvalidChannel)
	}
	chType := strings.TrimSpace(input.ChannelType)
	if chType == "" {
		return Channel{}, fmt.Errorf("%w: channelType is required", ErrInvalidChannel)
	}
	if s.registry != nil {
		if _, err := s.registry.Get(channels.ChannelType(chType)); err != nil {
			return Channel{}, ErrUnknownChannelType
		}
	}

	autoNotify := true
	if input.AutoNotify != nil {
		autoNotify = *input.AutoNotify
	}

	groupID, chatID, err := s.createBoundChat(name, input.ModelID)
	if err != nil {
		return Channel{}, fmt.Errorf("bot: bind channel chat: %w", err)
	}

	row := database.BotChannel{
		Name:        name,
		ChannelType: chType,
		Enabled:     false,
		Status:      StatusUnconfigured,
		ModelID:     input.ModelID,
		AutoNotify:  autoNotify,
		ChatGroupID: groupID,
		ChatID:      chatID,
	}
	if err := s.repo.CreateChannel(&row); err != nil {
		return Channel{}, err
	}
	dto := s.toDTO(row)
	s.broadcastChannelStatus(dto)
	return dto, nil
}

// UpdateChannel updates mutable configuration fields of a channel.
func (s *Service) UpdateChannel(ctx context.Context, id uint, input UpdateChannelInput) (Channel, error) {
	row, err := s.repo.GetChannel(id)
	if err != nil {
		return Channel{}, err
	}
	var chatUpdate chat.UpdateInput
	needChatSync := false

	if input.Name != nil {
		trimmed := strings.TrimSpace(*input.Name)
		if trimmed == "" {
			return Channel{}, fmt.Errorf("%w: name cannot be empty", ErrInvalidChannel)
		}
		if row.Name != trimmed {
			row.Name = trimmed
			title := fmt.Sprintf("Bot · %s", trimmed)
			chatUpdate.Title = &title
			needChatSync = true
		}
	}
	if input.ModelID != nil && row.ModelID != *input.ModelID {
		row.ModelID = *input.ModelID
		if *input.ModelID > 0 {
			mID := *input.ModelID
			chatUpdate.ModelID = &mID
			needChatSync = true
		}
	}
	if input.AutoNotify != nil {
		row.AutoNotify = *input.AutoNotify
	}
	if err := s.repo.UpdateChannel(&row); err != nil {
		return Channel{}, err
	}
	if needChatSync && row.ChatID > 0 && s.chats != nil {
		_, _ = s.chats.Update(row.ChatID, chatUpdate)
	}
	if input.Enabled != nil {
		if *input.Enabled && !row.Enabled {
			if err := s.StartChannel(id); err != nil {
				return Channel{}, err
			}
			return s.GetChannel(id)
		}
		if !*input.Enabled && row.Enabled {
			if err := s.StopChannel(id); err != nil {
				return Channel{}, err
			}
			return s.GetChannel(id)
		}
	}
	dto := s.toDTO(row)
	s.broadcastChannelStatus(dto)
	return dto, nil
}

// DeleteChannel stops the channel runtime, removes stored secrets and bound chat,
// and deletes the channel row.
func (s *Service) DeleteChannel(ctx context.Context, id uint) error {
	row, err := s.repo.GetChannel(id)
	if err != nil {
		return err
	}
	_ = s.CancelQRLogin(id)
	_ = s.StopChannel(id)
	if row.SecretRef != "" && s.secrets != nil {
		_ = s.secrets.Delete(ctx, row.SecretRef)
	}
	if row.ChatID > 0 && s.chats != nil {
		_ = s.chats.Delete(row.ChatID)
	}
	if err := s.repo.DeleteChannel(id); err != nil {
		return err
	}
	s.broadcastEvent(strconv.FormatUint(uint64(id), 10), "status_change", map[string]any{
		"channelId": id,
		"deleted":   true,
	})
	return nil
}

// StartQRLogin starts a QR code login flow for a channel and broadcasts live progress.
func (s *Service) StartQRLogin(ctx context.Context, id uint) (QRLoginState, error) {
	row, err := s.repo.GetChannel(id)
	if err != nil {
		return QRLoginState{}, err
	}
	adapter, err := s.registry.Get(channels.ChannelType(row.ChannelType))
	if err != nil {
		return QRLoginState{}, err
	}
	if !adapter.Descriptor().SupportsQRLogin {
		return QRLoginState{}, ErrUnsupportedLogin
	}

	_ = s.CancelQRLogin(id)
	_ = s.StopChannel(id)

	loginSess, err := adapter.StartLogin(ctx)
	if err != nil {
		_ = s.repo.UpdateChannelState(id, StatusError, err.Error(), nil)
		return QRLoginState{}, err
	}

	waitCtx, cancel := context.WithTimeout(s.baseCtx, qrLoginTimeout)
	initial := QRLoginState{
		ChannelID: id,
		Status:    string(channels.LoginPending),
		QRCode:    loginSess.QRCode,
		QRURL:     loginSess.QRURL,
	}

	s.mu.Lock()
	s.qrLogins[id] = cancel
	s.qrStates[id] = initial
	s.mu.Unlock()

	_ = s.repo.UpdateChannelState(id, StatusWaitingQR, "", nil)
	s.broadcastQRState(initial)
	if updated, gErr := s.GetChannel(id); gErr == nil {
		s.broadcastChannelStatus(updated)
	}

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer func() {
			cancel()
			s.mu.Lock()
			delete(s.qrLogins, id)
			s.mu.Unlock()
		}()

		creds, waitErr := loginSess.Wait(waitCtx, func(ev channels.LoginEvent) {
			state := QRLoginState{
				ChannelID: id,
				Status:    string(ev.Status),
				QRCode:    ev.QRCode,
				QRURL:     ev.QRURL,
				Error:     ev.Error,
			}
			s.mu.Lock()
			s.qrStates[id] = state
			s.mu.Unlock()
			s.broadcastQRState(state)

			if ev.Status == channels.LoginScanned {
				_ = s.repo.UpdateChannelState(id, StatusScanned, "", nil)
				if ch, err := s.GetChannel(id); err == nil {
					s.broadcastChannelStatus(ch)
				}
			}
		})

		if waitErr != nil {
			nextStatus := StatusError
			if errors.Is(waitErr, channels.ErrQRCodeExpired) || errors.Is(waitErr, channels.ErrLoginTimeout) {
				nextStatus = StatusExpired
			} else if errors.Is(waitErr, channels.ErrQRCodeCancelled) || errors.Is(waitErr, context.Canceled) {
				nextStatus = StatusUnconfigured
			}
			state := QRLoginState{
				ChannelID: id,
				Status:    nextStatus,
				Error:     waitErr.Error(),
			}
			s.mu.Lock()
			s.qrStates[id] = state
			s.mu.Unlock()
			_ = s.repo.UpdateChannelState(id, nextStatus, waitErr.Error(), nil)
			s.broadcastQRState(state)
			if ch, err := s.GetChannel(id); err == nil {
				s.broadcastChannelStatus(ch)
			}
			return
		}

		if creds != nil {
			if err := s.saveCredentialsAndStart(id, *creds); err != nil {
				state := QRLoginState{
					ChannelID: id,
					Status:    string(channels.LoginError),
					Error:     err.Error(),
				}
				s.mu.Lock()
				s.qrStates[id] = state
				s.mu.Unlock()
				_ = s.repo.UpdateChannelState(id, StatusError, err.Error(), nil)
				s.broadcastQRState(state)
				return
			}
			state := QRLoginState{
				ChannelID: id,
				Status:    string(channels.LoginConfirmed),
			}
			s.mu.Lock()
			s.qrStates[id] = state
			s.mu.Unlock()
			s.broadcastQRState(state)
		}
	}()

	return initial, nil
}

// CancelQRLogin aborts any active QR login session for the channel.
func (s *Service) CancelQRLogin(id uint) error {
	s.mu.Lock()
	cancel, ok := s.qrLogins[id]
	if ok {
		delete(s.qrLogins, id)
	}
	delete(s.qrStates, id)
	s.mu.Unlock()
	if ok && cancel != nil {
		cancel()
	}
	return nil
}

// QRLoginStatus returns the latest known QR login state for a channel.
func (s *Service) QRLoginStatus(id uint) (QRLoginState, error) {
	if _, err := s.repo.GetChannel(id); err != nil {
		return QRLoginState{}, err
	}
	s.mu.Lock()
	state, ok := s.qrStates[id]
	s.mu.Unlock()
	if !ok {
		return QRLoginState{ChannelID: id, Status: StatusUnconfigured}, nil
	}
	return state, nil
}

func (s *Service) saveCredentialsAndStart(id uint, creds channels.Credentials) error {
	row, err := s.repo.GetChannel(id)
	if err != nil {
		return err
	}
	ref := row.SecretRef
	if ref == "" {
		newRef, err := secrets.NewSecretID()
		if err != nil {
			return err
		}
		ref = newRef
	}
	raw, err := json.Marshal(creds)
	if err != nil {
		return err
	}
	if err := s.secrets.Set(s.baseCtx, ref, string(raw)); err != nil {
		return err
	}
	row.SecretRef = ref
	if creds.AccountID != "" {
		row.AccountID = creds.AccountID
	} else if creds.UserID != "" {
		row.AccountID = creds.UserID
	}
	row.Enabled = true
	row.LastError = ""
	if err := s.repo.UpdateChannel(&row); err != nil {
		return err
	}
	return s.StartChannel(id)
}

// StartChannel starts the long-polling listener and inbound worker for channel id.
func (s *Service) StartChannel(id uint) error {
	row, err := s.repo.GetChannel(id)
	if err != nil {
		return err
	}
	if row.SecretRef == "" {
		return ErrMissingCredentials
	}
	creds, err := s.loadCredentials(row.SecretRef)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrMissingCredentials, err)
	}
	adapter, err := s.registry.Get(channels.ChannelType(row.ChannelType))
	if err != nil {
		return err
	}

	_ = s.StopChannel(id)

	row.Enabled = true
	row.LastError = ""
	_ = s.repo.UpdateChannel(&row)

	rtCtx, cancel := context.WithCancel(s.baseCtx)
	rt := &channelRuntime{
		cancel: cancel,
		queue:  make(chan inboundWork, 64),
	}

	s.mu.Lock()
	s.runtimes[id] = rt
	s.mu.Unlock()

	// Worker goroutine processes inbound messages serially per channel.
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.runChannelWorker(rtCtx, id, rt.queue)
	}()

	// Listener goroutine runs the adapter's long-poll loop.
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer func() {
			cancel()
			s.mu.Lock()
			if cur, ok := s.runtimes[id]; ok && cur == rt {
				delete(s.runtimes, id)
			}
			s.mu.Unlock()
		}()

		err := adapter.Listen(rtCtx, creds, row.UpdateCursor, channels.ListenCallbacks{
			OnReady: func() {
				now := time.Now().UTC()
				_ = s.repo.UpdateChannelState(id, StatusConnected, "", &now)
				if dto, gErr := s.GetChannel(id); gErr == nil {
					s.broadcastChannelStatus(dto)
				}
			},
			OnCursor: func(cursor string) {
				_ = s.repo.UpdateChannelCursor(id, cursor)
			},
			OnMessage: func(msgCtx context.Context, msg channels.InboundMessage) error {
				select {
				case rt.queue <- inboundWork{msg: msg}:
					return nil
				case <-msgCtx.Done():
					return msgCtx.Err()
				}
			},
		})

		if rtCtx.Err() != nil {
			// Clean stop requested.
			return
		}

		if errors.Is(err, channels.ErrSessionExpired) {
			if row.SecretRef != "" && s.secrets != nil {
				_ = s.secrets.Delete(s.baseCtx, row.SecretRef)
			}
			if latest, gErr := s.repo.GetChannel(id); gErr == nil {
				latest.SecretRef = ""
				latest.Enabled = false
				latest.Status = StatusExpired
				latest.LastError = err.Error()
				_ = s.repo.UpdateChannel(&latest)
				s.broadcastChannelStatus(s.toDTO(latest))
			}
			return
		}

		errText := ""
		if err != nil {
			errText = err.Error()
		}
		_ = s.repo.UpdateChannelState(id, StatusError, errText, nil)
		if dto, gErr := s.GetChannel(id); gErr == nil {
			s.broadcastChannelStatus(dto)
		}
	}()

	return nil
}

// StopChannel stops a running channel listener cleanly.
func (s *Service) StopChannel(id uint) error {
	row, err := s.repo.GetChannel(id)
	if err != nil {
		return err
	}
	s.mu.Lock()
	rt, ok := s.runtimes[id]
	if ok {
		delete(s.runtimes, id)
	}
	s.mu.Unlock()
	if ok && rt != nil {
		rt.cancel()
	}

	row.Enabled = false
	if row.SecretRef == "" {
		row.Status = StatusUnconfigured
	} else {
		row.Status = StatusDisconnected
	}
	row.LastError = ""
	if err := s.repo.UpdateChannel(&row); err != nil {
		return err
	}
	s.broadcastChannelStatus(s.toDTO(row))
	return nil
}

func (s *Service) runChannelWorker(ctx context.Context, channelID uint, queue <-chan inboundWork) {
	for {
		select {
		case <-ctx.Done():
			return
		case work := <-queue:
			s.processInboundMessage(ctx, channelID, work.msg)
		}
	}
}

func (s *Service) processInboundMessage(ctx context.Context, channelID uint, msg channels.InboundMessage) {
	row, err := s.repo.GetChannel(channelID)
	if err != nil {
		return
	}
	if err := s.repo.UpsertContextToken(channelID, msg.FromUserID, msg.ContextToken, msg.Timestamp); err != nil {
		log.Printf("bot: persist context token for channel %d: %v", channelID, err)
	}

	creds, err := s.loadCredentials(row.SecretRef)
	if err != nil {
		return
	}
	adapter, err := s.registry.Get(channels.ChannelType(row.ChannelType))
	if err != nil {
		return
	}
	botCfg := s.loadBotConfig()

	text := strings.TrimSpace(msg.Text)
	if text == "" {
		return
	}
	ctxToken := strings.TrimSpace(msg.ContextToken)
	if ctxToken == "" {
		ctxToken, _ = s.repo.GetContextToken(channelID, msg.FromUserID)
	}
	log.Printf("bot: inbound channel=%d from=%s msg_len=%d has_ctx_token=%v", channelID, msg.FromUserID, len([]rune(text)), ctxToken != "")

	// Local slash commands handled immediately without calling LLM.
	switch {
	case strings.EqualFold(text, "/whoami"):
		_ = adapter.SendMessage(ctx, creds, channels.OutboundMessage{
			ToUserID:     msg.FromUserID,
			GroupID:      msg.GroupID,
			ContextToken: ctxToken,
			Text:         fmt.Sprintf("当前微信标识 ID：%s", msg.FromUserID),
		}, botCfg.MaxReplyLength)
		return
	case strings.EqualFold(text, "/help"):
		helpText := "【dev_platform Bot 助手】\n可直接通过自然语言向我发起指令：\n- 查看平台项目与工作树\n- 创建 Agent 编码会话并执行任务\n- 查询会话执行进度与最新结果\n- 继续向已有会话发送指令或中断执行\n\n快捷命令：\n/reset - 开启新的 Bot 对话上下文\n/whoami - 查看当前微信标识 ID"
		_ = adapter.SendMessage(ctx, creds, channels.OutboundMessage{
			ToUserID:     msg.FromUserID,
			GroupID:      msg.GroupID,
			ContextToken: ctxToken,
			Text:         helpText,
		}, botCfg.MaxReplyLength)
		return
	case strings.EqualFold(text, "/reset"):
		_, newChatID, cErr := s.createBoundChat(row.Name, row.ModelID)
		if cErr == nil && newChatID > 0 {
			row.ChatID = newChatID
			_ = s.repo.UpdateChannel(&row)
			s.broadcastChannelStatus(s.toDTO(row))
			_ = adapter.SendMessage(ctx, creds, channels.OutboundMessage{
				ToUserID:     msg.FromUserID,
				GroupID:      msg.GroupID,
				ContextToken: ctxToken,
				Text:         "已重置并开启新的 Bot 对话上下文。",
			}, botCfg.MaxReplyLength)
		}
		return
	}

	// Ensure channel has a bound Chat row.
	if row.ChatID == 0 {
		groupID, chatID, cErr := s.createBoundChat(row.Name, row.ModelID)
		if cErr != nil {
			log.Printf("bot: ensure bound chat for channel %d: %v", channelID, cErr)
			return
		}
		row.ChatGroupID = groupID
		row.ChatID = chatID
		_ = s.repo.UpdateChannel(&row)
	}

	// Keep typing indicator alive while LLM & tools execute.
	stopTyping := func() {}
	if botCfg.TypingIndicator {
		typingCtx, cancelTyping := context.WithCancel(ctx)
		_ = adapter.SendTyping(typingCtx, creds, msg.FromUserID, true)
		go func() {
			ticker := time.NewTicker(typingInterval)
			defer ticker.Stop()
			for {
				select {
				case <-typingCtx.Done():
					return
				case <-ticker.C:
					_ = adapter.SendTyping(typingCtx, creds, msg.FromUserID, true)
				}
			}
		}()
		stopTyping = func() {
			cancelTyping()
			endCtx, endCancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer endCancel()
			_ = adapter.SendTyping(endCtx, creds, msg.FromUserID, false)
		}
	}

	replyMsg, sendErr := s.chats.SendAndWait(ctx, row.ChatID, chat.SendInput{
		ModelID: row.ModelID,
		Text:    text,
	})
	stopTyping()

	replyText := replyMsg.Text
	if sendErr != nil {
		replyText = fmt.Sprintf("处理消息失败：%v", sendErr)
	} else if strings.TrimSpace(replyText) == "" {
		replyText = "（已完成处理，暂无文本输出）"
	}

	if err := adapter.SendMessage(ctx, creds, channels.OutboundMessage{
		ToUserID:     msg.FromUserID,
		GroupID:      msg.GroupID,
		ContextToken: ctxToken,
		Text:         replyText,
	}, botCfg.MaxReplyLength); err != nil {
		log.Printf("bot: send reply on channel %d failed: %v", channelID, err)
	} else {
		log.Printf("bot: send reply on channel %d ok to=%s reply_len=%d", channelID, msg.FromUserID, len([]rune(replyText)))
	}
}

// PolicyForChat implements chat.ToolPolicyResolver so any turn executed in a
// Bot-bound chat (whether triggered from WeChat or from the platform UI)
// strictly uses the Bot ReAct engine's tool allowlist, step budget and base prompt.
func (s *Service) PolicyForChat(chatID uint) (chat.ToolPolicy, bool) {
	if chatID == 0 {
		return chat.ToolPolicy{}, false
	}
	ch, err := s.repo.GetChannelByChatID(chatID)
	if err != nil {
		return chat.ToolPolicy{}, false
	}
	cfg := s.loadBotConfig()

	var toolExec chat.ToolExecutor
	disableTools := true
	if s.tools != nil && len(cfg.EnabledTools) > 0 {
		toolExec = s.tools.FilteredExecutor(cfg.EnabledTools)
		disableTools = false
	}

	channelID := ch.ID
	autoNotify := ch.AutoNotify && cfg.AutoNotifySession

	return chat.ToolPolicy{
		Executor:          toolExec,
		MaxSteps:          cfg.MaxSteps,
		ExtraSystemPrompt: botEngineBasePrompt,
		ModelID:           ch.ModelID,
		DisableTools:      disableTools,
		WrapContext: func(base context.Context) context.Context {
			if !autoNotify {
				return base
			}
			return platformtools.WithSessionWatchCallback(base, func(ev platformtools.SessionWatchEvent) {
				if ev.SessionID == "" || ev.ProjectID == 0 {
					return
				}
				userID, _, _ := s.repo.LatestUserForChannel(channelID)
				s.WatchSession(channelID, userID, ev.ProjectID, ev.SessionID)
			})
		},
	}, true
}

// WatchSession registers an asynchronous turn monitor so the originating
// channel receives a completion/failure summary when the session settles.
func (s *Service) WatchSession(channelID uint, externalUserID string, projectID uint, sessionID string) {
	if s.sessionObserver == nil || channelID == 0 || projectID == 0 || sessionID == "" {
		return
	}
	watch, err := s.repo.UpsertSessionWatch(database.BotSessionWatch{
		ChannelID:      channelID,
		ExternalUserID: externalUserID,
		ProjectID:      projectID,
		SessionID:      sessionID,
		WatchChannelID: channelID,
		Status:         WatchWatching,
	})
	if err != nil {
		log.Printf("bot: persist session watch (%s): %v", sessionID, err)
		return
	}
	s.spawnWatch(watch)
}

func (s *Service) spawnWatch(watch database.BotSessionWatch) {
	key := fmt.Sprintf("%d:%s", watch.ChannelID, watch.SessionID)

	s.mu.Lock()
	if prev, ok := s.watchCancels[key]; ok {
		prev()
	}
	watchCtx, cancel := context.WithTimeout(s.baseCtx, turnTimeout)
	s.watchCancels[key] = cancel
	s.mu.Unlock()

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer func() {
			cancel()
			s.mu.Lock()
			delete(s.watchCancels, key)
			s.mu.Unlock()
		}()

		res, err := sessions.WaitTurn(watchCtx, s.sessionObserver, watch.ProjectID, watch.SessionID, sessions.TurnWaitOptions{
			Timeout:                 turnTimeout,
			AutoResolveInteractions: true,
			AutoFormAnswer:          "当前会话由外部 Bot 渠道发起并在后台执行，请勿挂起提问，请自主推断并输出清晰结论。",
			AutoPermissionReason:    "auto-allowed by bot channel execution",
		})

		var notice string
		if err != nil || res.Status == "failed" {
			errMsg := res.Error
			if errMsg == "" && err != nil {
				errMsg = err.Error()
			}
			_ = s.repo.MarkWatchStatus(watch.ID, WatchFailed, errMsg)
			notice = fmt.Sprintf("❌ 会话 `%s` 执行异常：%s", watch.SessionID, errMsg)
		} else {
			_ = s.repo.MarkWatchStatus(watch.ID, WatchNotified, "")
			summary := strings.TrimSpace(res.Summary)
			if len([]rune(summary)) > 1500 {
				runes := []rune(summary)
				summary = string(runes[:1500]) + "\n...(内容过长，可在平台查看完整会话)"
			}
			if summary == "" {
				summary = "执行已完成。"
			}
			notice = fmt.Sprintf("✅ 会话 `%s` 执行完成：\n\n%s", watch.SessionID, summary)
		}

		row, gErr := s.repo.GetChannel(watch.ChannelID)
		if gErr != nil {
			return
		}
		if row.ChatID > 0 && s.chats != nil {
			_ = s.chats.AppendNotice(row.ChatID, notice)
		}

		if !row.Enabled || row.SecretRef == "" || !row.AutoNotify {
			return
		}
		creds, cErr := s.loadCredentials(row.SecretRef)
		if cErr != nil {
			return
		}
		adapter, aErr := s.registry.Get(channels.ChannelType(row.ChannelType))
		if aErr != nil {
			return
		}
		targetUser := watch.ExternalUserID
		ctxToken, _ := s.repo.GetContextToken(row.ID, targetUser)
		if targetUser == "" {
			targetUser, ctxToken, _ = s.repo.LatestUserForChannel(row.ID)
		}
		if targetUser == "" {
			return
		}
		botCfg := s.loadBotConfig()
		sendCtx, sendCancel := context.WithTimeout(s.baseCtx, 30*time.Second)
		defer sendCancel()
		if err := adapter.SendMessage(sendCtx, creds, channels.OutboundMessage{
			ToUserID:     targetUser,
			ContextToken: ctxToken,
			Text:         notice,
		}, botCfg.MaxReplyLength); err != nil {
			log.Printf("bot: push watch result for session %s on channel %d: %v", watch.SessionID, row.ID, err)
		}
	}()
}

func (s *Service) createBoundChat(channelName string, modelID uint) (uint, uint, error) {
	if s.chats == nil {
		return 0, 0, nil
	}
	groupID, err := s.ensureBotGroup()
	if err != nil {
		return 0, 0, err
	}
	welcome := fmt.Sprintf("已基于 Bot 引擎创建「%s」渠道专属对话。外部消息与平台对话将在此实时同步。", channelName)
	created, err := s.chats.CreateSilent(groupID, fmt.Sprintf("Bot · %s", channelName), modelID, "", welcome)
	if err != nil {
		return 0, 0, err
	}
	return groupID, created.ID, nil
}

func (s *Service) ensureBotGroup() (uint, error) {
	groups, err := s.chats.ListGroups()
	if err != nil {
		return 0, err
	}
	for _, g := range groups {
		if g.SystemKey == botGroupSystemKey || g.Name == botGroupName {
			if g.ShowInFloat {
				_, _ = s.chats.UpdateGroup(g.ID, chat.UpdateGroupInput{
					Name:                g.Name,
					DefaultModelID:      g.DefaultModelID,
					PromptID:            g.PromptID,
					SystemPrompt:        g.SystemPrompt,
					Icon:                g.Icon,
					ShowInFloat:         false,
					EnablePlatformTools: g.EnablePlatformTools,
					MaxToolSteps:        g.MaxToolSteps,
				})
			}
			return g.ID, nil
		}
	}
	created, err := s.chats.CreateGroup(chat.CreateGroupInput{
		Name:                botGroupName,
		Icon:                "🤖",
		ShowInFloat:         false,
		EnablePlatformTools: true,
		MaxToolSteps:        10,
	})
	if err != nil {
		return 0, err
	}
	return created.ID, nil
}

func (s *Service) loadCredentials(secretRef string) (channels.Credentials, error) {
	if s.secrets == nil || secretRef == "" {
		return channels.Credentials{}, ErrMissingCredentials
	}
	raw, err := s.secrets.Get(s.baseCtx, secretRef)
	if err != nil {
		return channels.Credentials{}, err
	}
	var creds channels.Credentials
	if err := json.Unmarshal([]byte(raw), &creds); err != nil {
		return channels.Credentials{}, err
	}
	return creds, nil
}

func (s *Service) loadBotConfig() settings.BotConfig {
	if s.settings != nil {
		if engines, err := s.settings.Engines(); err == nil {
			return engines.Bot
		}
	}
	return settings.DefaultEnginesSetting().Bot
}

func (s *Service) toDTO(row database.BotChannel) Channel {
	return Channel{
		ID:              row.ID,
		Name:            row.Name,
		ChannelType:     row.ChannelType,
		Enabled:         row.Enabled,
		Status:          row.Status,
		LastError:       row.LastError,
		HasCredentials:  row.SecretRef != "",
		AccountID:       row.AccountID,
		ModelID:         row.ModelID,
		AutoNotify:      row.AutoNotify,
		ChatGroupID:     row.ChatGroupID,
		ChatID:          row.ChatID,
		LastConnectedAt: row.LastConnectedAt,
		CreatedAt:       row.CreatedAt,
		UpdatedAt:       row.UpdatedAt,
	}
}

func (s *Service) broadcastChannelStatus(ch Channel) {
	s.broadcastEvent(strconv.FormatUint(uint64(ch.ID), 10), "status_change", map[string]any{
		"kind":    "channel",
		"channel": ch,
	})
	s.broadcastEvent("all", "status_change", map[string]any{
		"kind":    "channel",
		"channel": ch,
	})
}

func (s *Service) broadcastQRState(state QRLoginState) {
	s.broadcastEvent(strconv.FormatUint(uint64(state.ChannelID), 10), "status_change", map[string]any{
		"kind":  "qr_login",
		"login": state,
	})
	s.broadcastEvent("all", "status_change", map[string]any{
		"kind":  "qr_login",
		"login": state,
	})
}

func (s *Service) broadcastEvent(targetID, eventType string, payload any) {
	if s.eventsMgr == nil {
		return
	}
	_ = s.eventsMgr.BroadcastPayload(events.ChannelBot, targetID, eventType, payload)
}
