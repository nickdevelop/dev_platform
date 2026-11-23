package flow

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"devplatform/internal/app/sessions"
	"devplatform/internal/app/sessions/turns"
	"devplatform/internal/app/tasks"
	"devplatform/internal/core/agents"
)

// SessionManager defines the subset of session capabilities needed by the telemetry bus.
type SessionManager interface {
	Create(ctx context.Context, projectID uint, backend agents.BackendID, title, message, model string, images ...[]agents.ImageAttachment) (sessions.CreateOutcome, error)
	Send(ctx context.Context, projectID uint, sessionID, text, model string, images ...[]agents.ImageAttachment) (sessions.Session, error)
	Get(projectID uint, sessionID string) (sessions.Session, error)
}

// SessionHistoryReader is an optional interface to read transcripts.
type SessionHistoryReader interface {
	History(ctx context.Context, projectID uint, sessionID string) (sessions.History, error)
}

// SessionStreamer is an optional interface to stream live normalized events from a session.
type SessionStreamer interface {
	SubscribeSession(ctx context.Context, projectID uint, sessionID string) (<-chan agents.NormalizedEvent, error)
}

// InteractionAutoReplier is an optional interface to automatically resolve questions or permissions.
type InteractionAutoReplier interface {
	ReplyForm(ctx context.Context, projectID uint, sessionID, formID string, answer map[string]any) error
	ReplyPermissionRequest(ctx context.Context, projectID uint, sessionID, requestID, decision, message string) error
	ListForms(ctx context.Context, projectID uint, sessionID string) ([]sessions.FormInfo, error)
	ListPermissionRequests(ctx context.Context, projectID uint, sessionID string) ([]sessions.PermissionRequest, error)
}

// TurnOutcome represents the final execution outcome of a role session turn.
type TurnOutcome struct {
	SessionID  string `json:"sessionId"`
	RoleName   string `json:"roleName"`
	Backend    string `json:"backend"`
	Model      string `json:"model"`
	Summary    string `json:"summary"`
	Status     string `json:"status"` // "completed" | "failed"
	Error      string `json:"error,omitempty"`
	DurationMs int64  `json:"durationMs"`
}

// TelemetryBus monitors agent role session data streams during a collaborative flow run.
// It multiplexes live SSE chunks to the flow frontend while aggregating the final turn output
// back to the orchestrator agent.
type TelemetryBus struct {
	manager  SessionManager
	eventBus *EventBus
	runID    uint

	mu          sync.Mutex
	activeSubs  map[string]context.CancelFunc
	activeSess  map[string]struct{}
}

// NewTelemetryBus creates a new telemetry data collector for a flow run.
func NewTelemetryBus(manager SessionManager, eventBus *EventBus, runID uint) *TelemetryBus {
	return &TelemetryBus{
		manager:    manager,
		eventBus:   eventBus,
		runID:      runID,
		activeSubs: make(map[string]context.CancelFunc),
		activeSess: make(map[string]struct{}),
	}
}

// Close cancels all active stream listeners attached to this bus.
func (b *TelemetryBus) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, cancel := range b.activeSubs {
		cancel()
	}
	b.activeSubs = make(map[string]context.CancelFunc)
}

