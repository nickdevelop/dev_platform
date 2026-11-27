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
	"unicode/utf8"

	"devplatform/internal/app/chat"
	"devplatform/internal/app/database"
	"devplatform/internal/app/events"
	"devplatform/internal/app/platformtools"
	"devplatform/internal/app/projects"
	"devplatform/internal/app/prompts"
	"devplatform/internal/app/secrets"
	"devplatform/internal/app/sessions"
	"devplatform/internal/app/sessions/turns"
	"devplatform/internal/core/agents"
	"devplatform/internal/core/channels"
)

const (
	typingInterval        = 4 * time.Second
	qrLoginTimeout        = 5 * time.Minute
	turnTimeout           = 10 * time.Minute
	defaultMaxReplyLength = 2000
	permissionModeAuto    = "auto"
)

// SecretStore abstracts reading and writing channel credentials.
type SecretStore interface {
	Set(ctx context.Context, id, value string) error
	Get(ctx context.Context, id string) (string, error)
	Delete(ctx context.Context, id string) error
	Exists(ctx context.Context, id string) (bool, error)
}

// ChatBridge abstracts the subset of chat.Service capabilities needed by assistant-mode channels.
type ChatBridge interface {
	GetGroup(id uint) (chat.ChatGroup, error)
	CreateSilentWithMeta(groupID uint, title string, modelID uint, systemPrompt string, welcomeText string, source string, botChannelID uint) (chat.Chat, error)
	Update(chatID uint, input chat.UpdateInput) (chat.Chat, error)
	Delete(chatID uint) error
	SendAndWait(ctx context.Context, chatID uint, input chat.SendInput) (chat.Message, error)
	AppendNotice(chatID uint, text string) error
	Get(id uint) (chat.Chat, error)
}

// SessionController abstracts session creation, resume, message dispatch and turn waiting
// for agent-mode channels. *sessions.Service implements it.
type SessionController interface {
	sessions.TurnObserver
	CreateWithOptions(ctx context.Context, opts sessions.CreateOptions) (sessions.CreateOutcome, error)
	Continue(ctx context.Context, projectID uint, sessionID string) (sessions.Session, error)
	Send(ctx context.Context, projectID uint, sessionID, text, model string, images ...[]agents.ImageAttachment) (sessions.Session, error)
}

// PromptReader resolves stored role prompts. *prompts.Service implements it.
type PromptReader interface {
	Get(id uint) (prompts.Prompt, error)
}

// ProjectChecker verifies target project existence. *projects.Service implements it.
type ProjectChecker interface {
	Get(id uint) (projects.Project, error)
}

type channelRuntime struct {
	cancel context.CancelFunc
	queue  chan inboundWork
}

type inboundWork struct {
	msg channels.InboundMessage
}

