package opencode

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"devplatform/internal/agents"
	"devplatform/internal/platform"
)

// Compile-time proof that the opencode adapter satisfies the platform contract.
var _ agents.AgentAdapter = (*Adapter)(nil)

const (
	// streamRetryMin and streamRetryMax bound the reconnect backoff after the
	// event stream drops. Each instance reconnects its own stream only.
	streamRetryMin = 100 * time.Millisecond
	streamRetryMax = 5 * time.Second
	// defaultStreamIdleTimeout is how long a silent-but-open event stream may
	// go without delivering an event before the adapter treats it as dead and
	// reconnects. It bounds a hung connection that never sends and never
	// closes.
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
	// current binary path (the platform-level agent setting). An empty result
	// falls back to Executable.
	ResolveExecutable func() string
	// Hostname is the loopback host the per-instance server binds. Defaults
	// to 127.0.0.1.
	Hostname string
	// StartupTimeout bounds how long StartInstance waits for the server to
	// answer /api/info. Defaults to 20s.
	StartupTimeout time.Duration
	// StreamIdleTimeout bounds how long the event stream may go silent before
	// the adapter reconnects. Defaults to 60s.
	StreamIdleTimeout time.Duration
	// VersionProbe overrides how the CLI version is obtained. When nil the
	// adapter runs `<executable> --version`. Tests inject a stub so they do
	// not have to execute a real binary.
	VersionProbe func(ctx context.Context) (string, error)
	// Now overrides the clock; tests set it for deterministic timestamps.
	Now func() time.Time
	// NewID overrides identifier generation; tests may make it deterministic.
	NewID func() (string, error)
	// ServiceFile overrides the desktop registration file path used to
	// discover an already-running `opencode serve --service`
	// (XDG_STATE_HOME/opencode/service.json by default). Tests point it at a
	// temp file.
	ServiceFile string
	// ServiceDirectory is the working directory the service is started in when
	// the operator starts it from the platform settings. The service is
	// machine-wide and serves no project of its own, so this is a neutral
	// scratch directory (the platform data directory), not a project path.
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
	// watchInterval bounds how often watchShared re-probes the service. Tests
	// shorten it; production keeps the default.
	watchInterval time.Duration

	mu        sync.Mutex
	instances map[string]*instance

	sharedMu   sync.Mutex
	shared     *sharedServer
	sharedStop func(context.Context) error
}

// launchResult is a started opencode server: the client bound to it, its
// endpoint, the teardown hook that terminates the process, and a channel that
// closes when the server process exits. done is nil for launchers that have no
// process (for example a test double), in which case no process watcher runs.
type launchResult struct {
	client   *Client
	endpoint string
	// pid is the launched process's id, so a stop requested from the settings
	// page can report what it stopped.
	pid  int
	done <-chan struct{}
	stop func(context.Context) error
}

// launcher starts one server in a directory. It is a field so tests can inject
// a fake without launching a process.
type launcher func(ctx context.Context, directory string) (*launchResult, error)

// Register installs a default opencode adapter (executable "opencode") into the
// registry. Callers needing custom options use New and Registry.Register.
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

// execPath is the executable to run: the runtime override when one resolves,
// otherwise the configured default. It keeps a manual agent path effective for
// every launch without rebuilding the adapter.
func (a *Adapter) execPath() string {
	if a.resolveExecutable != nil {
		if path := a.resolveExecutable(); path != "" {
			return path
		}
	}
	return a.executable
}

// Capabilities declares what the v2 API actually supports. Streaming
// (session.text.delta), session discovery/create/continue, model selection
// (session.create model + session.switchModel), tool call/result events
// (session.tool.*), and tool confirmation (permission.asked + reply, form.* +
// reply via the opencode-only PermissionReplier extension) are all available.
func (a *Adapter) Capabilities() agents.Capabilities {
	return agents.Capabilities{
		Streaming:        true,
		Interrupt:        true,
		SessionDiscovery: true,
		SessionContinue:  true,
		Stop:             true,
		ModelSelection:   true,
		ToolCallEvents:   true,
		ToolConfirmation: true,
		SessionHistory:   true,
		RawEvents:        true,
	}
}

// Probe reports whether the opencode binary is resolvable. It never starts a
// server.
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

