package instances

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"devplatform/internal/agents"
	"devplatform/internal/database"
	"devplatform/internal/projects"
)

// openTestDB opens and migrates a throwaway platform database. The GORM logger
// is discarded because these tests deliberately exercise expected-error paths
// whose warnings would otherwise pollute the output.
func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := database.Open(t.TempDir())
	if err != nil {
		t.Fatalf("database.Open: %v", err)
	}
	if err := database.Migrate(db); err != nil {
		t.Fatalf("database.Migrate: %v", err)
	}
	db.Logger = logger.Discard
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

// fakeAdapter is a scriptable AgentAdapter. Each StartInstance call allocates a
// fresh instance id, so one fake stands in for several instances of the same
// backend. Subscribe returns a per-instance channel, so events can be targeted
// at exactly one instance.
type fakeAdapter struct {
	mu sync.Mutex

	startErr error
	stopErr  error

	next    int
	started []string
	stopped []string

	subs  map[string]chan agents.NormalizedEvent
	subEr map[string]error
}

func newFakeAdapter() *fakeAdapter {
	return &fakeAdapter{subs: make(map[string]chan agents.NormalizedEvent), subEr: make(map[string]error)}
}

func (f *fakeAdapter) Probe(context.Context) error { return nil }

func (f *fakeAdapter) StartInstance(_ context.Context, req agents.StartInstanceRequest) (*agents.Instance, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.startErr != nil {
		return nil, f.startErr
	}
	f.next++
	id := fmt.Sprintf("fake-%d", f.next)
	f.started = append(f.started, id)
	now := time.Now().UTC()
	return &agents.Instance{
		ID:        id,
		Backend:   agents.BackendPi,
		Directory: req.Directory,
		Status:    agents.InstanceRunning,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

func (f *fakeAdapter) StopInstance(_ context.Context, instanceID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.stopErr != nil {
		return f.stopErr
	}
	f.stopped = append(f.stopped, instanceID)
	if ch, ok := f.subs[instanceID]; ok {
		close(ch)
		delete(f.subs, instanceID)
	}
	return nil
}

func (f *fakeAdapter) ListSessions(context.Context, string) ([]agents.SessionRef, error) {
	return nil, nil
}

func (f *fakeAdapter) CreateSession(context.Context, string, agents.CreateSessionOptions) (*agents.SessionRef, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeAdapter) ContinueSession(context.Context, agents.SessionRef) (*agents.SessionRef, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeAdapter) SendMessage(context.Context, agents.SessionRef, agents.SendMessageRequest) error {
	return nil
}

func (f *fakeAdapter) Subscribe(ctx context.Context, instanceID string) (<-chan agents.NormalizedEvent, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if err := f.subEr[instanceID]; err != nil {
		return nil, err
	}
	ch := make(chan agents.NormalizedEvent, 16)
	f.subs[instanceID] = ch
	go func() {
		<-ctx.Done()
		f.mu.Lock()
		defer f.mu.Unlock()
		if current, ok := f.subs[instanceID]; ok && current == ch {
			close(current)
			delete(f.subs, instanceID)
		}
	}()
	return ch, nil
}

// emit delivers ev on the instance's subscription channel when one exists.
func (f *fakeAdapter) emit(t *testing.T, instanceID string, ev agents.NormalizedEvent) {
	t.Helper()

	f.mu.Lock()
	ch := f.subs[instanceID]
	f.mu.Unlock()
	if ch == nil {
		t.Fatalf("no active subscription for instance %s", instanceID)
	}
	ch <- ev
}

func (f *fakeAdapter) startCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.started)
}

func (f *fakeAdapter) stoppedIDs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.stopped...)
}

// newTestService builds an instances service over a throwaway database with one
// authorized workspace root and one registered project, returning the service,
// the database, the project id, the canonical project directory and the fake
// pi adapter.
func newTestService(t *testing.T) (*Service, *gorm.DB, uint, string, *fakeAdapter) {
	t.Helper()

	db := openTestDB(t)
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}
	if err := db.Create(&database.WorkspaceRoot{Path: root}).Error; err != nil {
		t.Fatalf("create workspace root: %v", err)
	}
	projectDir := filepath.Join(root, "alpha")
	if err := os.Mkdir(projectDir, 0o755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	project, err := projects.NewService(db).Register(projectDir)
	if err != nil {
		t.Fatalf("register project: %v", err)
	}

	registry := agents.NewRegistry()
	fake := newFakeAdapter()
	if err := registry.Register(agents.BackendPi, fake); err != nil {
		t.Fatalf("register fake adapter: %v", err)
	}
	return NewService(db, registry, projects.NewService(db)), db, project.ID, project.Path, fake
}

