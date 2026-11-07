package builtin

import (
	"context"
	"crypto/rand"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"devplatform/internal/core/agents"
	"devplatform/internal/providers/builtin/tools"
)

// defaultTurnSteps bounds how many tool-call steps one turn may take before
// the loop fails with a step-limit error.
const defaultTurnSteps = 30

// permissionKey builds the "always"-approval key of one tool invocation.
func permissionKey(action, resource string) string {
	return action + "|" + resource
}

// Options configures the embedded agent adapter. TranscriptStore and
// ModelResolver are required; everything else has a usable default.
type Options struct {
	// TranscriptStore persists the session transcripts in the platform
	// database. It is implemented in the app layer and bound at the
	// composition root.
	TranscriptStore TranscriptStore
	// ModelResolver maps model references onto OpenAI-compatible endpoints.
	ModelResolver ModelResolver
	// AgentGlobalPrompt returns the platform's global agent prompt
	// (dataDir/AGENTS.md). Nil or empty means no global prompt. The prompt
	// is re-read for every turn so operator edits apply at once.
	AgentGlobalPrompt func(ctx context.Context) (string, error)
	// PermissionMode reports the configured permission mode for every turn.
	// Nil means confirm.
	PermissionMode func() string
	// MaxTurnSteps overrides the per-turn tool-step budget.
	MaxTurnSteps int
	// HTTPClient overrides the completer's client; a nil value uses one
	// without an overall timeout (streaming turns can run for minutes).
	HTTPClient *http.Client
	// Now and NewID are test seams.
	Now   func() time.Time
	NewID func() (string, error)
}

// permissionPending is one approval the adapter is waiting on. The channel is
// buffered so a reply can never block the replying goroutine.
type permissionPending struct {
	request  agents.PermissionRequest
	decision chan string
}

// sessionState is the platform-side runtime state of one embedded session. The
// transcript itself lives in the store; persistent approvals survive only in
// memory, like every other backend.
type sessionState struct {
	title string
	model string
	// approvals holds "always" rules for the session.
	approvals map[string]bool
	// pending holds tool approvals the adapter is waiting on.
	pending map[string]*permissionPending
	// running publishes the in-flight turn, detectable turn state.
	running *turn
	// cancelled marks the running turn interrupted via Interrupt.
	cancelled context.CancelFunc
	// recordSeq counts appended transcript records (1-based).
	recordSeq int
}

// allPermissions returns pending permission payload snapshots.
func (s *sessionState) allPermissions() []agents.PermissionRequest {
	out := make([]agents.PermissionRequest, 0, len(s.pending))
	for _, entry := range s.pending {
		out = append(out, entry.request)
	}
	return out
}

// sessionKey scopes runtime session state to one instance.
type sessionKey struct {
	instanceID string
	sessionID  string
}

// Adapter implements agents.AgentAdapter for the embedded agent. Instances
// are lightweight in-process handles (no child process, no PTY); every turn
// runs in its own goroutine driving the model completion and the tool loop.
type Adapter struct {
	store       TranscriptStore
	resolver    ModelResolver
	prompt      func(ctx context.Context) (string, error)
	permMode    func() string
	maxSteps    int
	registry    *tools.Registry
	completions *completer
	now         func() time.Time
	newID       func() (string, error)

	mu        sync.Mutex
	instances map[string]*instance
	sessions  map[sessionKey]*sessionState
	pending   map[string]*permissionPending
}

// Register installs a default embedded agent. The composition root builds the
// adapter itself (it must wire the transcript store and the model resolver),
// so Register is deliberately absent.

// New builds the embedded agent adapter. It panics on a nil store or
// resolver: wiring halves only exists at the composition root, where a
// mistake must fail loudly and early.
func New(options Options) *Adapter {
	if options.TranscriptStore == nil {
		panic("builtin: TranscriptStore is required")
	}
	if options.ModelResolver == nil {
		panic("builtin: ModelResolver is required")
	}
	adapter := &Adapter{
		store:       options.TranscriptStore,
		resolver:    options.ModelResolver,
		prompt:      options.AgentGlobalPrompt,
		permMode:    options.PermissionMode,
		maxSteps:    options.MaxTurnSteps,
		registry:    tools.NewRegistry(),
		completions: &completer{client: options.HTTPClient},
		now:         options.Now,
		newID:       options.NewID,
		instances:   make(map[string]*instance),
		sessions:    make(map[sessionKey]*sessionState),
		pending:     make(map[string]*permissionPending),
	}
	if adapter.completions.client == nil {
		adapter.completions.client = &http.Client{}
	}
	if adapter.maxSteps <= 0 {
		adapter.maxSteps = defaultTurnSteps
	}
	if adapter.now == nil {
		adapter.now = func() time.Time { return time.Now().UTC() }
	}
	if adapter.newID == nil {
		adapter.newID = newUUID
	}
	if adapter.permMode == nil {
		adapter.permMode = func() string { return PermissionModeConfirm }
	}
	return adapter
}