// runVersionCommand asks the CLI for its version with a short timeout. The
// same binary serves the v2 API, so the CLI version is also the API version.
func (a *Adapter) runVersionCommand(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	command, err := a.backendCommand(ctx, "--version")
	if err != nil {
		return "", err
	}
	out, err := command.Output()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && len(exitErr.Stderr) > 0 {
		return "", fmt.Errorf("%w: %s", err, truncate(string(exitErr.Stderr)))
	}
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// Diagnostics runs the opencode pre-flight checks: executable presence, CLI
// version support against the tested range, and API reachability of every
// running instance. It never starts anything, honors ctx, and every message
// is actionable and redacted. A missing binary short-circuits the report:
// without it there is nothing left to probe.
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

// checkVersion classifies the CLI version against the tested range. An
// undeterminable or unsupported version is a warning, not an error: the
// backend may still work, and the instance lifecycle reports any real
// start failure.
func (a *Adapter) checkVersion(ctx context.Context, supported agents.SupportedVersion) (string, agents.Diagnostic) {
	raw, err := a.versionProbe(ctx)
	if err != nil {
		return "", agents.Diagnostic{
			Kind:    agents.DiagnosticVersion,
			Status:  agents.DiagnosticWarning,
			Message: fmt.Sprintf("could not determine the opencode CLI version (%s); the tested version is %s", redactSecrets(err.Error()), supported.Tested),
		}
	}
	parsed, ok := agents.ParseVersion(raw)
	if !ok {
		return "", agents.Diagnostic{
			Kind:    agents.DiagnosticVersion,
			Status:  agents.DiagnosticWarning,
			Message: fmt.Sprintf("opencode reported an unparseable version %q; the tested version is %s", truncate(redactSecrets(raw)), supported.Tested),
		}
	}
	if !parsed.InRange(supported) {
		return parsed.String(), agents.Diagnostic{
			Kind:    agents.DiagnosticVersion,
			Status:  agents.DiagnosticWarning,
			Message: fmt.Sprintf("opencode CLI %s is outside the tested range [%s, %s); the tested version is %s; upgrade or downgrade opencode if sessions fail", parsed, supported.Minimum, supported.Maximum, supported.Tested),
			Detail:  truncate(redactSecrets(raw)),
		}
	}
	return parsed.String(), agents.Diagnostic{
		Kind:    agents.DiagnosticVersion,
		Status:  agents.DiagnosticOK,
		Message: fmt.Sprintf("opencode CLI %s is within the tested range [%s, %s)", parsed, supported.Minimum, supported.Maximum),
		Detail:  truncate(redactSecrets(raw)),
	}
}

// runningTarget is one instance the API check probes.
type runningTarget struct {
	id       string
	endpoint string
	client   *Client
}

// checkAPI probes the shared server with GET /api/info. Instances share one
// server, so endpoints are deduplicated; with no instances the shared server
// itself is probed when present.
func (a *Adapter) checkAPI(ctx context.Context, supported agents.SupportedVersion) agents.Diagnostic {
	a.mu.Lock()
	seen := make(map[string]string)
	for id, inst := range a.instances {
		if _, ok := seen[inst.endpoint]; !ok {
			seen[inst.endpoint] = id
		}
		_ = inst
	}
	a.mu.Unlock()
	a.sharedMu.Lock()
	sharedClient := (*Client)(nil)
	sharedEndpoint := ""
	if a.shared != nil {
		sharedClient = a.shared.client
		sharedEndpoint = a.shared.endpoint
	}
	a.sharedMu.Unlock()
	targets := make([]runningTarget, 0, len(seen)+1)
	for endpoint, id := range seen {
		var client *Client
		if sharedClient != nil && endpoint == sharedEndpoint {
			client = sharedClient
		} else {
			continue
		}
		targets = append(targets, runningTarget{id: id, endpoint: endpoint, client: client})
	}
	if len(targets) == 0 && sharedClient != nil {
		targets = append(targets, runningTarget{id: "shared", endpoint: sharedEndpoint, client: sharedClient})
	}
	sort.Slice(targets, func(i, j int) bool { return targets[i].id < targets[j].id })

	if len(targets) == 0 {
		return agents.Diagnostic{
			Kind:    agents.DiagnosticAPI,
			Status:  agents.DiagnosticOK,
			Message: "no running opencode instances to probe",
		}
	}

	var unreachable, mismatched []string
	for _, target := range targets {
		probeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		info, err := target.client.Info(probeCtx)
		cancel()
		if err != nil {
			unreachable = append(unreachable, fmt.Sprintf("instance %s at %s (%s)", target.id, target.endpoint, redactSecrets(err.Error())))
			continue
		}
		parsed, ok := agents.ParseVersion(info.Version)
		if !ok {
			// Never treat an unparseable server version as compatible: the
			// instance cannot be verified, so surface it as a warning.
			mismatched = append(mismatched, fmt.Sprintf("instance %s reported an unparseable API version %q", target.id, truncate(redactSecrets(info.Version))))
			continue
		}
		if !parsed.InRange(supported) {
			mismatched = append(mismatched, fmt.Sprintf("instance %s reports API version %s", target.id, parsed))
		}
	}
	if len(unreachable) > 0 {
		return agents.Diagnostic{
			Kind:    agents.DiagnosticAPI,
			Status:  agents.DiagnosticError,
			Message: fmt.Sprintf("opencode service 不可达（%s）；请到平台设置 → Agent 检测并启动", strings.Join(unreachable, "; ")),
		}
	}
	if len(mismatched) > 0 {
		return agents.Diagnostic{
			Kind:    agents.DiagnosticAPI,
			Status:  agents.DiagnosticWarning,
			Message: fmt.Sprintf("%s; outside the tested range [%s, %s)", strings.Join(mismatched, "; "), supported.Minimum, supported.Maximum),
		}
	}
	return agents.Diagnostic{
		Kind:    agents.DiagnosticAPI,
		Status:  agents.DiagnosticOK,
		Message: fmt.Sprintf("%d running instance(s) answered the API probe", len(targets)),
	}
}

// capabilitiesOf returns a pointer copy so reports cannot alias the adapter.
func capabilitiesOf(capabilities agents.Capabilities) *agents.Capabilities {
	return &capabilities
}

// StartInstance validates the directory and binary, ensures the shared
// machine-wide server, and registers a running instance bound to it. The
// server process is shared across directories; per-instance state (sessions,
// subscriptions) stays isolated.
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

	now := a.now()
	instanceCtx, cancel := context.WithCancel(context.Background())
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
		toolNames:     make(map[string]string),
	}

	a.mu.Lock()
	a.instances[id] = inst
	a.mu.Unlock()

	// The shared server lifetime is observed by watchShared; per-instance
	// process watching is intentionally gone.

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

