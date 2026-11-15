package agy

import (
	"context"
	"crypto/rand"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"devplatform/internal/core/agents"
)

// Proof of contract implementations.
var (
	_ agents.AgentAdapter      = (*Adapter)(nil)
	_ agents.HistoryReader     = (*Adapter)(nil)
	_ agents.PermissionReplier = (*Adapter)(nil)
)

const (
	defaultStartTimeout = 60 * time.Second
	// pendingSessionPrefix marks a session whose conversation agy has not
	// started yet; its conversation id is bound on the first turn.
	pendingSessionPrefix = "pending-"
)

func isPendingSession(sessionID string) bool {
	return strings.HasPrefix(sessionID, pendingSessionPrefix)
}

// Options configures the agy adapter.
type Options struct {
	// Executable is the binary name or path. Defaults to "agy".
	Executable string
	// ResolveExecutable resolves the current binary path from settings.
	ResolveExecutable func() string
	// PermissionMode provides the current tool permission mode ("confirm" or "auto").
	PermissionMode func() string
	// DataDir is the platform data directory where AGENTS.md lives.
	DataDir string
	// SessionDir overrides the brain directory where transcript.jsonl lives.
	SessionDir string
	// Env overrides the child environment.
	Env []string
	// IdleTimeout configures how long an idle agy child stays running.
	IdleTimeout time.Duration
	// StartTimeout bounds the wait for agy's init when a child starts.
	// Defaults to defaultStartTimeout.
	StartTimeout time.Duration
	// Now overrides the clock for deterministic testing.
	Now func() time.Time
	// NewID overrides UUID generation for testing.
	NewID func() (string, error)
	// ListModelsOverride overrides `agy models` execution for testing.
	ListModelsOverride func(ctx context.Context, directory string) ([]agents.ModelInfo, error)
}

// Adapter implements agents.AgentAdapter for Google Antigravity CLI via ACP.
type Adapter struct {
	executable         string
	resolveExecutable  func() string
	permissionMode     func() string
	dataDir            string
	sessionDir         string
	env                []string
	idleTimeout        time.Duration
	startTimeout       time.Duration
	now                func() time.Time
	newID              func() (string, error)
	listModelsOverride func(ctx context.Context, directory string) ([]agents.ModelInfo, error)

	mu        sync.Mutex
	instances map[string]*instance
	sessions  map[sessionKey]*sessionState
}

type sessionKey struct {
	instanceID string
	sessionID  string
}

type sessionState struct {
	title string
	model string
}

// New creates an agy adapter instance.
func New(options Options) *Adapter {
	a := &Adapter{
		executable:         options.Executable,
		resolveExecutable:  options.ResolveExecutable,
		permissionMode:     options.PermissionMode,
		dataDir:            options.DataDir,
		sessionDir:         options.SessionDir,
		env:                options.Env,
		idleTimeout:        options.IdleTimeout,
		startTimeout:       options.StartTimeout,
		now:                options.Now,
		listModelsOverride: options.ListModelsOverride,
		instances:          make(map[string]*instance),
		sessions:           make(map[sessionKey]*sessionState),
	}
	if a.executable == "" {
		a.executable = "agy"
	}
	if a.idleTimeout <= 0 {
		a.idleTimeout = 10 * time.Minute
	}
	if a.startTimeout <= 0 {
		a.startTimeout = defaultStartTimeout
	}
	if a.now == nil {
		a.now = func() time.Time { return time.Now().UTC() }
	}
	if options.NewID != nil {
		a.newID = options.NewID
	} else {
		a.newID = newUUID
	}
	return a
}

func (a *Adapter) execPath() string {
	if a.resolveExecutable != nil {
		if path := a.resolveExecutable(); path != "" {
			return path
		}
	}
	return resolveExecutable(a.executable)
}

func (a *Adapter) permMode() string {
	if a.permissionMode != nil {
		if m := a.permissionMode(); m != "" {
			return m
		}
	}
	return PermissionModeAuto
}

// Capabilities declares agy's supported features under ACP.
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

// Probe checks whether agy is installed and executable.
func (a *Adapter) Probe(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	path := a.execPath()
	if _, err := exec.LookPath(path); err != nil {
		return fmt.Errorf("agy: binary %q not found: %w", path, err)
	}
	return nil
}