// Capabilities declares the embedded agent's feature set: streaming, tool
// call events, on-platform tool confirmation, interruption, per-turn model
// choice and platform-persisted history. Session discovery is not supported
// (the platform's session index is the authoritative list).
func (a *Adapter) Capabilities() agents.Capabilities {
	return agents.Capabilities{
		Streaming:        true,
		SessionDiscovery: false,
		SessionContinue:  true,
		Stop:             true,
		ModelSelection:   true,
		ToolCallEvents:   true,
		ToolConfirmation: true,
		Interrupt:        true,
		SessionHistory:   true,
	}
}

// Probe reports the adapter usable. The embedded agent has no external
// executable to find; only the model configuration can make it unusable, and
// that is checked per turn (a missing model list is a configuration error,
// not an install error). Diagnostics reports it.
func (a *Adapter) Probe(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

// Diagnostics runs the backend pre-flight checks: the embedded agent always
// resolves its "binary" (there is none) and only the model catalog carries
// risk. A resolver that answers no models is a warning — the agent refuses
// turns until an OpenAI-compatible model service is configured.
func (a *Adapter) Diagnostics(ctx context.Context) agents.BackendDiagnostics {
	report := agents.BackendDiagnostics{
		Backend: agents.BackendBuiltin,
		Checks: []agents.Diagnostic{
			{
				Kind:    agents.DiagnosticExecutable,
				Status:  agents.DiagnosticOK,
				Message: "embedded agent runs in-process; no executable check applies",
			},
		},
		Capabilities: capabilitiesOf(a.Capabilities()),
	}
	models, err := a.resolver.ListModels(ctx)
	switch {
	case err != nil:
		report.Checks = append(report.Checks, agents.Diagnostic{
			Kind:    agents.DiagnosticAPI,
			Status:  agents.DiagnosticError,
			Message: "could not resolve the platform model catalog: " + agents.RedactSecrets(err.Error()),
		})
	case len(models) == 0:
		report.Checks = append(report.Checks, agents.Diagnostic{
			Kind:    agents.DiagnosticAPI,
			Status:  agents.DiagnosticWarning,
			Message: "no usable OpenAI-compatible model configured; add one in the platform model settings",
		})
	default:
		report.Checks = append(report.Checks, agents.Diagnostic{
			Kind:    agents.DiagnosticAPI,
			Status:  agents.DiagnosticOK,
			Message: fmt.Sprintf("%d enabled models available", len(models)),
		})
	}
	report.Status = agents.AggregateStatus(report.Checks)
	return report
}

func capabilitiesOf(capabilities agents.Capabilities) *agents.Capabilities {
	return &capabilities
}

// newUUID returns an RFC 4122 version 4 UUID as a lowercase string.
func newUUID() (string, error) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	buf[6] = (buf[6] & 0x0f) | 0x40
	buf[8] = (buf[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", buf[0:4], buf[4:6], buf[6:8], buf[8:10], buf[10:16]), nil
}

// StartInstance registers a lightweight running instance for the authorized
// directory. There is no process to launch: the embedded agent lives in the
// platform process, so a start is allocation only.
func (a *Adapter) StartInstance(ctx context.Context, request agents.StartInstanceRequest) (*agents.Instance, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if request.Directory == "" {
		return nil, fmt.Errorf("builtin: instance directory is required")
	}
	info, err := os.Stat(request.Directory)
	if err != nil {
		return nil, fmt.Errorf("builtin: instance directory %q: %w", request.Directory, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("builtin: instance directory %q is not a directory", request.Directory)
	}
	id, err := a.newID()
	if err != nil {
		return nil, fmt.Errorf("builtin: allocate instance id: %w", err)
	}
	now := a.now()
	instanceCtx, cancel := context.WithCancel(context.Background())
	inst := &instance{
		id:        id,
		directory: request.Directory,
		status:    agents.InstanceRunning,
		ctx:       instanceCtx,
		cancel:    cancel,
		now:       a.now,
		done:      make(chan struct{}),
		subs:      make(map[*subscriber]struct{}),
	}

	a.mu.Lock()
	a.instances[id] = inst
	a.mu.Unlock()

	return &agents.Instance{
		ID:        id,
		Backend:   agents.BackendBuiltin,
		Directory: request.Directory,
		Status:    agents.InstanceRunning,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

// StopInstance tears the instance down: running turns are cancelled so their
// turns end as cancelled, and the event stream closes. Stopping an unknown
// instance is idempotently successful, like every other backend.
func (a *Adapter) StopInstance(ctx context.Context, instanceID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	a.mu.Lock()
	inst, ok := a.instances[instanceID]
	if ok {
		delete(a.instances, instanceID)
		for key := range a.sessions {
			if key.instanceID == instanceID {
				delete(a.sessions, key)
			}
		}
	}
	a.mu.Unlock()
	if !ok {
		return nil
	}
	inst.stop()
	return nil
}

// Subscribe returns the instance's normalized event stream. The channel
// closes when the caller's context ends or the instance stops.
func (a *Adapter) Subscribe(ctx context.Context, instanceID string) (<-chan agents.NormalizedEvent, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	inst, err := a.getInstance(instanceID)
	if err != nil {
		return nil, err
	}
	sub := newSubscriber()
	if err := inst.addSubscriber(sub); err != nil {
		return nil, err
	}
	go sub.forward()
	go sub.watch(ctx, inst)
	return sub.out, nil
}

// CreateSession allocates an agent-native session id and records the owning
// transcript row. No model call is involved; the conversation grows from the
// first message on.
func (a *Adapter) CreateSession(ctx context.Context, instanceID string, options agents.CreateSessionOptions) (*agents.SessionRef, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	inst, err := a.getInstance(instanceID)
	if err != nil {
		return nil, err
	}
	a.mu.Lock()
	if a.instances[instanceID] != inst {
		a.mu.Unlock()
		return nil, fmt.Errorf("builtin: instance %q is not running", instanceID)
	}
	id, err := a.newID()
	if err != nil {
		a.mu.Unlock()
		return nil, fmt.Errorf("builtin: allocate session id: %w", err)
	}
	a.sessions[sessionKey{instanceID: instanceID, sessionID: id}] = &sessionState{
		title:     options.Title,
		model:     options.Model,
		approvals: make(map[string]bool),
		pending:   make(map[string]*permissionPending),
	}
	a.mu.Unlock()

	if err := a.store.CreateSession(ctx, TranscriptSession{
		SessionID: id,
		Directory: inst.directory,
		Title:     options.Title,
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		return nil, fmt.Errorf("builtin: record transcript session: %w", err)
	}
	return &agents.SessionRef{
		InstanceID: instanceID,
		Backend:    agents.BackendBuiltin,
		SessionID:  id,
		Title:      options.Title,
	}, nil
}

// ContinueSession re-attaches a platform reference to an existing transcript.
// Restarts lose the in-memory state, so the transcript session row is ensured
// and runtime metadata is rebuilt from the store and the platform's title.
func (a *Adapter) ContinueSession(ctx context.Context, ref agents.SessionRef) (*agents.SessionRef, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := ref.Validate(); err != nil {
		return nil, err
	}
	if ref.Backend != agents.BackendBuiltin {
		return nil, fmt.Errorf("builtin: session backend %q is not the builtin backend", ref.Backend)
	}
	inst, err := a.getInstance(ref.InstanceID)
	if err != nil {
		return nil, err
	}
	a.mu.Lock()
	state, ok := a.sessions[sessionKey{instanceID: ref.InstanceID, sessionID: ref.SessionID}]
	if !ok {
		state = &sessionState{
			title:     ref.Title,
			approvals: make(map[string]bool),
			pending:   make(map[string]*permissionPending),
		}
		a.sessions[sessionKey{instanceID: ref.InstanceID, sessionID: ref.SessionID}] = state
	}
	if ref.Title != "" && state.title == "" {
		state.title = ref.Title
	}
	a.mu.Unlock()

	// Re-record the owning row after a restart of either side; an existing
	// row wins, a missing one is created so history replay never 404s.
	if err := a.store.CreateSession(ctx, TranscriptSession{
		SessionID: ref.SessionID,
		Directory: inst.directory,
		Title:     ref.Title,
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		return nil, fmt.Errorf("builtin: ensure transcript session: %w", err)
	}
	return &ref, nil
}

// SendMessage delivers one user turn to a session and returns once the turn
// is accepted; the agent loop runs in the background and reports through the
// event stream. A send while the session is already running a turn is
// rejected, mirroring the workbench's one turn at a time contract.
func (a *Adapter) SendMessage(ctx context.Context, ref agents.SessionRef, message agents.SendMessageRequest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := ref.Validate(); err != nil {
		return err
	}
	if ref.Backend != agents.BackendBuiltin {
		return fmt.Errorf("builtin: session backend %q is not the builtin backend", ref.Backend)
	}
	if strings.TrimSpace(message.Text) == "" && len(message.Images) == 0 {
		return fmt.Errorf("builtin: message text is required")
	}
	inst, err := a.getInstance(ref.InstanceID)
	if err != nil {
		return err
	}
	a.mu.Lock()
	state, ok := a.sessions[sessionKey{instanceID: ref.InstanceID, sessionID: ref.SessionID}]
	if !ok {
		state = &sessionState{
			title:     ref.Title,
			model:     message.Model,
			approvals: make(map[string]bool),
			pending:   make(map[string]*permissionPending),
		}
		a.sessions[sessionKey{instanceID: ref.InstanceID, sessionID: ref.SessionID}] = state
	}
	if state.running != nil {
		a.mu.Unlock()
		return fmt.Errorf("builtin: session %q already has a running turn", ref.SessionID)
	}
	if message.Model != "" {
		state.model = message.Model
	}
	if ref.Title != "" {
		state.title = ref.Title
	}
	turnCtx, cancel := context.WithCancel(inst.ctx)
	current := &turn{
		id:     inst.nextTurn(),
		ctx:    turnCtx,
		cancel: cancel,
	}
	state.running = current
	state.cancelled = current.cancel
	a.mu.Unlock()

	go a.runTurn(inst, ref, state, current, message)
	return nil
}

// Interrupt cancels the running turn of one session. The instance and its
// other sessions keep running.
func (a *Adapter) Interrupt(ctx context.Context, ref agents.SessionRef) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := ref.Validate(); err != nil {
		return err
	}
	if _, err := a.getInstance(ref.InstanceID); err != nil {
		return err
	}
	a.mu.Lock()
	state, ok := a.sessions[sessionKey{instanceID: ref.InstanceID, sessionID: ref.SessionID}]
	if ok && state.cancelled != nil {
		state.cancelled()
	}
	a.mu.Unlock()
	return nil
}

// ListSessions is unsupported: the platform's session index is already the
// authoritative list and the capability stays undeclared.
func (a *Adapter) ListSessions(ctx context.Context, instanceID string) ([]agents.SessionRef, error) {
	return nil, fmt.Errorf("builtin: session discovery is not supported")
}

// ListModels returns the platform's shared OpenAI-compatible model catalog.
// The catalog does not depend on the working directory, so it is ignored.
func (a *Adapter) ListModels(ctx context.Context, _ string) ([]agents.ModelInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return a.resolver.ListModels(ctx)
}

// DetectTurn reports whether the session is currently running a turn.
func (a *Adapter) DetectTurn(ctx context.Context, ref agents.SessionRef) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if err := ref.Validate(); err != nil {
		return false, err
	}
	if a.getSession(ref.InstanceID, ref.SessionID) == nil {
		return false, nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.sessions[sessionKey{instanceID: ref.InstanceID, sessionID: ref.SessionID}].running != nil, nil
}

// SessionBusy is the agentdata ActivityGate: whether the named transcript
// session is running a turn anywhere on this runtime. Deletions refuse busy
// sessions so a live turn never writes into rows being removed.
func (a *Adapter) SessionBusy(sessionID string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.busyLocked(sessionID)
}

// session returns a runtime session state or nil.
func (a *Adapter) getSession(instanceID, sessionID string) *sessionState {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.sessions[sessionKey{instanceID: instanceID, sessionID: sessionID}]
}

// busyLocked checks every session state for a running turn with the given
// native id. The adapter lock must be held.
func (a *Adapter) busyLocked(sessionID string) bool {
	for key, state := range a.sessions {
		if key.sessionID == sessionID && state.running != nil {
			return true
		}
	}
	return false
}

// getInstance returns a registered instance or an error.
func (a *Adapter) getInstance(instanceID string) (*instance, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	inst, ok := a.instances[instanceID]
	if !ok {
		return nil, fmt.Errorf("builtin: unknown instance %q", instanceID)
	}
	return inst, nil
}

// registerApproved records an "always" approval rule for one session.
func (a *Adapter) rememberApproval(instanceID, sessionID, key string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	state, ok := a.sessions[sessionKey{instanceID, sessionID}]
	if ok && state.approvals != nil {
		state.approvals[key] = true
	}
}

// sessionDirectory returns the working directory of a runtime session the
// adapter remembers; an empty result means the runtime no longer knows it and
// the platform's own authorization stands.
func (a *Adapter) sessionDirectory(sessionID string) (string, error) {
	instances, err := a.runtimeInstances()
	if err != nil {
		return "", err
	}
	for instanceID := range instances {
		state := a.getSession(instanceID, sessionID)
		if state == nil {
			continue
		}
		inst, err := a.getInstance(instanceID)
		if err != nil {
			continue
		}
		return inst.directory, nil
	}
	return "", nil
}

// runtimeInstances returns the instance ids under the adapter lock.
func (a *Adapter) runtimeInstances() (map[string]struct{}, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make(map[string]struct{}, len(a.instances))
	for id := range a.instances {
		out[id] = struct{}{}
	}
	return out, nil
}

// sessionApproved reports an "always" approval rule for one session.
func (a *Adapter) sessionApproved(instanceID, sessionID, key string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	state, ok := a.sessions[sessionKey{instanceID, sessionID}]
	if !ok || state.approvals == nil {
		return false
	}
	return state.approvals[key]
}