// watchProcess is retained for rollback reference. The shared model observes
// the server via watchShared instead of one watcher per instance.
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

// StopInstance detaches the instance and closes its event stream. The
// machine-wide opencode service is never touched: it also serves the desktop
// client and manually started servers, so stopping the platform's own instance
// stops nothing but that instance. An instance that is already gone is a
// successful stop.
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

// Subscribe returns the instance's normalized event stream. It opens the
// backend stream on first use. The channel closes when the caller's context
// ends or the instance stops.
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

// ContinueSession re-attaches to an existing native session and refreshes its
// title from the server.
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

// SendMessage admits one user turn on the session. It returns once the server
// accepted the message; replies and tool activity arrive on the subscription.
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
	if strings.TrimSpace(message.Text) == "" {
		return errors.New("opencode: message text is required")
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
		// The switch took effect, so the session's live messages now run this
		// model; remember it before the turn's events can arrive.
		inst.setSessionModel(ref.SessionID, model)
	}
	if err := inst.client.Prompt(ctx, ref.SessionID, message.Text); err != nil {
		return fmt.Errorf("opencode: send message on instance %q: %w", ref.InstanceID, err)
	}
	inst.registerSession(ref.SessionID, ref.Title)
	return nil
}

// ListSessions returns the native sessions the server knows for the instance's
// directory.
func (a *Adapter) ListSessions(ctx context.Context, instanceID string) ([]agents.SessionRef, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	inst, err := a.getInstance(instanceID)
	if err != nil {
		return nil, err
	}
	sessions, err := inst.client.ListSessions(ctx, inst.directory)
	if err != nil {
		return nil, fmt.Errorf("opencode: list sessions on instance %q: %w", instanceID, err)
	}
	refs := make([]agents.SessionRef, 0, len(sessions))
	for _, session := range sessions {
		if session.ID == "" {
			continue
		}
		inst.registerSession(session.ID, session.Title)
		if session.Model != nil {
			inst.setSessionModel(session.ID, *session.Model)
		}
		refs = append(refs, agents.SessionRef{
			InstanceID: instanceID,
			Backend:    agents.BackendOpenCodeV2,
			SessionID:  session.ID,
			Title:      session.Title,
		})
	}
	return refs, nil
}

// getInstance returns a registered instance or an error.
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
	// sleep is a test seam for the reconnect backoff; when nil, wait is used.
	sleep func(time.Duration) bool

	mu            sync.Mutex
	closed        bool
	streamStarted bool
	seq           uint64
	subs          map[*subscriber]struct{}
	sessions      map[string]string
	// sessionModels records the model each session currently runs, learned when
	// the session is created, continued or listed, and refreshed on a switch.
	// Live assistant events carry no model of their own, so the adapter stamps
	// them with this; replayed history uses each message's own model instead.
	sessionModels map[string]agents.MessageModel
	toolNames     map[string]string
}

// ensureRunning reports whether the instance can accept work.
func (inst *instance) ensureRunning() error {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	if inst.closed {
		return fmt.Errorf("opencode: instance %q is not running", inst.id)
	}
	return nil
}

// isClosed reports whether the instance has been stopped.
func (inst *instance) isClosed() bool {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	return inst.closed
}

// active reports whether the instance is still streaming.
func (inst *instance) active() bool {
	if inst.ctx.Err() != nil {
		return false
	}
	return !inst.isClosed()
}

// stop closes the instance's stream, tears down its server and is idempotent.
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

// fail marks the instance failed after its backend went away without a stop.
// It publishes an instance-scoped lifecycle event carrying the failed state so
// the platform's applyLifecycle accepts the transition (running -> failed) and
// dependent sessions reconcile to disconnected, then tears the instance down
// exactly like a stop. It is idempotent.
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

// registerSession remembers a native session so its events are routed to this
// instance. A non-empty title replaces a previously remembered empty one.
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

// knownSession reports whether a session belongs to this instance.
func (inst *instance) knownSession(sessionID string) bool {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	_, ok := inst.sessions[sessionID]
	return ok
}

// setSessionModel records the model a session currently runs, keyed by the
// native session id. It is learned from session create/continue/list and
// refreshed when the platform switches the model for a turn.
func (inst *instance) setSessionModel(sessionID string, ref ModelRef) {
	model := messageModel(ref)
	if sessionID == "" || model == nil {
		return
	}
	inst.mu.Lock()
	inst.sessionModels[sessionID] = *model
	inst.mu.Unlock()
}

// sessionModel returns the session's current model, or nil when none is known.
func (inst *instance) sessionModel(sessionID string) *agents.MessageModel {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	model, ok := inst.sessionModels[sessionID]
	if !ok {
		return nil
	}
	return &model
}

// messageModel converts a native ModelRef into the platform's MessageModel,
// using the provider/model id form the rest of the platform speaks. A ref with
// no id is not a model and yields nil.
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

