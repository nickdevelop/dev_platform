package agy

import (
	"context"
	"crypto/rand"
	"fmt"
	"os/exec"
	"sync"
	"time"

	"devplatform/internal/core/agents"
)

// Proof of contract implementations.
var (
	_ agents.AgentAdapter  = (*Adapter)(nil)
	_ agents.HistoryReader = (*Adapter)(nil)
)

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
	// Now overrides the clock for deterministic testing.
	Now func() time.Time
	// NewID overrides UUID generation for testing.
	NewID func() (string, error)
	// ListModelsOverride overrides `agy models` execution for testing.
	ListModelsOverride func(ctx context.Context, directory string) ([]agents.ModelInfo, error)
}

// Adapter implements agents.AgentAdapter for Google Antigravity CLI.
type Adapter struct {
	executable         string
	resolveExecutable  func() string
	permissionMode     func() string
	dataDir            string
	sessionDir         string
	env                []string
	idleTimeout        time.Duration
	now                func() time.Time
	newID              func() (string, error)
	newIDOverride      bool
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
		now:                options.Now,
		listModelsOverride: options.ListModelsOverride,
		instances:          make(map[string]*instance),
		sessions:           make(map[sessionKey]*sessionState),
	}
	if a.executable == "" {
		a.executable = "agy"
	}
	if a.now == nil {
		a.now = func() time.Time { return time.Now().UTC() }
	}
	if options.NewID != nil {
		a.newID = options.NewID
		a.newIDOverride = true
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

// Capabilities declares agy's supported features.
func (a *Adapter) Capabilities() agents.Capabilities {
	return agents.Capabilities{
		Streaming:        true,
		SessionDiscovery: false,
		SessionContinue:  true,
		Stop:             true,
		ModelSelection:   true,
		ToolCallEvents:   true,
		ToolConfirmation: false,
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

	var sessionID string
	if a.newIDOverride {
		id, err := a.newID()
		if err != nil {
			return nil, fmt.Errorf("agy: allocate session id: %w", err)
		}
		sessionID = id
	} else {
		child := newAgyChild(inst, "", options.Model, a.permMode(), a.idleTimeout, a.now)
		startCtx, cancel := context.WithTimeout(inst.ctx, 15*time.Second)
		err := child.start(startCtx, a.execPath(), a.env)
		cancel()
		if err == nil && child.sessionID != "" {
			sessionID = child.sessionID
			inst.setChild(sessionID, child)
		} else {
			id, err := a.newID()
			if err != nil {
				return nil, fmt.Errorf("agy: allocate session id: %w", err)
			}
			sessionID = id
		}
	}

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

// ContinueSession attaches to an existing session reference.
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

// SendMessage delivers one user turn to the agy child process.
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

	child := inst.getChild(ref.SessionID)
	if child == nil || child.closed {
		child = newAgyChild(inst, ref.SessionID, model, a.permMode(), a.idleTimeout, a.now)
		startCtx, cancel := context.WithTimeout(inst.ctx, 15*time.Second)
		err := child.start(startCtx, a.execPath(), a.env)
		cancel()
		if err != nil {
			return fmt.Errorf("agy: start child for session %q: %w", ref.SessionID, err)
		}
		inst.setChild(ref.SessionID, child)
	}

	return child.sendTurn(ctx, message.Text)
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

// Interrupt interrupts the running turn of a session.
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
	child := inst.getChild(ref.SessionID)
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
