package chat

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"

	"devplatform/internal/app/modelconfig"
	"devplatform/internal/core/textutil"
)

// maxGenerationDuration bounds one generation. It is a safety net for a
// wedged upstream connection, not a normal product limit: a timed-out run
// keeps its partial text and is reported as failed so it can be regenerated.
const maxGenerationDuration = 10 * time.Minute

// titleGenerationTimeout bounds the one-shot automatic title request.
const titleGenerationTimeout = 20 * time.Second

// destroyWait bounds how long a delete waits for the generation it cancelled.
const destroyWait = 5 * time.Second

// ModelResolver is the subset of the platform model configuration a chat
// needs. *modelconfig.Service implements it.
type ModelResolver interface {
	ResolveModel(id uint) (modelconfig.ResolvedModel, error)
}

// Option customizes the chat service. Tests use it to inject a fake upstream
// client or an in-memory repository.
type Option func(*Service)

// WithCompleter replaces the OpenAI-compatible upstream client.
func WithCompleter(completer Completer) Option {
	return func(s *Service) {
		if completer != nil {
			s.completer = completer
		}
	}
}

// WithRepository replaces the GORM repository.
func WithRepository(repo Repository) Option {
	return func(s *Service) {
		if repo != nil {
			s.repo = repo
		}
	}
}

// Service owns pure-chat conversations, their persistence and their in-process
// generation state.
type Service struct {
	repo      Repository
	models    ModelResolver
	completer Completer
	manager   *runManager

	baseCtx   context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	closeOnce sync.Once
}

// NewService builds the chat service over the platform database.
func NewService(db *gorm.DB, models ModelResolver, opts ...Option) *Service {
	return NewServiceWithRepository(NewGORMRepository(db), models, opts...)
}

// NewServiceWithRepository builds the chat service over a custom repository.
func NewServiceWithRepository(repo Repository, models ModelResolver, opts ...Option) *Service {
	baseCtx, cancel := context.WithCancel(context.Background())
	service := &Service{
		repo:      repo,
		models:    models,
		completer: NewOpenAICompleter(),
		manager:   newRunManager(),
		baseCtx:   baseCtx,
		cancel:    cancel,
	}
	for _, opt := range opts {
		opt(service)
	}
	if err := service.repo.MarkPendingFailed("generation_interrupted", "生成被中断，可重新生成。"); err != nil {
		log.Printf("chat: mark interrupted generations: %v", err)
	}
	return service
}

