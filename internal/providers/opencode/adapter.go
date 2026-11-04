package opencode

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"devplatform/internal/core/agents"
)

// Compile-time proof that the opencode adapter satisfies the platform contract.
var (
	_ agents.AgentAdapter        = (*Adapter)(nil)
	_ agents.SessionTurnDetector = (*Adapter)(nil)
)

const (
	// streamRetryMin and streamRetryMax bound the reconnect backoff after the
	// event stream drops. Each instance reconnects its own stream only.
	streamRetryMin = 100 * time.Millisecond
	streamRetryMax = 5 * time.Second
	// defaultStreamIdleTimeout is how long a silent-but-open event stream may
	// go without delivering an event before the adapter treats it as dead and
	// reconnects.
	defaultStreamIdleTimeout = 60 * time.Second
	// maxStartupOutputBytes bounds captured server output used for diagnostics.
	maxStartupOutputBytes = 8 * 1024
)

// Options configures the opencode adapter. The zero value is usable when the
// `opencode` binary is on PATH.
type Options struct {
	// Executable is the opencode binary name or path. Defaults to "opencode".
	Executable string
	// ResolveExecutable, when set, is consulted at each use to obtain the
	// current binary path (the platform-level agent setting).
	ResolveExecutable func() string
	// Hostname is the loopback host the per-instance server binds. Defaults to 127.0.0.1.
	Hostname string
	// StartupTimeout bounds how long StartInstance waits for the server to answer /api/info.
	StartupTimeout time.Duration
	// StreamIdleTimeout bounds how long the event stream may go silent before reconnecting.
	StreamIdleTimeout time.Duration
	// VersionProbe overrides how the CLI version is obtained.
	VersionProbe func(ctx context.Context) (string, error)
	// Now overrides the clock; tests set it for deterministic timestamps.
	Now func() time.Time
	// NewID overrides identifier generation; tests may make it deterministic.
	NewID func() (string, error)
	// ServiceFile overrides the desktop registration file path.
	ServiceFile string
	// ServiceDirectory is the working directory the service is started in.
	ServiceDirectory string
}

// Adapter implements agents.AgentAdapter for the opencode v2 HTTP API. All
// instances share one machine-wide `opencode serve --service` process (the
// desktop background-service model); each instance keeps its own id, working
// directory and event routing, while the typed client and endpoint are shared.
type Adapter struct {
	executable        string
	resolveExecutable func() string
	hostname          string
	startupTimeout    time.Duration
	streamIdle        time.Duration
	now               func() time.Time
	newID             func() (string, error)
	launch            launcher
	versionProbe      func(ctx context.Context) (string, error)
	serviceFilePath   string
	serviceDirectory  string
	watchInterval     time.Duration

	mu        sync.Mutex
	instances map[string]*instance

	sharedMu   sync.Mutex
	shared     *sharedServer
	sharedStop func(context.Context) error
}

// launchResult is a started opencode server.
type launchResult struct {
	client   *Client
	endpoint string
	pid      int
	done     <-chan struct{}
	stop     func(context.Context) error
}

// launcher starts one server in a directory.
type launcher func(ctx context.Context, directory string) (*launchResult, error)

// Register installs a default opencode adapter into the registry.
func Register(registry *agents.Registry) error {
	if registry == nil {
		return errors.New("opencode: registry is required")
	}
	return registry.Register(agents.BackendOpenCodeV2, New(Options{}))
}

// New builds an opencode adapter.
func New(options Options) *Adapter {
	adapter := &Adapter{
		executable:        options.Executable,
		resolveExecutable: options.ResolveExecutable,
		hostname:          options.Hostname,
		startupTimeout:    options.StartupTimeout,
		streamIdle:        options.StreamIdleTimeout,
		now:               options.Now,
		newID:             options.NewID,
		versionProbe:      options.VersionProbe,
		serviceFilePath:   options.ServiceFile,
		serviceDirectory:  options.ServiceDirectory,
		instances:         make(map[string]*instance),
	}
	if adapter.executable == "" {
		adapter.executable = "opencode"
	}
	if adapter.hostname == "" {
		adapter.hostname = "127.0.0.1"
	}
	if adapter.startupTimeout <= 0 {
		adapter.startupTimeout = 20 * time.Second
	}
	if adapter.streamIdle <= 0 {
		adapter.streamIdle = defaultStreamIdleTimeout
	}
	if adapter.now == nil {
		adapter.now = func() time.Time { return time.Now().UTC() }
	}
	if adapter.newID == nil {
		adapter.newID = newUUID
	}
	if adapter.versionProbe == nil {
		adapter.versionProbe = adapter.runVersionCommand
	}
	adapter.watchInterval = 5 * time.Second
	adapter.launch = adapter.launchSharedServer
	return adapter
}