// addSubscriber registers a subscriber unless the instance is already closed.
func (inst *instance) addSubscriber(sub *subscriber) error {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	if inst.closed {
		return fmt.Errorf("opencode: instance %q is not running", inst.id)
	}
	inst.subs[sub] = struct{}{}
	return nil
}

// removeSubscriber drops a finished subscriber.
func (inst *instance) removeSubscriber(sub *subscriber) {
	inst.mu.Lock()
	delete(inst.subs, sub)
	inst.mu.Unlock()
}

// ensureStream starts the instance's single backend event reader on first use.
func (inst *instance) ensureStream() {
	inst.mu.Lock()
	if inst.streamStarted || inst.closed {
		inst.mu.Unlock()
		return
	}
	inst.streamStarted = true
	inst.mu.Unlock()
	go inst.runStream()
}

// publish stamps the envelope, assigns the next per-instance sequence and
// delivers the event to every current subscriber.
func (inst *instance) publish(event agents.NormalizedEvent) {
	inst.mu.Lock()
	if inst.closed {
		inst.mu.Unlock()
		return
	}
	inst.seq++
	event.Sequence = inst.seq
	event.InstanceID = inst.id
	event.Backend = agents.BackendOpenCodeV2
	if event.Timestamp.IsZero() {
		event.Timestamp = inst.now()
	}
	subs := make([]*subscriber, 0, len(inst.subs))
	for sub := range inst.subs {
		subs = append(subs, sub)
	}
	inst.mu.Unlock()

	for _, sub := range subs {
		select {
		case sub.stash <- event:
		case <-sub.done:
		}
	}
}

// runStream reads the server's SSE stream, reconnects with backoff after a
// drop, and reports each error without affecting any other instance. The
// backoff is reset only when a connection actually delivered an event, so a
// server that accepts and immediately closes the stream backs off up to the
// cap instead of hammering it at the minimum delay. A malformed payload is
// reported with the malformed_event code so the UI can tell a bad stream from
// a dead server.
func (inst *instance) runStream() {
	backoff := streamRetryMin
	for {
		if !inst.active() {
			return
		}
		events, err := inst.consume()
		if !inst.active() {
			return
		}
		if err != nil {
			code := "stream_error"
			if errors.Is(err, errMalformedEvent) {
				code = "malformed_event"
			}
			inst.publishStreamError(code, err)
		}
		if events > 0 {
			// A connection that delivered events was healthy; reset so a
			// long-lived stream that later drops reconnects promptly.
			backoff = streamRetryMin
		}
		if !inst.pause(backoff) {
			return
		}
		backoff = nextBackoff(backoff)
	}
}

// consume opens one event-stream connection, guards it with an idle watchdog,
// and drains it. It returns the number of events read and the terminating
// error. A silent-but-open stream is canceled by the watchdog and reported as
// an error so runStream reconnects instead of blocking forever.
func (inst *instance) consume() (int, error) {
	attemptCtx, cancel := context.WithCancel(inst.ctx)
	defer cancel()

	activity := make(chan struct{}, 1)
	watchdogDone := make(chan struct{})
	if inst.streamIdle > 0 {
		go func() {
			defer close(watchdogDone)
			timer := time.NewTimer(inst.streamIdle)
			defer timer.Stop()
			for {
				select {
				case <-attemptCtx.Done():
					return
				case <-activity:
					if !timer.Stop() {
						select {
						case <-timer.C:
						default:
						}
					}
					timer.Reset(inst.streamIdle)
				case <-timer.C:
					// No data within the idle window: force the blocked
					// read to unblock so the stream reconnects.
					cancel()
					return
				}
			}
		}()
	} else {
		close(watchdogDone)
	}

	stream, err := inst.client.Events(attemptCtx)
	if err != nil {
		cancel()
		<-watchdogDone
		return 0, err
	}
	// Every line from the server — heartbeat comments included — resets the
	// idle watchdog. opencode sends ": heartbeat" every few seconds while a
	// turn is quiet, so only decoded events would let a healthy stream look
	// silent and be wrongly canceled.
	stream.SetActivity(func() {
		select {
		case activity <- struct{}{}:
		default:
		}
	})
	events, err := inst.pump(stream)
	_ = stream.Close()
	cancel()
	<-watchdogDone
	return events, err
}

// pump reads one stream to completion, normalizing and publishing each event.
// It returns the number of events read so the caller can tell a healthy
// connection from one that closed without ever producing data.
func (inst *instance) pump(stream *EventStream) (int, error) {
	events := 0
	for {
		event, err := stream.Next()
		if err != nil {
			return events, err
		}
		events++
		for _, normalized := range inst.normalize(event) {
			inst.publish(normalized)
		}
	}
}

// pause sleeps for the backoff through the test seam when set, otherwise using
// the instance's cancellation-aware wait. It reports false when the instance
// ended.
func (inst *instance) pause(d time.Duration) bool {
	if inst.sleep != nil {
		return inst.sleep(d)
	}
	return inst.wait(d)
}

// wait sleeps for the backoff or returns false when the instance ended.
func (inst *instance) wait(d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-inst.done:
		return false
	case <-inst.ctx.Done():
		return false
	}
}

// nextBackoff doubles the delay up to the cap.
func nextBackoff(current time.Duration) time.Duration {
	next := current * 2
	if next > streamRetryMax {
		next = streamRetryMax
	}
	return next
}