// Close cancels every generation and waits for the goroutines to finish. It is
// safe to call more than once; the supplied context bounds the wait.
func (s *Service) Close(ctx context.Context) {
	s.closeOnce.Do(func() {
		s.cancel()
		for _, run := range s.manager.cancelAll() {
			if !s.manager.wait(run, ctx) {
				break
			}
		}
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

// List returns conversations ordered by most recent activity. Running is the
// live generation state, not a persisted column.
func (s *Service) List(limit, offset int) ([]Chat, int, error) {
	chats, total, err := s.repo.ListChats(limit, offset)
	if err != nil {
		return nil, 0, err
	}
	for i := range chats {
		chats[i].Running = s.manager.running(chats[i].ID)
	}
	return chats, total, nil
}

// ListByGroup returns conversations in one group ordered by most recent
// activity. The group must exist; an unknown group is a 404, not an empty page.
func (s *Service) ListByGroup(groupID uint, limit, offset int) ([]Chat, int, error) {
	if _, err := s.repo.GetGroup(groupID); err != nil {
		return nil, 0, err
	}
	chats, total, err := s.repo.ListChatsByGroup(groupID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	for i := range chats {
		chats[i].Running = s.manager.running(chats[i].ID)
	}
	return chats, total, nil
}

// Get returns one conversation with its live running state.
func (s *Service) Get(id uint) (Chat, error) {
	chat, err := s.repo.GetChat(id)
	if err != nil {
		return Chat{}, err
	}
	chat.Running = s.manager.running(chat.ID)
	return chat, nil
}

// Create creates a conversation and its first exchange, then starts
// generation. The chat is returned immediately; the answer arrives on SSE.
func (s *Service) Create(ctx context.Context, input CreateInput) (Chat, error) {
	text, images, err := normalizeInput(input.Text, input.Images)
	if err != nil {
		return Chat{}, err
	}

	var (
		groupID      uint
		systemPrompt string
		modelID      = input.ModelID
	)
	if input.GroupID != nil && *input.GroupID != 0 {
		group, err := s.repo.GetGroup(*input.GroupID)
		if err != nil {
			return Chat{}, err
		}
		groupID = *input.GroupID
		systemPrompt = group.SystemPrompt
		if modelID == 0 && group.DefaultModelID != 0 {
			modelID = group.DefaultModelID
		}
	}

	model, err := s.resolveModel(modelID)
	if err != nil {
		return Chat{}, err
	}
	if len(images) > 0 && !model.SupportsImage {
		return Chat{}, ErrUnsupportedImage
	}

	now := time.Now().UTC()
	chat := Chat{
		GroupID:      groupID,
		Title:        fallbackTitle(text),
		ModelID:      model.ID,
		ModelName:    model.Name,
		SystemPrompt: systemPrompt,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	user := Message{
		Role:      RoleUser,
		Text:      text,
		Images:    images,
		Status:    StatusComplete,
		CreatedAt: now,
		UpdatedAt: now,
	}
	assistant := pendingAssistant(model, now)
	if err := s.repo.CreateChat(&chat, &user, &assistant); err != nil {
		return Chat{}, err
	}

	run, runCtx, err := s.begin(chat.ID)
	if err != nil {
		_ = s.repo.DeleteChat(chat.ID)
		return Chat{}, err
	}
	s.manager.setAssistantID(run, assistant.ID)
	if err := s.publishSnapshot(chat.ID); err != nil {
		log.Printf("chat: publish initial snapshot for %d: %v", chat.ID, err)
	}
	s.startGeneration(runCtx, run, chat, model, text)
	chat.Running = true
	return chat, nil
}

// Send appends a user message and an assistant placeholder, then starts
// generation. A supplied model switches the conversation for this and later
// turns; an empty model keeps the current one.
func (s *Service) Send(ctx context.Context, chatID uint, input SendInput) (Chat, error) {
	chat, err := s.repo.GetChat(chatID)
	if err != nil {
		return Chat{}, err
	}
	text, images, err := normalizeInput(input.Text, input.Images)
	if err != nil {
		return Chat{}, err
	}
	modelID := chat.ModelID
	if input.ModelID != 0 {
		modelID = input.ModelID
	}
	model, err := s.resolveModel(modelID)
	if err != nil {
		return Chat{}, err
	}
	if len(images) > 0 && !model.SupportsImage {
		return Chat{}, ErrUnsupportedImage
	}

	run, runCtx, err := s.begin(chat.ID)
	if err != nil {
		return Chat{}, err
	}
	now := time.Now().UTC()
	user := Message{
		Role:      RoleUser,
		Text:      text,
		Images:    images,
		Status:    StatusComplete,
		CreatedAt: now,
		UpdatedAt: now,
	}
	assistant := pendingAssistant(model, now)
	chat.ModelID = model.ID
	chat.ModelName = model.Name
	if err := s.repo.AppendExchange(&chat, &user, &assistant); err != nil {
		s.abort(run)
		return Chat{}, err
	}
	s.manager.setAssistantID(run, assistant.ID)
	if err := s.publishSnapshot(chat.ID); err != nil {
		log.Printf("chat: publish send snapshot for %d: %v", chat.ID, err)
	}
	s.startGeneration(runCtx, run, chat, model, "")
	chat.Running = true
	return chat, nil
}

// Stop cancels the conversation's active generation. Stopping an idle chat is
// a no-op so the endpoint stays idempotent.
func (s *Service) Stop(chatID uint) error {
	if _, err := s.repo.GetChat(chatID); err != nil {
		return err
	}
	s.manager.cancel(chatID)
	return nil
}

// Update renames a conversation and/or changes its model.
func (s *Service) Update(chatID uint, input UpdateInput) (Chat, error) {
	chat, err := s.repo.GetChat(chatID)
	if err != nil {
		return Chat{}, err
	}
	if input.Title != nil {
		title := strings.TrimSpace(*input.Title)
		if title == "" || utf8.RuneCountInString(title) > MaxTitleRunes {
			return Chat{}, ErrInvalidTitle
		}
		chat.Title = title
	}
	if input.ModelID != nil {
		model, err := s.resolveModel(*input.ModelID)
		if err != nil {
			return Chat{}, err
		}
		chat.ModelID = model.ID
		chat.ModelName = model.Name
	}
	if input.GroupID != nil {
		targetID := *input.GroupID
		if targetID != chat.GroupID && targetID != 0 {
			if _, err := s.repo.GetGroup(targetID); err != nil {
				return Chat{}, err
			}
		}
		chat.GroupID = targetID
	}
	if err := s.repo.UpdateChat(&chat); err != nil {
		return Chat{}, err
	}
	chat.Running = s.manager.running(chat.ID)
	s.manager.broadcast(chat.ID, Event{
		ChatID:       chat.ID,
		Timestamp:    time.Now().UTC(),
		Type:         EventConversation,
		Conversation: &chat,
	})
	return chat, nil
}

// Delete removes a conversation and all its messages after cancelling any
// active generation. Subscribers receive a closed event before the stream is
// released.
func (s *Service) Delete(chatID uint) error {
	if _, err := s.repo.GetChat(chatID); err != nil {
		return err
	}
	if run := s.manager.currentRun(chatID); run != nil {
		run.cancel()
		waitCtx, cancel := context.WithTimeout(context.Background(), destroyWait)
		s.manager.wait(run, waitCtx)
		cancel()
	}
	if err := s.repo.DeleteChat(chatID); err != nil {
		return err
	}
	s.manager.broadcast(chatID, Event{
		ChatID:    chatID,
		Timestamp: time.Now().UTC(),
		Type:      EventClosed,
		Reason:    "deleted",
	})
	s.manager.remove(chatID)
	return nil
}

// Regenerate removes the last assistant answer (everything after the last user
// message) and asks the model again.
func (s *Service) Regenerate(ctx context.Context, chatID uint) (Chat, error) {
	chat, err := s.repo.GetChat(chatID)
	if err != nil {
		return Chat{}, err
	}
	model, err := s.resolveModel(chat.ModelID)
	if err != nil {
		return Chat{}, err
	}
	messages, err := s.repo.ListMessages(chat.ID)
	if err != nil {
		return Chat{}, err
	}
	lastUser, ok := lastUserMessage(messages)
	if !ok {
		return Chat{}, ErrInvalidMessage
	}

	run, runCtx, err := s.begin(chat.ID)
	if err != nil {
		return Chat{}, err
	}
	now := time.Now().UTC()
	assistant := pendingAssistant(model, now)
	if err := s.repo.Regenerate(&chat, lastUser.ID, &assistant); err != nil {
		s.abort(run)
		return Chat{}, err
	}
	s.manager.setAssistantID(run, assistant.ID)
	if err := s.publishSnapshot(chat.ID); err != nil {
		log.Printf("chat: publish regenerate snapshot for %d: %v", chat.ID, err)
	}
	s.startGeneration(runCtx, run, chat, model, "")
	chat.Running = true
	return chat, nil
}

// EditAndResend validates a user message, truncates it and everything after
// it, inserts the edited message plus a fresh assistant placeholder, and
// starts generation. This is deliberately a destructive, branch-free edit.
func (s *Service) EditAndResend(ctx context.Context, chatID, messageID uint, input EditInput) (Chat, error) {
	chat, err := s.repo.GetChat(chatID)
	if err != nil {
		return Chat{}, err
	}
	target, err := s.repo.GetMessage(chatID, messageID)
	if err != nil {
		return Chat{}, err
	}
	if target.Role != RoleUser {
		return Chat{}, ErrMessageNotEditable
	}
	text, images, err := normalizeInput(input.Text, input.Images)
	if err != nil {
		return Chat{}, err
	}
	model, err := s.resolveModel(chat.ModelID)
	if err != nil {
		return Chat{}, err
	}
	if len(images) > 0 && !model.SupportsImage {
		return Chat{}, ErrUnsupportedImage
	}

	messages, err := s.repo.ListMessages(chatID)
	if err != nil {
		return Chat{}, err
	}
	isFirstUser := true
	for _, message := range messages {
		if message.ID >= target.ID {
			break
		}
		if message.Role == RoleUser {
			isFirstUser = false
			break
		}
	}
	firstText := ""
	oldTitle := chat.Title
	titleChanged := false
	if isFirstUser {
		firstText = text
		if chat.Title == fallbackTitle(target.Text) {
			chat.Title = fallbackTitle(text)
			titleChanged = chat.Title != oldTitle
		}
	}

	run, runCtx, err := s.begin(chat.ID)
	if err != nil {
		return Chat{}, err
	}
	now := time.Now().UTC()
	user := Message{
		Role:      RoleUser,
		Text:      text,
		Images:    images,
		Status:    StatusComplete,
		CreatedAt: now,
		UpdatedAt: now,
	}
	assistant := pendingAssistant(model, now)
	if err := s.repo.ReplaceFromMessage(&chat, target.ID, &user, &assistant); err != nil {
		s.abort(run)
		return Chat{}, err
	}
	if titleChanged {
		if _, err := s.repo.UpdateChatTitleIf(chat.ID, oldTitle, chat.Title); err != nil {
			log.Printf("chat: update edited fallback title for %d: %v", chat.ID, err)
		}
	}
	s.manager.setAssistantID(run, assistant.ID)
	if err := s.publishSnapshot(chat.ID); err != nil {
		log.Printf("chat: publish edit snapshot for %d: %v", chat.ID, err)
	}
	s.startGeneration(runCtx, run, chat, model, firstText)
	chat.Running = true
	return chat, nil
}

func (s *Service) begin(chatID uint) (*runState, context.Context, error) {
	runCtx, cancel := context.WithTimeout(s.baseCtx, maxGenerationDuration)
	run := &runState{
		chatID: chatID,
		cancel: cancel,
		done:   make(chan struct{}),
	}
	if err := s.manager.tryStart(run); err != nil {
		cancel()
		return nil, nil, err
	}
	return run, runCtx, nil
}

func (s *Service) abort(run *runState) {
	run.cancel()
	s.manager.abort(run)
}

func (s *Service) startGeneration(ctx context.Context, run *runState, chat Chat, model modelconfig.ResolvedModel, firstUserText string) {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.runGeneration(ctx, run, chat, model, firstUserText)
	}()
}

func (s *Service) runGeneration(ctx context.Context, run *runState, chat Chat, model modelconfig.ResolvedModel, firstUserText string) {
	request, err := s.completionRequest(chat.ID, run.assistantID, model)
	if err != nil {
		s.finishRun(run, chat, StatusFailed, OutcomeFailed, "generation_failed", err.Error(),
			&runView{AssistantID: run.assistantID})
		return
	}

	streamErr := s.completer.Stream(ctx, request, func(delta CompletionDelta) error {
		if delta.Usage != nil {
			s.manager.setUsage(run, delta.Usage)
		}
		s.manager.emitDelta(run, delta.Text, delta.Reasoning)
		return nil
	})

	status, outcome, code, detail := classifyStreamError(ctx, streamErr)
	view := s.manager.view(run)
	if view == nil {
		view = &runView{AssistantID: run.assistantID}
	}
	autoTitle := status == StatusComplete && firstUserText != "" && chat.Title == fallbackTitle(firstUserText)
	s.finishRun(run, chat, status, outcome, code, detail, view)
	if autoTitle {
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.generateTitle(chat.ID, firstUserText, fallbackTitle(firstUserText))
		}()
	}
}

func classifyStreamError(ctx context.Context, err error) (status, outcome, code, detail string) {
	if err == nil {
		return StatusComplete, OutcomeCompleted, "", ""
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		if errors.Is(ctxErr, context.DeadlineExceeded) {
			return StatusFailed, OutcomeFailed, "generation_timeout", "生成超时，可重新生成。"
		}
		return StatusCancelled, OutcomeCancelled, "", ""
	}
	if errors.Is(err, ErrUpstreamUnreachable) {
		return StatusFailed, OutcomeFailed, "upstream_unreachable", err.Error()
	}
	if errors.Is(err, ErrUpstreamError) {
		return StatusFailed, OutcomeFailed, "upstream_error", err.Error()
	}
	return StatusFailed, OutcomeFailed, "upstream_error", err.Error()
}

func (s *Service) finishRun(run *runState, chat Chat, status, outcome, code, detail string, view *runView) {
	now := time.Now().UTC()
	createdAt := now
	if existing, err := s.repo.GetMessage(chat.ID, run.assistantID); err == nil {
		createdAt = existing.CreatedAt
	}
	final := Message{
		ID:        run.assistantID,
		ChatID:    chat.ID,
		Role:      RoleAssistant,
		Text:      view.Text,
		Reasoning: view.Reasoning,
		Status:    status,
		ModelID:   chat.ModelID,
		ModelName: chat.ModelName,
		Usage:     view.Usage,
		ErrorCode: code,
		Error:     detail,
		CreatedAt: createdAt,
		UpdatedAt: now,
	}
	if err := s.repo.UpdateMessage(&final); err != nil {
		log.Printf("chat: persist final message %d: %v", final.ID, err)
	}
	// The conversation may have been renamed or switched to another model while
	// this answer streamed. Re-read it so finishing a turn never overwrites a
	// concurrent change; the message keeps the model that actually produced it.
	current := chat
	if latest, err := s.repo.GetChat(chat.ID); err == nil {
		current = latest
	}
	current.UpdatedAt = now
	current.Running = false
	if err := s.repo.TouchChat(chat.ID); err != nil {
		log.Printf("chat: touch chat %d after generation: %v", chat.ID, err)
	}
	s.manager.finishAndPublish(run,
		Event{ChatID: chat.ID, Timestamp: now, Type: EventMessageUpdate, Message: &final},
		Event{ChatID: chat.ID, Timestamp: now, Type: EventTurn, Turn: &Turn{Phase: TurnEnded, Outcome: outcome}},
		Event{ChatID: chat.ID, Timestamp: now, Type: EventConversation, Conversation: &current},
	)
}

func (s *Service) completionRequest(chatID, assistantID uint, model modelconfig.ResolvedModel) (CompletionRequest, error) {
	chat, err := s.repo.GetChat(chatID)
	if err != nil {
		return CompletionRequest{}, err
	}
	messages, err := s.repo.ListMessages(chatID)
	if err != nil {
		return CompletionRequest{}, err
	}
	out := make([]CompletionMessage, 0, len(messages)+1)
	if strings.TrimSpace(chat.SystemPrompt) != "" {
		out = append(out, CompletionMessage{
			Role: "system",
			Text: strings.TrimSpace(chat.SystemPrompt),
		})
	}
	for _, message := range messages {
		if message.ID == assistantID {
			continue
		}
		if message.Role == RoleAssistant {
			if message.Status != StatusComplete || strings.TrimSpace(message.Text) == "" {
				continue
			}
		}
		if strings.TrimSpace(message.Text) == "" && len(message.Images) == 0 {
			continue
		}
		out = append(out, CompletionMessage{Role: message.Role, Text: message.Text, Images: message.Images})
	}
	return CompletionRequest{
		BaseURL:  model.BaseURL,
		APIKey:   model.APIKey,
		Model:    model.ModelID,
		Messages: out,
	}, nil
}

func (s *Service) generateTitle(chatID uint, source, fallback string) {
	ctx, cancel := context.WithTimeout(s.baseCtx, titleGenerationTimeout)
	defer cancel()

	chat, err := s.repo.GetChat(chatID)
	if err != nil || chat.Title != fallback {
		return
	}
	// The title prompt only needs the opening of a long first message; sending
	// the whole transcript would spend tokens for no extra signal.
	source = textutil.DeriveTitleLimit(source, 500)
	if source == "" {
		return
	}
	model, err := s.models.ResolveModel(chat.ModelID)
	if err != nil {
		return
	}
	title, _, err := s.completer.Complete(ctx, CompletionRequest{
		BaseURL: model.BaseURL,
		APIKey:  model.APIKey,
		Model:   model.ModelID,
		Messages: []CompletionMessage{
			{Role: "system", Text: "你是一个对话标题生成器。请用不超过 20 个汉字概括用户消息，只输出标题本身，不要引号、句号或解释。"},
			{Role: "user", Text: source},
		},
	}, 32)
	if err != nil {
		return
	}
	title = sanitizeGeneratedTitle(title)
	if title == "" || title == fallback {
		return
	}
	chat, err = s.repo.GetChat(chatID)
	if err != nil || chat.Title != fallback {
		return
	}
	applied, err := s.repo.UpdateChatTitleIf(chatID, fallback, title)
	if err != nil || !applied {
		return
	}
	chat.Title = title
	chat.UpdatedAt = time.Now().UTC()
	chat.Running = s.manager.running(chat.ID)
	s.manager.broadcast(chat.ID, Event{
		ChatID:       chat.ID,
		Timestamp:    time.Now().UTC(),
		Type:         EventConversation,
		Conversation: &chat,
	})
}

func (s *Service) resolveModel(id uint) (modelconfig.ResolvedModel, error) {
	if id == 0 || s.models == nil {
		return modelconfig.ResolvedModel{}, ErrModelUnavailable
	}
	model, err := s.models.ResolveModel(id)
	if err != nil {
		return modelconfig.ResolvedModel{}, fmt.Errorf("%w: %v", ErrModelUnavailable, err)
	}
	return model, nil
}

func (s *Service) snapshot(chatID uint, run *runView) (Event, error) {
	chat, err := s.repo.GetChat(chatID)
	if err != nil {
		return Event{}, err
	}
	messages, truncated, err := s.repo.ListRecentMessages(chatID, MaxSnapshotMessages)
	if err != nil {
		return Event{}, err
	}
	chat.Running = run != nil
	if run != nil {
		for i := range messages {
			if messages[i].ID != run.AssistantID {
				continue
			}
			messages[i].Text = run.Text
			messages[i].Reasoning = run.Reasoning
			messages[i].Status = StatusPending
			messages[i].Usage = run.Usage
			break
		}
	}
	return Event{
		ChatID:    chatID,
		Timestamp: time.Now().UTC(),
		Type:      EventSnapshot,
		Snapshot: &Snapshot{
			Conversation: chat,
			Messages:     messages,
			Running:      run != nil,
			Truncated:    truncated,
		},
	}, nil
}

func (s *Service) publishSnapshot(chatID uint) error {
	_, err := s.manager.publish(chatID, func(run *runView) (Event, error) {
		return s.snapshot(chatID, run)
	})
	return err
}

func normalizeInput(text string, images []ImageAttachment) (string, []ImageAttachment, error) {
	text = strings.TrimSpace(text)
	if text == "" && len(images) == 0 {
		return "", nil, ErrInvalidMessage
	}
	if len(images) > MaxImages {
		return "", nil, fmt.Errorf("%w: at most %d images", ErrInvalidImage, MaxImages)
	}
	out := make([]ImageAttachment, 0, len(images))
	for index, image := range images {
		mediaType := strings.ToLower(strings.TrimSpace(image.MediaType))
		switch mediaType {
		case "image/jpeg", "image/png", "image/webp", "image/gif":
		default:
			return "", nil, fmt.Errorf("%w: unsupported media type %q", ErrInvalidImage, image.MediaType)
		}
		data := image.Data
		if comma := strings.Index(data, ","); strings.HasPrefix(data, "data:") && comma >= 0 {
			data = data[comma+1:]
		}
		decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(data))
		if err != nil || len(decoded) == 0 {
			return "", nil, fmt.Errorf("%w: image %d is not valid base64", ErrInvalidImage, index+1)
		}
		if len(decoded) > MaxImageBytes {
			return "", nil, fmt.Errorf("%w: image %d exceeds %d bytes", ErrInvalidImage, index+1, MaxImageBytes)
		}
		name := strings.TrimSpace(image.Name)
		if name == "" {
			name = fmt.Sprintf("image-%d", index+1)
		}
		id := strings.TrimSpace(image.ID)
		if id == "" {
			id = fmt.Sprintf("img-%d-%d", time.Now().UnixNano(), index)
		}
		out = append(out, ImageAttachment{
			ID:        id,
			Name:      name,
			MediaType: mediaType,
			Data:      base64.StdEncoding.EncodeToString(decoded),
			Size:      len(decoded),
		})
	}
	return text, out, nil
}

func pendingAssistant(model modelconfig.ResolvedModel, now time.Time) Message {
	return Message{
		Role:      RoleAssistant,
		Status:    StatusPending,
		ModelID:   model.ID,
		ModelName: model.Name,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func lastUserMessage(messages []Message) (Message, bool) {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == RoleUser {
			return messages[i], true
		}
	}
	return Message{}, false
}

func fallbackTitle(text string) string {
	if strings.TrimSpace(text) == "" {
		return "图片对话"
	}
	return textutil.DeriveTitle(text)
}

func sanitizeGeneratedTitle(raw string) string {
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		line = strings.Trim(line, "\"'`“”‘’")
		line = strings.TrimPrefix(line, "标题：")
		line = strings.TrimPrefix(line, "标题:")
		line = strings.TrimSpace(line)
		line = strings.Trim(line, "\"'`“”‘’")
		line = strings.TrimRight(line, "。.!！?？")
		line = strings.Join(strings.Fields(line), " ")
		if line == "" {
			continue
		}
		return textutil.DeriveTitleLimit(line, 40)
	}
	return ""
}

func normalizeGroupName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" {
		return "", ErrInvalidGroupName
	}
	if utf8.RuneCountInString(name) > MaxGroupNameRunes {
		return "", ErrInvalidGroupName
	}
	return name, nil
}