func (a *Adapter) execPath() string {
	if a.resolveExecutable != nil {
		if path := a.resolveExecutable(); path != "" {
			return path
		}
	}
	return a.executable
}

// Capabilities declares what the v2 API actually supports.
func (a *Adapter) Capabilities() agents.Capabilities {
	return agents.Capabilities{
		Streaming:        true,
		Interrupt:        true,
		SessionDiscovery: false,
		SessionContinue:  true,
		Stop:             true,
		ModelSelection:   true,
		ToolCallEvents:   true,
		ToolConfirmation: true,
		SessionHistory:   true,
		RawEvents:        true,
	}
}

// Probe reports whether the opencode binary is resolvable.
func (a *Adapter) Probe(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	executable := a.execPath()
	path, err := a.probePath()
	if err != nil {
		return fmt.Errorf("opencode: executable %q not found: %w", executable, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("opencode: stat executable %q: %w", path, err)
	}
	if info.IsDir() {
		return fmt.Errorf("opencode: executable %q is a directory", path)
	}
	return nil
}

// Diagnostics runs the opencode pre-flight checks.
func (a *Adapter) Diagnostics(ctx context.Context) agents.BackendDiagnostics {
	supported := Descriptor.SupportedVersion()
	report := agents.BackendDiagnostics{
		Backend:       agents.BackendOpenCodeV2,
		TestedVersion: supported.Tested,
		Checks:        []agents.Diagnostic{},
		Capabilities:  capabilitiesOf(a.Capabilities()),
	}

	if err := a.Probe(ctx); err != nil {
		report.Checks = append(report.Checks, agents.Diagnostic{
			Kind:    agents.DiagnosticExecutable,
			Status:  agents.DiagnosticError,
			Message: fmt.Sprintf("%s; install opencode (https://opencode.ai) or point the adapter at the correct binary", redactSecrets(err.Error())),
		})
		report.Status = agents.DiagnosticError
		return report
	}
	report.Checks = append(report.Checks, agents.Diagnostic{
		Kind:    agents.DiagnosticExecutable,
		Status:  agents.DiagnosticOK,
		Message: fmt.Sprintf("opencode executable %q found", a.execPath()),
	})

	version, versionCheck := a.checkVersion(ctx, supported)
	report.Version = version
	report.Checks = append(report.Checks, versionCheck)

	report.Checks = append(report.Checks, a.checkAPI(ctx, supported))
	report.Status = agents.AggregateStatus(report.Checks)
	return report
}

func capabilitiesOf(capabilities agents.Capabilities) *agents.Capabilities {
	return &capabilities
}

// StartInstance registers a running instance bound to the shared server.
func (a *Adapter) StartInstance(ctx context.Context, request agents.StartInstanceRequest) (*agents.Instance, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if request.Directory == "" {
		return nil, errors.New("opencode: instance directory is required")
	}
	info, err := os.Stat(request.Directory)
	if err != nil {
		return nil, fmt.Errorf("opencode: instance directory %q: %w", request.Directory, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("opencode: instance directory %q is not a directory", request.Directory)
	}
	if err := a.Probe(ctx); err != nil {
		return nil, err
	}

	shared, err := a.ensureShared(ctx)
	if err != nil {
		return nil, err
	}

	id, err := a.newID()
	if err != nil {
		return nil, fmt.Errorf("opencode: allocate instance id: %w", err)
	}

	instanceCtx, cancel := context.WithCancel(context.Background())
	now := a.now()
	inst := &instance{
		id:            id,
		directory:     request.Directory,
		endpoint:      shared.endpoint,
		client:        shared.client,
		proc:          nil,
		ctx:           instanceCtx,
		cancel:        cancel,
		done:          make(chan struct{}),
		now:           a.now,
		streamIdle:    a.streamIdle,
		subs:          make(map[*subscriber]struct{}),
		sessions:      make(map[string]string),
		sessionModels: make(map[string]agents.MessageModel),
		toolCalls:     make(map[string]toolCallInfo),
		turns:         make(map[string]string),
	}

	a.mu.Lock()
	a.instances[id] = inst
	a.mu.Unlock()

	return &agents.Instance{
		ID:        id,
		Backend:   agents.BackendOpenCodeV2,
		Directory: request.Directory,
		Status:    agents.InstanceRunning,
		Endpoint:  shared.endpoint,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

// watchProcess is retained for rollback reference.
func (a *Adapter) watchProcess(inst *instance, procDone <-chan struct{}) {
	select {
	case <-inst.ctx.Done():
		return
	case <-procDone:
	}
	if !inst.active() {
		return
	}
	a.mu.Lock()
	if a.instances[inst.id] == inst {
		delete(a.instances, inst.id)
	}
	a.mu.Unlock()
	inst.fail("opencode server process exited unexpectedly")
}

// StopInstance detaches the instance and closes its event stream.
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
	return inst.stop(ctx)
}

// Subscribe returns the instance's normalized event stream.
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
	inst.ensureStream()
	go sub.forward()
	go sub.watch(ctx, inst)
	return sub.out, nil
}

// CreateSession creates a native session in the instance's directory.
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
	var model *ModelRef
	if options.Model != "" {
		parsed, err := parseModelRef(options.Model)
		if err != nil {
			return nil, err
		}
		model = &parsed
	}
	info, err := inst.client.CreateSession(ctx, CreateSessionRequest{
		Title:    options.Title,
		Model:    model,
		Location: &LocationRef{Directory: inst.directory},
	})
	if err != nil {
		return nil, fmt.Errorf("opencode: create session on instance %q: %w", instanceID, err)
	}
	if info.ID == "" {
		return nil, errors.New("opencode: server returned a session without an id")
	}
	inst.registerSession(info.ID, info.Title)
	if info.Model != nil {
		inst.setSessionModel(info.ID, *info.Model)
	}
	return &agents.SessionRef{
		InstanceID: instanceID,
		Backend:    agents.BackendOpenCodeV2,
		SessionID:  info.ID,
		Title:      info.Title,
	}, nil
}

// ContinueSession re-attaches to an existing native session and refreshes its title.
func (a *Adapter) ContinueSession(ctx context.Context, ref agents.SessionRef) (*agents.SessionRef, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := ref.Validate(); err != nil {
		return nil, err
	}
	if ref.Backend != agents.BackendOpenCodeV2 {
		return nil, fmt.Errorf("opencode: session backend %q is not opencode-v2", ref.Backend)
	}
	inst, err := a.getInstance(ref.InstanceID)
	if err != nil {
		return nil, err
	}
	if err := inst.ensureRunning(); err != nil {
		return nil, err
	}
	info, err := inst.client.GetSession(ctx, ref.SessionID)
	if err != nil {
		return nil, fmt.Errorf("opencode: continue session %q: %w", ref.SessionID, err)
	}
	title := ref.Title
	if info.Title != "" {
		title = info.Title
	}
	inst.registerSession(ref.SessionID, title)
	if info.Model != nil {
		inst.setSessionModel(ref.SessionID, *info.Model)
	}
	return &agents.SessionRef{
		InstanceID: ref.InstanceID,
		Backend:    agents.BackendOpenCodeV2,
		SessionID:  ref.SessionID,
		Title:      title,
	}, nil
}

// SendMessage admits one user turn on the session.
func (a *Adapter) SendMessage(ctx context.Context, ref agents.SessionRef, message agents.SendMessageRequest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := ref.Validate(); err != nil {
		return err
	}
	if ref.Backend != agents.BackendOpenCodeV2 {
		return fmt.Errorf("opencode: session backend %q is not opencode-v2", ref.Backend)
	}
	if strings.TrimSpace(message.Text) == "" && len(message.Images) == 0 {
		return errors.New("opencode: message text or images required")
	}
	inst, err := a.getInstance(ref.InstanceID)
	if err != nil {
		return err
	}
	if err := inst.ensureRunning(); err != nil {
		return err
	}
	if message.Model != "" {
		model, err := parseModelRef(message.Model)
		if err != nil {
			return err
		}
		if err := inst.client.SwitchModel(ctx, ref.SessionID, model); err != nil {
			return fmt.Errorf("opencode: switch model for session %q: %w", ref.SessionID, err)
		}
		inst.setSessionModel(ref.SessionID, model)
	}
	if err := inst.client.Prompt(ctx, ref.SessionID, message.Text, message.Images); err != nil {
		return fmt.Errorf("opencode: send message on instance %q: %w", ref.InstanceID, err)
	}
	inst.registerSession(ref.SessionID, ref.Title)
	return nil
}

// ErrSessionDiscoveryUnsupported reports that opencode session discovery is not supported.
var ErrSessionDiscoveryUnsupported = errors.New("opencode: session discovery is not supported")

// ListSessions is disabled on the opencode backend: session discovery is not supported.
func (a *Adapter) ListSessions(ctx context.Context, instanceID string) ([]agents.SessionRef, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if _, err := a.getInstance(instanceID); err != nil {
		return nil, err
	}
	return nil, ErrSessionDiscoveryUnsupported
}

func (a *Adapter) getInstance(instanceID string) (*instance, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	inst, ok := a.instances[instanceID]
	if !ok {
		return nil, fmt.Errorf("opencode: unknown instance %q", instanceID)
	}
	return inst, nil
}

// instance is one running opencode server bound to a working directory.
type instance struct {
	id         string
	directory  string
	endpoint   string
	client     *Client
	proc       *launchResult
	ctx        context.Context
	cancel     context.CancelFunc
	done       chan struct{}
	now        func() time.Time
	streamIdle time.Duration
	sleep      func(time.Duration) bool

	mu            sync.Mutex
	closed        bool
	streamStarted bool
	seq           uint64
	subs          map[*subscriber]struct{}
	sessions      map[string]string
	sessionModels map[string]agents.MessageModel
	toolCalls     map[string]toolCallInfo
	turns         map[string]string
	turnSeq       uint64
}

func (inst *instance) ensureRunning() error {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	if inst.closed {
		return fmt.Errorf("opencode: instance %q is not running", inst.id)
	}
	return nil
}

func (inst *instance) isClosed() bool {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	return inst.closed
}

func (inst *instance) active() bool {
	if inst.ctx.Err() != nil {
		return false
	}
	return !inst.isClosed()
}

func (inst *instance) stop(ctx context.Context) error {
	inst.mu.Lock()
	if inst.closed {
		inst.mu.Unlock()
		return nil
	}
	inst.closed = true
	close(inst.done)
	inst.mu.Unlock()

	inst.cancel()
	if inst.proc != nil && inst.proc.stop != nil {
		return inst.proc.stop(ctx)
	}
	return nil
}

func (inst *instance) fail(detail string) {
	event := inst.event("")
	event.Type = agents.EventLifecycle
	event.Lifecycle = &agents.LifecycleEvent{State: string(agents.InstanceFailed), Detail: truncate(redactSecrets(detail))}
	inst.publish(event)

	inst.mu.Lock()
	if inst.closed {
		inst.mu.Unlock()
		return
	}
	inst.closed = true
	close(inst.done)
	inst.mu.Unlock()
	inst.cancel()
}

func (inst *instance) registerSession(sessionID, title string) {
	if sessionID == "" {
		return
	}
	inst.mu.Lock()
	if existing, ok := inst.sessions[sessionID]; !ok || (title != "" && title != existing) {
		inst.sessions[sessionID] = title
	}
	inst.mu.Unlock()
}

func (inst *instance) knownSession(sessionID string) bool {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	_, ok := inst.sessions[sessionID]
	return ok
}

func (inst *instance) setSessionModel(sessionID string, ref ModelRef) {
	model := messageModel(ref)
	if sessionID == "" || model == nil {
		return
	}
	inst.mu.Lock()
	inst.sessionModels[sessionID] = *model
	inst.mu.Unlock()
}

func (inst *instance) sessionModel(sessionID string) *agents.MessageModel {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	model, ok := inst.sessionModels[sessionID]
	if !ok {
		return nil
	}
	return &model
}

func messageModel(ref ModelRef) *agents.MessageModel {
	if ref.ID == "" {
		return nil
	}
	id := ref.ID
	if ref.ProviderID != "" {
		id = ref.ProviderID + "/" + ref.ID
	}
	return &agents.MessageModel{ID: id, Provider: ref.ProviderID, Variant: ref.Variant}
}

// DetectTurn reports whether the native session is actively running a turn.
func (a *Adapter) DetectTurn(ctx context.Context, ref agents.SessionRef) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if err := ref.Validate(); err != nil {
		return false, err
	}
	if ref.Backend != agents.BackendOpenCodeV2 {
		return false, fmt.Errorf("opencode: session backend %q is not opencode-v2", ref.Backend)
	}
	inst, err := a.getInstance(ref.InstanceID)
	if err != nil {
		return false, err
	}
	if err := inst.ensureRunning(); err != nil {
		return false, err
	}
	active, err := inst.client.ActiveSessions(ctx)
	if err != nil {
		return false, fmt.Errorf("opencode: check active sessions: %w", err)
	}
	return active[ref.SessionID], nil
}

