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

	"devplatform/internal/agents"
	"devplatform/internal/agents/instances"
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

// fakeAdapter is a scriptable AgentAdapter with an explicit Capabilities()
// declaration. It records the references and requests the service passes down
// so tests can assert the native ID mapping is used verbatim.
type fakeAdapter struct {
	mu sync.Mutex

	caps agents.Capabilities

	createRef  *agents.SessionRef
	createErr  error
	createOpts []agents.CreateSessionOptions

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
		subs: make(map[string]chan agents.NormalizedEvent),
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

// newTestEnv builds one workspace root, one project, one registered pi adapter
// and one running instance of it.
func newTestEnv(t *testing.T) *testEnv {
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
	instanceService := instances.NewService(db, registry, projectService)
	instance, err := instanceService.Start(context.Background(), project.ID, agents.BackendPi)
	if err != nil {
		t.Fatalf("start instance: %v", err)
	}

	return &testEnv{
		svc:        NewService(db, registry, instanceService),
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

	session, err := env.svc.Create(context.Background(), env.projectID, agents.BackendPi, nativeID)
	if err != nil {
		t.Fatalf("Create(%s): %v", nativeID, err)
	}
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
	session, err := env.svc.Create(context.Background(), env.projectID, agents.BackendPi, "reuse")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
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
	session, err := env.svc.Create(context.Background(), otherProject, agents.BackendPi, "started")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
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

	_, err := env.svc.Create(context.Background(), otherProject, agents.BackendPi, "nope")
	if !errors.Is(err, instances.ErrStartFailed) {
		t.Fatalf("Create error = %v, want instances.ErrStartFailed", err)
	}
	if got := sessionCount(t, env.db); got != 0 {
		t.Errorf("session count = %d, want 0 after a failed start", got)
	}
}

// TestCreateRefusesDisabledBackend verifies create does not silently start a
// backend the project has disabled.
func TestCreateRefusesDisabledBackend(t *testing.T) {
	env := newTestEnv(t)
	otherProject := env.registerProject(t, "beta")
	if _, err := env.instances.SetBackendEnabled(otherProject, agents.BackendPi, false); err != nil {
		t.Fatalf("disable backend: %v", err)
	}

	_, err := env.svc.Create(context.Background(), otherProject, agents.BackendPi, "t")
	if !errors.Is(err, instances.ErrBackendDisabled) {
		t.Fatalf("Create error = %v, want instances.ErrBackendDisabled", err)
	}
}

// TestCreateRejectsInvalidBackend verifies create refuses a missing or invalid
// backend rather than guessing one.
func TestCreateRejectsInvalidBackend(t *testing.T) {
	env := newTestEnv(t)

	if _, err := env.svc.Create(context.Background(), env.projectID, "", "t"); !errors.Is(err, ErrInvalidRequest) {
		t.Errorf("Create with empty backend error = %v, want ErrInvalidRequest", err)
	}
	if _, err := env.svc.Create(context.Background(), env.projectID, agents.BackendID("nope"), "t"); !errors.Is(err, ErrInvalidRequest) {
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

	session, err := env.svc.Create(context.Background(), env.projectID, agents.BackendPi, "")
	if err != nil {
		t.Fatalf("Create after a duplicate native-session race: %v", err)
	}
	if session.ID != "existing-1" {
		t.Errorf("Create returned %q, want the already-indexed id existing-1", session.ID)
	}
	if got := sessionCount(t, env.db); got != 1 {
		t.Errorf("session count = %d, want 1 after the race", got)
	}
}

// TestDiscoverIndexesNativeSessionsAndSkipsMalformed verifies discovery maps
// valid native sessions into the index exactly once and never attaches
// malformed or misattributed native metadata to the project.
func TestDiscoverIndexesNativeSessionsAndSkipsMalformed(t *testing.T) {
	env := newTestEnv(t)
	env.fake.mu.Lock()
	env.fake.listRefs = []agents.SessionRef{
		{InstanceID: env.instanceID, Backend: agents.BackendPi, SessionID: "n1", Title: "one"},
		{InstanceID: env.instanceID, Backend: agents.BackendPi, SessionID: "", Title: "malformed"},
		{InstanceID: "some-other-instance", Backend: agents.BackendPi, SessionID: "n2", Title: "misattributed"},
		{InstanceID: env.instanceID, Backend: agents.BackendOpenCodeV2, SessionID: "n3", Title: "wrong backend"},
	}
	env.fake.mu.Unlock()

	first, err := env.svc.Discover(context.Background(), env.projectID, env.instanceID)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(first) != 1 {
		t.Fatalf("Discover returned %d sessions, want 1 (malformed skipped)", len(first))
	}
	if first[0].NativeSessionID != "n1" || first[0].Title != "one" || first[0].Status != SessionActive {
		t.Errorf("discovered session = %+v, want native n1 title one active", first[0])
	}

	// Discovery is idempotent: the same native session keeps its platform id.
	second, err := env.svc.Discover(context.Background(), env.projectID, env.instanceID)
	if err != nil {
		t.Fatalf("second Discover: %v", err)
	}
	if len(second) != 1 || second[0].ID != first[0].ID {
		t.Errorf("second Discover = %+v, want the same platform id %q", second, first[0].ID)
	}
	if got := sessionCount(t, env.db); got != 1 {
		t.Errorf("session count = %d, want 1 after two discoveries", got)
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
	// fresh instance rather than refused, which is what makes opening a task
	// after a platform restart work without manual instance management.
	continued, err := env.svc.Continue(context.Background(), env.projectID, session.ID)
	if err != nil {
		t.Fatalf("Continue on stale session error = %v, want it to re-attach", err)
	}
	if continued.Status != SessionActive || continued.InstanceID == env.instanceID {
		t.Errorf("Continue = %+v, want an active session rebound onto a new instance", continued)
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
	if _, err := env.svc.Discover(context.Background(), otherProject, env.instanceID); !errors.Is(err, instances.ErrInstanceNotFound) {
		t.Errorf("Discover cross-project error = %v, want ErrInstanceNotFound", err)
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

	if _, err := env.svc.Discover(context.Background(), env.projectID, env.instanceID); !errors.Is(err, ErrUnsupportedCapability) {
		t.Errorf("Discover error = %v, want ErrUnsupportedCapability", err)
	}
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

// TestStopIsIdempotentAndRaceSafe verifies concurrent session stops are
// single-flight at the adapter, the target ends stopped, its siblings end
// disconnected, and a second stop is a no-op.
func TestStopIsIdempotentAndRaceSafe(t *testing.T) {
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
	if got := env.fake.stopCallCount(); got != 1 {
		t.Errorf("adapter StopInstance calls = %d, want 1", got)
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
	if gotSibling.Status != SessionDisconnected {
		t.Errorf("sibling status = %q, want %q", gotSibling.Status, SessionDisconnected)
	}

	// A second stop is idempotent and does not call the adapter again.
	again, err := env.svc.Stop(context.Background(), env.projectID, target.ID)
	if err != nil {
		t.Fatalf("second Stop: %v", err)
	}
	if again.Status != SessionStopped {
		t.Errorf("second stop status = %q, want %q", again.Status, SessionStopped)
	}
	if got := env.fake.stopCallCount(); got != 1 {
		t.Errorf("adapter StopInstance calls after second stop = %d, want 1", got)
	}
}

// TestStopFailureIsReportedAsBackendFailure verifies an adapter stop failure is
// labelled as a backend operation failure (502) and the session is left
// disconnected rather than reported as stopped.
func TestStopFailureIsReportedAsBackendFailure(t *testing.T) {
	env := newTestEnv(t)
	session := createNativeSession(t, env, "native-1")

	env.fake.mu.Lock()
	env.fake.stopErr = errors.New("adapter boom")
	env.fake.mu.Unlock()

	got, err := env.svc.Stop(context.Background(), env.projectID, session.ID)
	if !errors.Is(err, ErrBackendFailed) {
		t.Fatalf("Stop error = %v, want ErrBackendFailed", err)
	}
	if got.Status != SessionDisconnected {
		t.Errorf("session status after failed stop = %q, want %q", got.Status, SessionDisconnected)
	}
	inst, getErr := env.instances.Get(env.instanceID)
	if getErr != nil {
		t.Fatalf("Get instance: %v", getErr)
	}
	if inst.Status != agents.InstanceFailed {
		t.Errorf("instance status = %q, want %q", inst.Status, agents.InstanceFailed)
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

	err := env.svc.StreamSession(context.Background(), env.projectID, session.ID, &recordingWriter{}, nil)
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

	err = env.svc.StreamSession(context.Background(), env.projectID, row.ID, &recordingWriter{}, nil)
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
	var created Session
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created session: %v", err)
	}
	if created.NativeSessionID != "native-http" {
		t.Errorf("created session = %+v, want native native-http", created)
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
	req = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/projects/%d/sessions/%s/messages", env.projectID, created.ID), strings.NewReader(string(empty)))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("POST empty message = %d, want 400", rec.Code)
	}

	// The SSE route sets text/event-stream for an active session.
	ctx, cancel := context.WithCancel(context.Background())
	rec = httptest.NewRecorder()
	sseReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/projects/%d/sessions/%s/events", env.projectID, created.ID), nil).WithContext(ctx)
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

	// An adapter stop failure maps to the documented 502 agent_operation_failed.
	env.fake.mu.Lock()
	env.fake.stopErr = errors.New("adapter boom")
	env.fake.mu.Unlock()
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/projects/%d/sessions/%s/stop", env.projectID, created.ID), nil)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Errorf("POST stop with adapter failure = %d, want 502", rec.Code)
	}
}

// TestHandleCreateMapsStartFailures verifies create maps a project-disabled
// backend to 409 backend_disabled and a failed auto-start to 502
// instance_start_failed, matching the instance routes' existing contract.
func TestHandleCreateMapsStartFailures(t *testing.T) {
	gin.SetMode(gin.TestMode)
	env := newTestEnv(t)
	router := gin.New()
	env.svc.RegisterRoutes(router)

	disabled := env.registerProject(t, "beta-disabled")
	if _, err := env.instances.SetBackendEnabled(disabled, agents.BackendPi, false); err != nil {
		t.Fatalf("disable backend: %v", err)
	}

	post := func(projectID uint) *httptest.ResponseRecorder {
		body, _ := json.Marshal(createSessionRequest{Backend: agents.BackendPi})
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/projects/%d/sessions", projectID), strings.NewReader(string(body)))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(rec, req)
		return rec
	}

	rec := post(disabled)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "backend_disabled") {
		t.Fatalf("POST disabled backend = %d %s, want 409 backend_disabled", rec.Code, rec.Body.String())
	}

	failing := env.registerProject(t, "beta-failing")
	env.fake.mu.Lock()
	env.fake.startErr = errors.New("adapter cannot start")
	env.fake.mu.Unlock()
	rec = post(failing)
	if rec.Code != http.StatusBadGateway || !strings.Contains(rec.Body.String(), "instance_start_failed") {
		t.Fatalf("POST start failure = %d %s, want 502 instance_start_failed", rec.Code, rec.Body.String())
	}
}

// TestListModelsReturnsAdapterModels verifies the model list surfaces the
// adapter's result unchanged.
func TestListModelsReturnsAdapterModels(t *testing.T) {
	env := newTestEnv(t)
	env.fake.mu.Lock()
	env.fake.models = []agents.ModelInfo{
		{ID: "pi/gpt-5", Name: "gpt-5", Provider: "pi"},
		{ID: "pi/claude", Name: "claude", Provider: "pi", Variants: []string{"fast"}},
	}
	env.fake.mu.Unlock()

	models, err := env.svc.ListModels(context.Background(), env.projectID, env.instanceID)
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(models) != 2 || models[0].ID != "pi/gpt-5" || len(models[1].Variants) != 1 {
		t.Errorf("ListModels = %+v, want the adapter's two models", models)
	}
}

// TestListModelsRejectsUnsupportedCapability verifies a backend without model
// selection is refused rather than asked for models.
func TestListModelsRejectsUnsupportedCapability(t *testing.T) {
	env := newTestEnv(t)
	env.fake.mu.Lock()
	env.fake.caps = agents.Capabilities{Streaming: true}
	env.fake.mu.Unlock()

	if _, err := env.svc.ListModels(context.Background(), env.projectID, env.instanceID); !errors.Is(err, ErrUnsupportedCapability) {
		t.Fatalf("ListModels error = %v, want ErrUnsupportedCapability", err)
	}
}

// TestListModelsRejectsCrossProjectInstance verifies the instance must belong
// to the authorized project.
func TestListModelsRejectsCrossProjectInstance(t *testing.T) {
	env := newTestEnv(t)
	otherProject := env.registerProject(t, "beta")

	if _, err := env.svc.ListModels(context.Background(), otherProject, env.instanceID); !errors.Is(err, instances.ErrInstanceNotFound) {
		t.Fatalf("ListModels cross-project error = %v, want ErrInstanceNotFound", err)
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
	env.fake.emit(t, env.instanceID, lifecycleFor(env.instanceID, session.NativeSessionID, "execution_started"))
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
	env.fake.emit(t, env.instanceID, lifecycleFor(env.instanceID, session.NativeSessionID, "agent_start"))
	waitForTurn(t, env.svc, env.projectID, session.ID, TurnRunning)
	env.fake.mu.Lock()
	env.fake.interruptErr = errors.New("adapter boom")
	env.fake.mu.Unlock()

	if _, err := env.svc.Interrupt(context.Background(), env.projectID, session.ID); !errors.Is(err, ErrBackendFailed) {
		t.Fatalf("Interrupt error = %v, want ErrBackendFailed", err)
	}
}

// TestHandleModelsAndInterrupt verifies the routes expose the model list and
// return the session DTO for an interrupt, mapping unsupported capability to
// 409.
func TestHandleModelsAndInterrupt(t *testing.T) {
	gin.SetMode(gin.TestMode)
	env := newTestEnv(t)
	router := gin.New()
	env.svc.RegisterRoutes(router)
	session := createNativeSession(t, env, "native-1")

	env.fake.mu.Lock()
	env.fake.models = []agents.ModelInfo{{ID: "pi/gpt", Name: "gpt", Provider: "pi"}}
	env.fake.mu.Unlock()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/projects/%d/instances/%s/models", env.projectID, env.instanceID), nil)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"id":"pi/gpt"`) {
		t.Fatalf("GET models = %d %s, want 200 with the model", rec.Code, rec.Body.String())
	}

	// Interrupting an idle session is idempotent and returns the session DTO.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/projects/%d/sessions/%s/interrupt", env.projectID, session.ID), nil)
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

	// A backend without model selection is refused with 409.
	env.fake.mu.Lock()
	env.fake.caps = agents.Capabilities{Streaming: true}
	env.fake.mu.Unlock()
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/projects/%d/instances/%s/models", env.projectID, env.instanceID), nil)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "unsupported_capability") {
		t.Fatalf("GET models unsupported = %d %s, want 409 unsupported_capability", rec.Code, rec.Body.String())
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

// TestContinueRebindMergesDuplicateRowOnNewInstance pins the merge: because the
// unique key includes the instance, discovery can index the same native session
// a second time under a new instance, and rebinding must not leave the user
// with two tasks for one conversation.
func TestContinueRebindMergesDuplicateRowOnNewInstance(t *testing.T) {
	env := newTestEnv(t)
	session := createNativeSession(t, env, "native-dup")
	stopEnvInstance(t, env)

	// A second index row for the same native session, already bound to the
	// replacement instance discovery would have created.
	replacement, err := env.instances.Start(context.Background(), env.projectID, agents.BackendPi)
	if err != nil {
		t.Fatalf("start replacement instance: %v", err)
	}
	now := time.Now().UTC()
	duplicate := database.AgentSession{
		ID:              "duplicate-row",
		ProjectID:       env.projectID,
		Backend:         string(agents.BackendPi),
		InstanceID:      replacement.ID,
		NativeSessionID: "native-dup",
		Title:           "native-dup",
		Status:          string(SessionActive),
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := env.db.Create(&duplicate).Error; err != nil {
		t.Fatalf("create duplicate row: %v", err)
	}

	continued, err := env.svc.Continue(context.Background(), env.projectID, session.ID)
	if err != nil {
		t.Fatalf("Continue: %v", err)
	}
	if continued.ID != session.ID {
		t.Errorf("continued session id = %q, want the session the user opened (%q)", continued.ID, session.ID)
	}
	if continued.InstanceID != replacement.ID {
		t.Errorf("InstanceID = %q, want the replacement instance %q", continued.InstanceID, replacement.ID)
	}
	if got := sessionCount(t, env.db); got != 1 {
		t.Errorf("session count = %d, want the duplicate merged away (1)", got)
	}
	var rows []database.AgentSession
	if err := env.db.Where("native_session_id = ?", "native-dup").Find(&rows).Error; err != nil {
		t.Fatalf("load rows: %v", err)
	}
	if len(rows) != 1 || rows[0].ID != session.ID {
		t.Errorf("native-session rows = %+v, want only the opened session %q", rows, session.ID)
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

// TestDiscoverProjectUsesRunnableInstance pins the project-level scan: the
// caller names a backend, and the platform picks the instance that serves it.
func TestDiscoverProjectUsesRunnableInstance(t *testing.T) {
	env := newTestEnv(t)
	env.fake.mu.Lock()
	env.fake.listRefs = []agents.SessionRef{
		{InstanceID: env.instanceID, Backend: agents.BackendPi, SessionID: "n-project", Title: "found"},
	}
	env.fake.mu.Unlock()
	before := env.fake.startCount()

	found, err := env.svc.DiscoverProject(context.Background(), env.projectID, agents.BackendPi)
	if err != nil {
		t.Fatalf("DiscoverProject: %v", err)
	}
	if len(found) != 1 || found[0].NativeSessionID != "n-project" {
		t.Fatalf("DiscoverProject = %+v, want the native session n-project", found)
	}
	if got := env.fake.startCount(); got != before {
		t.Errorf("StartInstance calls = %d, want %d (the running instance is reused)", got, before)
	}

	// Re-scanning keeps the platform id instead of indexing a duplicate.
	again, err := env.svc.DiscoverProject(context.Background(), env.projectID, agents.BackendPi)
	if err != nil {
		t.Fatalf("second DiscoverProject: %v", err)
	}
	if len(again) != 1 || again[0].ID != found[0].ID {
		t.Errorf("second DiscoverProject = %+v, want the same platform id %q", again, found[0].ID)
	}
	if got := sessionCount(t, env.db); got != 1 {
		t.Errorf("session count = %d, want 1", got)
	}
}

// TestDiscoverProjectStartsInstanceWhenNeeded pins that scanning a backend with
// no running instance brings one up instead of failing.
func TestDiscoverProjectStartsInstanceWhenNeeded(t *testing.T) {
	env := newTestEnv(t)
	stopEnvInstance(t, env)
	before := env.fake.startCount()
	env.fake.mu.Lock()
	env.fake.listRefs = []agents.SessionRef{
		{InstanceID: env.instanceID, Backend: agents.BackendPi, SessionID: "n-late", Title: "late"},
	}
	env.fake.listBindInstance = true
	env.fake.mu.Unlock()

	found, err := env.svc.DiscoverProject(context.Background(), env.projectID, agents.BackendPi)
	if err != nil {
		t.Fatalf("DiscoverProject: %v", err)
	}
	if got := env.fake.startCount(); got != before+1 {
		t.Errorf("StartInstance calls = %d, want %d (one auto-start)", got, before+1)
	}
	if len(found) != 1 {
		t.Fatalf("DiscoverProject = %+v, want the discovered session", found)
	}
}

// TestDiscoverProjectRejectsUnknownBackend pins the error mapping for a
// backend the platform does not know.
func TestDiscoverProjectRejectsUnknownBackend(t *testing.T) {
	env := newTestEnv(t)

	_, err := env.svc.DiscoverProject(context.Background(), env.projectID, agents.BackendID("nope"))
	if !errors.Is(err, instances.ErrUnknownBackend) {
		t.Errorf("DiscoverProject error = %v, want ErrUnknownBackend", err)
	}
}