// messageEvent builds a valid assistant message event for instanceID.
func messageEvent(instanceID, text string) agents.NormalizedEvent {
	return agents.NormalizedEvent{
		InstanceID: instanceID,
		Backend:    agents.BackendPi,
		SessionID:  "ses-1",
		Sequence:   1,
		Timestamp:  time.Now().UTC(),
		Type:       agents.EventMessage,
		Message:    &agents.MessagePayload{Role: "assistant", Text: text, Final: true},
	}
}

// nextEvent reads one event or fails the test after a bounded wait.
func nextEvent(t *testing.T, ch <-chan agents.NormalizedEvent) agents.NormalizedEvent {
	t.Helper()

	select {
	case ev, ok := <-ch:
		if !ok {
			t.Fatal("event channel closed while waiting for an event")
		}
		return ev
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for an event")
		return agents.NormalizedEvent{}
	}
}

// expectNoEvent asserts no event arrives within a short window.
func expectNoEvent(t *testing.T, ch <-chan agents.NormalizedEvent) {
	t.Helper()

	select {
	case ev, ok := <-ch:
		if !ok {
			return
		}
		t.Fatalf("unexpected cross-instance event: %+v", ev)
	case <-time.After(150 * time.Millisecond):
	}
}

// waitForStatus polls until the instance reports want, or fails the test.
func waitForStatus(t *testing.T, svc *Service, instanceID string, want agents.InstanceStatus) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	var last Instance
	for time.Now().Before(deadline) {
		inst, err := svc.Get(instanceID)
		if err != nil {
			t.Fatalf("Get(%s): %v", instanceID, err)
		}
		last = inst
		if inst.Status == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("instance %s status = %q, want %q", instanceID, last.Status, want)
}

func instanceCount(t *testing.T, db *gorm.DB) int64 {
	t.Helper()

	var n int64
	if err := db.Model(&database.AgentInstance{}).Count(&n).Error; err != nil {
		t.Fatalf("count instances: %v", err)
	}
	return n
}