func parseModelRef(value string) (ModelRef, error) {
	if strings.TrimSpace(value) == "" {
		return ModelRef{}, errors.New("opencode: model is required")
	}
	raw := value
	variant := ""
	if idx := strings.Index(raw, "#"); idx >= 0 {
		variant = raw[idx+1:]
		raw = raw[:idx]
	}
	provider := ""
	id := raw
	if idx := strings.Index(raw, "/"); idx >= 0 {
		provider = raw[:idx]
		id = raw[idx+1:]
	}
	if provider == "" || id == "" {
		return ModelRef{}, fmt.Errorf("opencode: model %q must be provider/model", value)
	}
	return ModelRef{ID: id, ProviderID: provider, Variant: variant}, nil
}

func sameDirectory(a, b string) bool {
	if a == b {
		return true
	}
	resolvedA, errA := filepath.EvalSymlinks(a)
	resolvedB, errB := filepath.EvalSymlinks(b)
	if errA != nil || errB != nil {
		return filepath.Clean(a) == filepath.Clean(b)
	}
	return resolvedA == resolvedB
}

func truncate(value string) string {
	if len(value) <= maxEventBytes {
		return value
	}
	cut := maxEventBytes
	for cut > 0 && !utf8.RuneStart(value[cut]) {
		cut--
	}
	return value[:cut] + "…(truncated)"
}

