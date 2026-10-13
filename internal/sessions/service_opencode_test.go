package sessions

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"devplatform/internal/agents"
	"devplatform/internal/agents/instances"
	"devplatform/internal/agents/opencode"
	"devplatform/internal/projects"
)

// opencode permission extension on the shared fakeAdapter. These methods make
// the fake assertable as the opencode PermissionReplier shape without
// importing the opencode package.
func (f *fakeAdapter) ListPermissions(_ context.Context, _ agents.SessionRef) ([]PermissionRequest, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.listErr != nil {
		return nil, f.listErr
	}
	return []PermissionRequest{{ID: "per_1", SessionID: "ses-native-1", Action: "read", Resources: []string{"/tmp"}}}, nil
}

func (f *fakeAdapter) ReplyPermission(_ context.Context, _ agents.SessionRef, requestID, decision, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if requestID == "" {
		return errors.New("fake: request id required")
	}
	if f.replyErr != nil {
		return f.replyErr
	}
	switch decision {
	case "once", "always", "reject":
		return f.sendErr
	default:
		return errors.New("fake: bad decision")
	}
}

func (f *fakeAdapter) ListForms(_ context.Context, _ agents.SessionRef) ([]FormInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.listErr != nil {
		return nil, f.listErr
	}
	return []FormInfo{{ID: "frm_1", SessionID: "ses-native-1", Title: "confirm"}}, nil
}

func (f *fakeAdapter) ReplyForm(_ context.Context, _ agents.SessionRef, formID string, answer map[string]any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if formID == "" || len(answer) == 0 {
		return errors.New("fake: form id and answer required")
	}
	if f.replyErr != nil {
		return f.replyErr
	}
	return f.sendErr
}

func (f *fakeAdapter) OpenRawEvents(ctx context.Context) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("data: {\"type\":\"server.connected\"}\n\n")), nil
}

func newOpencodeTestEnv(t *testing.T) *testEnv {
	t.Helper()
	db := openTestDB(t)
	root := t.TempDir()
	projectDir := root + "/alpha"
	mustMkdir(t, projectDir)
	projectService := projects.NewService(db)
	project, err := projectService.Register(projectDir)
	if err != nil {
		t.Fatalf("register project: %v", err)
	}
	registry := agents.NewRegistry()
	fake := newFakeAdapter()
	fake.caps.ToolConfirmation = true
	if err := registry.Register(agents.BackendOpenCodeV2, fake); err != nil {
		t.Fatalf("register fake opencode adapter: %v", err)
	}
	instanceService := instances.NewService(db, registry, projectService)
	instance, err := instanceService.Start(context.Background(), project.ID, agents.BackendOpenCodeV2)
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

func mustMkdir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
}

func TestOpencodePermissionReplyHappyPath(t *testing.T) {
	env := newOpencodeTestEnv(t)
	ctx := context.Background()
	env.fake.mu.Lock()
	env.fake.createRef = &agents.SessionRef{Backend: agents.BackendOpenCodeV2, SessionID: "ses-native-1", Title: "t"}
	env.fake.mu.Unlock()
	session, err := env.svc.Create(ctx, env.projectID, agents.BackendOpenCodeV2, "t")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	list, err := env.svc.ListOpencodePermissions(ctx, env.projectID, session.ID)
	if err != nil {
		t.Fatalf("ListPermissions: %v", err)
	}
	if len(list) != 1 || list[0].ID != "per_1" {
		t.Fatalf("permissions = %+v, want one per_1", list)
	}
	if err := env.svc.ReplyOpencodePermission(ctx, env.projectID, session.ID, "per_1", "once", ""); err != nil {
		t.Fatalf("ReplyPermission: %v", err)
	}
	if err := env.svc.ReplyOpencodePermission(ctx, env.projectID, session.ID, "per_1", "sometimes", ""); err == nil {
		t.Fatal("ReplyPermission(bad decision) = nil, want error")
	}

	forms, err := env.svc.ListOpencodeForms(ctx, env.projectID, session.ID)
	if err != nil {
		t.Fatalf("ListForms: %v", err)
	}
	if len(forms) != 1 || forms[0].ID != "frm_1" {
		t.Fatalf("forms = %+v, want one frm_1", forms)
	}
	if err := env.svc.ReplyOpencodeForm(ctx, env.projectID, session.ID, "frm_1", map[string]any{"confirm": true}); err != nil {
		t.Fatalf("ReplyForm: %v", err)
	}
}