// Service manages external IM message channels, QR login flows, single-channel
// inbound routing (bound to either an AI Assistant or an embedded Agent session),
// and asynchronous session completion watches.
type Service struct {
	repo                *Repository
	registry            *channels.Registry
	secrets             SecretStore
	chats               ChatBridge
	sessions            SessionController
	prompts             PromptReader
	projects            ProjectChecker
	defaultAgentBackend agents.BackendID
	eventsMgr           *events.Manager

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

// NewService constructs a Channel Bot Service. The defaultAgentBackend is injected
// by the composition root so generic app code stays free of concrete provider names.
func NewService(
	repo *Repository,
	registry *channels.Registry,
	secretStore SecretStore,
	chatSvc ChatBridge,
	sessionCtrl SessionController,
	promptReader PromptReader,
	projectChecker ProjectChecker,
	defaultAgentBackend agents.BackendID,
	eventsMgr *events.Manager,
) *Service {
	baseCtx, cancel := context.WithCancel(context.Background())
	return &Service{
		repo:                repo,
		registry:            registry,
		secrets:             secretStore,
		chats:               chatSvc,
		sessions:            sessionCtrl,
		prompts:             promptReader,
		projects:            projectChecker,
		defaultAgentBackend: defaultAgentBackend,
		eventsMgr:           eventsMgr,
		baseCtx:             baseCtx,
		cancel:              cancel,
		runtimes:            make(map[uint]*channelRuntime),
		qrLogins:            make(map[uint]context.CancelFunc),
		qrStates:            make(map[uint]QRLoginState),
		watchCancels:        make(map[string]context.CancelFunc),
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

// ListChannels returns all configured channels.
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

// CreateChannel creates a new message channel bound to either an AI Assistant or Project+Role+Embedded Agent.
// Conversations and sessions are lazily created when inbound dialogue begins (single-channel mode).
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

	mode, err := normalizeTargetMode(input.TargetMode)
	if err != nil {
		return Channel{}, err
	}

	typingIndicator := true
	if input.TypingIndicator != nil {
		typingIndicator = *input.TypingIndicator
	}
	autoNotify := true
	if input.AutoNotify != nil {
		autoNotify = *input.AutoNotify
	}
	maxReplyLength := defaultMaxReplyLength
	if input.MaxReplyLength != nil && *input.MaxReplyLength > 0 {
		maxReplyLength = *input.MaxReplyLength
	}

	row := database.BotChannel{
		Name:             name,
		ChannelType:      chType,
		Enabled:          false,
		Status:           StatusUnconfigured,
		TargetMode:       mode,
		ModelID:          input.ModelID,
		PromptID:         input.PromptID,
		SystemPrompt:     strings.TrimSpace(input.SystemPrompt),
		DefaultProjectID: input.DefaultProjectID,
		WorktreeID:       input.WorktreeID,
		DefaultBackend:   string(s.defaultAgentBackend),
		AgentModel:       strings.TrimSpace(input.AgentModel),
		ChatGroupID:      input.ChatGroupID,
		TypingIndicator:  typingIndicator,
		AutoNotify:       autoNotify,
		MaxReplyLength:   maxReplyLength,
	}

	if err := s.validateChannelBinding(&row); err != nil {
		return Channel{}, err
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
	if row.TargetMode == "" {
		row.TargetMode = TargetModeAssistant
	}

	var chatUpdate chat.UpdateInput
	needChatSync := false
	prevMode := row.TargetMode
	prevGroupID := row.ChatGroupID
	prevProjectID := row.DefaultProjectID
	prevWorktreeID := row.WorktreeID

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
	if input.TargetMode != nil {
		mode, mErr := normalizeTargetMode(*input.TargetMode)
		if mErr != nil {
			return Channel{}, mErr
		}
		row.TargetMode = mode
	}
	if input.ChatGroupID != nil {
		row.ChatGroupID = *input.ChatGroupID
	}
	if input.ModelID != nil && row.ModelID != *input.ModelID {
		row.ModelID = *input.ModelID
		if *input.ModelID > 0 {
			mID := *input.ModelID
			chatUpdate.ModelID = &mID
			needChatSync = true
		}
	}
	if input.DefaultProjectID != nil {
		row.DefaultProjectID = *input.DefaultProjectID
	}
	if input.WorktreeID != nil {
		row.WorktreeID = *input.WorktreeID
	}
	if input.PromptID != nil {
		row.PromptID = *input.PromptID
	}
	if input.SystemPrompt != nil {
		row.SystemPrompt = strings.TrimSpace(*input.SystemPrompt)
	}
	if input.AgentModel != nil {
		row.AgentModel = strings.TrimSpace(*input.AgentModel)
	}
	if input.TypingIndicator != nil {
		row.TypingIndicator = *input.TypingIndicator
	}
	if input.AutoNotify != nil {
		row.AutoNotify = *input.AutoNotify
	}
	if input.MaxReplyLength != nil && *input.MaxReplyLength > 0 {
		row.MaxReplyLength = *input.MaxReplyLength
	}
	if row.TargetMode == TargetModeAgent && row.DefaultBackend == "" {
		row.DefaultBackend = string(s.defaultAgentBackend)
	}

	if err := s.validateChannelBinding(&row); err != nil {
		return Channel{}, err
	}

	// Reset stale active binding when target service coordinates change so the next
	// message creates a context matching the updated binding.
	if row.TargetMode != prevMode || (row.TargetMode == TargetModeAssistant && row.ChatGroupID != prevGroupID) {
		row.ChatID = 0
		needChatSync = false
	}
	if row.TargetMode != prevMode || (row.TargetMode == TargetModeAgent && (row.DefaultProjectID != prevProjectID || row.WorktreeID != prevWorktreeID)) {
		row.ActiveSessionID = ""
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

// ResetChannelContext explicitly creates a fresh bot chat or agent session and binds
// it to the channel as the active single-channel context.
func (s *Service) ResetChannelContext(ctx context.Context, id uint) (Channel, error) {
	row, err := s.repo.GetChannel(id)
	if err != nil {
		return Channel{}, err
	}
	if row.TargetMode == "" {
		row.TargetMode = TargetModeAssistant
	}

	switch row.TargetMode {
	case TargetModeAgent:
		if _, err := s.createAndBindAgentSession(ctx, &row, ""); err != nil {
			return Channel{}, err
		}
	default:
		if _, err := s.createAndBindAssistantChat(&row); err != nil {
			return Channel{}, err
		}
	}
	dto := s.toDTO(row)
	s.broadcastChannelStatus(dto)
	return dto, nil
}

// DeleteChannel stops the channel runtime, removes stored secrets and active chat pointer,
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

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.runChannelWorker(rtCtx, id, rt.queue)
	}()

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
	if row.TargetMode == "" {
		row.TargetMode = TargetModeAssistant
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

	text := strings.TrimSpace(msg.Text)
	if text == "" {
		return
	}
	ctxToken := strings.TrimSpace(msg.ContextToken)
	if ctxToken == "" {
		ctxToken, _ = s.repo.GetContextToken(channelID, msg.FromUserID)
	}
	maxLen := effectiveMaxReplyLength(row.MaxReplyLength)
	log.Printf("bot: inbound channel=%d mode=%s from=%s msg_len=%d has_ctx_token=%v", channelID, row.TargetMode, msg.FromUserID, len([]rune(text)), ctxToken != "")

	sendReply := func(body string) {
		if err := adapter.SendMessage(ctx, creds, channels.OutboundMessage{
			ToUserID:     msg.FromUserID,
			GroupID:      msg.GroupID,
			ContextToken: ctxToken,
			Text:         body,
		}, maxLen); err != nil {
			log.Printf("bot: send reply on channel %d failed: %v", channelID, err)
		}
	}

	// Built-in local commands handled without spending an LLM turn.
	switch {
	case strings.EqualFold(text, "/whoami"):
		sendReply(fmt.Sprintf("当前消息标识 ID：%s", msg.FromUserID))
		return
	case strings.EqualFold(text, "/help"):
		modeLabel := "AI 助手模式"
		if row.TargetMode == TargetModeAgent {
			modeLabel = "内置 Agent 模式"
		}
		helpText := fmt.Sprintf("【%s · 消息渠道】\n当前服务模式：%s\n\n快捷命令：\n/new | new | 新建会话 - 创建并切换至全新对话/会话上下文\n/status - 查看当前渠道绑定的上下文状态\n/whoami - 查看当前消息发送方 ID\n/help - 查看帮助信息", row.Name, modeLabel)
		sendReply(helpText)
		return
	case strings.EqualFold(text, "/status"):
		sendReply(s.formatChannelStatus(row))
		return
	}

	isNewCmd, remainder := parseNewCommand(text)

	if row.TargetMode == TargetModeAgent {
		s.handleAgentInbound(ctx, &row, adapter, creds, msg.FromUserID, msg.GroupID, ctxToken, maxLen, isNewCmd, remainder, text, sendReply)
		return
	}
	s.handleAssistantInbound(ctx, &row, adapter, creds, msg.FromUserID, msg.GroupID, ctxToken, maxLen, isNewCmd, remainder, text, sendReply)
}

func (s *Service) handleAssistantInbound(
	ctx context.Context,
	row *database.BotChannel,
	adapter channels.ChannelAdapter,
	creds channels.Credentials,
	fromUserID string,
	groupID string,
	ctxToken string,
	maxLen int,
	isNewCmd bool,
	remainder string,
	rawText string,
	sendReply func(string),
) {
	if s.chats == nil {
		sendReply("渠道对话服务未就绪。")
		return
	}
	if row.ChatGroupID == 0 {
		sendReply("当前渠道尚未绑定 AI 助手，请先在平台渠道设置中选择 AI 助手。")
		return
	}

	userPrompt := rawText
	if isNewCmd {
		created, err := s.createAndBindAssistantChat(row)
		if err != nil {
			sendReply(fmt.Sprintf("新建对话失败：%v", err))
			return
		}
		s.broadcastChannelStatus(s.toDTO(*row))
		if remainder == "" {
			sendReply(fmt.Sprintf("已创建并切换至新的 AI 助手对话（#%d），请发送您的问题。", created.ID))
			return
		}
		userPrompt = remainder
	} else {
		// Validate active chat exists; lazily provision one if absent or deleted.
		needCreate := row.ChatID == 0
		if !needCreate {
			if _, err := s.chats.Get(row.ChatID); err != nil {
				needCreate = true
			}
		}
		if needCreate {
			if _, err := s.createAndBindAssistantChat(row); err != nil {
				log.Printf("bot: ensure bound chat for channel %d: %v", row.ID, err)
				sendReply(fmt.Sprintf("创建渠道对话失败：%v", err))
				return
			}
			s.broadcastChannelStatus(s.toDTO(*row))
		}
	}

	stopTyping := s.startTypingLoop(ctx, adapter, creds, fromUserID, row.TypingIndicator)
	replyMsg, sendErr := s.chats.SendAndWait(ctx, row.ChatID, chat.SendInput{
		ModelID: row.ModelID,
		Text:    userPrompt,
	})
	stopTyping()

	replyText := replyMsg.Text
	if sendErr != nil {
		replyText = fmt.Sprintf("处理消息失败：%v", sendErr)
	} else if strings.TrimSpace(replyText) == "" {
		replyText = "（已完成处理，暂无文本输出）"
	}
	sendReply(replyText)
}

func (s *Service) handleAgentInbound(
	ctx context.Context,
	row *database.BotChannel,
	adapter channels.ChannelAdapter,
	creds channels.Credentials,
	fromUserID string,
	groupID string,
	ctxToken string,
	maxLen int,
	isNewCmd bool,
	remainder string,
	rawText string,
	sendReply func(string),
) {
	if s.sessions == nil {
		sendReply("内置 Agent 会话服务未就绪。")
		return
	}
	if row.DefaultProjectID == 0 {
		sendReply("当前渠道尚未绑定目标项目，请先在平台渠道设置中选择项目与角色。")
		return
	}
	backendID := agents.BackendID(row.DefaultBackend)
	if backendID == "" {
		backendID = s.defaultAgentBackend
	}
	if !backendID.Valid() {
		sendReply("未配置可用的内置 Agent 后端。")
		return
	}

	if isNewCmd && remainder == "" {
		created, err := s.createAndBindAgentSession(ctx, row, "")
		if err != nil {
			sendReply(fmt.Sprintf("新建会话失败：%v", err))
			return
		}
		s.broadcastChannelStatus(s.toDTO(*row))
		sendReply(fmt.Sprintf("已创建并切换至新的内置 Agent 会话（ID: %s），请发送您的任务指令。", created.ID))
		return
	}

	userMessage := rawText
	if isNewCmd {
		userMessage = remainder
	}

	stopTyping := s.startTypingLoop(ctx, adapter, creds, fromUserID, row.TypingIndicator)
	defer stopTyping()

	var targetSessionID string
	var needCreate bool
	var isFirstTurn bool

	if isNewCmd || strings.TrimSpace(row.ActiveSessionID) == "" {
		needCreate = true
	} else {
		sess, err := s.sessions.Get(row.DefaultProjectID, row.ActiveSessionID)
		if err != nil {
			needCreate = true
		} else {
			if sess.Turn == turns.Running {
				sendReply(fmt.Sprintf("当前会话（%s）正在执行上一个任务，请稍候或发送 /new 开启新会话。", sess.ID))
				return
			}
			if sess.Status != sessions.SessionActive {
				resumed, cErr := s.sessions.Continue(ctx, row.DefaultProjectID, sess.ID)
				if cErr != nil {
					if errors.Is(cErr, sessions.ErrSessionNotFound) {
						needCreate = true
					} else {
						sendReply(fmt.Sprintf("恢复内置 Agent 会话失败：%v", cErr))
						return
					}
				} else {
					sess = resumed
				}
			}
			if !needCreate {
				targetSessionID = sess.ID
				// Bare /new creates an untitled session so the very next user message injects the role prompt once.
				isFirstTurn = strings.TrimSpace(sess.Title) == ""
			}
		}
	}

	if needCreate {
		payloadText := s.wrapRolePrompt(row, userMessage)
		created, err := s.createAndBindAgentSession(ctx, row, payloadText)
		if err != nil {
			sendReply(fmt.Sprintf("创建并启动内置 Agent 会话失败：%v", err))
			return
		}
		s.broadcastChannelStatus(s.toDTO(*row))
		targetSessionID = created.ID
	} else {
		payloadText := userMessage
		if isFirstTurn {
			payloadText = s.wrapRolePrompt(row, userMessage)
		}
		if _, err := s.sessions.Send(ctx, row.DefaultProjectID, targetSessionID, payloadText, row.AgentModel); err != nil {
			sendReply(fmt.Sprintf("发送任务指令失败：%v", err))
			return
		}
	}

	res, waitErr := sessions.WaitTurn(ctx, s.sessions, row.DefaultProjectID, targetSessionID, sessions.TurnWaitOptions{
		Timeout:                 turnTimeout,
		AutoResolveInteractions: true,
		AutoFormAnswer:          "当前会话由外部消息渠道发起并在后台执行，请勿挂起提问，请自主推断并输出清晰结论。",
		AutoPermissionReason:    "auto-allowed by bot channel agent execution",
	})

	if waitErr != nil || res.Status == "failed" {
		errMsg := res.Error
		if errMsg == "" && waitErr != nil {
			errMsg = waitErr.Error()
		}
		sendReply(fmt.Sprintf("执行失败（会话 %s）：%s", targetSessionID, errMsg))
		return
	}

	summary := strings.TrimSpace(res.Summary)
	if summary == "" {
		summary = fmt.Sprintf("（会话 %s 已完成执行，暂无文本输出）", targetSessionID)
	}
	sendReply(summary)
}

func (s *Service) createAndBindAssistantChat(row *database.BotChannel) (chat.Chat, error) {
	if s.chats == nil {
		return chat.Chat{}, errors.New("chat service unavailable")
	}
	if row.ChatGroupID == 0 {
		return chat.Chat{}, fmt.Errorf("%w: chatGroupId is required", ErrInvalidChannel)
	}
	group, err := s.chats.GetGroup(row.ChatGroupID)
	if err != nil {
		return chat.Chat{}, err
	}
	modelID := row.ModelID
	if modelID == 0 {
		modelID = group.DefaultModelID
	}
	title := fmt.Sprintf("Bot · %s", row.Name)
	created, err := s.chats.CreateSilentWithMeta(row.ChatGroupID, title, modelID, "", "", chat.SourceBot, row.ID)
	if err != nil {
		return chat.Chat{}, err
	}
	row.ChatID = created.ID
	if err := s.repo.UpdateChannel(row); err != nil {
		return chat.Chat{}, err
	}
	return created, nil
}

func (s *Service) createAndBindAgentSession(ctx context.Context, row *database.BotChannel, initialMessage string) (sessions.Session, error) {
	if s.sessions == nil {
		return sessions.Session{}, errors.New("session service unavailable")
	}
	if row.DefaultProjectID == 0 {
		return sessions.Session{}, fmt.Errorf("%w: defaultProjectId is required", ErrInvalidChannel)
	}
	backendID := agents.BackendID(row.DefaultBackend)
	if backendID == "" {
		backendID = s.defaultAgentBackend
	}
	if !backendID.Valid() {
		return sessions.Session{}, fmt.Errorf("%w: agent backend is not configured", ErrInvalidChannel)
	}

	outcome, err := s.sessions.CreateWithOptions(ctx, sessions.CreateOptions{
		ProjectID:      row.DefaultProjectID,
		WorktreeID:     row.WorktreeID,
		Backend:        backendID,
		Message:        initialMessage,
		Model:          strings.TrimSpace(row.AgentModel),
		Source:         chat.SourceBot,
		BotChannelID:   row.ID,
		PermissionMode: permissionModeAuto,
	})
	if err != nil {
		return sessions.Session{}, err
	}
	if outcome.MessageError != "" {
		return outcome.Session, fmt.Errorf("agent message dispatch error: %s", outcome.MessageError)
	}
	row.ActiveSessionID = outcome.Session.ID
	row.DefaultBackend = string(backendID)
	if err := s.repo.UpdateChannel(row); err != nil {
		return outcome.Session, err
	}
	return outcome.Session, nil
}

func (s *Service) wrapRolePrompt(row *database.BotChannel, message string) string {
	var roleName, roleBody string
	if row.PromptID > 0 && s.prompts != nil {
		if p, err := s.prompts.Get(row.PromptID); err == nil {
			roleName = strings.TrimSpace(p.Name)
			roleBody = strings.TrimSpace(p.Content)
		}
	}
	if roleBody == "" && strings.TrimSpace(row.SystemPrompt) != "" {
		roleName = strings.TrimSpace(row.Name)
		roleBody = strings.TrimSpace(row.SystemPrompt)
	} else if custom := strings.TrimSpace(row.SystemPrompt); custom != "" && roleBody != "" {
		roleBody = roleBody + "\n\n" + custom
	}
	if roleBody == "" {
		return message
	}
	if roleName == "" {
		roleName = "渠道角色"
	}
	safeName := strings.ReplaceAll(roleName, `"`, `'`)
	return fmt.Sprintf("<Role name=%q>\n%s\n</Role>\n\n%s", safeName, roleBody, message)
}

func (s *Service) startTypingLoop(
	ctx context.Context,
	adapter channels.ChannelAdapter,
	creds channels.Credentials,
	fromUserID string,
	enabled bool,
) func() {
	if !enabled || adapter == nil {
		return func() {}
	}
	typingCtx, cancelTyping := context.WithCancel(ctx)
	_ = adapter.SendTyping(typingCtx, creds, fromUserID, true)
	go func() {
		ticker := time.NewTicker(typingInterval)
		defer ticker.Stop()
		for {
			select {
			case <-typingCtx.Done():
				return
			case <-ticker.C:
				_ = adapter.SendTyping(typingCtx, creds, fromUserID, true)
			}
		}
	}()
	return func() {
		cancelTyping()
		endCtx, endCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer endCancel()
		_ = adapter.SendTyping(endCtx, creds, fromUserID, false)
	}
}

func (s *Service) validateChannelBinding(row *database.BotChannel) error {
	switch row.TargetMode {
	case TargetModeAssistant:
		if row.ChatGroupID == 0 {
			return fmt.Errorf("%w: chatGroupId is required in assistant mode", ErrInvalidChannel)
		}
		if s.chats != nil {
			if _, err := s.chats.GetGroup(row.ChatGroupID); err != nil {
				return fmt.Errorf("%w: chatGroupId %d not found", ErrInvalidChannel, row.ChatGroupID)
			}
		}
	case TargetModeAgent:
		if row.DefaultProjectID == 0 {
			return fmt.Errorf("%w: defaultProjectId is required in agent mode", ErrInvalidChannel)
		}
		if s.projects != nil {
			if _, err := s.projects.Get(row.DefaultProjectID); err != nil {
				return fmt.Errorf("%w: defaultProjectId %d not found", ErrInvalidChannel, row.DefaultProjectID)
			}
		}
		if row.PromptID > 0 && s.prompts != nil {
			if _, err := s.prompts.Get(row.PromptID); err != nil {
				return fmt.Errorf("%w: promptId %d not found", ErrInvalidChannel, row.PromptID)
			}
		}
		backendID := agents.BackendID(row.DefaultBackend)
		if backendID == "" {
			backendID = s.defaultAgentBackend
		}
		if !backendID.Valid() {
			return fmt.Errorf("%w: defaultBackend is required in agent mode", ErrInvalidChannel)
		}
		row.DefaultBackend = string(backendID)
	default:
		return fmt.Errorf("%w: invalid targetMode %q", ErrInvalidChannel, row.TargetMode)
	}
	return nil
}

func normalizeTargetMode(raw string) (string, error) {
	trimmed := strings.TrimSpace(strings.ToLower(raw))
	if trimmed == "" {
		return TargetModeAssistant, nil
	}
	if trimmed != TargetModeAssistant && trimmed != TargetModeAgent {
		return "", fmt.Errorf("%w: unsupported targetMode %q", ErrInvalidChannel, raw)
	}
	return trimmed, nil
}

// parseNewCommand checks whether text triggers a new-conversation/session command
// (/new, new, 新建会话, /reset, optionally followed by a space/colon and initial text).
func parseNewCommand(text string) (bool, string) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return false, ""
	}
	lower := strings.ToLower(trimmed)
	for _, cmd := range []string{"/new", "/reset", "new", "新建会话"} {
		if lower == cmd {
			return true, ""
		}
		if strings.HasPrefix(lower, cmd) {
			rest := trimmed[len(cmd):]
			if rest == "" {
				return true, ""
			}
			firstRune, size := utf8.DecodeRuneInString(rest)
			if firstRune == ' ' || firstRune == '\t' || firstRune == '\n' || firstRune == ':' || firstRune == '：' {
				return true, strings.TrimSpace(rest[size:])
			}
		}
	}
	return false, ""
}

func (s *Service) formatChannelStatus(row database.BotChannel) string {
	if row.TargetMode == TargetModeAgent {
		sessionLabel := "尚未创建（发送首条消息自动创建）"
		if row.ActiveSessionID != "" {
			sessionLabel = row.ActiveSessionID
		}
		return fmt.Sprintf("【%s 状态】\n服务模式：内置 Agent 模式（自动放行）\n项目 ID：%d\n当前会话：%s", row.Name, row.DefaultProjectID, sessionLabel)
	}
	chatLabel := "尚未创建（发送首条消息自动创建）"
	if row.ChatID > 0 {
		chatLabel = fmt.Sprintf("#%d", row.ChatID)
	}
	return fmt.Sprintf("【%s 状态】\n服务模式：AI 助手模式\n助手 ID：%d\n当前对话：%s", row.Name, row.ChatGroupID, chatLabel)
}

// PolicyForChat implements chat.ToolPolicyResolver without overriding the bound AI Assistant's
// own tools, prompt or step budget. It only attaches the asynchronous session watch callback
// when the channel has AutoNotify enabled and the assistant launches background agent work.
func (s *Service) PolicyForChat(chatID uint) (chat.ToolPolicy, bool) {
	if chatID == 0 {
		return chat.ToolPolicy{}, false
	}
	ch, err := s.repo.GetChannelByChatID(chatID)
	if err != nil {
		return chat.ToolPolicy{}, false
	}
	if !ch.AutoNotify {
		return chat.ToolPolicy{}, false
	}
	channelID := ch.ID
	return chat.ToolPolicy{
		WrapContext: func(base context.Context) context.Context {
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
// channel receives a completion/failure summary when a background session settles.
func (s *Service) WatchSession(channelID uint, externalUserID string, projectID uint, sessionID string) {
	if s.sessions == nil || channelID == 0 || projectID == 0 || sessionID == "" {
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
	if s.sessions == nil {
		return
	}
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

		res, err := sessions.WaitTurn(watchCtx, s.sessions, watch.ProjectID, watch.SessionID, sessions.TurnWaitOptions{
			Timeout:                 turnTimeout,
			AutoResolveInteractions: true,
			AutoFormAnswer:          "当前会话由外部消息渠道发起并在后台执行，请勿挂起提问，请自主推断并输出清晰结论。",
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
		maxLen := effectiveMaxReplyLength(row.MaxReplyLength)
		sendCtx, sendCancel := context.WithTimeout(s.baseCtx, 30*time.Second)
		defer sendCancel()
		if err := adapter.SendMessage(sendCtx, creds, channels.OutboundMessage{
			ToUserID:     targetUser,
			ContextToken: ctxToken,
			Text:         notice,
		}, maxLen); err != nil {
			log.Printf("bot: push watch result for session %s on channel %d: %v", watch.SessionID, row.ID, err)
		}
	}()
}

func effectiveMaxReplyLength(v int) int {
	if v <= 0 {
		return defaultMaxReplyLength
	}
	return v
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

func (s *Service) toDTO(row database.BotChannel) Channel {
	mode := row.TargetMode
	if mode == "" {
		mode = TargetModeAssistant
	}
	return Channel{
		ID:               row.ID,
		Name:             row.Name,
		ChannelType:      row.ChannelType,
		Enabled:          row.Enabled,
		Status:           row.Status,
		LastError:        row.LastError,
		HasCredentials:   row.SecretRef != "",
		AccountID:        row.AccountID,
		TargetMode:       mode,
		ModelID:          row.ModelID,
		PromptID:         row.PromptID,
		SystemPrompt:     row.SystemPrompt,
		DefaultProjectID: row.DefaultProjectID,
		WorktreeID:       row.WorktreeID,
		DefaultBackend:   row.DefaultBackend,
		AgentModel:       row.AgentModel,
		ChatGroupID:      row.ChatGroupID,
		ChatID:           row.ChatID,
		ActiveSessionID:  row.ActiveSessionID,
		TypingIndicator:  row.TypingIndicator,
		AutoNotify:       row.AutoNotify,
		MaxReplyLength:   effectiveMaxReplyLength(row.MaxReplyLength),
		LastConnectedAt:  row.LastConnectedAt,
		CreatedAt:        row.CreatedAt,
		UpdatedAt:        row.UpdatedAt,
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
