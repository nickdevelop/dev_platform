package sessions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"devplatform/internal/core/agents"
	"devplatform/internal/app/instances"
	"devplatform/internal/app/database"
	"devplatform/internal/app/projects"
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

// fakeAdapter is a scriptable AgentAdapter with an explicit Capabilities()
// declaration. It records the references and requests the service passes down
// so tests can assert the native ID mapping is used verbatim.
type fakeAdapter struct {
	mu sync.Mutex

	caps agents.Capabilities

	createRef  *agents.SessionRef
	createErr  error
	createOpts []agents.CreateSessionOptions
	createFn   func(instanceID string, options agents.CreateSessionOptions) (*agents.SessionRef, error)

	listRefs  []agents.SessionRef
	listErr   error
	listCalls int
	// listBindInstance makes ListSessions rebind its refs to the instance it
	// was asked about, for tests that cannot predict the instance id.
	listBindInstance bool

	continueRef *agents.SessionRef
	continueErr error

	sendErr  error
	sends    []agents.SendMessageRequest
	sentRefs []agents.SessionRef

	// replyErr scripts the opencode-only permission/form reply path.
	replyErr error

	stopErr   error
	stopCalls int

	interruptCalls int
	interruptRefs  []agents.SessionRef
	interruptErr   error

	models    []agents.ModelInfo
	modelsErr error

	historyResult agents.HistoryResult
	historyErr    error
	historyReqs   []agents.ListHistoryRequest

	startErr  error
	next      int
	subBuffer int
	subs      map[string]chan agents.NormalizedEvent

	// diagnostics is the canned startup report the adapter hands back when
	// the service collects backend diagnostics.
	diagnostics agents.BackendDiagnostics

	detectTurnMap   map[string]bool
	detectTurnErr   error
	hasTurnDetector bool
}

func newFakeAdapter() *fakeAdapter {
	return &fakeAdapter{
		caps: agents.Capabilities{
			Streaming:        true,
			SessionDiscovery: true,
			SessionContinue:  true,
			Stop:             true,
			ModelSelection:   true,
			Interrupt:        true,
			ToolCallEvents:   true,
			ToolConfirmation: false,
			SessionHistory:   true,
		},
		subs:          make(map[string]chan agents.NormalizedEvent),
		detectTurnMap: make(map[string]bool),
	}
}

func (f *fakeAdapter) Probe(context.Context) error { return nil }

func (f *fakeAdapter) Capabilities() agents.Capabilities {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.caps
}

// Diagnostics returns the canned report; it makes fakeAdapter a
// agents.Diagnostician so the service's diagnostics collection is testable.
func (f *fakeAdapter) Diagnostics(context.Context) agents.BackendDiagnostics {
	return f.diagnostics
}

func (f *fakeAdapter) StartInstance(_ context.Context, req agents.StartInstanceRequest) (*agents.Instance, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.startErr != nil {
		return nil, f.startErr
	}
	f.next++
	now := time.Now().UTC()
	return &agents.Instance{
		ID:        fmt.Sprintf("fake-inst-%d", f.next),
		Backend:   agents.BackendPi,
		Directory: req.Directory,
		Status:    agents.InstanceRunning,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

func (f *fakeAdapter) StopInstance(_ context.Context, instanceID string) error {
	f.mu.Lock()
	f.stopCalls++
	stopErr := f.stopErr
	if stopErr != nil {
		f.mu.Unlock()
		return stopErr
	}
	if ch, ok := f.subs[instanceID]; ok {
		close(ch)
		delete(f.subs, instanceID)
	}
	f.mu.Unlock()
	return nil
}

func (f *fakeAdapter) ListSessions(_ context.Context, instanceID string) ([]agents.SessionRef, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.listCalls++
	refs := append([]agents.SessionRef(nil), f.listRefs...)
	// Discovery tests that cannot know the instance id in advance (because the
	// service starts one) ask the fake to bind what it returns to the instance
	// it was queried on, which is what a real adapter does.
	if f.listBindInstance {
		for i := range refs {
			refs[i].InstanceID = instanceID
		}
	}
	return refs, f.listErr
}

func (f *fakeAdapter) CreateSession(_ context.Context, instanceID string, options agents.CreateSessionOptions) (*agents.SessionRef, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.createOpts = append(f.createOpts, options)
	if f.createFn != nil {
		return f.createFn(instanceID, options)
	}
	if f.createErr != nil {
		return nil, f.createErr
	}
	if f.createRef == nil {
		return nil, errors.New("fake: CreateSession not scripted")
	}
	ref := *f.createRef
	if ref.InstanceID == "" {
		ref.InstanceID = instanceID
	}
	return &ref, nil
}

func (f *fakeAdapter) ContinueSession(_ context.Context, ref agents.SessionRef) (*agents.SessionRef, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.continueErr != nil {
		return nil, f.continueErr
	}
	if f.continueRef == nil {
		return &ref, nil
	}
	out := *f.continueRef
	return &out, nil
}

func (f *fakeAdapter) SendMessage(_ context.Context, ref agents.SessionRef, message agents.SendMessageRequest) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sends = append(f.sends, message)
	f.sentRefs = append(f.sentRefs, ref)
	return f.sendErr
}

func (f *fakeAdapter) ListModels(context.Context, string) ([]agents.ModelInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]agents.ModelInfo(nil), f.models...), f.modelsErr
}

func (f *fakeAdapter) Interrupt(_ context.Context, ref agents.SessionRef) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.interruptCalls++
	f.interruptRefs = append(f.interruptRefs, ref)
	return f.interruptErr
}

// ListHistory makes fakeAdapter an agents.HistoryReader so the service's
// history path is testable. It records the requests it saw; the capability
// flag, not the method's presence, is what the service must gate on.
func (f *fakeAdapter) ListHistory(_ context.Context, request agents.ListHistoryRequest) (agents.HistoryResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.historyReqs = append(f.historyReqs, request)
	if f.historyErr != nil {
		return agents.HistoryResult{}, f.historyErr
	}
	return f.historyResult, nil
}

func (f *fakeAdapter) DetectTurn(_ context.Context, ref agents.SessionRef) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.hasTurnDetector {
		return false, ErrUnsupportedCapability
	}
	if f.detectTurnErr != nil {
		return false, f.detectTurnErr
	}
	return f.detectTurnMap[ref.SessionID], nil
}


func (f *fakeAdapter) Subscribe(ctx context.Context, instanceID string) (<-chan agents.NormalizedEvent, error) {
	f.mu.Lock()
	buffer := f.subBuffer
	if buffer <= 0 {
		buffer = 64
	}
	ch := make(chan agents.NormalizedEvent, buffer)
	f.subs[instanceID] = ch
	f.mu.Unlock()

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

func (f *fakeAdapter) stopCallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.stopCalls
}

// startCount reports how many instances the adapter has started, so a test can
// assert create reused a running instance instead of starting another.
func (f *fakeAdapter) startCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.next
}

// interruptCallCount reports how many times the adapter was asked to interrupt
// a turn, so a test can assert an idle interrupt is a no-op.
func (f *fakeAdapter) interruptCallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.interruptCalls
}

// emit delivers ev on the instance's current subscription channel, failing the
// test when no subscription exists or delivery blocks.
func (f *fakeAdapter) emit(t *testing.T, instanceID string, ev agents.NormalizedEvent) {
	t.Helper()

	f.mu.Lock()
	ch := f.subs[instanceID]
	f.mu.Unlock()
	if ch == nil {
		t.Fatalf("no active subscription for instance %s", instanceID)
	}
	select {
	case ch <- ev:
	case <-time.After(2 * time.Second):
		t.Fatal("emit blocked on the instance subscription channel")
	}
}

// testEnv is a fully wired sessions service over a real database, a real
// instance lifecycle service and a scriptable fake adapter.
type testEnv struct {
	svc        *Service
	db         *gorm.DB
	instances  *instances.Service
	registry   *agents.Registry
	fake       *fakeAdapter
	root       string
	projectID  uint
	instanceID string
	projectDir string
}

// newTestEnv builds one project, one registered pi adapter and one running
// instance of it. An optional platformEnabled lookup gates both the instance
// and session services.
func newTestEnv(t *testing.T, platformEnabled ...func(agents.BackendID) bool) *testEnv {
	t.Helper()

	var enabled func(agents.BackendID) bool
	if len(platformEnabled) > 0 {
		enabled = platformEnabled[0]
	}

	db := openTestDB(t)
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}
	projectDir := filepath.Join(root, "alpha")
	if err := os.Mkdir(projectDir, 0o755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	projectService := projects.NewService(db)
	project, err := projectService.Register(projectDir)
	if err != nil {
		t.Fatalf("register project: %v", err)
	}

	registry := agents.NewRegistry()
	fake := newFakeAdapter()
	if err := registry.Register(agents.BackendPi, fake); err != nil {
		t.Fatalf("register fake adapter: %v", err)
	}
	instanceService := instances.NewService(db, registry, projectService, instances.WithPlatformEnabled(enabled))
	instance, err := instanceService.Start(context.Background(), project.ID, agents.BackendPi)
	if err != nil {
		t.Fatalf("start instance: %v", err)
	}

	return &testEnv{
		svc:        NewService(db, registry, instanceService, WithPlatformEnabled(enabled)),
		db:         db,
		instances:  instanceService,
		registry:   registry,
		fake:       fake,
		root:       root,
		projectID:  project.ID,
		instanceID: instance.ID,
		projectDir: projectDir,
	}
}