// publishStreamError surfaces an instance-level stream failure with the given
// diagnostic code. The contract requires a session id on error events, so
// when a session is known the error is attached to it; otherwise it is
// reported as an instance-scoped lifecycle event, which is valid without a
// session id.
func (inst *instance) publishStreamError(code string, err error) {
	message := truncate(redactSecrets(err.Error()))
	inst.mu.Lock()
	sessionID := ""
	for id := range inst.sessions {
		sessionID = id
		break
	}
	inst.mu.Unlock()

	if sessionID != "" {
		inst.publish(inst.errorEvent(sessionID, code, message))
		return
	}
	event := inst.event("")
	event.Type = agents.EventLifecycle
	event.Lifecycle = &agents.LifecycleEvent{State: code, Detail: message}
	inst.publish(event)
}

// event builds an envelope populated with the instance identity. Sequence is
// assigned later, centrally, at publish time.
func (inst *instance) event(sessionID string) agents.NormalizedEvent {
	return agents.NormalizedEvent{
		InstanceID: inst.id,
		SessionID:  sessionID,
		Backend:    agents.BackendOpenCodeV2,
		Timestamp:  inst.now(),
	}
}

// errorEvent builds a typed error event.
func (inst *instance) errorEvent(sessionID, code, message string) agents.NormalizedEvent {
	event := inst.event(sessionID)
	event.Type = agents.EventError
	event.Error = &agents.ErrorPayload{Code: code, Message: truncate(redactSecrets(message))}
	return event
}

// lifecycleEvent builds a session-scoped lifecycle event.
func (inst *instance) lifecycleEvent(sessionID, state, detail string) agents.NormalizedEvent {
	event := inst.event(sessionID)
	event.Type = agents.EventLifecycle
	event.Lifecycle = &agents.LifecycleEvent{State: state, Detail: truncate(detail)}
	return event
}

// unknown preserves an event the adapter could not normalize, byte for byte.
func (inst *instance) unknown(event *Event, sessionID string) agents.NormalizedEvent {
	out := inst.event(sessionID)
	out.Type = agents.EventUnknown
	out.Raw = append(json.RawMessage(nil), event.Raw()...)
	return out
}

// owns reports whether an event belongs to this instance: its session was seen
// before, or its location matches the instance directory (and the session is
// then learned).
func (inst *instance) owns(event *Event, sessionID string) bool {
	if sessionID == "" {
		return false
	}
	if inst.knownSession(sessionID) {
		return true
	}
	if event.Location != nil && sameDirectory(event.Location.Directory, inst.directory) {
		inst.registerSession(sessionID, "")
		return true
	}
	return false
}

// normalize maps one opencode event onto the normalized union. Events for other
// locations or sessions are ignored; a known session's unmapped event is
// preserved as EventUnknown.
func (inst *instance) normalize(event *Event) []agents.NormalizedEvent {
	data := decodeEventData(event.Data)
	sessionID := data.SessionID
	if !inst.owns(event, sessionID) {
		return nil
	}

	switch event.Type {
	case "session.text.delta":
		out := inst.event(sessionID)
		out.Type = agents.EventMessage
		out.Message = &agents.MessagePayload{Role: "assistant", Text: data.Delta, Final: false, Model: inst.sessionModel(sessionID)}
		return []agents.NormalizedEvent{out}

	case "session.text.ended":
		// Always carry the part's full text: opencode ends each text part with
		// a complete snapshot, so a consumer that joined after the deltas
		// streamed — a chat pane opened while create-with-first-message
		// already started the turn — converges on the whole block instead of
		// keeping a truncated prefix. Consumers replace on final instead of
		// concatenating.
		out := inst.event(sessionID)
		out.Type = agents.EventMessage
		out.Message = &agents.MessagePayload{Role: "assistant", Text: data.Text, Final: true, Model: inst.sessionModel(sessionID)}
		return []agents.NormalizedEvent{out}

	case "session.text.started":
		return []agents.NormalizedEvent{inst.lifecycleEvent(sessionID, "text_started", "")}

	// Reasoning streams under its own event family and becomes its own kind of
	// entry: the deltas carry it and reasoning.ended closes the block with its
	// full snapshot. No event is emitted for reasoning.started, because nothing
	// consumes a start marker — the workbench opens the block on the first
	// chunk.
	case "session.reasoning.started":
		return nil

	case "session.reasoning.delta":
		out := inst.event(sessionID)
		out.Type = agents.EventThinking
		out.Thinking = &agents.ThinkingPayload{Text: data.Delta, Final: false}
		return []agents.NormalizedEvent{out}

	case "session.reasoning.ended":
		// The same full-snapshot rule as session.text.ended: the ended event
		// always carries the block's whole text so a late subscriber converges.
		out := inst.event(sessionID)
		out.Type = agents.EventThinking
		out.Thinking = &agents.ThinkingPayload{Text: data.Text, Final: true}
		return []agents.NormalizedEvent{out}

	case "session.tool.input.started":
		inst.rememberToolName(sessionID, data.AssistantMessageID, data.ID, data.Name)
		return nil

	case "session.tool.called":
		out := inst.event(sessionID)
		out.Type = agents.EventToolCall
		out.ToolCall = &agents.ToolCallPayload{
			CallID: data.ID,
			Name:   inst.toolName(sessionID, data.AssistantMessageID, data.ID),
			Input:  data.Input,
		}
		return []agents.NormalizedEvent{out}

	case "session.tool.success":
		inst.forgetToolName(sessionID, data.AssistantMessageID, data.ID)
		out := inst.event(sessionID)
		out.Type = agents.EventToolResult
		out.ToolResult = &agents.ToolResultPayload{CallID: data.ID, Output: flattenContent(data.Content), IsError: false, Metadata: boundedMetadata(data.Metadata)}
		return []agents.NormalizedEvent{out}

	case "session.tool.failed":
		inst.forgetToolName(sessionID, data.AssistantMessageID, data.ID)
		out := inst.event(sessionID)
		out.Type = agents.EventToolResult
		out.ToolResult = &agents.ToolResultPayload{CallID: data.ID, Output: structuredErrorMessage(data, data.Content), IsError: true, Metadata: boundedMetadata(data.Metadata)}
		return []agents.NormalizedEvent{out}

	case "session.execution.failed", "session.step.failed", "session.error":
		return []agents.NormalizedEvent{inst.errorEvent(sessionID, errorCode(event.Type), structuredErrorMessage(data, nil))}

	case "session.retry.scheduled":
		return []agents.NormalizedEvent{inst.errorEvent(sessionID, "retry_scheduled", retryMessage(data))}

	case "session.created":
		inst.registerSession(sessionID, data.Title)
		return []agents.NormalizedEvent{inst.lifecycleEvent(sessionID, "created", data.Title)}

	case "session.updated", "session.renamed", "session.moved", "session.deleted",
		"session.forked", "session.instructions.updated", "session.skill.activated",
		"session.execution.started", "session.execution.succeeded", "session.execution.interrupted",
		"session.step.started", "session.step.streamed",
		"session.compaction.started", "session.compaction.ended",
		"session.revert.staged", "session.revert.cleared", "session.revert.committed":
		return []agents.NormalizedEvent{inst.lifecycleEvent(sessionID, lifecycleState(event.Type), lifecycleDetail(data))}

	case "session.step.ended":
		// A finished step reports the turn's token usage; it becomes the
		// workbench's freshest context anchor. The usage rides the lifecycle
		// event because no message renders for a step boundary.
		lifecycle := inst.lifecycleEvent(sessionID, lifecycleState(event.Type), lifecycleDetail(data))
		lifecycle.Lifecycle.Usage = data.Tokens.Usage()
		return []agents.NormalizedEvent{lifecycle}

	default:
		return []agents.NormalizedEvent{inst.unknown(event, sessionID)}
	}
}

