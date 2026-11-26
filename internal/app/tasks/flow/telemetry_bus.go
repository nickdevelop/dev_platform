package flow

import (
	"context"
	"sync"
	"time"

	"devplatform/internal/app/sessions"
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
type SessionHistoryReader = sessions.HistoryFetcher

// SessionStreamer is an optional interface to stream live normalized events from a session.
type SessionStreamer = sessions.StreamSubscriber

// InteractionAutoReplier is an optional interface to automatically resolve questions or permissions.
type InteractionAutoReplier = sessions.InteractionAutoReplier

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

	mu         sync.Mutex
	activeSubs map[string]context.CancelFunc
	activeSess map[string]struct{}
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

	res, err := sessions.WaitTurn(watchCtx, b.manager, projectID, sessionID, sessions.TurnWaitOptions{
		Timeout:                 timeout,
		AutoResolveInteractions: true,
		AutoFormAnswer:          "【中控自动应答】: 当前为中控自动化协同执行，请勿挂起提问，请自主推断并在总结报告中呈现。",
		AutoPermissionReason:    "auto-allowed by flow",
		OnChunk: func(chunkText string) {
			if b.eventBus == nil || chunkText == "" {
				return
			}
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
		},
	})
	return &TurnOutcome{
		SessionID:  res.SessionID,
		RoleName:   roleName,
		Backend:    res.Backend,
		Model:      res.Model,
		Summary:    res.Summary,
		Status:     res.Status,
		Error:      res.Error,
		DurationMs: res.DurationMs,
	}, err
}