// registerProject creates and registers another project under the env's root.
func (e *testEnv) registerProject(t *testing.T, name string) uint {
	t.Helper()

	dir := filepath.Join(e.root, name)
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatalf("Mkdir %s: %v", name, err)
	}
	project, err := projects.NewService(e.db).Register(dir)
	if err != nil {
		t.Fatalf("register project %s: %v", name, err)
	}
	return project.ID
}

// createNativeSession drives the create flow with the fake adapter returning
// nativeID as the agent-native session id.
func createNativeSession(t *testing.T, env *testEnv, nativeID string) Session {
	t.Helper()

	env.fake.mu.Lock()
	env.fake.createRef = &agents.SessionRef{
		InstanceID: env.instanceID,
		Backend:    agents.BackendPi,
		SessionID:  nativeID,
		Title:      nativeID,
	}
	env.fake.mu.Unlock()

	created, err := env.svc.Create(context.Background(), env.projectID, agents.BackendPi, nativeID, "", "")
	if err != nil {
		t.Fatalf("Create(%s): %v", nativeID, err)
	}
	session := created.Session
	return session
}

func sessionCount(t *testing.T, db *gorm.DB) int64 {
	t.Helper()

	var n int64
	if err := db.Model(&database.AgentSession{}).Count(&n).Error; err != nil {
		t.Fatalf("count sessions: %v", err)
	}
	return n
}

// TestCreateIndexesPlatformToNativeMapping verifies create maps a native
// session to a durable platform id and persists only the index fields.
func TestCreateIndexesPlatformToNativeMapping(t *testing.T) {
	env := newTestEnv(t)
	session := createNativeSession(t, env, "native-1")

	if session.ID == "" {
		t.Fatal("Create returned an empty platform session id")
	}
	if session.NativeSessionID != "native-1" {
		t.Errorf("NativeSessionID = %q, want native-1", session.NativeSessionID)
	}
	if session.ProjectID != env.projectID {
		t.Errorf("ProjectID = %d, want %d", session.ProjectID, env.projectID)
	}
	if session.InstanceID != env.instanceID {
		t.Errorf("InstanceID = %q, want %q", session.InstanceID, env.instanceID)
	}
	if session.Backend != agents.BackendPi {
		t.Errorf("Backend = %q, want %q", session.Backend, agents.BackendPi)
	}
	if session.Status != SessionActive {
		t.Errorf("Status = %q, want %q", session.Status, SessionActive)
	}
	if session.Title != "native-1" {
		t.Errorf("Title = %q, want native-1", session.Title)
	}
	if !session.Capabilities.ModelSelection || session.Capabilities.ToolConfirmation {
		t.Errorf("Capabilities = %+v, want the adapter's declaration", session.Capabilities)
	}

	// The index row persists the mapping and nothing agent-native.
	var row database.AgentSession
	if err := env.db.First(&row, "id = ?", session.ID).Error; err != nil {
		t.Fatalf("load persisted session: %v", err)
	}
	if row.NativeSessionID != "native-1" || row.InstanceID != env.instanceID || row.ProjectID != env.projectID {
		t.Errorf("persisted row = %+v, want the native mapping", row)
	}
	if row.CreatedAt.IsZero() || row.UpdatedAt.IsZero() {
		t.Error("persisted session timestamps are zero")
	}

	if got := sessionCount(t, env.db); got != 1 {
		t.Errorf("session count = %d, want 1", got)
	}
}