// ListGroups returns all chat conversation groups.
func (s *Service) ListGroups() ([]ChatGroup, error) {
	return s.repo.ListGroups()
}

// CreateGroup creates a new chat conversation group / assistant.
func (s *Service) CreateGroup(input CreateGroupInput) (ChatGroup, error) {
	name, err := normalizeGroupName(input.Name)
	if err != nil {
		return ChatGroup{}, err
	}
	group := ChatGroup{
		Name:           name,
		DefaultModelID: input.DefaultModelID,
		SystemPrompt:   input.SystemPrompt,
		Icon:           input.Icon,
		ShowInFloat:    input.ShowInFloat,
	}
	if err := s.repo.CreateGroup(&group); err != nil {
		return ChatGroup{}, err
	}
	return group, nil
}

// UpdateGroup updates an existing chat conversation group / assistant.
func (s *Service) UpdateGroup(id uint, input UpdateGroupInput) (ChatGroup, error) {
	group, err := s.repo.GetGroup(id)
	if err != nil {
		return ChatGroup{}, err
	}
	name, err := normalizeGroupName(input.Name)
	if err != nil {
		return ChatGroup{}, err
	}
	group.Name = name
	group.DefaultModelID = input.DefaultModelID
	group.SystemPrompt = input.SystemPrompt
	group.Icon = input.Icon
	group.ShowInFloat = input.ShowInFloat
	if err := s.repo.UpdateGroup(&group); err != nil {
		return ChatGroup{}, err
	}
	return group, nil
}

// DeleteGroup removes a chat group / assistant and all chats inside it.
func (s *Service) DeleteGroup(id uint) error {
	if _, err := s.repo.GetGroup(id); err != nil {
		return err
	}
	chats, _, err := s.repo.ListChats(MaxListLimit, 0)
	if err == nil {
		for _, c := range chats {
			if c.GroupID == id {
				if run := s.manager.currentRun(c.ID); run != nil {
					run.cancel()
					waitCtx, cancel := context.WithTimeout(context.Background(), destroyWait)
					s.manager.wait(run, waitCtx)
					cancel()
				}
				s.manager.broadcast(c.ID, Event{
					ChatID:    c.ID,
					Timestamp: time.Now().UTC(),
					Type:      EventClosed,
					Reason:    "group_deleted",
				})
				s.manager.remove(c.ID)
			}
		}
	}
	return s.repo.DeleteGroup(id)
}