func TestStartAssociatesProjectAndPersistsInstance(t *testing.T) {
	svc, db, projectID, projectDir, _ := newTestService(t)

	inst, err := svc.Start(context.Background(), projectID, agents.BackendPi)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if inst.ID == "" {
		t.Fatal("Start returned an instance with an empty id")
	}
	if inst.ProjectID != projectID {
		t.Errorf("ProjectID = %d, want %d", inst.ProjectID, projectID)
	}
	if inst.Backend != agents.BackendPi {
		t.Errorf("Backend = %q, want %q", inst.Backend, agents.BackendPi)
	}
	if inst.Directory != projectDir {
		t.Errorf("Directory = %q, want the project directory %q", inst.Directory, projectDir)
	}
	if inst.Status != agents.InstanceRunning {
		t.Errorf("Status = %q, want %q", inst.Status, agents.InstanceRunning)
	}
	if inst.CreatedAt.IsZero() || inst.UpdatedAt.IsZero() {
		t.Error("instance timestamps are zero")
	}
	if got := instanceCount(t, db); got != 1 {
		t.Fatalf("persisted instance count = %d, want 1", got)
	}

	// The durable id survives a fresh read.
	again, err := svc.Get(inst.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if again.ID != inst.ID || again.Status != agents.InstanceRunning {
		t.Errorf("Get returned %+v, want the running instance %+v", again, inst)
	}

	list, err := svc.List(projectID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0].ID != inst.ID {
		t.Fatalf("List = %+v, want exactly the started instance", list)
	}
}

func TestStartRejectsUnknownProject(t *testing.T) {
	svc, db, _, _, fake := newTestService(t)

	_, err := svc.Start(context.Background(), 9999, agents.BackendPi)
	if !errors.Is(err, ErrProjectNotFound) {
		t.Fatalf("Start with unknown project error = %v, want ErrProjectNotFound", err)
	}
	if got := instanceCount(t, db); got != 0 {
		t.Errorf("persisted instance count = %d, want 0", got)
	}
	if fake.startCount() != 0 {
		t.Errorf("adapter was started %d times, want 0", fake.startCount())
	}
}

func TestStartRejectsUnavailableProjectDirectory(t *testing.T) {
	svc, _, projectID, projectDir, fake := newTestService(t)

	if err := os.Remove(projectDir); err != nil {
		t.Fatalf("Remove project directory: %v", err)
	}

	_, err := svc.Start(context.Background(), projectID, agents.BackendPi)
	if err == nil {
		t.Fatal("Start with a removed project directory succeeded, want error")
	}
	if !errors.Is(err, projects.ErrDirectoryMissing) {
		t.Errorf("Start error = %v, want projects.ErrDirectoryMissing", err)
	}
	if fake.startCount() != 0 {
		t.Errorf("adapter was started %d times, want 0", fake.startCount())
	}
}

func TestStartRejectsUnknownOrUnregisteredBackend(t *testing.T) {
	svc, db, projectID, _, _ := newTestService(t)

	if _, err := svc.Start(context.Background(), projectID, "claude-code"); !errors.Is(err, ErrUnknownBackend) {
		t.Errorf("Start with unknown backend error = %v, want ErrUnknownBackend", err)
	}

	// A backend id that is valid but has no registered adapter is refused too.
	empty := NewService(db, agents.NewRegistry(), projects.NewService(db))
	if _, err := empty.Start(context.Background(), projectID, agents.BackendPi); !errors.Is(err, ErrUnknownBackend) {
		t.Errorf("Start with unregistered backend error = %v, want ErrUnknownBackend", err)
	} else if !errors.Is(err, agents.ErrBackendNotRegistered) {
		t.Errorf("Start error = %v, want it to wrap agents.ErrBackendNotRegistered", err)
	}
}

func TestStartRecordsAdapterFailure(t *testing.T) {
	svc, db, projectID, _, fake := newTestService(t)
	fake.mu.Lock()
	fake.startErr = errors.New("cannot launch backend")
	fake.mu.Unlock()

	inst, err := svc.Start(context.Background(), projectID, agents.BackendPi)
	if !errors.Is(err, ErrStartFailed) {
		t.Fatalf("Start error = %v, want ErrStartFailed", err)
	}
	if inst.ID == "" {
		t.Fatal("failed instance has an empty id")
	}
	if inst.Status != agents.InstanceFailed {
		t.Errorf("failed instance status = %q, want %q", inst.Status, agents.InstanceFailed)
	}
	if got := instanceCount(t, db); got != 1 {
		t.Fatalf("persisted instance count = %d, want 1 failed record", got)
	}
	list, err := svc.List(projectID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0].Status != agents.InstanceFailed {
		t.Fatalf("List = %+v, want one failed instance", list)
	}
}

func TestMultipleSameBackendInstancesAreIndependent(t *testing.T) {
	svc, _, projectID, _, fake := newTestService(t)

	first, err := svc.Start(context.Background(), projectID, agents.BackendPi)
	if err != nil {
		t.Fatalf("Start first: %v", err)
	}
	second, err := svc.Start(context.Background(), projectID, agents.BackendPi)
	if err != nil {
		t.Fatalf("Start second: %v", err)
	}
	if first.ID == second.ID {
		t.Fatalf("two instances share the id %q", first.ID)
	}

	list, err := svc.List(projectID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("List length = %d, want 2", len(list))
	}

	// Stopping the first instance must leave the second running.
	stopped, err := svc.Stop(context.Background(), first.ID)
	if err != nil {
		t.Fatalf("Stop first: %v", err)
	}
	if stopped.Status != agents.InstanceStopped {
		t.Errorf("stopped status = %q, want %q", stopped.Status, agents.InstanceStopped)
	}
	if got := fake.stoppedIDs(); len(got) != 1 || got[0] != first.ID {
		t.Errorf("adapter stopped %v, want only [%s]", got, first.ID)
	}

	survivor, err := svc.Get(second.ID)
	if err != nil {
		t.Fatalf("Get second: %v", err)
	}
	if survivor.Status != agents.InstanceRunning {
		t.Errorf("second instance status = %q after stopping the first, want running", survivor.Status)
	}
}

func TestStopIsIdempotent(t *testing.T) {
	svc, _, projectID, _, fake := newTestService(t)

	inst, err := svc.Start(context.Background(), projectID, agents.BackendPi)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	if _, err := svc.Stop(context.Background(), inst.ID); err != nil {
		t.Fatalf("first Stop: %v", err)
	}
	second, err := svc.Stop(context.Background(), inst.ID)
	if err != nil {
		t.Fatalf("second Stop: %v", err)
	}
	if second.Status != agents.InstanceStopped {
		t.Errorf("second Stop status = %q, want stopped", second.Status)
	}
	if got := fake.stoppedIDs(); len(got) != 1 {
		t.Errorf("adapter StopInstance called %d times (%v), want exactly once", len(got), got)
	}

	if _, err := svc.Stop(context.Background(), "does-not-exist"); !errors.Is(err, ErrInstanceNotFound) {
		t.Errorf("Stop unknown instance error = %v, want ErrInstanceNotFound", err)
	}
}

func TestSubscriptionsAreIsolatedPerInstance(t *testing.T) {
	svc, _, projectID, _, fake := newTestService(t)

	first, err := svc.Start(context.Background(), projectID, agents.BackendPi)
	if err != nil {
		t.Fatalf("Start first: %v", err)
	}
	second, err := svc.Start(context.Background(), projectID, agents.BackendPi)
	if err != nil {
		t.Fatalf("Start second: %v", err)
	}

	ctxFirst, cancelFirst := context.WithCancel(context.Background())
	defer cancelFirst()
	ctxSecond, cancelSecond := context.WithCancel(context.Background())
	defer cancelSecond()

	firstEvents, err := svc.Subscribe(ctxFirst, first.ID)
	if err != nil {
		t.Fatalf("Subscribe first: %v", err)
	}
	secondEvents, err := svc.Subscribe(ctxSecond, second.ID)
	if err != nil {
		t.Fatalf("Subscribe second: %v", err)
	}

	fake.emit(t, first.ID, messageEvent(first.ID, "hello from first"))
	fake.emit(t, second.ID, messageEvent(second.ID, "hello from second"))

	gotFirst := nextEvent(t, firstEvents)
	if gotFirst.InstanceID != first.ID || gotFirst.Message == nil || gotFirst.Message.Text != "hello from first" {
		t.Fatalf("first instance received %+v, want its own message", gotFirst)
	}
	gotSecond := nextEvent(t, secondEvents)
	if gotSecond.InstanceID != second.ID || gotSecond.Message == nil || gotSecond.Message.Text != "hello from second" {
		t.Fatalf("second instance received %+v, want its own message", gotSecond)
	}

	// An event targeted at the first instance must never surface on the second.
	fake.emit(t, first.ID, messageEvent(first.ID, "only for first"))
	if ev := nextEvent(t, firstEvents); ev.Message == nil || ev.Message.Text != "only for first" {
		t.Fatalf("first instance received %+v, want the follow-up message", ev)
	}
	expectNoEvent(t, secondEvents)
}

func TestLifecycleEventUpdatesOnlyItsInstance(t *testing.T) {
	svc, _, projectID, _, fake := newTestService(t)

	first, err := svc.Start(context.Background(), projectID, agents.BackendPi)
	if err != nil {
		t.Fatalf("Start first: %v", err)
	}
	second, err := svc.Start(context.Background(), projectID, agents.BackendPi)
	if err != nil {
		t.Fatalf("Start second: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if _, err := svc.Subscribe(ctx, first.ID); err != nil {
		t.Fatalf("Subscribe first: %v", err)
	}

	fake.emit(t, first.ID, agents.NormalizedEvent{
		InstanceID: first.ID,
		Backend:    agents.BackendPi,
		Timestamp:  time.Now().UTC(),
		Type:       agents.EventLifecycle,
		Lifecycle:  &agents.LifecycleEvent{State: string(agents.InstanceFailed), Detail: "backend crashed"},
	})

	waitForStatus(t, svc, first.ID, agents.InstanceFailed)

	survivor, err := svc.Get(second.ID)
	if err != nil {
		t.Fatalf("Get second: %v", err)
	}
	if survivor.Status != agents.InstanceRunning {
		t.Errorf("second instance status = %q after the first failed, want running", survivor.Status)
	}
}

func TestSubscribeRejectsUnknownOrStoppedInstance(t *testing.T) {
	svc, _, projectID, _, _ := newTestService(t)

	if _, err := svc.Subscribe(context.Background(), "nope"); !errors.Is(err, ErrInstanceNotFound) {
		t.Errorf("Subscribe unknown error = %v, want ErrInstanceNotFound", err)
	}

	inst, err := svc.Start(context.Background(), projectID, agents.BackendPi)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if _, err := svc.Stop(context.Background(), inst.ID); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if _, err := svc.Subscribe(context.Background(), inst.ID); !errors.Is(err, ErrInstanceNotRunning) {
		t.Errorf("Subscribe stopped error = %v, want ErrInstanceNotRunning", err)
	}
}