// TestCreateReusesRunningInstanceForBackend verifies create selects the
// project's running instance of the requested backend instead of starting a
// new one.
func TestCreateReusesRunningInstanceForBackend(t *testing.T) {
	env := newTestEnv(t)
	env.fake.mu.Lock()
	env.fake.createRef = &agents.SessionRef{InstanceID: env.instanceID, Backend: agents.BackendPi, SessionID: "native-reuse", Title: "reuse"}
	env.fake.mu.Unlock()

	before := env.fake.startCount()
	created, err := env.svc.Create(context.Background(), env.projectID, agents.BackendPi, "reuse", "", "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	session := created.Session
	if session.InstanceID != env.instanceID {
		t.Errorf("session instance = %q, want the running instance %q", session.InstanceID, env.instanceID)
	}
	if got := env.fake.startCount(); got != before {
		t.Errorf("StartInstance calls = %d, want %d (a running instance must be reused)", got, before)
	}
	if got := sessionCount(t, env.db); got != 1 {
		t.Errorf("session count = %d, want 1", got)
	}
}

// TestCreateStartsInstanceWhenNoneRunning verifies create starts one instance
// of the requested backend when the project has none running.
func TestCreateStartsInstanceWhenNoneRunning(t *testing.T) {
	env := newTestEnv(t)
	otherProject := env.registerProject(t, "beta")
	env.fake.mu.Lock()
	env.fake.createRef = &agents.SessionRef{Backend: agents.BackendPi, SessionID: "native-started", Title: "started"}
	env.fake.mu.Unlock()

	before := env.fake.startCount()
	created, err := env.svc.Create(context.Background(), otherProject, agents.BackendPi, "started", "", "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	session := created.Session
	if got := env.fake.startCount(); got != before+1 {
		t.Errorf("StartInstance calls = %d, want %d (one auto-start)", got, before+1)
	}
	if session.ProjectID != otherProject {
		t.Errorf("session project = %d, want %d", session.ProjectID, otherProject)
	}
	if session.NativeSessionID != "native-started" {
		t.Errorf("native session = %q, want native-started", session.NativeSessionID)
	}
}

// TestCreateReportsStartFailureWithoutIndexing verifies a failed auto-start is
// surfaced and leaves no session index row behind.
func TestCreateReportsStartFailureWithoutIndexing(t *testing.T) {
	env := newTestEnv(t)
	otherProject := env.registerProject(t, "beta")
	env.fake.mu.Lock()
	env.fake.startErr = errors.New("adapter cannot start")
	env.fake.mu.Unlock()

	_, err := env.svc.Create(context.Background(), otherProject, agents.BackendPi, "nope", "", "")
	if !errors.Is(err, instances.ErrStartFailed) {
		t.Fatalf("Create error = %v, want instances.ErrStartFailed", err)
	}
	if got := sessionCount(t, env.db); got != 0 {
		t.Errorf("session count = %d, want 0 after a failed start", got)
	}
}

// TestCreateAutoRecoversStaleInstance verifies that when CreateSession fails on
// an existing instance (e.g. instance invalidated after server restart), Create
// starts a fresh instance and successfully creates the session.
func TestCreateAutoRecoversStaleInstance(t *testing.T) {
	env := newTestEnv(t)
	attempts := 0
	env.fake.mu.Lock()
	env.fake.createFn = func(instanceID string, options agents.CreateSessionOptions) (*agents.SessionRef, error) {
		attempts++
		if attempts == 1 {
			// First attempt on the stale instance fails with ErrInvalidInstance
			return nil, agents.ErrInvalidInstance
		}
		return &agents.SessionRef{
			InstanceID: instanceID,
			Backend:    agents.BackendPi,
			SessionID:  "native-recovered",
			Title:      options.Title,
		}, nil
	}
	env.fake.mu.Unlock()

	created, err := env.svc.Create(context.Background(), env.projectID, agents.BackendPi, "Recovered Title", "", "")
	if err != nil {
		t.Fatalf("Create should have auto-recovered stale instance, got error: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("expected 2 CreateSession attempts (stale failed, fresh succeeded), got %d", attempts)
	}
	if created.Session.NativeSessionID != "native-recovered" {
		t.Errorf("NativeSessionID = %q, want native-recovered", created.Session.NativeSessionID)
	}
}

// TestPlatformSwitchRefusesNewWorkOnExistingSession verifies the platform
// switch gates every new-work entry point (create, continue, send) while
// interrupt is not gated.
func TestPlatformSwitchRefusesNewWorkOnExistingSession(t *testing.T) {
	enabled := true
	env := newTestEnv(t, func(agents.BackendID) bool { return enabled })
	session := createNativeSession(t, env, "native-gate")

	enabled = false
	if _, err := env.svc.Create(context.Background(), env.projectID, agents.BackendPi, "t2", "", ""); !errors.Is(err, ErrAgentDisabled) {
		t.Errorf("Create while disabled error = %v, want ErrAgentDisabled", err)
	}
	if _, err := env.svc.Continue(context.Background(), env.projectID, session.ID); !errors.Is(err, ErrAgentDisabled) {
		t.Errorf("Continue while disabled error = %v, want ErrAgentDisabled", err)
	}
	if _, err := env.svc.Send(context.Background(), env.projectID, session.ID, "hello", ""); !errors.Is(err, ErrAgentDisabled) {
		t.Errorf("Send while disabled error = %v, want ErrAgentDisabled", err)
	}
	if _, err := env.svc.Interrupt(context.Background(), env.projectID, session.ID); errors.Is(err, ErrAgentDisabled) {
		t.Errorf("Interrupt while disabled error = %v; interrupt must not be gated", err)
	}
}

// TestCreateRejectsInvalidBackend verifies create refuses a missing or invalid
// backend rather than guessing one.
func TestCreateRejectsInvalidBackend(t *testing.T) {
	env := newTestEnv(t)

	if _, err := env.svc.Create(context.Background(), env.projectID, "", "t", "", ""); !errors.Is(err, ErrInvalidRequest) {
		t.Errorf("Create with empty backend error = %v, want ErrInvalidRequest", err)
	}
	if _, err := env.svc.Create(context.Background(), env.projectID, agents.BackendID("nope"), "t", "", ""); !errors.Is(err, ErrInvalidRequest) {
		t.Errorf("Create with unknown backend error = %v, want ErrInvalidRequest", err)
	}
}

// TestCreateResolvesDuplicateNativeSessionRace verifies a create that loses a
// concurrent race for the same (instance, native) pair returns the already
// indexed session instead of surfacing a unique-index 500.
func TestCreateResolvesDuplicateNativeSessionRace(t *testing.T) {
	env := newTestEnv(t)

	// A concurrent create already indexed this native session.
	now := time.Now().UTC()
	existing := database.AgentSession{
		ID:              "existing-1",
		ProjectID:       env.projectID,
		Backend:         string(agents.BackendPi),
		InstanceID:      env.instanceID,
		NativeSessionID: "native-race",
		Status:          string(SessionActive),
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := env.db.Create(&existing).Error; err != nil {
		t.Fatalf("seed existing session: %v", err)
	}

	env.fake.mu.Lock()
	env.fake.createRef = &agents.SessionRef{InstanceID: env.instanceID, Backend: agents.BackendPi, SessionID: "native-race"}
	env.fake.mu.Unlock()

	created, err := env.svc.Create(context.Background(), env.projectID, agents.BackendPi, "", "", "")
	if err != nil {
		t.Fatalf("Create after a duplicate native-session race: %v", err)
	}
	session := created.Session
	if session.ID != "existing-1" {
		t.Errorf("Create returned %q, want the already-indexed id existing-1", session.ID)
	}
	if got := sessionCount(t, env.db); got != 1 {
		t.Errorf("session count = %d, want 1 after the race", got)
	}
}

// TestStaleSessionReflectsInstanceLifecycle verifies a session whose instance
// has stopped is reported disconnected, and continue/send refuse with a clear
// not-running error instead of pretending the session is usable.
func TestStaleSessionReflectsInstanceLifecycle(t *testing.T) {
	env := newTestEnv(t)
	session := createNativeSession(t, env, "native-1")

	if _, err := env.instances.Stop(context.Background(), env.instanceID); err != nil {
		t.Fatalf("stop instance: %v", err)
	}

	list, err := env.svc.List(env.projectID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0].Status != SessionDisconnected {
		t.Fatalf("List = %+v, want one disconnected session", list)
	}

	if _, err := env.svc.Send(context.Background(), env.projectID, session.ID, "hi", ""); !errors.Is(err, instances.ErrInstanceNotRunning) {
		t.Errorf("Send on stale session error = %v, want ErrInstanceNotRunning", err)
	}
	// Continue is the repair path: a stale session is re-attached by starting a
	// fresh instance rather than refused, which is what makes opening a session
	// after a platform restart work without manual instance management.
	continued, err := env.svc.Continue(context.Background(), env.projectID, session.ID)
	if err != nil {
		t.Fatalf("Continue on stale session error = %v, want it to re-attach", err)
	}
	if continued.Status != SessionActive || continued.InstanceID == env.instanceID {
		t.Errorf("Continue = %+v, want an active session rebound onto a new instance", continued)
	}
}

// TestContinueLeavesTheActivityTimestampAlone verifies opening a session is not
// an interaction: continue persists the re-attach (status, title, a rebound
// instance) without refreshing updated_at — the column the workbench sorts its
// session list by — while a real send does refresh it.
func TestContinueLeavesTheActivityTimestampAlone(t *testing.T) {
	env := newTestEnv(t)
	session := createNativeSession(t, env, "native-1")

	past := time.Now().UTC().Add(-time.Hour)
	if err := env.db.Model(&database.AgentSession{}).Where("id = ?", session.ID).
		Update("updated_at", past).Error; err != nil {
		t.Fatalf("age session: %v", err)
	}

	continued, err := env.svc.Continue(context.Background(), env.projectID, session.ID)
	if err != nil {
		t.Fatalf("Continue: %v", err)
	}

	var row database.AgentSession
	if err := env.db.First(&row, "id = ?", session.ID).Error; err != nil {
		t.Fatalf("load session row: %v", err)
	}
	if !row.UpdatedAt.Equal(past) {
		t.Errorf("persisted updated_at = %v after Continue, want the aged %v: opening a session is not activity", row.UpdatedAt, past)
	}
	if continued.Status != SessionActive {
		t.Errorf("continued status = %s, want active", continued.Status)
	}

	// A real interaction still moves the timestamp: sending a message must
	// raise the session to the top of the list.
	sent, err := env.svc.Send(context.Background(), env.projectID, session.ID, "hello", "")
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if !sent.UpdatedAt.After(past) {
		t.Errorf("Send updated_at = %v, want it to refresh past %v", sent.UpdatedAt, past)
	}
}

// TestReconcilePreservesLastInteractionTime verifies status changes during
// listing reconciliation or stream disconnects do not overwrite updated_at.
func TestReconcilePreservesLastInteractionTime(t *testing.T) {
	env := newTestEnv(t)
	session := createNativeSession(t, env, "native-reconcile-time")

	past := time.Now().UTC().Add(-2 * time.Hour).Truncate(time.Second)
	if err := env.db.Model(&database.AgentSession{}).Where("id = ?", session.ID).
		Update("updated_at", past).Error; err != nil {
		t.Fatalf("age session: %v", err)
	}

	// Stop instance so the session is reconciled to disconnected.
	if err := env.db.Model(&database.AgentInstance{}).Where("id = ?", env.instanceID).
		Update("status", string(agents.InstanceStopped)).Error; err != nil {
		t.Fatalf("stop instance: %v", err)
	}

	list, err := env.svc.List(env.projectID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("List returned %d sessions, want 1", len(list))
	}
	if list[0].Status != SessionDisconnected {
		t.Errorf("session status = %v, want disconnected", list[0].Status)
	}
	if !list[0].UpdatedAt.Equal(past) {
		t.Errorf("reconciled session updated_at = %v, want preserved %v", list[0].UpdatedAt, past)
	}

	var row database.AgentSession
	if err := env.db.First(&row, "id = ?", session.ID).Error; err != nil {
		t.Fatalf("load row: %v", err)
	}
	if !row.UpdatedAt.Equal(past) {
		t.Errorf("persisted updated_at = %v, want %v", row.UpdatedAt, past)
	}
}

// TestProjectScopingRejectsCrossProjectAccess verifies every session operation
// validates that the session belongs to the authorized project.
func TestProjectScopingRejectsCrossProjectAccess(t *testing.T) {
	env := newTestEnv(t)
	otherProject := env.registerProject(t, "beta")
	session := createNativeSession(t, env, "native-1")

	if _, err := env.svc.Get(otherProject, session.ID); !errors.Is(err, ErrSessionNotFound) {
		t.Errorf("Get cross-project error = %v, want ErrSessionNotFound", err)
	}
	if _, err := env.svc.Send(context.Background(), otherProject, session.ID, "hi", ""); !errors.Is(err, ErrSessionNotFound) {
		t.Errorf("Send cross-project error = %v, want ErrSessionNotFound", err)
	}
	if _, err := env.svc.Continue(context.Background(), otherProject, session.ID); !errors.Is(err, ErrSessionNotFound) {
		t.Errorf("Continue cross-project error = %v, want ErrSessionNotFound", err)
	}
	if _, err := env.svc.Stop(context.Background(), otherProject, session.ID); !errors.Is(err, ErrSessionNotFound) {
		t.Errorf("Stop cross-project error = %v, want ErrSessionNotFound", err)
	}
}

// TestSendAndContinueUseNativeMapping verifies send and continue hand the
// adapter the agent-native session id, never the platform id.
func TestSendAndContinueUseNativeMapping(t *testing.T) {
	env := newTestEnv(t)
	session := createNativeSession(t, env, "native-7")

	env.fake.mu.Lock()
	env.fake.continueRef = &agents.SessionRef{
		InstanceID: env.instanceID,
		Backend:    agents.BackendPi,
		SessionID:  "native-7",
		Title:      "renamed",
	}
	env.fake.mu.Unlock()

	continued, err := env.svc.Continue(context.Background(), env.projectID, session.ID)
	if err != nil {
		t.Fatalf("Continue: %v", err)
	}
	if continued.Title != "renamed" || continued.Status != SessionActive {
		t.Errorf("continued session = %+v, want title renamed active", continued)
	}

	sent, err := env.svc.Send(context.Background(), env.projectID, session.ID, "hello", "model-x")
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if sent.ID != session.ID {
		t.Errorf("Send returned session %q, want %q", sent.ID, session.ID)
	}

	env.fake.mu.Lock()
	defer env.fake.mu.Unlock()
	if len(env.fake.sends) != 1 || env.fake.sends[0].Text != "hello" || env.fake.sends[0].Model != "model-x" {
		t.Errorf("adapter sends = %+v, want one hello/model-x", env.fake.sends)
	}
	if len(env.fake.sentRefs) != 1 || env.fake.sentRefs[0].SessionID != "native-7" {
		t.Errorf("adapter received refs = %+v, want the native session id", env.fake.sentRefs)
	}
}

// TestUnsupportedCapabilitiesAreRejected verifies the platform refuses an
// operation the backend did not declare instead of presenting it as available.
func TestUnsupportedCapabilitiesAreRejected(t *testing.T) {
	env := newTestEnv(t)
	env.fake.mu.Lock()
	env.fake.caps = agents.Capabilities{Streaming: true}
	env.fake.mu.Unlock()

	session := createNativeSession(t, env, "native-1")

	if _, err := env.svc.Continue(context.Background(), env.projectID, session.ID); !errors.Is(err, ErrUnsupportedCapability) {
		t.Errorf("Continue error = %v, want ErrUnsupportedCapability", err)
	}
	if _, err := env.svc.Send(context.Background(), env.projectID, session.ID, "hi", "model-x"); !errors.Is(err, ErrUnsupportedCapability) {
		t.Errorf("Send with model error = %v, want ErrUnsupportedCapability", err)
	}
	if _, err := env.svc.Stop(context.Background(), env.projectID, session.ID); !errors.Is(err, ErrUnsupportedCapability) {
		t.Errorf("Stop error = %v, want ErrUnsupportedCapability", err)
	}

	// Sending without a model is still allowed when the backend streams.
	if _, err := env.svc.Send(context.Background(), env.projectID, session.ID, "hi", ""); err != nil {
		t.Errorf("Send without model error = %v, want nil", err)
	}
}

// TestStopIsSessionScopedAndIdempotent verifies stopping a session marks only
// that session stopped: its siblings stay active, the instance keeps running and
// the adapter's instance stop is never called. Concurrent and repeated stops are
// safe.
func TestStopIsSessionScopedAndIdempotent(t *testing.T) {
	env := newTestEnv(t)
	target := createNativeSession(t, env, "native-a")
	sibling := createNativeSession(t, env, "native-b")

	const callers = 8
	errs := make([]error, callers)
	var wg sync.WaitGroup
	wg.Add(callers)
	for i := 0; i < callers; i++ {
		go func(i int) {
			defer wg.Done()
			_, errs[i] = env.svc.Stop(context.Background(), env.projectID, target.ID)
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("concurrent Stop[%d] error = %v", i, err)
		}
	}
	if got := env.fake.stopCallCount(); got != 0 {
		t.Errorf("adapter StopInstance calls = %d, want 0: a session stop must not stop the instance", got)
	}
	if got := env.fake.interruptCallCount(); got != 0 {
		t.Errorf("adapter Interrupt calls = %d, want 0 for an idle session", got)
	}

	gotTarget, err := env.svc.Get(env.projectID, target.ID)
	if err != nil {
		t.Fatalf("Get target: %v", err)
	}
	if gotTarget.Status != SessionStopped {
		t.Errorf("target status = %q, want %q", gotTarget.Status, SessionStopped)
	}
	gotSibling, err := env.svc.Get(env.projectID, sibling.ID)
	if err != nil {
		t.Fatalf("Get sibling: %v", err)
	}
	if gotSibling.Status != SessionActive {
		t.Errorf("sibling status = %q, want %q", gotSibling.Status, SessionActive)
	}
	inst, err := env.instances.Get(env.instanceID)
	if err != nil {
		t.Fatalf("Get instance: %v", err)
	}
	if inst.Status != agents.InstanceRunning {
		t.Errorf("instance status = %q, want %q", inst.Status, agents.InstanceRunning)
	}

	again, err := env.svc.Stop(context.Background(), env.projectID, target.ID)
	if err != nil {
		t.Fatalf("second Stop: %v", err)
	}
	if again.Status != SessionStopped {
		t.Errorf("second stop status = %q, want %q", again.Status, SessionStopped)
	}
}

// TestStopInterruptsOnlyItsOwnRunningTurn verifies stopping a session with a
// turn in flight interrupts that turn through the adapter, and leaves the
// sibling session and the instance alone.
func TestStopInterruptsOnlyItsOwnRunningTurn(t *testing.T) {
	env := newTestEnv(t)
	target := createNativeSession(t, env, "native-a")
	sibling := createNativeSession(t, env, "native-b")
	env.fake.emit(t, env.instanceID, turnFor(env.instanceID, target.NativeSessionID, "t1", agents.TurnStarted, ""))
	waitForTurn(t, env.svc, env.projectID, target.ID, TurnRunning)

	got, err := env.svc.Stop(context.Background(), env.projectID, target.ID)
	if err != nil {
		t.Fatalf("Stop running session: %v", err)
	}
	if got.Status != SessionStopped {
		t.Errorf("status = %q, want %q", got.Status, SessionStopped)
	}
	if calls := env.fake.interruptCallCount(); calls != 1 {
		t.Errorf("adapter Interrupt calls = %d, want 1", calls)
	}
	if calls := env.fake.stopCallCount(); calls != 0 {
		t.Errorf("adapter StopInstance calls = %d, want 0", calls)
	}
	gotSibling, err := env.svc.Get(env.projectID, sibling.ID)
	if err != nil {
		t.Fatalf("Get sibling: %v", err)
	}
	if gotSibling.Status != SessionActive {
		t.Errorf("sibling status = %q, want %q", gotSibling.Status, SessionActive)
	}
}

// TestStopFailureKeepsSessionActive verifies a failed interrupt is reported as a
// backend operation failure and leaves the session as it was, so the user can
// try again instead of believing the run ended.
func TestStopFailureKeepsSessionActive(t *testing.T) {
	env := newTestEnv(t)
	session := createNativeSession(t, env, "native-1")
	env.fake.emit(t, env.instanceID, turnFor(env.instanceID, session.NativeSessionID, "t1", agents.TurnStarted, ""))
	waitForTurn(t, env.svc, env.projectID, session.ID, TurnRunning)

	env.fake.mu.Lock()
	env.fake.interruptErr = errors.New("adapter boom")
	env.fake.mu.Unlock()

	if _, err := env.svc.Stop(context.Background(), env.projectID, session.ID); !errors.Is(err, ErrBackendFailed) {
		t.Fatalf("Stop error = %v, want ErrBackendFailed", err)
	}
	got, err := env.svc.Get(env.projectID, session.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != SessionActive {
		t.Errorf("session status after failed stop = %q, want %q", got.Status, SessionActive)
	}
}

// TestStreamSessionRequiresStreamingCapability verifies the SSE route refuses a
// backend that does not declare streaming support.
func TestStreamSessionRequiresStreamingCapability(t *testing.T) {
	env := newTestEnv(t)
	env.fake.mu.Lock()
	env.fake.caps = agents.Capabilities{SessionDiscovery: true, Stop: true}
	env.fake.mu.Unlock()

	session := createNativeSession(t, env, "native-1")

	err := env.svc.StreamSession(context.Background(), env.projectID, session.ID, 0, &recordingWriter{}, nil)
	if !errors.Is(err, ErrUnsupportedCapability) {
		t.Fatalf("StreamSession error = %v, want ErrUnsupportedCapability", err)
	}
}

// TestStreamSessionRevalidatesInstanceProject verifies StreamSession enforces
// the instance↔project relationship even when the index row claims the project,
// so a corrupted mapping cannot stream another project's instance.
func TestStreamSessionRevalidatesInstanceProject(t *testing.T) {
	env := newTestEnv(t)
	otherProject := env.registerProject(t, "beta")
	otherInstance, err := env.instances.Start(context.Background(), otherProject, agents.BackendPi)
	if err != nil {
		t.Fatalf("start other instance: %v", err)
	}

	now := time.Now().UTC()
	row := database.AgentSession{
		ID:              "cross-instance",
		ProjectID:       env.projectID,
		Backend:         string(agents.BackendPi),
		InstanceID:      otherInstance.ID,
		NativeSessionID: "n",
		Status:          string(SessionActive),
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := env.db.Create(&row).Error; err != nil {
		t.Fatalf("insert cross-project session: %v", err)
	}

	err = env.svc.StreamSession(context.Background(), env.projectID, row.ID, 0, &recordingWriter{}, nil)
	if !errors.Is(err, instances.ErrInstanceNotFound) {
		t.Fatalf("StreamSession error = %v, want ErrInstanceNotFound", err)
	}
}

// TestUnknownSessionOperationsAreNotFound verifies unknown ids fail cleanly.
func TestUnknownSessionOperationsAreNotFound(t *testing.T) {
	env := newTestEnv(t)

	if _, err := env.svc.Get(env.projectID, "missing"); !errors.Is(err, ErrSessionNotFound) {
		t.Errorf("Get unknown error = %v, want ErrSessionNotFound", err)
	}
	if _, err := env.svc.Stop(context.Background(), env.projectID, "missing"); !errors.Is(err, ErrSessionNotFound) {
		t.Errorf("Stop unknown error = %v, want ErrSessionNotFound", err)
	}
}

// TestDeleteRemovesOnlyThePlatformIndex verifies deleting a session drops its
// platform index row and leaves the agent-native session alone, so a later
// discovery of the same native session re-indexes it.
func TestDeleteRemovesOnlyThePlatformIndex(t *testing.T) {
	env := newTestEnv(t)
	created := createNativeSession(t, env, "native-delete")

	if err := env.svc.Delete(env.projectID, created.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := env.svc.Get(env.projectID, created.ID); !errors.Is(err, ErrSessionNotFound) {
		t.Errorf("Get after Delete error = %v, want ErrSessionNotFound", err)
	}
	if got := sessionCount(t, env.db); got != 0 {
		t.Errorf("session count after delete = %d, want 0", got)
	}
}

// TestDeleteRejectsCrossProjectAccess verifies a session id cannot be deleted
// through a project that does not own it.
func TestDeleteRejectsCrossProjectAccess(t *testing.T) {
	env := newTestEnv(t)
	created := createNativeSession(t, env, "native-owner")
	other := env.registerProject(t, "beta")

	if err := env.svc.Delete(other, created.ID); !errors.Is(err, ErrSessionNotFound) {
		t.Errorf("Delete from other project error = %v, want ErrSessionNotFound", err)
	}
	if _, err := env.svc.Get(env.projectID, created.ID); err != nil {
		t.Errorf("session was removed by a cross-project delete: %v", err)
	}
}

// TestHandleDeleteRemovesSession verifies the DELETE route removes the index,
// answers 204, and reports an unknown id as 404.
func TestHandleDeleteRemovesSession(t *testing.T) {
	gin.SetMode(gin.TestMode)
	env := newTestEnv(t)
	router := gin.New()
	env.svc.RegisterRoutes(router)

	created := createNativeSession(t, env, "native-http-delete")

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/projects/%d/sessions/%s", env.projectID, created.ID), nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE session = %d %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/projects/%d/sessions/%s", env.projectID, created.ID), nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("DELETE missing session = %d, want 404", rec.Code)
	}
}

// TestBackendsReportsDeclaredCapabilities verifies the capability endpoint
// returns each registered backend's declaration for the workbench UI.
func TestBackendsReportsDeclaredCapabilities(t *testing.T) {
	env := newTestEnv(t)

	backends := env.svc.Backends()
	if len(backends) != 1 || backends[0].Backend != agents.BackendPi {
		t.Fatalf("Backends = %+v, want only pi", backends)
	}
	if !backends[0].Capabilities.ModelSelection || backends[0].Capabilities.ToolConfirmation {
		t.Errorf("capabilities = %+v, want the adapter declaration", backends[0].Capabilities)
	}
}

// TestHandlersServeSessionJSONAndSSE verifies the session handlers expose the
// capability list, create sessions, map service errors to statuses, and set
// the SSE headers for an active session.
func TestHandlersServeSessionJSONAndSSE(t *testing.T) {
	gin.SetMode(gin.TestMode)
	env := newTestEnv(t)
	router := gin.New()
	env.svc.RegisterRoutes(router)

	get := func(path string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		return rec
	}

	// The capability list is served.
	rec := get("/agent-backends")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"pi"`) {
		t.Fatalf("GET /agent-backends = %d %s", rec.Code, rec.Body.String())
	}

	// Creating a session returns 201 with the platform mapping.
	env.fake.mu.Lock()
	env.fake.createRef = &agents.SessionRef{InstanceID: env.instanceID, Backend: agents.BackendPi, SessionID: "native-http"}
	env.fake.mu.Unlock()
	body, _ := json.Marshal(createSessionRequest{Backend: agents.BackendPi, Title: "t"})
	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/projects/%d/sessions", env.projectID), strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST sessions = %d %s", rec.Code, rec.Body.String())
	}
	var created createSessionResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created session: %v", err)
	}
	if created.Session.NativeSessionID != "native-http" {
		t.Errorf("created session = %+v, want native native-http", created.Session)
	}

	// A body that still carries the removed instanceId is ignored: without a
	// backend it is a 400, never a silently-created session.
	legacy, _ := json.Marshal(map[string]string{"instanceId": env.instanceID})
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/projects/%d/sessions", env.projectID), strings.NewReader(string(legacy)))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("POST legacy instanceId body = %d, want 400", rec.Code)
	}

	// Sending an empty message is a 400.
	empty, _ := json.Marshal(sendMessageRequest{Text: "  "})
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/projects/%d/sessions/%s/messages", env.projectID, created.Session.ID), strings.NewReader(string(empty)))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("POST empty message = %d, want 400", rec.Code)
	}

	// The SSE route sets text/event-stream for an active session.
	ctx, cancel := context.WithCancel(context.Background())
	rec = httptest.NewRecorder()
	sseReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/projects/%d/sessions/%s/events", env.projectID, created.Session.ID), nil).WithContext(ctx)
	done := make(chan struct{})
	go func() {
		router.ServeHTTP(rec, sseReq)
		close(done)
	}()
	waitForSubscriber(t, env)
	cancel()
	<-done
	if got := rec.Header().Get("Content-Type"); got != "text/event-stream" {
		t.Errorf("SSE Content-Type = %q, want text/event-stream", got)
	}

	// Stopping an idle session is a session-level change and succeeds.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/projects/%d/sessions/%s/stop", env.projectID, created.Session.ID), nil)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("POST stop = %d, want 200", rec.Code)
	}
}

// TestHandleCreateMapsStartFailures verifies create maps a platform-disabled
// backend to 409 agent_disabled and a failed auto-start to 502
// instance_start_failed, matching the instance routes' existing contract.
func TestHandleCreateMapsStartFailures(t *testing.T) {
	gin.SetMode(gin.TestMode)
	enabled := true
	env := newTestEnv(t, func(agents.BackendID) bool { return enabled })
	router := gin.New()
	env.svc.RegisterRoutes(router)

	disabled := env.registerProject(t, "beta-disabled")
	enabled = false

	post := func(projectID uint) *httptest.ResponseRecorder {
		body, _ := json.Marshal(createSessionRequest{Backend: agents.BackendPi})
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/projects/%d/sessions", projectID), strings.NewReader(string(body)))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(rec, req)
		return rec
	}

	rec := post(disabled)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "agent_disabled") {
		t.Fatalf("POST disabled backend = %d %s, want 409 agent_disabled", rec.Code, rec.Body.String())
	}

	enabled = true
	failing := env.registerProject(t, "beta-failing")
	env.fake.mu.Lock()
	env.fake.startErr = errors.New("adapter cannot start")
	env.fake.mu.Unlock()
	rec = post(failing)
	if rec.Code != http.StatusBadGateway || !strings.Contains(rec.Body.String(), "instance_start_failed") {
		t.Fatalf("POST start failure = %d %s, want 502 instance_start_failed", rec.Code, rec.Body.String())
	}
}

// TestInterruptIdleSessionIsNoop verifies interrupting an idle session succeeds
// without calling the adapter and leaves the turn idle.
func TestInterruptIdleSessionIsNoop(t *testing.T) {
	env := newTestEnv(t)
	session := createNativeSession(t, env, "native-idle")

	got, err := env.svc.Interrupt(context.Background(), env.projectID, session.ID)
	if err != nil {
		t.Fatalf("Interrupt idle: %v", err)
	}
	if got.Turn != TurnIdle {
		t.Errorf("interrupted idle session turn = %q, want idle", got.Turn)
	}
	if calls := env.fake.interruptCallCount(); calls != 0 {
		t.Errorf("adapter Interrupt calls = %d, want 0 for an idle session", calls)
	}
}

// TestInterruptRunningSessionCallsAdapterOnce verifies a running turn is
// interrupted through the adapter exactly once, with the native session id.
func TestInterruptRunningSessionCallsAdapterOnce(t *testing.T) {
	env := newTestEnv(t)
	session := createNativeSession(t, env, "native-busy")
	env.fake.emit(t, env.instanceID, turnFor(env.instanceID, session.NativeSessionID, "t1", agents.TurnStarted, ""))
	waitForTurn(t, env.svc, env.projectID, session.ID, TurnRunning)

	if _, err := env.svc.Interrupt(context.Background(), env.projectID, session.ID); err != nil {
		t.Fatalf("Interrupt running: %v", err)
	}
	if calls := env.fake.interruptCallCount(); calls != 1 {
		t.Fatalf("adapter Interrupt calls = %d, want 1", calls)
	}
	env.fake.mu.Lock()
	refs := append([]agents.SessionRef(nil), env.fake.interruptRefs...)
	env.fake.mu.Unlock()
	if len(refs) != 1 || refs[0].SessionID != "native-busy" {
		t.Errorf("adapter Interrupt refs = %+v, want the native session id", refs)
	}
}

// TestInterruptRejectsUnsupportedCapability verifies a backend without the
// interrupt capability is refused.
func TestInterruptRejectsUnsupportedCapability(t *testing.T) {
	env := newTestEnv(t)
	session := createNativeSession(t, env, "native-1")
	env.fake.mu.Lock()
	env.fake.caps = agents.Capabilities{Streaming: true}
	env.fake.mu.Unlock()

	if _, err := env.svc.Interrupt(context.Background(), env.projectID, session.ID); !errors.Is(err, ErrUnsupportedCapability) {
		t.Fatalf("Interrupt error = %v, want ErrUnsupportedCapability", err)
	}
}

// TestInterruptReportsBackendFailure verifies an adapter interrupt failure is
// labelled a backend operation failure.
func TestInterruptReportsBackendFailure(t *testing.T) {
	env := newTestEnv(t)
	session := createNativeSession(t, env, "native-busy")
	env.fake.emit(t, env.instanceID, turnFor(env.instanceID, session.NativeSessionID, "t1", agents.TurnStarted, ""))
	waitForTurn(t, env.svc, env.projectID, session.ID, TurnRunning)
	env.fake.mu.Lock()
	env.fake.interruptErr = errors.New("adapter boom")
	env.fake.mu.Unlock()

	if _, err := env.svc.Interrupt(context.Background(), env.projectID, session.ID); !errors.Is(err, ErrBackendFailed) {
		t.Fatalf("Interrupt error = %v, want ErrBackendFailed", err)
	}
}

// TestHandleInterruptReturnsSessionDTO verifies the interrupt route returns the
// session DTO for an idle session.
func TestHandleInterruptReturnsSessionDTO(t *testing.T) {
	gin.SetMode(gin.TestMode)
	env := newTestEnv(t)
	router := gin.New()
	env.svc.RegisterRoutes(router)
	session := createNativeSession(t, env, "native-1")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/projects/%d/sessions/%s/interrupt", env.projectID, session.ID), nil)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST interrupt = %d %s, want 200", rec.Code, rec.Body.String())
	}
	var got Session
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode interrupted session: %v", err)
	}
	if got.ID != session.ID || got.Turn != TurnIdle {
		t.Errorf("interrupted session = %+v, want idle session %s", got, session.ID)
	}
}

// historyEvents builds a history result the fake adapter hands back. The
// events deliberately carry no instance or session identity: stamping is the
// service's job, so a test that forgot to stamp would fail Validate.
func historyEvents(observed time.Time, texts ...string) []agents.NormalizedEvent {
	events := make([]agents.NormalizedEvent, 0, len(texts))
	for _, text := range texts {
		events = append(events, agents.NormalizedEvent{
			Type:      agents.EventMessage,
			Backend:   agents.BackendPi,
			Timestamp: observed,
			Replayed:  true,
			Message:   &agents.MessagePayload{Role: "assistant", Text: text, Final: true},
		})
	}
	return events
}

// TestHistoryStampsEnvelopeAndReturnsEvents pins the service's part of the
// contract: the adapter may leave instance and session identity empty, and the
// service fills it from the session row before anything reaches a client.
func TestHistoryStampsEnvelopeAndReturnsEvents(t *testing.T) {
	env := newTestEnv(t)
	session := createNativeSession(t, env, "native-hist")
	observed := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)

	env.fake.mu.Lock()
	env.fake.historyResult = agents.HistoryResult{Events: historyEvents(observed, "第一轮", "第二轮")}
	env.fake.mu.Unlock()

	history, err := env.svc.History(context.Background(), env.projectID, session.ID)
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(history.Events) != 2 {
		t.Fatalf("History returned %d events, want 2", len(history.Events))
	}
	for i, event := range history.Events {
		if event.InstanceID != env.instanceID {
			t.Errorf("events[%d].InstanceID = %q, want the session's instance %q", i, event.InstanceID, env.instanceID)
		}
		if event.SessionID != "native-hist" {
			t.Errorf("events[%d].SessionID = %q, want the native session id", i, event.SessionID)
		}
		if event.Backend != agents.BackendPi {
			t.Errorf("events[%d].Backend = %q, want pi", i, event.Backend)
		}
		if !event.Replayed {
			t.Errorf("events[%d] lost its replayed marker", i)
		}
		if err := event.Validate(); err != nil {
			t.Errorf("events[%d] does not validate after stamping: %v", i, err)
		}
	}

	env.fake.mu.Lock()
	defer env.fake.mu.Unlock()
	if len(env.fake.historyReqs) != 1 {
		t.Fatalf("adapter saw %d history requests, want 1", len(env.fake.historyReqs))
	}
	request := env.fake.historyReqs[0]
	if request.Directory != env.projectDir || request.SessionID != "native-hist" {
		t.Errorf("history request = %+v, want the project directory and the native session id", request)
	}
}

// TestHistoryTruncatedFlagPropagates pins that the truncation verdict comes
// from the adapter that did the bounding, not from the service re-deriving it.
func TestHistoryTruncatedFlagPropagates(t *testing.T) {
	env := newTestEnv(t)
	session := createNativeSession(t, env, "native-trunc")
	observed := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)

	env.fake.mu.Lock()
	env.fake.historyResult = agents.HistoryResult{Events: historyEvents(observed, "只有一条"), Truncated: false}
	env.fake.mu.Unlock()

	history, err := env.svc.History(context.Background(), env.projectID, session.ID)
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if history.Truncated {
		t.Error("Truncated = true for an untruncated read")
	}

	env.fake.mu.Lock()
	env.fake.historyResult = agents.HistoryResult{Events: historyEvents(observed, "最后一条"), Truncated: true}
	env.fake.mu.Unlock()

	history, err = env.svc.History(context.Background(), env.projectID, session.ID)
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if !history.Truncated {
		t.Error("Truncated = false, want the adapter's verdict to reach the response")
	}
}

// TestHistoryUnsupportedCapability pins that a backend without the capability
// is refused rather than asked for a transcript it cannot read.
func TestHistoryUnsupportedCapability(t *testing.T) {
	env := newTestEnv(t)
	session := createNativeSession(t, env, "native-nohist")
	env.fake.mu.Lock()
	env.fake.caps = agents.Capabilities{Streaming: true}
	env.fake.mu.Unlock()

	_, err := env.svc.History(context.Background(), env.projectID, session.ID)
	if !errors.Is(err, ErrUnsupportedCapability) {
		t.Errorf("History error = %v, want ErrUnsupportedCapability", err)
	}
}

// TestHistoryBackendFailureIsReported pins that a failing native read reaches
// the client as a backend failure, not as an empty history.
func TestHistoryBackendFailureIsReported(t *testing.T) {
	env := newTestEnv(t)
	session := createNativeSession(t, env, "native-broken")
	env.fake.mu.Lock()
	env.fake.historyErr = errors.New("transcript unreadable")
	env.fake.mu.Unlock()

	_, err := env.svc.History(context.Background(), env.projectID, session.ID)
	if !errors.Is(err, ErrBackendFailed) {
		t.Errorf("History error = %v, want ErrBackendFailed", err)
	}
}

// TestHistoryUnknownSessionIsNotFound pins project and session scoping.
func TestHistoryUnknownSessionIsNotFound(t *testing.T) {
	env := newTestEnv(t)

	if _, err := env.svc.History(context.Background(), env.projectID, "no-such-session"); !errors.Is(err, ErrSessionNotFound) {
		t.Errorf("History for an unknown session error = %v, want ErrSessionNotFound", err)
	}

	session := createNativeSession(t, env, "native-scoped")
	other := env.registerProject(t, "beta")
	if _, err := env.svc.History(context.Background(), other, session.ID); !errors.Is(err, ErrSessionNotFound) {
		t.Errorf("History through another project error = %v, want ErrSessionNotFound", err)
	}
}

// TestHandlersServeSessionHistory pins the HTTP shape and the error mapping of
// the history endpoint.
func TestHandlersServeSessionHistory(t *testing.T) {
	gin.SetMode(gin.TestMode)
	env := newTestEnv(t)
	router := gin.New()
	env.svc.RegisterRoutes(router)
	session := createNativeSession(t, env, "native-http-history")
	observed := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)

	env.fake.mu.Lock()
	env.fake.historyResult = agents.HistoryResult{Events: historyEvents(observed, "历史一条"), Truncated: true}
	env.fake.mu.Unlock()

	get := func(path string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		return rec
	}

	rec := get(fmt.Sprintf("/projects/%d/sessions/%s/history", env.projectID, session.ID))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET history = %d %s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Events    []agents.NormalizedEvent `json:"events"`
		Truncated bool                     `json:"truncated"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode history response %s: %v", rec.Body.String(), err)
	}
	if len(payload.Events) != 1 || payload.Events[0].SessionID != "native-http-history" || !payload.Truncated {
		t.Fatalf("history response = %+v, want one stamped event and truncated", payload)
	}

	// Unknown session → 404.
	if rec = get(fmt.Sprintf("/projects/%d/sessions/nope/history", env.projectID)); rec.Code != http.StatusNotFound {
		t.Errorf("GET unknown history = %d %s, want 404", rec.Code, rec.Body.String())
	}

	// Capability missing → 409.
	env.fake.mu.Lock()
	env.fake.caps = agents.Capabilities{Streaming: true}
	env.fake.mu.Unlock()
	if rec = get(fmt.Sprintf("/projects/%d/sessions/%s/history", env.projectID, session.ID)); rec.Code != http.StatusConflict {
		t.Errorf("GET unsupported history = %d %s, want 409", rec.Code, rec.Body.String())
	}

	// Backend failure → 502.
	env.fake.mu.Lock()
	env.fake.caps = agents.Capabilities{Streaming: true, SessionHistory: true}
	env.fake.historyErr = errors.New("boom")
	env.fake.mu.Unlock()
	if rec = get(fmt.Sprintf("/projects/%d/sessions/%s/history", env.projectID, session.ID)); rec.Code != http.StatusBadGateway {
		t.Errorf("GET failed history = %d %s, want 502", rec.Code, rec.Body.String())
	}
}

// stopEnvInstance stops the environment's instance without deleting its row,
// which is the state a platform restart leaves: the session row still points
// at an instance that is no longer running.
func stopEnvInstance(t *testing.T, env *testEnv) {
	t.Helper()
	if _, err := env.instances.Stop(context.Background(), env.instanceID); err != nil {
		t.Fatalf("stop instance: %v", err)
	}
}

// TestContinueStartsInstanceWhenNoneRunning pins the click-to-use path: a
// session whose instance is gone is re-attached by starting a new instance in
// the same project rather than asking the user to start one.
func TestContinueStartsInstanceWhenNoneRunning(t *testing.T) {
	env := newTestEnv(t)
	session := createNativeSession(t, env, "native-resume")
	stopEnvInstance(t, env)
	before := env.fake.startCount()

	continued, err := env.svc.Continue(context.Background(), env.projectID, session.ID)
	if err != nil {
		t.Fatalf("Continue: %v", err)
	}
	if got := env.fake.startCount(); got != before+1 {
		t.Errorf("StartInstance calls = %d, want %d (one auto-start)", got, before+1)
	}
	if continued.InstanceID == env.instanceID {
		t.Fatalf("InstanceID = %q, want a rebind onto the newly started instance", continued.InstanceID)
	}
	if continued.Status != SessionActive {
		t.Errorf("Status = %q, want active after continue", continued.Status)
	}

	var row database.AgentSession
	if err := env.db.First(&row, "id = ?", session.ID).Error; err != nil {
		t.Fatalf("load session row: %v", err)
	}
	if row.InstanceID != continued.InstanceID {
		t.Errorf("persisted instance id = %q, want %q", row.InstanceID, continued.InstanceID)
	}
	if row.NativeSessionID != "native-resume" {
		t.Errorf("native session id = %q, want it untouched", row.NativeSessionID)
	}
}

// TestContinueIsIdempotentWhenInstanceRuns pins that the auto-start path costs
// nothing when the instance is already usable.
func TestContinueIsIdempotentWhenInstanceRuns(t *testing.T) {
	env := newTestEnv(t)
	session := createNativeSession(t, env, "native-idle")
	before := env.fake.startCount()

	continued, err := env.svc.Continue(context.Background(), env.projectID, session.ID)
	if err != nil {
		t.Fatalf("Continue: %v", err)
	}
	if got := env.fake.startCount(); got != before {
		t.Errorf("StartInstance calls = %d, want %d (no start on a running instance)", got, before)
	}
	if continued.InstanceID != env.instanceID {
		t.Errorf("InstanceID = %q, want the running instance %q", continued.InstanceID, env.instanceID)
	}
}

// TestContinueRejectsOtherProject pins project scoping.
func TestContinueRejectsOtherProject(t *testing.T) {
	env := newTestEnv(t)
	session := createNativeSession(t, env, "native-scope")
	other := env.registerProject(t, "beta")

	if _, err := env.svc.Continue(context.Background(), other, session.ID); !errors.Is(err, ErrSessionNotFound) {
		t.Errorf("Continue through another project error = %v, want ErrSessionNotFound", err)
	}
}

// TestCreateRejectsPlatformDisabledAgent verifies that a platform-level disable
// blocks creating new sessions even when a running instance already exists (so
// the ones Start's gate would otherwise be bypassed).
func TestCreateRejectsPlatformDisabledAgent(t *testing.T) {
	enabled := true
	env := newTestEnv(t, func(agents.BackendID) bool { return enabled })
	enabled = false

	if _, err := env.svc.Create(context.Background(), env.projectID, agents.BackendPi, "blocked", "", ""); !errors.Is(err, ErrAgentDisabled) {
		t.Fatalf("Create error = %v, want ErrAgentDisabled", err)
	}
}

// TestBackendsReportsPlatformEnablement verifies the backend list carries the
// platform-level enablement flags.
func TestBackendsReportsPlatformEnablement(t *testing.T) {
	enabled := true
	env := newTestEnv(t, func(agents.BackendID) bool { return enabled })
	enabled = false

	for _, backend := range env.svc.Backends() {
		if backend.Enabled {
			t.Errorf("backend %s reported enabled while platform-disabled", backend.Backend)
		}
	}
}

// TestSendBackfillsEmptyTitleFromFirstMessage verifies a session created
// without a title derives one from the first message, and that the derived
// title reaches the adapter so the backend can persist it.
func TestSendBackfillsEmptyTitleFromFirstMessage(t *testing.T) {
	env := newTestEnv(t)
	env.fake.mu.Lock()
	env.fake.createRef = &agents.SessionRef{InstanceID: env.instanceID, Backend: agents.BackendPi, SessionID: "native-t1"}
	env.fake.mu.Unlock()
	created, err := env.svc.Create(context.Background(), env.projectID, agents.BackendPi, "", "", "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	session := created.Session
	if session.Title != "" {
		t.Fatalf("created session title = %q, want empty", session.Title)
	}

	sent, err := env.svc.Send(context.Background(), env.projectID, session.ID, "帮我\n部署日志系统", "")
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if sent.Title != "帮我 部署日志系统" {
		t.Errorf("sent session title = %q, want %q", sent.Title, "帮我 部署日志系统")
	}
	env.fake.mu.Lock()
	defer env.fake.mu.Unlock()
	if len(env.fake.sentRefs) != 1 || env.fake.sentRefs[0].Title != "帮我 部署日志系统" {
		t.Errorf("adapter sentRefs = %+v, want the derived title so the backend persists it", env.fake.sentRefs)
	}
}

// TestSendKeepsExistingTitle verifies Send never rewrites a title that exists.
func TestSendKeepsExistingTitle(t *testing.T) {
	env := newTestEnv(t)
	session := createNativeSession(t, env, "native-t2") // helper titles it "native-t2"

	sent, err := env.svc.Send(context.Background(), env.projectID, session.ID, "新消息", "")
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if sent.Title != "native-t2" {
		t.Errorf("sent session title = %q, want native-t2 unchanged", sent.Title)
	}
	env.fake.mu.Lock()
	defer env.fake.mu.Unlock()
	if len(env.fake.sentRefs) != 1 || env.fake.sentRefs[0].Title != "native-t2" {
		t.Errorf("adapter sentRefs = %+v, want the existing title", env.fake.sentRefs)
	}
}

// TestCreateWithFirstMessageAdmitsTheTurn pins the merged create: the model
// rides on the session create itself (no switch call follows), an untitled
// message names the session, and the message is admitted on the created
// native ref with no per-message model.
func TestCreateWithFirstMessageAdmitsTheTurn(t *testing.T) {
	env := newTestEnv(t)
	env.fake.mu.Lock()
	env.fake.createRef = &agents.SessionRef{InstanceID: env.instanceID, Backend: agents.BackendPi, SessionID: "native-first"}
	env.fake.mu.Unlock()

	const text = "帮我检查构建失败"
	outcome, err := env.svc.Create(context.Background(), env.projectID, agents.BackendPi, "", text, "pi/model-x")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if outcome.MessageError != "" {
		t.Fatalf("MessageError = %q, want empty", outcome.MessageError)
	}
	if outcome.Session.ID == "" || outcome.Session.NativeSessionID != "native-first" {
		t.Fatalf("session = %+v, want the indexed native-first mapping", outcome.Session)
	}
	if len(env.fake.createOpts) != 1 || env.fake.createOpts[0].Model != "pi/model-x" {
		t.Fatalf("createOpts = %+v, want the model chosen at create", env.fake.createOpts)
	}
	if env.fake.createOpts[0].Title != agents.DeriveTitle(text) {
		t.Fatalf("create title = %q, want the message-derived %q", env.fake.createOpts[0].Title, agents.DeriveTitle(text))
	}
	if len(env.fake.sends) != 1 || env.fake.sends[0].Text != text || env.fake.sends[0].Model != "" {
		t.Fatalf("sends = %+v, want one model-less message with the text", env.fake.sends)
	}
	if len(env.fake.sentRefs) != 1 || env.fake.sentRefs[0].SessionID != "native-first" {
		t.Fatalf("sentRefs = %+v, want the created native ref", env.fake.sentRefs)
	}
}

// TestCreateWithFirstMessageFailureKeepsSession pins the recovery contract: a
// refused first message does not fail the create — the session stays indexed
// and the outcome carries the failure code the workbench reports.
func TestCreateWithFirstMessageFailureKeepsSession(t *testing.T) {
	env := newTestEnv(t)
	env.fake.mu.Lock()
	env.fake.createRef = &agents.SessionRef{InstanceID: env.instanceID, Backend: agents.BackendPi, SessionID: "native-refused"}
	env.fake.sendErr = errors.New("backend refused the prompt")
	env.fake.mu.Unlock()

	outcome, err := env.svc.Create(context.Background(), env.projectID, agents.BackendPi, "t", "hello", "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if outcome.MessageError != "agent_operation_failed" {
		t.Fatalf("MessageError = %q, want agent_operation_failed", outcome.MessageError)
	}
	if outcome.Session.ID == "" {
		t.Fatal("session missing despite the created ref")
	}
	if got := sessionCount(t, env.db); got != 1 {
		t.Errorf("session count = %d, want 1 (the session survives the failed message)", got)
	}
}

// TestCreateWithModelRequiresCapability keeps the model parameter honest: a
// backend without model selection refuses a create-with-model instead of
// ignoring it.
func TestCreateWithModelRequiresCapability(t *testing.T) {
	env := newTestEnv(t)
	env.fake.mu.Lock()
	env.fake.caps.ModelSelection = false
	env.fake.createRef = &agents.SessionRef{InstanceID: env.instanceID, Backend: agents.BackendPi, SessionID: "n"}
	env.fake.mu.Unlock()

	if _, err := env.svc.Create(context.Background(), env.projectID, agents.BackendPi, "", "hi", "pi/model-x"); !errors.Is(err, ErrUnsupportedCapability) {
		t.Fatalf("Create = %v, want ErrUnsupportedCapability", err)
	}
}

// TestCreateWithoutTitleLeavesTitleEmpty verifies an untitled create stores
// no placeholder — never the platform id.
func TestCreateWithoutTitleLeavesTitleEmpty(t *testing.T) {
	env := newTestEnv(t)
	env.fake.mu.Lock()
	env.fake.createRef = &agents.SessionRef{InstanceID: env.instanceID, Backend: agents.BackendPi, SessionID: "n-untitled"}
	env.fake.mu.Unlock()

	created, err := env.svc.Create(context.Background(), env.projectID, agents.BackendPi, "", "", "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	session := created.Session
	if session.Title != "" {
		t.Errorf("created title = %q, want empty", session.Title)
	}
	var row database.AgentSession
	if err := env.db.First(&row, "id = ?", session.ID).Error; err != nil {
		t.Fatalf("load row: %v", err)
	}
	if row.Title != "" {
		t.Errorf("row title = %q, want empty", row.Title)
	}
}

// TestBackendsCarryCatalogMetadata proves the list gives the workbench the
// display label, presentation order and service support from the catalog, and
// falls back to the raw id for a backend the catalog does not describe.
func TestBackendsCarryCatalogMetadata(t *testing.T) {
	env := newTestEnv(t)
	catalog, err := agents.NewCatalog(agents.BackendDescriptor{
		ID: agents.BackendPi, Label: "Pi Agent", Binary: "pi", Order: 2, ServiceSupported: false, Color: "#F97316",
	})
	if err != nil {
		t.Fatalf("NewCatalog: %v", err)
	}
	WithCatalog(catalog)(env.svc)

	backends := env.svc.Backends()
	if len(backends) != 1 {
		t.Fatalf("Backends = %+v, want one", backends)
	}
	if got := backends[0]; got.Label != "Pi Agent" || got.Order != 2 || got.ServiceSupported || got.Color != "#F97316" {
		t.Fatalf("metadata = %+v, want label Pi Agent, order 2, no service, color #F97316", got)
	}

	WithCatalog(nil)(env.svc)
	if got := env.svc.Backends()[0]; got.Label != string(agents.BackendPi) {
		t.Fatalf("label without catalog = %q, want the raw id", got.Label)
	}
}

// TestListSessionsPaginationAndFiltering verifies pagination and turn filtering on the list endpoint.
func TestListSessionsPaginationAndFiltering(t *testing.T) {
	gin.SetMode(gin.TestMode)
	env := newTestEnv(t)
	router := gin.New()
	env.svc.RegisterRoutes(router)

	// Create 3 sessions
	s1 := createNativeSession(t, env, "native-p1")
	s2 := createNativeSession(t, env, "native-p2")
	s3 := createNativeSession(t, env, "native-p3")

	// 1. Full list without limit
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/projects/%d/sessions", env.projectID), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("full list code = %d", rec.Code)
	}
	var fullResp sessionListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &fullResp); err != nil {
		t.Fatalf("unmarshal full list: %v", err)
	}
	if len(fullResp.Sessions) != 3 || fullResp.Total != 3 {
		t.Fatalf("full list count = %d, total = %d, want 3", len(fullResp.Sessions), fullResp.Total)
	}

	// 2. Pagination with limit=2, offset=0
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/projects/%d/sessions?limit=2&offset=0", env.projectID), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("page 1 list code = %d", rec.Code)
	}
	var pageResp sessionListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &pageResp); err != nil {
		t.Fatalf("unmarshal page list: %v", err)
	}
	if len(pageResp.Sessions) != 2 || pageResp.Total != 3 {
		t.Fatalf("page 1 count = %d, total = %d, want 2 and 3", len(pageResp.Sessions), pageResp.Total)
	}

	// 3. Pagination with limit=2, offset=2 (next page)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/projects/%d/sessions?limit=2&offset=2", env.projectID), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("page 2 list code = %d", rec.Code)
	}
	var page2Resp sessionListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &page2Resp); err != nil {
		t.Fatalf("unmarshal page 2 list: %v", err)
	}
	if len(page2Resp.Sessions) != 1 || page2Resp.Total != 3 {
		t.Fatalf("page 2 count = %d, total = %d, want 1 and 3", len(page2Resp.Sessions), page2Resp.Total)
	}

	// 4. Filter by turn=running
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/projects/%d/sessions?turn=running", env.projectID), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("turn filter list code = %d", rec.Code)
	}
	var runningResp sessionListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &runningResp); err != nil {
		t.Fatalf("unmarshal running list: %v", err)
	}
	// Initially none are running
	if len(runningResp.Sessions) != 0 || runningResp.Total != 0 {
		t.Fatalf("running count = %d, want 0", len(runningResp.Sessions))
	}

	_ = s1
	_ = s2
	_ = s3
}
