package sessions

import (
	"context"
	"testing"
	"time"

	"devplatform/internal/core/agents"
)

func lifecycleFor(instanceID, nativeID, state string) agents.NormalizedEvent {
	return agents.NormalizedEvent{
		Type:       agents.EventLifecycle,
		InstanceID: instanceID,
		SessionID:  nativeID,
		Lifecycle:  &agents.LifecycleEvent{State: state},
	}
}

func turnFor(instanceID, nativeID, id string, phase agents.TurnPhase, outcome agents.TurnOutcome) agents.NormalizedEvent {
	return agents.NormalizedEvent{
		Type:       agents.EventTurn,
		InstanceID: instanceID,
		SessionID:  nativeID,
		Turn:       &agents.TurnPayload{ID: id, Phase: phase, Outcome: outcome},
	}
}

func waitForTurn(t *testing.T, svc *Service, projectID uint, sessionID string, want TurnState) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		got, err := svc.Get(projectID, sessionID)
		if err != nil {
			t.Fatalf("Get(%s): %v", sessionID, err)
		}
		if got.Turn == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("session %s turn = %q, want %q", sessionID, got.Turn, want)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestTurnTracksOnlyTheSessionWhoseTurnChanged(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	env.fake.createRef = &agents.SessionRef{InstanceID: env.instanceID, Backend: agents.BackendPi, SessionID: "native-busy"}
	busyOutcome, err := env.svc.Create(ctx, env.projectID, agents.BackendPi, "busy", "", "")
	if err != nil {
		t.Fatal(err)
	}
	busy := busyOutcome.Session
	env.fake.createRef = &agents.SessionRef{InstanceID: env.instanceID, Backend: agents.BackendPi, SessionID: "native-other"}
	otherOutcome, err := env.svc.Create(ctx, env.projectID, agents.BackendPi, "other", "", "")
	if err != nil {
		t.Fatal(err)
	}
	other := otherOutcome.Session
	if busy.Turn != TurnIdle {
		t.Fatalf("new session turn = %q, want idle", busy.Turn)
	}

	env.fake.emit(t, env.instanceID, turnFor(env.instanceID, busy.NativeSessionID, "t1", agents.TurnStarted, ""))
	waitForTurn(t, env.svc, env.projectID, busy.ID, TurnRunning)
	waitForTurn(t, env.svc, env.projectID, other.ID, TurnIdle)

	env.fake.emit(t, env.instanceID, turnFor(env.instanceID, busy.NativeSessionID, "t1", agents.TurnEnded, agents.TurnCompleted))
	waitForTurn(t, env.svc, env.projectID, busy.ID, TurnIdle)
}

func TestTurnEndsOnlyOnATypedTurnBoundary(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	env.fake.createRef = &agents.SessionRef{InstanceID: env.instanceID, Backend: agents.BackendPi, SessionID: "native-failing"}
	sessionOutcome, err := env.svc.Create(ctx, env.projectID, agents.BackendPi, "failing", "", "")
	if err != nil {
		t.Fatal(err)
	}
	session := sessionOutcome.Session

	env.fake.emit(t, env.instanceID, turnFor(env.instanceID, session.NativeSessionID, "t1", agents.TurnStarted, ""))
	waitForTurn(t, env.svc, env.projectID, session.ID, TurnRunning)

	// An error alone describes the failure but does not end the turn: the
	// adapter reports the turn boundary, and the session layer follows it.
	failed := agents.NormalizedEvent{
		Type:       agents.EventError,
		InstanceID: env.instanceID,
		SessionID:  session.NativeSessionID,
		Error:      &agents.ErrorPayload{Code: "execution_failed", Message: "boom"},
	}
	env.fake.emit(t, env.instanceID, failed)
	env.fake.emit(t, env.instanceID, turnFor(env.instanceID, session.NativeSessionID, "t1", agents.TurnEnded, agents.TurnFailed))
	waitForTurn(t, env.svc, env.projectID, session.ID, TurnIdle)

	env.fake.emit(t, env.instanceID, turnFor(env.instanceID, session.NativeSessionID, "t2", agents.TurnStarted, ""))
	waitForTurn(t, env.svc, env.projectID, session.ID, TurnRunning)
}