// StartInstance creates an instance bound to a directory.
func (a *Adapter) StartInstance(ctx context.Context, request agents.StartInstanceRequest) (*agents.Instance, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if request.Directory == "" {
		return nil, fmt.Errorf("%w: directory is empty", agents.ErrInvalidInstance)
	}

	_ = EnsureWorkspaceTrusted(request.Directory)

	id, err := a.newID()
	if err != nil {
		return nil, fmt.Errorf("agy: allocate instance id: %w", err)
	}

	inst := newInstance(id, request.Directory, a.now)
	a.mu.Lock()
	a.instances[id] = inst
	a.mu.Unlock()

	now := a.now()
	return &agents.Instance{
		ID:        id,
		Backend:   agents.BackendAgy,
		Directory: request.Directory,
		Status:    agents.InstanceRunning,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

// StopInstance stops an instance and terminates its running child processes.
func (a *Adapter) StopInstance(ctx context.Context, instanceID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	a.mu.Lock()
	inst, ok := a.instances[instanceID]
	if ok {
		delete(a.instances, instanceID)
	}
	a.mu.Unlock()

	if !ok {
		return nil
	}
	inst.stop()
	return nil
}

// ListSessions returns an empty list as agy discovery is not scoped by directory.
func (a *Adapter) ListSessions(ctx context.Context, instanceID string) ([]agents.SessionRef, error) {
	return []agents.SessionRef{}, nil
}

// CreateSession allocates a new session reference for the instance.
func (a *Adapter) CreateSession(ctx context.Context, instanceID string, options agents.CreateSessionOptions) (*agents.SessionRef, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	inst, err := a.getInstance(instanceID)
	if err != nil {
		return nil, err
	}
	if err := inst.ensureRunning(); err != nil {
		return nil, err
	}

	_ = EnsureWorkspaceTrusted(inst.directory)

	id, err := a.newID()
	if err != nil {
		return nil, fmt.Errorf("agy: allocate session id: %w", err)
	}
	sessionID := pendingSessionPrefix + id

	a.mu.Lock()
	a.sessions[sessionKey{instanceID: instanceID, sessionID: sessionID}] = &sessionState{
		title: options.Title,
		model: options.Model,
	}
	a.mu.Unlock()

	return &agents.SessionRef{
		InstanceID: instanceID,
		Backend:    agents.BackendAgy,
		SessionID:  sessionID,
		Title:      options.Title,
	}, nil
}

// ContinueSession resumes an existing agy session.
func (a *Adapter) ContinueSession(ctx context.Context, ref agents.SessionRef) (*agents.SessionRef, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := ref.Validate(); err != nil {
		return nil, err
	}
	inst, err := a.getInstance(ref.InstanceID)
	if err != nil {
		return nil, err
	}
	if err := inst.ensureRunning(); err != nil {
		return nil, err
	}

	_ = EnsureWorkspaceTrusted(inst.directory)

	a.mu.Lock()
	key := sessionKey{instanceID: ref.InstanceID, sessionID: ref.SessionID}
	state, ok := a.sessions[key]
	if !ok {
		state = &sessionState{title: ref.Title}
		a.sessions[key] = state
	}
	a.mu.Unlock()

	return &agents.SessionRef{
		InstanceID: ref.InstanceID,
		Backend:    agents.BackendAgy,
		SessionID:  ref.SessionID,
		Title:      state.title,
	}, nil
}

// ResumeSession adopts an external session reference into the adapter's state.
func (a *Adapter) ResumeSession(ctx context.Context, ref agents.SessionRef) (*agents.SessionRef, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := ref.Validate(); err != nil {
		return nil, err
	}
	inst, err := a.getInstance(ref.InstanceID)
	if err != nil {
		return nil, err
	}
	if err := inst.ensureRunning(); err != nil {
		return nil, err
	}

	_ = EnsureWorkspaceTrusted(inst.directory)

	a.mu.Lock()
	key := sessionKey{instanceID: ref.InstanceID, sessionID: ref.SessionID}
	state, ok := a.sessions[key]
	if !ok {
		state = &sessionState{title: ref.Title}
		a.sessions[key] = state
	}
	a.mu.Unlock()

	return &agents.SessionRef{
		InstanceID: ref.InstanceID,
		Backend:    agents.BackendAgy,
		SessionID:  ref.SessionID,
		Title:      state.title,
	}, nil
}

// SendMessage delivers one user turn to the agy child process via ACP.
func (a *Adapter) SendMessage(ctx context.Context, ref agents.SessionRef, message agents.SendMessageRequest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := ref.Validate(); err != nil {
		return err
	}
	inst, err := a.getInstance(ref.InstanceID)
	if err != nil {
		return err
	}
	if err := inst.ensureRunning(); err != nil {
		return err
	}

	_ = EnsureWorkspaceTrusted(inst.directory)

	a.mu.Lock()
	key := sessionKey{instanceID: ref.InstanceID, sessionID: ref.SessionID}
	state := a.sessions[key]
	if state == nil {
		state = &sessionState{}
		a.sessions[key] = state
	}
	model := message.Model
	if model == "" {
		model = state.model
	}
	a.mu.Unlock()

	child := inst.getChild(inst.resolve(ref.SessionID))
	if child == nil || child.closed {
		var err error
		child, err = a.openChild(inst, ref.SessionID, model, message.OnNativeSessionBound)
		if err != nil {
			return err
		}
	}

	return child.sendTurn(ctx, message.Text, message.Model)
}

// openChild starts the child process serving a session.
func (a *Adapter) openChild(inst *instance, sessionID, model string, bind func(nativeSessionID string) error) (*agyChild, error) {
	inst.bindMu.Lock()
	defer inst.bindMu.Unlock()

	key := inst.resolve(sessionID)
	if child := inst.getChild(key); child != nil && !child.closed {
		return child, nil
	}

	pending := isPendingSession(key)
	resumeID := key
	if pending {
		resumeID = ""
	}
	child := newAgyChild(inst, resumeID, model, a.permMode(), a.idleTimeout, a.now)
	startCtx, cancel := context.WithTimeout(inst.ctx, a.startTimeout)
	err := child.start(startCtx, a.execPath(), a.env, bind)
	cancel()
	if err != nil {
		return nil, fmt.Errorf("agy: start child for session %q: %w", sessionID, err)
	}

	if !pending {
		inst.setChild(key, child)
		return child, nil
	}

	native := child.sessionID
	if native == "" {
		_ = child.close()
		return nil, fmt.Errorf("agy: start child for session %q: acp reported no session id", sessionID)
	}

	a.mu.Lock()
	pendingKey := sessionKey{instanceID: inst.id, sessionID: key}
	if state, ok := a.sessions[pendingKey]; ok {
		a.sessions[sessionKey{instanceID: inst.id, sessionID: native}] = state
		delete(a.sessions, pendingKey)
	}
	a.mu.Unlock()

	inst.setAlias(key, native)
	inst.setChild(native, child)
	return child, nil
}

// ListPermissions returns pending tool approvals for the session.
func (a *Adapter) ListPermissions(ctx context.Context, ref agents.SessionRef) ([]agents.PermissionRequest, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := ref.Validate(); err != nil {
		return nil, err
	}
	inst, err := a.getInstance(ref.InstanceID)
	if err != nil {
		return nil, err
	}
	child := inst.getChild(inst.resolve(ref.SessionID))
	if child == nil {
		return []agents.PermissionRequest{}, nil
	}
	return child.listPermissions(), nil
}

// ReplyPermission answers a pending confirmation for the session.
func (a *Adapter) ReplyPermission(ctx context.Context, ref agents.SessionRef, requestID, decision, message string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := ref.Validate(); err != nil {
		return err
	}
	inst, err := a.getInstance(ref.InstanceID)
	if err != nil {
		return err
	}
	child := inst.getChild(inst.resolve(ref.SessionID))
	if child == nil {
		return fmt.Errorf("%w: agy: no live session %q", agents.ErrReplyExpired, ref.SessionID)
	}
	return child.replyPermission(requestID, decision)
}

// ListForms returns pending question forms (none for agy).
func (a *Adapter) ListForms(ctx context.Context, ref agents.SessionRef) ([]agents.FormInfo, error) {
	return []agents.FormInfo{}, nil
}

// ReplyForm answers a pending question form (none for agy).
func (a *Adapter) ReplyForm(ctx context.Context, ref agents.SessionRef, formID string, answer map[string]any) error {
	return nil
}

// Subscribe returns the normalized event channel for the instance.
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

// Interrupt interrupts the running turn of a session via ACP cancel.
func (a *Adapter) Interrupt(ctx context.Context, ref agents.SessionRef) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := ref.Validate(); err != nil {
		return err
	}
	inst, err := a.getInstance(ref.InstanceID)
	if err != nil {
		return err
	}
	child := inst.getChild(inst.resolve(ref.SessionID))
	if child == nil {
		return nil
	}
	return child.interrupt(ctx)
}

func (a *Adapter) getInstance(id string) (*instance, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	inst, ok := a.instances[id]
	if !ok {
		return nil, fmt.Errorf("agy: unknown instance %q", id)
	}
	return inst, nil
}

func newUUID() (string, error) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	buf[6] = (buf[6] & 0x0f) | 0x40
	buf[8] = (buf[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		buf[0:4], buf[4:6], buf[6:8], buf[8:10], buf[10:16]), nil
}