// rememberToolName records a tool's name until its call is emitted.
func (inst *instance) rememberToolName(sessionID, messageID, toolID, name string) {
	if toolID == "" {
		return
	}
	inst.mu.Lock()
	inst.toolNames[toolKey(sessionID, messageID, toolID)] = name
	inst.mu.Unlock()
}

// toolName returns a recorded tool name, if any.
func (inst *instance) toolName(sessionID, messageID, toolID string) string {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	return inst.toolNames[toolKey(sessionID, messageID, toolID)]
}

// forgetToolName drops a tool's recorded name once it finishes.
func (inst *instance) forgetToolName(sessionID, messageID, toolID string) {
	inst.mu.Lock()
	delete(inst.toolNames, toolKey(sessionID, messageID, toolID))
	inst.mu.Unlock()
}

// toolKey scopes adapter-side tool bookkeeping to one message's tool.
func toolKey(sessionID, messageID, toolID string) string {
	return sessionID + "|" + messageID + "|" + toolID
}

// subscriber is one consumer of an instance's stream. stash is written by
// publishers and never closed; out is owned and closed by forward.
type subscriber struct {
	stash chan agents.NormalizedEvent
	out   chan agents.NormalizedEvent
	done  chan struct{}
	once  sync.Once
}

func newSubscriber() *subscriber {
	return &subscriber{
		stash: make(chan agents.NormalizedEvent, 64),
		out:   make(chan agents.NormalizedEvent, 64),
		done:  make(chan struct{}),
	}
}

// drainTimeout bounds how long forward waits to flush an already-queued event
// to a consumer at teardown, so a terminal event is not lost while a gone
// consumer can never block the goroutine forever.
const drainTimeout = 100 * time.Millisecond

// forward copies the subscriber's stash to its output and closes the output
// when the subscription ends. On teardown it flushes events already queued
// (for example a terminal `failed` lifecycle published immediately before the
// instance was torn down) so the consumer sees them before the channel closes.
func (s *subscriber) forward() {
	defer close(s.out)
	for {
		select {
		case event := <-s.stash:
			select {
			case s.out <- event:
			case <-s.done:
				// The subscription ended, but this event was already read
				// from the stash: deliver it (bounded) before flushing the
				// rest, so a terminal event is never dropped.
				s.deliver(event)
				s.flush()
				return
			}
		case <-s.done:
			s.flush()
			return
		}
	}
}

// deliver sends one event to the output, giving up after drainTimeout so a
// gone consumer cannot wedge the goroutine.
func (s *subscriber) deliver(event agents.NormalizedEvent) {
	select {
	case s.out <- event:
	case <-time.After(drainTimeout):
	}
}

// flush delivers the subscriber's already-queued events to its output.
func (s *subscriber) flush() {
	for {
		select {
		case event := <-s.stash:
			s.deliver(event)
		default:
			return
		}
	}
}

