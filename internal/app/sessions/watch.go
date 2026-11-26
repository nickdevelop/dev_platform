package sessions

import (
	"context"
	"fmt"
	"strings"
	"time"

	"devplatform/internal/app/sessions/turns"
	"devplatform/internal/core/agents"
)

// TurnObserver reads the live status and turn state of an agent session.
type TurnObserver interface {
	Get(projectID uint, sessionID string) (Session, error)
}

// HistoryFetcher reads the normalized event history of a session.
type HistoryFetcher interface {
	History(ctx context.Context, projectID uint, sessionID string) (History, error)
}

// StreamSubscriber subscribes to real-time normalized events of a session.
type StreamSubscriber interface {
	SubscribeSession(ctx context.Context, projectID uint, sessionID string) (<-chan agents.NormalizedEvent, error)
}

// InteractionAutoReplier resolves pending forms and permission requests on
// headless / automated turns so execution does not hang indefinitely.
type InteractionAutoReplier interface {
	ReplyForm(ctx context.Context, projectID uint, sessionID, formID string, answer map[string]any) error
	ReplyPermissionRequest(ctx context.Context, projectID uint, sessionID, requestID, decision, message string) error
	ListForms(ctx context.Context, projectID uint, sessionID string) ([]FormInfo, error)
	ListPermissionRequests(ctx context.Context, projectID uint, sessionID string) ([]PermissionRequest, error)
}

// TurnWaitOptions configures WaitTurn behavior.
type TurnWaitOptions struct {
	Timeout                 time.Duration
	StartGracePeriod        time.Duration
	AutoResolveInteractions bool
	AutoFormAnswer          string
	AutoPermissionReason    string
	OnChunk                 func(chunk string)
}

// TurnResult captures the outcome of a single watched agent turn.
type TurnResult struct {
	SessionID  string `json:"sessionId"`
	Backend    string `json:"backend"`
	Model      string `json:"model"`
	Summary    string `json:"summary"`
	Status     string `json:"status"` // "completed" | "failed"
	Error      string `json:"error,omitempty"`
	DurationMs int64  `json:"durationMs"`
}

const (
	defaultTurnWaitTimeout  = 10 * time.Minute
	defaultStartGracePeriod = 45 * time.Second
	defaultTurnPollInterval = 300 * time.Millisecond
)

// WaitTurn monitors a session until its current turn settles (Running -> Idle)
// or terminates, optionally streaming assistant chunks and auto-resolving
// blocking interactions. Both collaborative flows and bot watches share this helper.
func WaitTurn(
	ctx context.Context,
	observer TurnObserver,
	projectID uint,
	sessionID string,
	opts TurnWaitOptions,
) (*TurnResult, error) {
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = defaultTurnWaitTimeout
	}
	grace := opts.StartGracePeriod
	if grace <= 0 {
		grace = defaultStartGracePeriod
	}

	watchCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	startTime := time.Now()

	if subscriber, ok := observer.(StreamSubscriber); ok && (opts.OnChunk != nil || opts.AutoResolveInteractions) {
		go pipeSessionStream(watchCtx, observer, subscriber, projectID, sessionID, opts)
	}

	ticker := time.NewTicker(defaultTurnPollInterval)
	defer ticker.Stop()

	hasObservedRunning := false

	for {
		select {
		case <-watchCtx.Done():
			return &TurnResult{
				SessionID:  sessionID,
				Status:     "failed",
				Error:      fmt.Sprintf("等待 Agent 会话响应超时或被取消（会话 ID: %s）", sessionID),
				DurationMs: time.Since(startTime).Milliseconds(),
			}, watchCtx.Err()
		case <-ticker.C:
		}

		if opts.AutoResolveInteractions {
			if replier, ok := observer.(InteractionAutoReplier); ok {
				AutoResolvePendingInteractions(watchCtx, replier, projectID, sessionID, opts.AutoFormAnswer, opts.AutoPermissionReason)
			}
		}

		sess, err := observer.Get(projectID, sessionID)
		if err != nil {
			continue
		}

		if sess.Status == SessionStopped || sess.Status == SessionDisconnected {
			return &TurnResult{
				SessionID:  sessionID,
				Backend:    string(sess.Backend),
				Model:      sess.Model,
				Status:     "failed",
				Error:      fmt.Sprintf("Agent 会话异常终止（状态: %s）", sess.Status),
				DurationMs: time.Since(startTime).Milliseconds(),
			}, fmt.Errorf("session terminated with status: %s", sess.Status)
		}

		if sess.Turn == turns.Running {
			hasObservedRunning = true
			continue
		}

		summary := ""
		if fetcher, ok := observer.(HistoryFetcher); ok {
			summary = ExtractLatestSummary(watchCtx, fetcher, projectID, sessionID)
		}

		if hasObservedRunning {
			return &TurnResult{
				SessionID:  sessionID,
				Backend:    string(sess.Backend),
				Model:      sess.Model,
				Summary:    summary,
				Status:     "completed",
				DurationMs: time.Since(startTime).Milliseconds(),
			}, nil
		}

		if summary != "" {
			return &TurnResult{
				SessionID:  sessionID,
				Backend:    string(sess.Backend),
				Model:      sess.Model,
				Summary:    summary,
				Status:     "completed",
				DurationMs: time.Since(startTime).Milliseconds(),
			}, nil
		}

		if time.Since(startTime) > grace {
			return &TurnResult{
				SessionID:  sessionID,
				Backend:    string(sess.Backend),
				Model:      sess.Model,
				Summary:    "（已在工作区就绪，未产生新增答复文本）",
				Status:     "completed",
				DurationMs: time.Since(startTime).Milliseconds(),
			}, nil
		}
	}
}