func TestOpencodeReplyExpiredMapsTo409(t *testing.T) {
	env := newOpencodeTestEnv(t)
	ctx := context.Background()
	env.fake.mu.Lock()
	env.fake.createRef = &agents.SessionRef{Backend: agents.BackendOpenCodeV2, SessionID: "ses-native-1", Title: "t"}
	env.fake.replyErr = fmt.Errorf("%w: gone", agents.ErrReplyExpired)
	env.fake.mu.Unlock()
	session, err := env.svc.Create(ctx, env.projectID, agents.BackendOpenCodeV2, "t")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	// A reply for a request the server no longer holds is an expired reply,
	// never a generic backend failure (which the API maps to 502).
	err = env.svc.ReplyOpencodePermission(ctx, env.projectID, session.ID, "per_gone", "once", "")
	if !errors.Is(err, ErrPermissionExpired) {
		t.Fatalf("ReplyOpencodePermission = %v, want ErrPermissionExpired", err)
	}
	if errors.Is(err, ErrBackendFailed) {
		t.Fatalf("ReplyOpencodePermission = %v, must not be ErrBackendFailed", err)
	}

	env.fake.mu.Lock()
	env.fake.replyErr = fmt.Errorf("%w: settled", agents.ErrReplyExpired)
	env.fake.mu.Unlock()
	err = env.svc.ReplyOpencodeForm(ctx, env.projectID, session.ID, "frm_gone", map[string]any{"a": "b"})
	if !errors.Is(err, ErrFormExpired) {
		t.Fatalf("ReplyOpencodeForm = %v, want ErrFormExpired", err)
	}
}

// TestRealOpencodeAdapterSatisfiesService guards the production wiring the
// 502 came from: the service must assert the real adapter through the shared
// opencode interfaces, so a signature drift fails here instead of silently
// degrading to unsupported/502 at runtime.
func TestRealOpencodeAdapterSatisfiesService(t *testing.T) {
	var adapter agents.AgentAdapter = opencode.New(opencode.Options{})
	if _, ok := adapter.(opencode.PermissionReplier); !ok {
		t.Fatal("real opencode adapter does not implement opencode.PermissionReplier")
	}
	if _, ok := adapter.(opencode.RawEventOpener); !ok {
		t.Fatal("real opencode adapter does not implement opencode.RawEventOpener")
	}
}

func TestOpencodeRawStreamCopiesBytes(t *testing.T) {
	env := newOpencodeTestEnv(t)
	ctx := context.Background()
	env.fake.mu.Lock()
	env.fake.createRef = &agents.SessionRef{Backend: agents.BackendOpenCodeV2, SessionID: "ses-native-1", Title: "t"}
	env.fake.mu.Unlock()
	session, err := env.svc.Create(ctx, env.projectID, agents.BackendOpenCodeV2, "t")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	var out strings.Builder
	streamCtx, cancel := context.WithCancel(ctx)
	// Cancel after the first copy so the test terminates: the stub body is
	// finite, so copyRawSSE returns on EOF without needing the cancel.
	defer cancel()
	if err := env.svc.StreamOpencodeRaw(streamCtx, env.projectID, session.ID, &out, nil); err != nil {
		t.Fatalf("StreamOpencodeRaw: %v", err)
	}
	if !strings.Contains(out.String(), "server.connected") {
		t.Fatalf("raw output = %q, want server.connected", out.String())
	}
}