// watch closes the subscriber when either the caller's context ends or the
// instance stops, then unregisters it.
func (s *subscriber) watch(ctx context.Context, inst *instance) {
	select {
	case <-ctx.Done():
	case <-inst.done:
	}
	s.once.Do(func() { close(s.done) })
	inst.removeSubscriber(s)
}

// eventData is the union of the data bodies the adapter inspects. Unknown
// fields are ignored.
type eventData struct {
	SessionID          string           `json:"sessionID"`
	AssistantMessageID string           `json:"assistantMessageID"`
	Delta              string           `json:"delta"`
	Text               string           `json:"text"`
	ID                 string           `json:"id"`
	Name               string           `json:"name"`
	Input              json.RawMessage  `json:"input"`
	Content            json.RawMessage  `json:"content"`
	Metadata           json.RawMessage  `json:"metadata"`
	Error              *structuredError `json:"error"`
	Title              string           `json:"title"`
	Reason             string           `json:"reason"`
	Attempt            int              `json:"attempt"`
	// Tokens carries a finished step's token usage (session.step.ended).
	// Absent on every other event; a zero total is treated as unknown.
	Tokens *WireTokens `json:"tokens"`
}

// structuredError mirrors Session.StructuredError.
type structuredError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
	Status  int    `json:"status"`
}

// decodeEventData parses an event body into the union; a malformed body yields
// the zero value and is later preserved as EventUnknown.
func decodeEventData(raw json.RawMessage) eventData {
	var data eventData
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &data)
	}
	return data
}

// errorCode classifies a failure event type.
func errorCode(eventType string) string {
	switch eventType {
	case "session.error":
		return "session_error"
	case "session.step.failed":
		return "step_failed"
	default:
		return "execution_failed"
	}
}

// lifecycleDetail chooses human-readable context for a lifecycle event.
func lifecycleDetail(data eventData) string {
	if data.Title != "" {
		return data.Title
	}
	return data.Reason
}

// structuredErrorMessage extracts a failure's message, falling back to content.
func structuredErrorMessage(data eventData, content json.RawMessage) string {
	if data.Error != nil {
		if data.Error.Message != "" {
			return data.Error.Message
		}
		if data.Error.Type != "" {
			return data.Error.Type
		}
	}
	if len(content) > 0 {
		if text := flattenContent(content); text != "" {
			return text
		}
	}
	return "opencode reported a failure"
}

// retryMessage renders a scheduled-retry event.
func retryMessage(data eventData) string {
	message := structuredErrorMessage(data, nil)
	if data.Attempt > 0 {
		return fmt.Sprintf("retry attempt %d: %s", data.Attempt, message)
	}
	return message
}

// lifecycleState converts a native event type into a lifecycle state name.
func lifecycleState(eventType string) string {
	return strings.ReplaceAll(strings.TrimPrefix(eventType, "session."), ".", "_")
}

// boundedMetadata keeps only a size-bounded metadata body carrying edit file
// diffs; anything larger or without files is dropped so the event stays small.
func boundedMetadata(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 || len(raw) > agents.MaxRawEventBytes {
		return nil
	}
	meta := agents.ParseToolResultMetadata(raw)
	if meta == nil || len(meta.Files) == 0 {
		return nil
	}
	return append(json.RawMessage(nil), raw...)
}

// flattenContent renders a tool result content value (string, text blocks or a
// wrapped object) as display text.
func flattenContent(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text
	}
	var blocks []json.RawMessage
	if err := json.Unmarshal(raw, &blocks); err == nil {
		parts := make([]string, 0, len(blocks))
		for _, block := range blocks {
			if rendered := flattenContent(block); rendered != "" {
				parts = append(parts, rendered)
			}
		}
		return strings.Join(parts, "\n")
	}
	var object struct {
		Text    string          `json:"text"`
		Output  string          `json:"output"`
		Message string          `json:"message"`
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(raw, &object); err == nil {
		if object.Text != "" {
			return object.Text
		}
		if len(object.Content) > 0 {
			if rendered := flattenContent(object.Content); rendered != "" {
				return rendered
			}
		}
		if object.Output != "" {
			return object.Output
		}
		if object.Message != "" {
			return object.Message
		}
	}
	return truncate(string(raw))
}

// parseModelRef parses "provider/model#variant" into the API's ModelRef. The
// provider is required: the API identifies models by provider and id.
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

// sameDirectory compares two directories after cleaning and symlink-resolving
// them, so routing is not fooled by a trailing slash or a symlinked root.
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

// truncate bounds a diagnostic string. It backs off to a UTF-8 rune boundary so
// a multi-byte character is never split into invalid bytes.
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

// freePort asks the OS for an unused loopback port.
func freePort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("opencode: find free port: %w", err)
	}
	defer listener.Close()
	address, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		return 0, errors.New("opencode: unexpected listener address")
	}
	return address.Port, nil
}

// randomPassword returns a 256-bit base64url server password.
func randomPassword() (string, error) {
	var buf [32]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", fmt.Errorf("opencode: generate server password: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf[:]), nil
}

