package flow

import (
	"context"
	"sync"
	"testing"
	"time"

	"devplatform/internal/app/sessions"
	"devplatform/internal/app/sessions/turns"
	"devplatform/internal/core/agents"
)

type fakeSessionManager struct {
	mu        sync.Mutex
	sessions  map[string]sessions.Session
	histories map[string]sessions.History
}

func (m *fakeSessionManager) Create(ctx context.Context, projectID uint, backend agents.BackendID, title, message, model string, images ...[]agents.ImageAttachment) (sessions.CreateOutcome, error) {
	return sessions.CreateOutcome{}, nil
}

func (m *fakeSessionManager) Send(ctx context.Context, projectID uint, sessionID, text, model string, images ...[]agents.ImageAttachment) (sessions.Session, error) {
	return sessions.Session{ID: sessionID}, nil
}

func (m *fakeSessionManager) Get(projectID uint, sessionID string) (sessions.Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.sessions[sessionID]; ok {
		return s, nil
	}
	return sessions.Session{}, sessions.ErrSessionNotFound
}

func (m *fakeSessionManager) History(ctx context.Context, projectID uint, sessionID string) (sessions.History, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
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
		mgr.mu.Lock()
		s := mgr.sessions["sess-1"]
		s.Turn = turns.Idle
		mgr.sessions["sess-1"] = s
		mgr.mu.Unlock()
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

type fakeReplierManager struct {
	fakeSessionManager
	repliedForms sync.Map
	repliedPerms sync.Map
	forms        []sessions.FormInfo
	perms        []sessions.PermissionRequest
	eventCh      chan agents.NormalizedEvent
}

func (m *fakeReplierManager) ReplyForm(ctx context.Context, projectID uint, sessionID, formID string, answer map[string]any) error {
	m.repliedForms.Store(formID, answer)
	m.mu.Lock()
	var remaining []sessions.FormInfo
	for _, f := range m.forms {
		if f.ID != formID {
			remaining = append(remaining, f)
		}
	}
	m.forms = remaining
	m.mu.Unlock()
	return nil
}

func (m *fakeReplierManager) ReplyPermissionRequest(ctx context.Context, projectID uint, sessionID, requestID, decision, message string) error {
	m.repliedPerms.Store(requestID, decision)
	m.mu.Lock()
	var remaining []sessions.PermissionRequest
	for _, p := range m.perms {
		if p.ID != requestID {
			remaining = append(remaining, p)
		}
	}
	m.perms = remaining
	m.mu.Unlock()
	return nil
}

func (m *fakeReplierManager) ListForms(ctx context.Context, projectID uint, sessionID string) ([]sessions.FormInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.forms, nil
}

func (m *fakeReplierManager) ListPermissionRequests(ctx context.Context, projectID uint, sessionID string) ([]sessions.PermissionRequest, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.perms, nil
}

func (m *fakeReplierManager) SubscribeSession(ctx context.Context, projectID uint, sessionID string) (<-chan agents.NormalizedEvent, error) {
	return m.eventCh, nil
}

func TestTelemetryBusInteractionAutoReply_PollAndStream(t *testing.T) {
	evCh := make(chan agents.NormalizedEvent, 10)
	mgr := &fakeReplierManager{
		fakeSessionManager: fakeSessionManager{
			sessions: map[string]sessions.Session{
				"sess-auto": {
					ID:      "sess-auto",
					Backend: "opencode-v2",
					Model:   "gpt-4o",
					Status:  sessions.SessionActive,
					Turn:    turns.Running,
				},
			},
			histories: map[string]sessions.History{
				"sess-auto": {
					Events: []agents.NormalizedEvent{
						{
							Type: agents.EventMessage,
							Message: &agents.MessagePayload{
								Role: "assistant",
								Text: "自动化任务执行完成",
							},
						},
					},
				},
			},
		},
		forms: []sessions.FormInfo{
			{ID: "form-q1", SessionID: "sess-auto"},
		},
		perms: []sessions.PermissionRequest{
			{ID: "perm-p1", SessionID: "sess-auto"},
		},
		eventCh: evCh,
	}

	bus := NewEventBus()
	telemetry := NewTelemetryBus(mgr, bus, 200)
	defer telemetry.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Push a live stream interaction event to verify pipeLiveStream immediate handling
	evCh <- agents.NormalizedEvent{
		Type: agents.EventInteraction,
		Interaction: &agents.InteractionPayload{
			ID:    "stream-form-1",
			Phase: agents.InteractionRequested,
			Kind:  agents.InteractionForm,
		},
	}

	go func() {
		// Wait for poll ticker to trigger ReplyForm/ReplyPermissionRequest, then complete turn
		time.Sleep(350 * time.Millisecond)
		mgr.mu.Lock()
		s := mgr.sessions["sess-auto"]
		s.Turn = turns.Idle
		mgr.sessions["sess-auto"] = s
		mgr.mu.Unlock()
	}()

	outcome, err := telemetry.CollectSessionTurn(ctx, 1, "sess-auto", "测试专家", 20, 1*time.Second)
	if err != nil {
		t.Fatalf("CollectSessionTurn error: %v", err)
	}

	if outcome.Status != "completed" {
		t.Errorf("expected status completed, got %s", outcome.Status)
	}

	// Verify polled question form was auto replied
	if _, ok := mgr.repliedForms.Load("form-q1"); !ok {
		t.Errorf("expected form-q1 to be automatically replied")
	}

	// Verify polled permission was auto replied
	if val, ok := mgr.repliedPerms.Load("perm-p1"); !ok || val != "once" {
		t.Errorf("expected perm-p1 to be auto-approved, got %v", val)
	}

	// Verify stream form was auto replied
	if _, ok := mgr.repliedForms.Load("stream-form-1"); !ok {
		t.Errorf("expected stream-form-1 to be automatically replied")
	}
}