func pipeSessionStream(
	ctx context.Context,
	observer TurnObserver,
	subscriber StreamSubscriber,
	projectID uint,
	sessionID string,
	opts TurnWaitOptions,
) {
	eventsCh, err := subscriber.SubscribeSession(ctx, projectID, sessionID)
	if err != nil {
		return
	}
	replier, hasReplier := observer.(InteractionAutoReplier)

	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-eventsCh:
			if !ok {
				return
			}
			if opts.AutoResolveInteractions && hasReplier {
				AutoResolveStreamInteraction(ctx, replier, projectID, sessionID, ev, opts.AutoFormAnswer, opts.AutoPermissionReason)
			}
			if opts.OnChunk != nil && ev.Type == agents.EventMessage && ev.Message != nil && ev.Message.Role == "assistant" {
				if ev.Message.Text != "" {
					opts.OnChunk(ev.Message.Text)
				}
			}
		}
	}
}

// AutoResolvePendingInteractions answers all currently pending forms and permission
// requests for an unattended session turn.
func AutoResolvePendingInteractions(
	ctx context.Context,
	replier InteractionAutoReplier,
	projectID uint,
	sessionID string,
	formAnswer string,
	permReason string,
) {
	if replier == nil {
		return
	}
	if strings.TrimSpace(formAnswer) == "" {
		formAnswer = "当前为自动化后台执行，请勿挂起提问，请自主推断并在总结报告中呈现。"
	}
	if strings.TrimSpace(permReason) == "" {
		permReason = "auto-allowed by automation"
	}
	if forms, err := replier.ListForms(ctx, projectID, sessionID); err == nil {
		for _, f := range forms {
			_ = replier.ReplyForm(ctx, projectID, sessionID, f.ID, map[string]any{
				"answer": formAnswer,
			})
		}
	}
	if perms, err := replier.ListPermissionRequests(ctx, projectID, sessionID); err == nil {
		for _, p := range perms {
			_ = replier.ReplyPermissionRequest(ctx, projectID, sessionID, p.ID, "once", permReason)
		}
	}
}

// AutoResolveStreamInteraction immediately answers an incoming interaction event
// from a live normalized stream.
func AutoResolveStreamInteraction(
	ctx context.Context,
	replier InteractionAutoReplier,
	projectID uint,
	sessionID string,
	ev agents.NormalizedEvent,
	formAnswer string,
	permReason string,
) {
	if replier == nil || ev.Type != agents.EventInteraction || ev.Interaction == nil || ev.Interaction.Phase != agents.InteractionRequested {
		return
	}
	if strings.TrimSpace(formAnswer) == "" {
		formAnswer = "当前为自动化后台执行，请勿挂起提问，请自主推断并在总结报告中呈现。"
	}
	if strings.TrimSpace(permReason) == "" {
		permReason = "auto-allowed by automation"
	}
	switch ev.Interaction.Kind {
	case agents.InteractionForm:
		_ = replier.ReplyForm(ctx, projectID, sessionID, ev.Interaction.ID, map[string]any{
			"answer": formAnswer,
		})
	case agents.InteractionPermission:
		_ = replier.ReplyPermissionRequest(ctx, projectID, sessionID, ev.Interaction.ID, "once", permReason)
	}
}

// ExtractLatestSummary returns the most recent non-empty assistant text message
// from the session's history.
func ExtractLatestSummary(ctx context.Context, reader HistoryFetcher, projectID uint, sessionID string) string {
	if reader == nil {
		return ""
	}
	hist, err := reader.History(ctx, projectID, sessionID)
	if err != nil || len(hist.Events) == 0 {
		return ""
	}
	for i := len(hist.Events) - 1; i >= 0; i-- {
		ev := hist.Events[i]
		if ev.Type == agents.EventMessage && ev.Message != nil && ev.Message.Role == "assistant" && strings.TrimSpace(ev.Message.Text) != "" {
			return strings.TrimSpace(ev.Message.Text)
		}
	}
	return ""
}