// ListModels returns the models the machine-wide server offers.
func (a *Adapter) ListModels(ctx context.Context, directory string) ([]agents.ModelInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	shared, err := a.ensureShared(ctx)
	if err != nil {
		return nil, err
	}
	records, err := shared.client.ListModels(ctx, directory)
	if err != nil {
		return nil, fmt.Errorf("opencode: list models for %q: %w", directory, err)
	}
	models := make([]agents.ModelInfo, 0, len(records))
	for _, record := range records {
		modelID := record.ProviderID + "/" + record.ModelID
		model := agents.ModelInfo{
			ID:             modelID,
			Name:           record.Name,
			Provider:       record.ProviderID,
			SupportsImages: isMultimodalModel(modelID),
		}
		for _, variant := range record.Variants {
			model.Variants = append(model.Variants, variant.ID)
		}
		if record.Limit != nil && record.Limit.Context > 0 {
			limit := record.Limit.Context
			model.ContextLimit = &limit
		}
		models = append(models, model)
	}
	return models, nil
}

// Interrupt ends the running turn of one session through the server's interrupt route.
func (a *Adapter) Interrupt(ctx context.Context, ref agents.SessionRef) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := ref.Validate(); err != nil {
		return err
	}
	if ref.Backend != agents.BackendOpenCodeV2 {
		return fmt.Errorf("opencode: session backend %q is not opencode-v2", ref.Backend)
	}
	inst, err := a.getInstance(ref.InstanceID)
	if err != nil {
		return err
	}
	if err := inst.client.Interrupt(ctx, ref.SessionID); err != nil {
		return fmt.Errorf("opencode: interrupt session %q: %w", ref.SessionID, err)
	}
	return nil
}
