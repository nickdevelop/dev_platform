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

	continueRef *agents.SessionRef
	continueErr error

	sendErr  error
	sends    []agents.SendMessageRequest
	sentRefs []agents.SessionRef

	stopErr   error
	stopCalls int

	next      int
	subBuffer int
	subs      map[string]chan agents.NormalizedEvent
}

func newFakeAdapter() *fakeAdapter {
	return &fakeAdapter{
		caps: agents.Capabilities{
			Streaming:        true,
			SessionDiscovery: true,
			SessionContinue:  true,
			Stop:             true,
			ModelSelection:   true,
			ToolCallEvents:   true,
			ToolConfirmation: false,
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

func (f *fakeAdapter) StartInstance(_ context.Context, req agents.StartInstanceRequest) (*agents.Instance, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
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

func (f *fakeAdapter) ListSessions(context.Context, string) ([]agents.SessionRef, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.listCalls++
	return append([]agents.SessionRef(nil), f.listRefs...), f.listErr
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

	session, err := env.svc.Create(context.Background(), env.projectID, env.instanceID, nativeID, "")
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

// TestCreateRejectsWhenInstanceBelongsToAnotherProject verifies a client may
// only create sessions within its authorized project.
func TestCreateRejectsWhenInstanceBelongsToAnotherProject(t *testing.T) {
	env := newTestEnv(t)
	otherProject := env.registerProject(t, "beta")

	env.fake.mu.Lock()
	env.fake.createRef = &agents.SessionRef{InstanceID: env.instanceID, Backend: agents.BackendPi, SessionID: "n"}
	env.fake.mu.Unlock()

	_, err := env.svc.Create(context.Background(), otherProject, env.instanceID, "", "")
	if !errors.Is(err, instances.ErrInstanceNotFound) {
		t.Fatalf("Create cross-project error = %v, want ErrInstanceNotFound", err)
	}
	if got := sessionCount(t, env.db); got != 0 {
		t.Errorf("session count = %d, want 0 after a rejected create", got)
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
	if _, err := env.svc.Continue(context.Background(), env.projectID, session.ID); !errors.Is(err, instances.ErrInstanceNotRunning) {
		t.Errorf("Continue on stale session error = %v, want ErrInstanceNotRunning", err)
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

	if _, err := env.svc.Create(context.Background(), env.projectID, env.instanceID, "", "model-x"); !errors.Is(err, ErrUnsupportedCapability) {
		t.Errorf("Create with model error = %v, want ErrUnsupportedCapability", err)
	}
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
	body, _ := json.Marshal(createSessionRequest{InstanceID: env.instanceID, Title: "t"})
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
	waitForSubscriber(t, env.fake, env.instanceID)
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