// launchServer starts `opencode serve` on a loopback port with a generated
// password (passed via the environment, never argv), waits for /api/info, and
// returns a handle whose stop kills the process group.
func (a *Adapter) launchServer(ctx context.Context, directory string) (*launchResult, error) {
	port, err := freePort()
	if err != nil {
		return nil, err
	}
	password, err := randomPassword()
	if err != nil {
		return nil, err
	}

	// exec.Command, not CommandContext: the server must outlive the start
	// request. Teardown is owned by the returned stop hook; a canceled start
	// context is honored by waitReady.
	command, err := a.backendCommand(context.Background(), "serve", "--hostname", a.hostname, "--port", strconv.Itoa(port))
	if err != nil {
		return nil, err
	}
	command.Dir = directory
	command.Env = append(os.Environ(), "OPENCODE_SERVER_PASSWORD="+password)
	output := &limitedWriter{limit: maxStartupOutputBytes}
	command.Stdout = output
	command.Stderr = output
	if err := platform.New().Process().Configure(command); err != nil {
		return nil, fmt.Errorf("opencode: configure server process: %w", err)
	}
	command.WaitDelay = 2 * time.Second

	if err := command.Start(); err != nil {
		return nil, fmt.Errorf("opencode: start server process: %w", err)
	}
	// Enroll the server in platform supervision (the Windows Job Object);
	// a server the platform cannot supervise must not be left behind.
	if err := platform.New().Process().Attach(command.Process); err != nil {
		_ = platform.New().Process().Terminate(command.Process.Pid)
		_ = command.Wait()
		return nil, fmt.Errorf("opencode: supervise server process: %w", err)
	}
	process := &launchedProcess{cmd: command, done: make(chan struct{})}
	go process.wait()

	endpoint := "http://" + net.JoinHostPort(a.hostname, strconv.Itoa(port))
	client, err := NewClient(ClientOptions{BaseURL: endpoint, Username: defaultUsername, Password: password})
	if err != nil {
		_ = platform.New().Process().Terminate(command.Process.Pid)
		_ = process.wait()
		return nil, err
	}
	if err := a.waitReady(ctx, process, client); err != nil {
		_ = platform.New().Process().Terminate(command.Process.Pid)
		_ = process.wait()
		// Redact the exact generated password (which may appear in a
		// non-key=value shape) as well as generic credential shapes.
		return nil, fmt.Errorf("%w: %s", err, client.redact(output.String()))
	}

	return &launchResult{
		client:   client,
		endpoint: endpoint,
		done:     process.done,
		stop: func(stopCtx context.Context) error {
			_ = platform.New().Process().Terminate(command.Process.Pid)
			waited := make(chan error, 1)
			go func() { waited <- process.wait() }()
			select {
			case <-waited:
			case <-stopCtx.Done():
			case <-time.After(3 * time.Second):
			}
			return nil
		},
	}, nil
}

// waitReady polls /api/info until the server answers, the process exits, the
// context ends, or the startup deadline passes.
func (a *Adapter) waitReady(ctx context.Context, process *launchedProcess, client *Client) error {
	deadline := time.NewTimer(a.startupTimeout)
	defer deadline.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	probe := func() error {
		probeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		_, err := client.Info(probeCtx)
		return err
	}
	if err := probe(); err != nil {
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusUnauthorized {
			return fmt.Errorf("opencode: server rejected generated credentials: %w", err)
		}
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return errors.New("opencode: server did not become ready in time")
		case <-process.done:
			return errors.New("opencode: server exited before becoming ready")
		case <-ticker.C:
			if err := probe(); err == nil {
				return nil
			}
		}
	}
}

// launchedProcess reaps a child exactly once and exposes its completion.
type launchedProcess struct {
	cmd  *exec.Cmd
	once sync.Once
	err  error
	done chan struct{}
}

// wait reaps the child once and closes done. Concurrent callers block until the
// single reap completes.
func (p *launchedProcess) wait() error {
	p.once.Do(func() {
		p.err = p.cmd.Wait()
		close(p.done)
	})
	<-p.done
	return p.err
}

// limitedWriter captures at most limit bytes and always reports the full write
// so a chatty child never blocks. It is safe for concurrent use.
type limitedWriter struct {
	mu      sync.Mutex
	buffer  bytes.Buffer
	limit   int
	written int
}

// Write appends up to the limit and discards the remainder.
func (l *limitedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.written < l.limit {
		remaining := l.limit - l.written
		chunk := p
		if len(chunk) > remaining {
			chunk = chunk[:remaining]
		}
		n, err := l.buffer.Write(chunk)
		l.written += n
		if err != nil {
			return n, err
		}
	}
	return len(p), nil
}

// String returns the captured output.
func (l *limitedWriter) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buffer.String()
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

// ListModels returns the models the machine-wide server offers for a working
// directory, identified as provider/model with each variant listed under its
// model. The shared service answers, so no instance has to exist: the workbench
// chooses a model before it has an instance, and a platform restart does not
// hide the catalog behind a stale instance record.
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
		model := agents.ModelInfo{
			ID:       record.ProviderID + "/" + record.ModelID,
			Name:     record.Name,
			Provider: record.ProviderID,
		}
		for _, variant := range record.Variants {
			model.Variants = append(model.Variants, variant.ID)
		}
		// The server reports the context window per model (Model.Info.limit);
		// a non-positive value is treated as unknown rather than a limit.
		if record.Limit != nil && record.Limit.Context > 0 {
			limit := record.Limit.Context
			model.ContextLimit = &limit
		}
		models = append(models, model)
	}
	return models, nil
}

// Interrupt ends the running turn of one session through the server's
// interrupt route. The instance and its other sessions keep running.
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