// CollectSessionTurn monitors a role session turn until completion, broadcasting real-time chunks
// and returning the final TurnOutcome to the caller (orchestrator tool).
func (b *TelemetryBus) CollectSessionTurn(
	ctx context.Context,
	projectID uint,
	sessionID string,
	roleName string,
	roleMessageID uint,
	timeout time.Duration,
) (*TurnOutcome, error) {
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}

	watchCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	b.mu.Lock()
	b.activeSubs[sessionID] = cancel
	b.activeSess[sessionID] = struct{}{}
	b.mu.Unlock()

	defer func() {
		b.mu.Lock()
		delete(b.activeSubs, sessionID)
		b.mu.Unlock()
	}()

	startTime := time.Now()

	// 1. Try launching a live stream collector in background to push SSE chunks to frontend.
	if streamer, ok := b.manager.(SessionStreamer); ok {
		go b.pipeLiveStream(watchCtx, streamer, projectID, sessionID, roleName, roleMessageID)
	}

	// 2. State-machine tracking: wait until session reaches idle/stopped/failed
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()

	hasObservedRunning := false
	startGracePeriod := 45 * time.Second

	for {
		select {
		case <-watchCtx.Done():
			return &TurnOutcome{
				SessionID:  sessionID,
				RoleName:   roleName,
				Status:     "failed",
				Error:      fmt.Sprintf("等待 Agent 会话响应超时或被取消（会话 ID: %s）", sessionID),
				DurationMs: time.Since(startTime).Milliseconds(),
			}, watchCtx.Err()
		case <-ticker.C:
		}

		// Auto-resolve any pending question forms or permission requests to prevent the session from hanging
		if replier, ok := b.manager.(InteractionAutoReplier); ok {
			if forms, err := replier.ListForms(watchCtx, projectID, sessionID); err == nil {
				for _, f := range forms {
					_ = replier.ReplyForm(watchCtx, projectID, sessionID, f.ID, map[string]any{
						"answer": "【中控自动应答】: 当前为中控自动化协同执行，请勿挂起提问，请自主推断并在总结报告中呈现。",
					})
				}
			}
			if perms, err := replier.ListPermissionRequests(watchCtx, projectID, sessionID); err == nil {
				for _, p := range perms {
					_ = replier.ReplyPermissionRequest(watchCtx, projectID, sessionID, p.ID, "once", "auto-allowed by flow")
				}
			}
		}

		sess, err := b.manager.Get(projectID, sessionID)
		if err != nil {
			continue
		}

		// Handle abnormal termination
		if sess.Status == sessions.SessionStopped || sess.Status == sessions.SessionDisconnected {
			return &TurnOutcome{
				SessionID:  sessionID,
				RoleName:   roleName,
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

		// Session turn is currently not Running:
		if hasObservedRunning {
			// Phase 2 complete: was running, now settled to idle
			summary := b.extractLatestSummary(watchCtx, projectID, sessionID)
			return &TurnOutcome{
				SessionID:  sessionID,
				RoleName:   roleName,
				Backend:    string(sess.Backend),
				Model:      sess.Model,
				Summary:    summary,
				Status:     "completed",
				DurationMs: time.Since(startTime).Milliseconds(),
			}, nil
		}

		// Has not yet observed Running:
		summary := b.extractLatestSummary(watchCtx, projectID, sessionID)
		if summary != "" {
			// Transient turn finished very quickly
			return &TurnOutcome{
				SessionID:  sessionID,
				RoleName:   roleName,
				Backend:    string(sess.Backend),
				Model:      sess.Model,
				Summary:    summary,
				Status:     "completed",
				DurationMs: time.Since(startTime).Milliseconds(),
			}, nil
		}

		// Check if past startup grace period
		if time.Since(startTime) > startGracePeriod {
			// If session never started running, treat as finished or error based on summary
			if summary == "" {
				summary = "（已在工作区就绪，未产生新增答复文本）"
			}
			return &TurnOutcome{
				SessionID:  sessionID,
				RoleName:   roleName,
				Backend:    string(sess.Backend),
				Model:      sess.Model,
				Summary:    summary,
				Status:     "completed",
				DurationMs: time.Since(startTime).Milliseconds(),
			}, nil
		}
	}
}

func (b *TelemetryBus) pipeLiveStream(
	ctx context.Context,
	streamer SessionStreamer,
	projectID uint,
	sessionID string,
	roleName string,
	roleMessageID uint,
) {
	eventsCh, err := streamer.SubscribeSession(ctx, projectID, sessionID)
	if err != nil {
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-eventsCh:
			if !ok {
				return
			}
			// Auto-resolve interactions immediately when requested on the stream
			if replier, ok := b.manager.(InteractionAutoReplier); ok {
				if ev.Type == agents.EventInteraction && ev.Interaction != nil && ev.Interaction.Phase == agents.InteractionRequested {
					if ev.Interaction.Kind == agents.InteractionForm {
						_ = replier.ReplyForm(ctx, projectID, sessionID, ev.Interaction.ID, map[string]any{
							"answer": "【中控自动应答】: 当前为中控自动化协同执行，请勿挂起提问，请自主推断并在总结报告中呈现。",
						})
					} else if ev.Interaction.Kind == agents.InteractionPermission {
						_ = replier.ReplyPermissionRequest(ctx, projectID, sessionID, ev.Interaction.ID, "once", "auto-allowed by flow")
					}
				}
			}
			chunkText := ""
			if ev.Type == agents.EventMessage && ev.Message != nil && ev.Message.Role == "assistant" {
				chunkText = ev.Message.Text
			}
			if chunkText != "" && b.eventBus != nil {
				b.eventBus.Publish(b.runID, FlowEvent{
					Type:  EventChunk,
					RunID: b.runID,
					Payload: FlowChunkPayload{
						MessageID:  roleMessageID,
						SenderName: roleName,
						SenderType: string(tasks.SenderAgent),
						Chunk:      chunkText,
					},
				})
			}
		}
	}
}

func (b *TelemetryBus) extractLatestSummary(ctx context.Context, projectID uint, sessionID string) string {
	reader, ok := b.manager.(SessionHistoryReader)
	if !ok {
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
