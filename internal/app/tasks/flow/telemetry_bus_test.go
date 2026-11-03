package flow

import (
	"context"
	"testing"
	"time"

	"devplatform/internal/app/sessions"
	"devplatform/internal/app/sessions/turns"
	"devplatform/internal/core/agents"
)

type fakeSessionManager struct {
	sessions map[string]sessions.Session
	histories map[string]sessions.History
}

func (m *fakeSessionManager) Create(ctx context.Context, projectID uint, backend agents.BackendID, title, message, model string, images ...[]agents.ImageAttachment) (sessions.CreateOutcome, error) {
	return sessions.CreateOutcome{}, nil
}

func (m *fakeSessionManager) Send(ctx context.Context, projectID uint, sessionID, text, model string, images ...[]agents.ImageAttachment) (sessions.Session, error) {
	return sessions.Session{ID: sessionID}, nil
}

func (m *fakeSessionManager) Get(projectID uint, sessionID string) (sessions.Session, error) {
	if s, ok := m.sessions[sessionID]; ok {
		return s, nil
	}
	return sessions.Session{}, sessions.ErrSessionNotFound
}

func (m *fakeSessionManager) History(ctx context.Context, projectID uint, sessionID string) (sessions.History, error) {
	if h, ok := m.histories[sessionID]; ok {
		return h, nil
	}
	return sessions.History{}, nil
}

func TestTelemetryBusCollectSessionTurn(t *testing.T) {
	mgr := &fakeSessionManager{
		sessions: map[string]sessions.Session{
			"sess-1": {
				ID:      "sess-1",
				Backend: "opencode-v2",
				Model:   "gpt-4o",
				Status:  sessions.SessionActive,
				Turn:    turns.Running,
			},
		},
		histories: map[string]sessions.History{
			"sess-1": {
				Events: []agents.NormalizedEvent{
					{
						Type: agents.EventMessage,
						Message: &agents.MessagePayload{
							Role: "assistant",
							Text: "模块设计与代码实现已就绪",
						},
					},
				},
			},
		},
	}

	bus := NewEventBus()
	telemetry := NewTelemetryBus(mgr, bus, 100)
	defer telemetry.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Simulate turn changing from Running to Idle after 200ms
	go func() {
		time.Sleep(200 * time.Millisecond)
		s := mgr.sessions["sess-1"]
		s.Turn = turns.Idle
		mgr.sessions["sess-1"] = s
	}()

	outcome, err := telemetry.CollectSessionTurn(ctx, 1, "sess-1", "开发工程师", 10, 1*time.Second)
	if err != nil {
		t.Fatalf("CollectSessionTurn error: %v", err)
	}

	if outcome.Status != "completed" {
		t.Errorf("expected status completed, got %s", outcome.Status)
	}
	if outcome.Summary != "模块设计与代码实现已就绪" {
		t.Errorf("unexpected summary: %s", outcome.Summary)
	}
}