func TestContinueSessionAlignsTurnWhenAgentRunning(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	env.fake.createRef = &agents.SessionRef{InstanceID: env.instanceID, Backend: agents.BackendPi, SessionID: "native-detect"}
	outcome, err := env.svc.Create(ctx, env.projectID, agents.BackendPi, "detect-test", "", "")
	if err != nil {
		t.Fatal(err)
	}
	session := outcome.Session
	if session.Turn != TurnIdle {
		t.Fatalf("new session turn = %q, want idle", session.Turn)
	}

	// Underlying agent is running on the backend, but no TurnStarted event was emitted on the stream.
	env.fake.hasTurnDetector = true
	env.fake.detectTurnMap[session.NativeSessionID] = true

	// Opening/continuing the session should detect the running state and align it.
	continued, err := env.svc.Continue(ctx, env.projectID, session.ID)
	if err != nil {
		t.Fatalf("Continue: %v", err)
	}
	if continued.Turn != TurnRunning {
		t.Fatalf("continued session turn = %q, want %q", continued.Turn, TurnRunning)
	}

	// Subsequent Get reflects the aligned running state.
	got, err := env.svc.Get(env.projectID, session.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Turn != TurnRunning {
		t.Fatalf("Get session turn = %q, want %q", got.Turn, TurnRunning)
	}

	// When agent turn ends on the stream, it transitions back to idle.
	env.fake.emit(t, env.instanceID, turnFor(env.instanceID, session.NativeSessionID, "t1", agents.TurnEnded, agents.TurnCompleted))
	waitForTurn(t, env.svc, env.projectID, session.ID, TurnIdle)
}

func TestContinueSessionAlignsTurnWhenAgentAlreadyIdle(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	env.fake.createRef = &agents.SessionRef{InstanceID: env.instanceID, Backend: agents.BackendPi, SessionID: "native-idle-detect"}
	outcome, err := env.svc.Create(ctx, env.projectID, agents.BackendPi, "idle-detect-test", "", "")
	if err != nil {
		t.Fatal(err)
	}
	session := outcome.Session

	// 模拟之前平台记录成了 TurnRunning（比如收到了 TurnStarted 或手动置位）
	env.svc.turnTracker.Set(agents.BackendPi, session.NativeSessionID, TurnRunning)
	if got := env.svc.turnTracker.Of(agents.BackendPi, session.NativeSessionID); got != TurnRunning {
		t.Fatalf("setup turnTracker = %q, want running", got)
	}

	// 底层 agent 已经空闲（丢失了 TurnEnded 事件帧）
	env.fake.hasTurnDetector = true
	env.fake.detectTurnMap[session.NativeSessionID] = false

	// 打开/恢复会话时，双向对齐应检测到底层已空闲，自动回正为 Idle
	continued, err := env.svc.Continue(ctx, env.projectID, session.ID)
	if err != nil {
		t.Fatalf("Continue: %v", err)
	}
	if continued.Turn != TurnIdle {
		t.Fatalf("continued session turn = %q, want %q", continued.Turn, TurnIdle)
	}

	got, err := env.svc.Get(env.projectID, session.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Turn != TurnIdle {
		t.Fatalf("Get session turn = %q, want %q", got.Turn, TurnIdle)
	}
}

func TestBackgroundReconcilerClearsStaleRunningTurns(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	env.fake.createRef = &agents.SessionRef{InstanceID: env.instanceID, Backend: agents.BackendPi, SessionID: "native-stale-bg"}
	outcome, err := env.svc.Create(ctx, env.projectID, agents.BackendPi, "stale-bg-test", "", "")
	if err != nil {
		t.Fatal(err)
	}
	session := outcome.Session

	// 模拟平台标记为 TurnRunning（关联实例ID）
	env.svc.turnTracker.SetWithInstance(agents.BackendPi, session.NativeSessionID, env.instanceID, TurnRunning)
	if got := env.svc.turnTracker.Of(agents.BackendPi, session.NativeSessionID); got != TurnRunning {
		t.Fatalf("setup turnTracker = %q, want running", got)
	}

	// 底层 agent 实际已经空闲（丢弃了结束事件）
	env.fake.hasTurnDetector = true
	env.fake.detectTurnMap[session.NativeSessionID] = false

	// 执行一次后台巡检（可以直接调用内部巡检方法，或等待其周期执行）
	env.svc.reconcileRunningTurns()

	// 巡检后 Tracker 状态应自动被清回 Idle
	if got := env.svc.turnTracker.Of(agents.BackendPi, session.NativeSessionID); got != TurnIdle {
		t.Errorf("turnTracker.Of after reconcile = %q, want idle", got)
	}

	got, err := env.svc.Get(env.projectID, session.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Turn != TurnIdle {
		t.Errorf("Get session turn after reconcile = %q, want idle", got.Turn)
	}

	// 优雅关闭服务
	env.svc.Close()
}



