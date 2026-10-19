package pi

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"devplatform/internal/core/agents"
	"devplatform/internal/core/platform"
)

// Compile-time proof that the pi adapter satisfies the platform contract.
var (
	_ agents.AgentAdapter        = (*Adapter)(nil)
	_ agents.SessionTurnDetector = (*Adapter)(nil)
)

// maxSessionScanBytes bounds how much of a session file is scanned for its
// display name. Discovery reads the header plus a bounded prefix; it never
// loads a whole conversation into memory.
const maxSessionScanBytes = 4 * 1024 * 1024

// Operating modes for the pi adapter. CLI is the one-shot `pi --mode json`
// process per prompt; RPC is a long-lived `pi --mode rpc` child per active
// session. CLI remains the default until RPC has soaked, and the switch is the
// rollback lever if a protocol drift breaks RPC.
const (
	// ModeCLI drives pi with one `--mode json` process per prompt.
	ModeCLI = "cli"
	// ModeRPC drives pi with one long-lived `--mode rpc` child per session.
	ModeRPC = "rpc"
)

// Options configures the pi adapter. The zero value is usable when the `pi`
// binary is on PATH: it uses pi's own default session directory.
type Options struct {
	// Executable is the pi binary name or path. Defaults to "pi".
	Executable string
	// ResolveExecutable, when set, is consulted at each use to obtain the
	// current binary path (the platform-level agent setting). An empty result
	// falls back to Executable.
	ResolveExecutable func() string
	// SessionDir overrides pi's session storage directory. When set it is
	// passed to pi as --session-dir and scanned by ListSessions, so writes and
	// reads agree. When empty the adapter uses PI_CODING_AGENT_SESSION_DIR or
	// pi's default location for discovery and passes nothing to pi.
	SessionDir string
	// Env overrides the child environment. The zero value inherits the parent
	// process environment.
	Env []string
	// Mode selects the transport: ModeCLI (default) or ModeRPC.
	Mode string
	// Extensions lists pi extension paths passed as --extension to every RPC
	// child. An approval extension lives here: pi's core does not request tool
	// approval, so confirmations only exist when an extension asks for them.
	Extensions []string
	// VersionProbe overrides how the CLI version is obtained. When nil the
	// adapter runs `<executable> --version`. Tests inject a stub so they do
	// not have to execute a real binary.
	VersionProbe func(ctx context.Context) (string, error)
	// Now overrides the clock; tests set it for deterministic timestamps.
	Now func() time.Time
	// NewID overrides identifier generation; tests may make it deterministic.
	NewID func() (string, error)
	// RPCIdleTimeout overrides how long an RPC child may sit idle before it is
	// reclaimed. Zero uses defaultRPCIdleTimeout; a negative value disables
	// reclamation (tests that want to inspect a live child).
	RPCIdleTimeout time.Duration
	// RPCReapInterval overrides how often the idle reaper runs. Zero uses
	// defaultRPCReapInterval.
	RPCReapInterval time.Duration
}

// Adapter implements agents.AgentAdapter for the pi CLI. Each instance is a
// logical handle (id + authorized directory) that owns the directory, the
// live prompt processes or per-session RPC children, and the normalized event
// fan-out. In CLI mode pi is a one-shot process per prompt; in RPC mode each
// active session gets a long-lived child (see rpc_child.go).
type Adapter struct {
	executable        string
	resolveExecutable func() string
	sessionDir        string
	passSessionDir    bool
	env               []string
	mode              string
	extensions        []string
	rpcIdleTimeout    time.Duration
	rpcReapInterval   time.Duration
	now               func() time.Time
	newID             func() (string, error)
	versionProbe      func(ctx context.Context) (string, error)

	mu        sync.Mutex
	instances map[string]*instance
	sessions  map[sessionKey]*sessionState
}

// sessionKey scopes adapter-side session metadata to one instance.
type sessionKey struct {
	instanceID string
	sessionID  string
}

// sessionState is the small amount of platform-side metadata the adapter keeps
// for a session (a display title, a default model and any "always" approval
// rules). It is not conversation state: the transcript stays inside pi.
type sessionState struct {
	title     string
	model     string
	approvals map[string]bool
}

// Register installs a default pi adapter (executable "pi", pi's own session
// directory) into the registry. It exists so the composition root has a single
// explicit hook per adapters package; callers needing custom options use New
// and Registry.Register directly.
func Register(registry *agents.Registry) error {
	if registry == nil {
		return errors.New("pi: registry is required")
	}
	return registry.Register(agents.BackendPi, New(Options{}))
}

// New builds a pi adapter. Pi runs exclusively in RPC mode.
func New(options Options) *Adapter {
	adapter := &Adapter{
		executable:        options.Executable,
		resolveExecutable: options.ResolveExecutable,
		sessionDir:        options.SessionDir,
		env:               options.Env,
		mode:              ModeRPC,
		extensions:        options.Extensions,
		rpcIdleTimeout:    options.RPCIdleTimeout,
		rpcReapInterval:   options.RPCReapInterval,
		now:               options.Now,
		newID:             options.NewID,
		versionProbe:      options.VersionProbe,
		instances:         make(map[string]*instance),
		sessions:          make(map[sessionKey]*sessionState),
	}
	if adapter.executable == "" {
		adapter.executable = "pi"
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
	if adapter.rpcIdleTimeout == 0 {
		adapter.rpcIdleTimeout = defaultRPCIdleTimeout
	}
	if adapter.rpcReapInterval <= 0 {
		adapter.rpcReapInterval = defaultRPCReapInterval
	}
	if adapter.sessionDir != "" {
		adapter.passSessionDir = true
	} else {
		adapter.sessionDir = defaultSessionDir()
	}
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

// Capabilities declares what this adapter supports. It targets pi's event
// stream, which both `--mode json` and `--mode rpc` share: streaming text
// deltas, tool call/result records and on-disk session files. Tool
// confirmation requires the extension UI subprotocol, which only exists in
// RPC mode, so it is declared exactly when the adapter runs in RPC mode and an
// approval extension can ask for confirmation.
func (a *Adapter) Capabilities() agents.Capabilities {
	return agents.Capabilities{
		Streaming:        true,
		SessionDiscovery: a.sessionDir != "",
		SessionContinue:  true,
		Stop:             true,
		ModelSelection:   true,
		ToolCallEvents:   true,
		ToolConfirmation: true,
		Interrupt:        true,
		SessionHistory:   true,
	}
}

// Probe reports whether the pi binary is resolvable. It never starts pi.
func (a *Adapter) Probe(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	executable := a.execPath()
	path, err := a.probePath()
	if err != nil {
		return fmt.Errorf("pi: executable %q not found: %w", executable, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("pi: stat executable %q: %w", path, err)
	}
	if info.IsDir() {
		return fmt.Errorf("pi: executable %q is a directory", path)
	}
	return nil
}

// runVersionCommand asks the CLI for its version with a short timeout.
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

// Diagnostics runs the pi pre-flight checks: executable presence and CLI
// version support against the tested range. pi has no API server, so there is
// deliberately no api check; a malformed record on a prompt's event stream is
// diagnosed at runtime as a parse_error event without ending the stream.
// Messages are actionable and redacted, and nothing is started.
func (a *Adapter) Diagnostics(ctx context.Context) agents.BackendDiagnostics {
	supported := Descriptor.SupportedVersion()
	report := agents.BackendDiagnostics{
		Backend:       agents.BackendPi,
		TestedVersion: supported.Tested,
		Checks:        []agents.Diagnostic{},
		Capabilities:  capabilitiesOf(a.Capabilities()),
	}

	if err := a.Probe(ctx); err != nil {
		report.Checks = append(report.Checks, agents.Diagnostic{
			Kind:    agents.DiagnosticExecutable,
			Status:  agents.DiagnosticError,
			Message: fmt.Sprintf("%s; install the pi CLI (npm install -g @earendil-works/pi-coding-agent) or point the adapter at the correct binary", redactSecrets(err.Error())),
		})
		report.Status = agents.DiagnosticError
		return report
	}
	report.Checks = append(report.Checks, agents.Diagnostic{
		Kind:    agents.DiagnosticExecutable,
		Status:  agents.DiagnosticOK,
		Message: fmt.Sprintf("pi executable %q found", a.execPath()),
	})

	version, versionCheck := a.checkVersion(ctx, supported)
	report.Version = version
	report.Checks = append(report.Checks, versionCheck)
	report.Status = agents.AggregateStatus(report.Checks)
	return report
}

// checkVersion classifies the CLI version against the tested range. An
// undeterminable or unsupported version is a warning, not an error: the CLI
// may still work, and a real failure surfaces per prompt as an error event.
// It returns the detected version (empty when undeterminable) and the check.
func (a *Adapter) checkVersion(ctx context.Context, supported agents.SupportedVersion) (string, agents.Diagnostic) {
	raw, err := a.versionProbe(ctx)
	if err != nil {
		return "", agents.Diagnostic{
			Kind:    agents.DiagnosticVersion,
			Status:  agents.DiagnosticWarning,
			Message: fmt.Sprintf("could not determine the pi CLI version (%s); the tested version is %s", redactSecrets(err.Error()), supported.Tested),
		}
	}
	parsed, ok := agents.ParseVersion(raw)
	if !ok {
		return "", agents.Diagnostic{
			Kind:    agents.DiagnosticVersion,
			Status:  agents.DiagnosticWarning,
			Message: fmt.Sprintf("pi reported an unparseable version %q; the tested version is %s", truncate(redactSecrets(raw)), supported.Tested),
		}
	}
	if !parsed.InRange(supported) {
		return parsed.String(), agents.Diagnostic{
			Kind:    agents.DiagnosticVersion,
			Status:  agents.DiagnosticWarning,
			Message: fmt.Sprintf("pi CLI %s is outside the tested range [%s, %s); the tested version is %s; upgrade or downgrade pi if sessions fail", parsed, supported.Minimum, supported.Maximum, supported.Tested),
			Detail:  truncate(redactSecrets(raw)),
		}
	}
	return parsed.String(), agents.Diagnostic{
		Kind:    agents.DiagnosticVersion,
		Status:  agents.DiagnosticOK,
		Message: fmt.Sprintf("pi CLI %s is within the tested range [%s, %s)", parsed, supported.Minimum, supported.Maximum),
		Detail:  truncate(redactSecrets(raw)),
	}
}

// capabilitiesOf returns a pointer copy so reports cannot alias the adapter.
func capabilitiesOf(capabilities agents.Capabilities) *agents.Capabilities {
	return &capabilities
}

// StartInstance validates the authorized directory and the binary, then
// registers a running logical instance. No long-lived process is started: pi
// runs one process per prompt, launched by SendMessage.
func (a *Adapter) StartInstance(ctx context.Context, request agents.StartInstanceRequest) (*agents.Instance, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if request.Directory == "" {
		return nil, errors.New("pi: instance directory is required")
	}
	info, err := os.Stat(request.Directory)
	if err != nil {
		return nil, fmt.Errorf("pi: instance directory %q: %w", request.Directory, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("pi: instance directory %q is not a directory", request.Directory)
	}
	if err := a.Probe(ctx); err != nil {
		return nil, err
	}

	id, err := a.newID()
	if err != nil {
		return nil, fmt.Errorf("pi: allocate instance id: %w", err)
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
		procs:     make(map[*process]struct{}),
		children:  make(map[string]*rpcChild),
		starts:    make(map[string]*childStart),
	}
	inst.startReaper(a.rpcIdleTimeout, a.rpcReapInterval)

	a.mu.Lock()
	a.instances[id] = inst
	a.mu.Unlock()

	return &agents.Instance{
		ID:        id,
		Backend:   agents.BackendPi,
		Directory: request.Directory,
		Status:    agents.InstanceRunning,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

// StopInstance terminates the instance's live prompt processes and closes its
// event stream. An instance that is already gone is treated as a successful
// stop so stop stays idempotent across restarts.
func (a *Adapter) StopInstance(ctx context.Context, instanceID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	a.mu.Lock()
	inst, ok := a.instances[instanceID]
	if ok {
		delete(a.instances, instanceID)
		// Drop the instance's session metadata with the instance so the
		// adapter's session map cannot grow without bound. Doing this under
		// the same lock that CreateSession re-checks the instance under makes
		// the teardown atomic with respect to session creation.
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
	inst.stop(ctx)
	return nil
}

// Subscribe returns the instance's normalized event stream. The returned
// channel closes when the caller's context ends or the instance stops.
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

// CreateSession allocates an agent-native session id. pi creates the session
// on first use (`--session-id` creates it if missing), so no process or model
// call is needed here.
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
	id, err := a.newID()
	if err != nil {
		return nil, fmt.Errorf("pi: allocate session id: %w", err)
	}
	a.mu.Lock()
	// Re-check under the adapter lock: a stop that deleted the instance while
	// this call was in flight must not leave a session entry behind.
	if a.instances[instanceID] != inst {
		a.mu.Unlock()
		return nil, fmt.Errorf("pi: instance %q is not running", instanceID)
	}
	a.sessions[sessionKey{instanceID: instanceID, sessionID: id}] = &sessionState{title: options.Title, model: options.Model, approvals: make(map[string]bool)}
	a.mu.Unlock()

	return &agents.SessionRef{
		InstanceID: instanceID,
		Backend:    agents.BackendPi,
		SessionID:  id,
		Title:      options.Title,
	}, nil
}

// ContinueSession re-attaches to an existing session reference. pi resumes the
// named session on the next prompt, so the adapter validates the reference and
// returns it refreshed with any platform-known title.
func (a *Adapter) ContinueSession(ctx context.Context, ref agents.SessionRef) (*agents.SessionRef, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := ref.Validate(); err != nil {
		return nil, err
	}
	if ref.Backend != agents.BackendPi {
		return nil, fmt.Errorf("pi: session backend %q is not pi", ref.Backend)
	}
	inst, err := a.getInstance(ref.InstanceID)
	if err != nil {
		return nil, err
	}
	if err := inst.ensureRunning(); err != nil {
		return nil, err
	}
	if ref.Title == "" {
		ref.Title = a.sessionTitle(ref.InstanceID, ref.SessionID)
	}
	return &ref, nil
}

// SendMessage delivers one user turn to a session. In CLI mode it launches one
// `pi --mode json` process for the turn; in RPC mode it ensures the session's
// long-lived child is running and sends a prompt command. In both modes it
// returns once the turn is accepted; the reply, tool activity and diagnostics
// arrive on the subscription stream.
func (a *Adapter) SendMessage(ctx context.Context, ref agents.SessionRef, message agents.SendMessageRequest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := ref.Validate(); err != nil {
		return err
	}
	if ref.Backend != agents.BackendPi {
		return fmt.Errorf("pi: session backend %q is not pi", ref.Backend)
	}
	if strings.TrimSpace(message.Text) == "" {
		return errors.New("pi: message text is required")
	}
	inst, err := a.getInstance(ref.InstanceID)
	if err != nil {
		return err
	}
	if err := inst.ensureRunning(); err != nil {
		return err
	}

	state, _ := a.lookupSession(ref.InstanceID, ref.SessionID)
	model := message.Model
	if model == "" && state != nil {
		model = state.model
	}
	// A title the platform set after session creation (an auto-derived title
	// or a rename) rides along as --name, which pi persists as a session
	// name; remembering it keeps later launches named too. Title reads and
	// writes go through helpers that hold the adapter lock: sends for one
	// session run concurrently.
	name := ref.Title
	if name != "" {
		a.rememberSessionTitle(ref.InstanceID, ref.SessionID, name)
	} else {
		name = a.sessionTitle(ref.InstanceID, ref.SessionID)
	}

	return a.sendRPC(ctx, inst, ref, message, model, name)
}

// sendRPC delivers a turn through the session's RPC child, starting it lazily
// if this is the session's first prompt after a reclaim or restart. While a run
// is streaming, a second prompt is queued with followUp rather than rejected,
// matching pi's own queueing semantics.
func (a *Adapter) sendRPC(ctx context.Context, inst *instance, ref agents.SessionRef, message agents.SendMessageRequest, model, name string) error {
	child, err := inst.ensureChild(ctx, ref.SessionID, func() (*rpcChild, error) {
		return a.startRPCChild(inst, ref.SessionID, model, name)
	})
	if err != nil {
		return fmt.Errorf("pi: send message on instance %q: %w", ref.InstanceID, err)
	}
	defer child.touch()

	// Apply a model or name the platform changed after the child started.
	if model != "" && model != child.currentModel() {
		if err := child.setModel(ctx, model); err != nil {
			return fmt.Errorf("pi: switch model before prompt: %w", err)
		}
	}
	if name != "" && name != child.currentName() {
		if err := child.setName(ctx, name); err != nil {
			return fmt.Errorf("pi: set session name before prompt: %w", err)
		}
	}

	behavior := ""
	if child.isStreaming() {
		behavior = "followUp"
	}
	response, err := child.conn.request(ctx, newPromptCommand("", message.Text, behavior, message.Images), rpcCommandTimeout)
	if err != nil {
		return fmt.Errorf("pi: send message on instance %q: %w", ref.InstanceID, err)
	}
	if !response.Success {
		// A run may have started between our state read and the command. The
		// protocol requires an explicit behavior mid-run, so retry once as a
		// follow-up instead of surfacing a spurious failure.
		if behavior == "" && isStreamingRejection(response.Error) {
			response, err = child.conn.request(ctx, newPromptCommand("", message.Text, "followUp", message.Images), rpcCommandTimeout)
			if err != nil {
				return fmt.Errorf("pi: send message on instance %q: %w", ref.InstanceID, err)
			}
		}
		if !response.Success {
			return fmt.Errorf("pi: prompt rejected: %s", redactSecrets(firstNonEmpty(response.Error, "unknown error")))
		}
	}
	// Run state is tracked from lifecycle events (agent_start/agent_settled),
	// not set optimistically here: a fast turn can settle before this call
	// returns, and a late "streaming = true" would leave the child stuck busy.
	return nil
}

// isStreamingRejection reports whether pi refused a prompt because a run was
// already streaming and no streaming behavior was given.
func isStreamingRejection(message string) bool {
	return strings.Contains(strings.ToLower(message), "streaming")
}

// ListModels returns pi's configured models. The catalog is pi's own
// configuration — not a session's or an instance's property — so a caller with
// no instance yet gets the same answer as one that has: when an RPC child is
// already running it is asked with get_available_models (it holds the loaded
// configuration), otherwise `pi --list-models` answers in a short-lived
// process. pi's catalog does not depend on the working directory, so the
// directory is not consulted.
func (a *Adapter) ListModels(ctx context.Context, _ string) ([]agents.ModelInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if child := a.anyChild(); child != nil {
		rpcCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		if response, err := child.conn.request(rpcCtx, newGetAvailableModelsCommand(""), rpcCommandTimeout); err == nil && response.Success {
			if models, err := parseAvailableModels(response.Data); err == nil && len(models) > 0 {
				return models, nil
			}
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	command, err := a.backendCommand(ctx, "--list-models")
	if err != nil {
		return nil, fmt.Errorf("pi: list models: %s", redactSecrets(err.Error()))
	}
	out, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("pi: list models: %s", redactSecrets(err.Error()))
	}
	return parseModelList(string(out)), nil
}

// anyChild returns a live RPC child from any instance, or nil. Any child answers
// get_available_models for the whole machine, so the workbench's model list does
// not depend on which instance happens to have a session running.
func (a *Adapter) anyChild() *rpcChild {
	a.mu.Lock()
	instances := make([]*instance, 0, len(a.instances))
	for _, inst := range a.instances {
		instances = append(instances, inst)
	}
	a.mu.Unlock()
	for _, inst := range instances {
		if child := inst.anyChild(); child != nil {
			return child
		}
	}
	return nil
}

// Interrupt ends the running prompt of one session. In CLI mode the process is
// killed and its turn settles as aborted; in RPC mode an abort command asks pi
// to cancel gracefully, with the process-group kill reserved as a backstop. The
// instance and its other sessions keep running. A session with no running
// prompt is a no-op.
func (a *Adapter) Interrupt(ctx context.Context, ref agents.SessionRef) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := ref.Validate(); err != nil {
		return err
	}
	if ref.Backend != agents.BackendPi {
		return fmt.Errorf("pi: session backend %q is not pi", ref.Backend)
	}
	inst, err := a.getInstance(ref.InstanceID)
	if err != nil {
		return err
	}
	child := inst.child(ref.SessionID)
	if child == nil {
		return nil
	}
	if err := child.abort(ctx); err != nil && !errors.Is(err, errRPCClosed) {
		return fmt.Errorf("pi: interrupt session %q: %w", ref.SessionID, err)
	}
	return nil
}

// ListSessions reads pi's on-disk JSONL sessions for the instance directory.
// pi groups session files by working directory, so a project's sessions live in
// a single folder under the session root. Missing folders are not an error.
// Each reference carries the session's explicit name (if any) in Title and the
// first user message in Preview, so discovery can index a display title even
// for sessions pi never named.
func (a *Adapter) ListSessions(ctx context.Context, instanceID string) ([]agents.SessionRef, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	inst, err := a.getInstance(instanceID)
	if err != nil {
		return nil, err
	}
	if a.sessionDir == "" {
		return nil, errors.New("pi: session directory is unavailable")
	}

	dir := filepath.Join(a.sessionDir, sessionDirName(inst.directory))
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return []agents.SessionRef{}, nil
		}
		return nil, fmt.Errorf("pi: read session directory %q: %w", dir, err)
	}

	refs := make([]agents.SessionRef, 0, len(entries))
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		ref, ok := readSessionRef(filepath.Join(dir, entry.Name()), instanceID, inst.directory)
		if ok {
			refs = append(refs, ref)
		}
	}
	// pi names files with a leading timestamp, so ReadDir's lexical order is
	// already chronological; preserve it.
	return refs, nil
}

// getInstance returns a registered instance or an error.
func (a *Adapter) getInstance(instanceID string) (*instance, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	inst, ok := a.instances[instanceID]
	if !ok {
		return nil, fmt.Errorf("pi: unknown instance %q", instanceID)
	}
	return inst, nil
}

// lookupSession returns cached platform metadata for a session, if any.
func (a *Adapter) lookupSession(instanceID, sessionID string) (*sessionState, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	state, ok := a.sessions[sessionKey{instanceID: instanceID, sessionID: sessionID}]
	return state, ok
}

// sessionTitle returns the cached display title for a session (empty when
// there is none). Locks: titles are read and written from concurrent sends.
func (a *Adapter) sessionTitle(instanceID, sessionID string) string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if state, ok := a.sessions[sessionKey{instanceID: instanceID, sessionID: sessionID}]; ok {
		return state.title
	}
	return ""
}

// rememberSessionTitle caches a session's display title for later launches.
func (a *Adapter) rememberSessionTitle(instanceID, sessionID, title string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if state, ok := a.sessions[sessionKey{instanceID: instanceID, sessionID: sessionID}]; ok {
		state.title = title
	}
}

// sessionApproved reports whether an "always" approval rule already covers the
// confirmation key for a session.
func (a *Adapter) sessionApproved(instanceID, sessionID, key string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	state, ok := a.sessions[sessionKey{instanceID: instanceID, sessionID: sessionID}]
	if !ok || state.approvals == nil {
		return false
	}
	return state.approvals[key]
}

// rememberApproval records an "always" approval rule for a session. Rules are
// dropped with the instance's session metadata, so they never outlive it.
func (a *Adapter) rememberApproval(instanceID, sessionID, key string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	state, ok := a.sessions[sessionKey{instanceID: instanceID, sessionID: sessionID}]
	if !ok {
		return
	}
	if state.approvals == nil {
		state.approvals = make(map[string]bool)
	}
	state.approvals[key] = true
}

// instance is a logical pi instance: a directory, its live prompt processes,
// its per-session RPC children and its subscriber set. Sequence allocation and
// status are guarded by mu, which is never held while blocking on a subscriber
// or a process.
type instance struct {
	id        string
	directory string
	ctx       context.Context
	cancel    context.CancelFunc
	now       func() time.Time
	done      chan struct{}

	mu       sync.Mutex
	status   agents.InstanceStatus
	closed   bool
	seq      uint64
	subs     map[*subscriber]struct{}
	procs    map[*process]struct{}
	children map[string]*rpcChild
	starts   map[string]*childStart
	// turnSeq mints turn ids unique within the instance.
	turnSeq uint64
}

// childStart is an in-flight lazy start of one session's RPC child. Concurrent
// sends wait on ready instead of launching a second child.
type childStart struct {
	ready chan struct{}
	err   error
}

// ensureRunning reports whether the instance can accept work.
func (inst *instance) ensureRunning() error {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	if inst.closed || inst.status != agents.InstanceRunning {
		return fmt.Errorf("pi: instance %q is not running", inst.id)
	}
	return nil
}

// isClosed reports whether the instance has been stopped.
func (inst *instance) isClosed() bool {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	return inst.closed
}

// stop closes the instance's stream, stops its RPC children in an orderly way,
// then cancels and kills any prompt process still alive. It is idempotent.
func (inst *instance) stop(ctx context.Context) {
	inst.mu.Lock()
	if inst.closed {
		inst.mu.Unlock()
		return
	}
	inst.closed = true
	inst.status = agents.InstanceStopped
	close(inst.done)
	procs := make([]*process, 0, len(inst.procs))
	for proc := range inst.procs {
		procs = append(procs, proc)
	}
	children := make([]*rpcChild, 0, len(inst.children))
	for _, child := range inst.children {
		children = append(children, child)
	}
	inst.mu.Unlock()

	// RPC children stop orderly first: closing stdin asks pi to dispose its
	// runtime and exit, which preserves an in-progress turn's transcript. The
	// wait is bounded so a hung child cannot stall the stop.
	if len(children) > 0 {
		var wg sync.WaitGroup
		for _, child := range children {
			wg.Add(1)
			go func(c *rpcChild) {
				defer wg.Done()
				c.shutdown(ctx)
			}(child)
		}
		waited := make(chan struct{})
		go func() { wg.Wait(); close(waited) }()
		select {
		case <-waited:
		case <-ctx.Done():
		case <-time.After(2 * time.Second):
		}
	}

	// Cancel the shared context so CommandContext kills every live prompt
	// process group, then reinforce with an explicit kill.
	inst.cancel()
	for _, proc := range procs {
		proc.kill()
	}
	for _, child := range children {
		child.kill()
	}
	for _, proc := range procs {
		select {
		case <-proc.done:
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}
	}
}

// addSubscriber registers a subscriber unless the instance is already closed.
func (inst *instance) addSubscriber(sub *subscriber) error {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	if inst.closed {
		return fmt.Errorf("pi: instance %q is not running", inst.id)
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

// addProcess registers a live prompt process, reporting false when the instance
// is stopping so the caller can abandon it.
func (inst *instance) addProcess(proc *process) bool {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	if inst.closed {
		return false
	}
	inst.procs[proc] = struct{}{}
	return true
}

// attachProcess hands a freshly started process to exactly one reaper. It
// reports whether the process was registered before the instance stopped; when
// it was not, the process is killed and still reaped by runProcess, so a stop
// racing a send can never leave an unreaped (zombie) child.
func (inst *instance) attachProcess(proc *process, sessionID string) bool {
	proc.sessionID = sessionID
	registered := inst.addProcess(proc)
	if !registered {
		proc.kill()
	}
	go inst.runProcess(proc, sessionID)
	return registered
}

// interruptSession kills the running prompt processes of one session. Each is
// marked interrupted first, so its reaper settles the turn as aborted instead
// of reporting the kill as a process error.
func (inst *instance) interruptSession(sessionID string) {
	inst.mu.Lock()
	var targets []*process
	for proc := range inst.procs {
		if proc.sessionID == sessionID {
			targets = append(targets, proc)
		}
	}
	inst.mu.Unlock()
	for _, proc := range targets {
		proc.interrupted.Store(true)
		proc.kill()
	}
}

// removeProcess drops a reaped process.
func (inst *instance) removeProcess(proc *process) {
	inst.mu.Lock()
	delete(inst.procs, proc)
	inst.mu.Unlock()
}

// child returns the live RPC child for a session, or nil when none exists.
func (inst *instance) child(sessionID string) *rpcChild {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	child := inst.children[sessionID]
	if child != nil && (child.dead.Load() || child.retiring.Load()) {
		return nil
	}
	return child
}

// anyChild returns any live RPC child on the instance, or nil. It backs
// instance-scoped operations (get_available_models) that any session's child
// can answer.
func (inst *instance) anyChild() *rpcChild {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	for _, child := range inst.children {
		if !child.dead.Load() && !child.retiring.Load() {
			return child
		}
	}
	return nil
}

// ensureChild returns the session's RPC child, starting one lazily on first
// use. Concurrent sends for one session share a single start; a child that
// died is replaced with a fresh process bound to the same session id, so the
// transcript on disk continues where it left off.
func (inst *instance) ensureChild(ctx context.Context, sessionID string, start func() (*rpcChild, error)) (*rpcChild, error) {
	for {
		inst.mu.Lock()
		if inst.closed {
			inst.mu.Unlock()
			return nil, fmt.Errorf("pi: instance %q is not running", inst.id)
		}
		if child, ok := inst.children[sessionID]; ok && !child.dead.Load() && !child.retiring.Load() {
			inst.mu.Unlock()
			return child, nil
		}
		if inFlight, ok := inst.starts[sessionID]; ok {
			inst.mu.Unlock()
			select {
			case <-inFlight.ready:
				if inFlight.err != nil {
					return nil, inFlight.err
				}
				continue
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		startEntry := &childStart{ready: make(chan struct{})}
		inst.starts[sessionID] = startEntry
		inst.mu.Unlock()

		child, err := start()

		inst.mu.Lock()
		startEntry.err = err
		if err == nil {
			if child.dead.Load() {
				startEntry.err = errRPCClosed
			} else {
				inst.children[sessionID] = child
			}
		}
		delete(inst.starts, sessionID)
		inst.mu.Unlock()
		close(startEntry.ready)

		if startEntry.err != nil {
			return nil, startEntry.err
		}
		return child, nil
	}
}

// removeChild unregisters a child that exited, if it is still the registered
// one (a replacement may already have taken its place).
func (inst *instance) removeChild(child *rpcChild) {
	inst.mu.Lock()
	if inst.children[child.sessionID] == child {
		delete(inst.children, child.sessionID)
	}
	inst.mu.Unlock()
}

// startReaper launches the idle reaper. A non-positive timeout disables it.
func (inst *instance) startReaper(idleTimeout, interval time.Duration) {
	if idleTimeout <= 0 {
		return
	}
	if interval <= 0 {
		interval = defaultRPCReapInterval
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-inst.done:
				return
			case <-ticker.C:
				inst.reapIdle(idleTimeout)
			}
		}
	}()
}

// reapIdle shuts down RPC children that have been unused past the timeout and
// are not mid-run. Reclamation runs in the background so it never blocks the
// ticker.
func (inst *instance) reapIdle(idleTimeout time.Duration) {
	inst.mu.Lock()
	var idle []*rpcChild
	for _, child := range inst.children {
		if child.idle(idleTimeout) {
			idle = append(idle, child)
		}
	}
	inst.mu.Unlock()
	for _, child := range idle {
		// Only the reaper that claims the child shuts it down, so a send
		// that read the map concurrently starts a fresh child instead of
		// using a child whose stdin is about to close.
		if child.beginRetire() {
			go child.shutdown(context.Background())
		}
	}
}

// publish stamps the envelope, assigns the next per-instance sequence and
// delivers the event to every current subscriber. Delivery honors backpressure:
// a subscriber that cannot keep up blocks its own copy, which in turn applies
// pipe backpressure to the pi process.
func (inst *instance) publish(event agents.NormalizedEvent) {
	inst.mu.Lock()
	if inst.closed {
		inst.mu.Unlock()
		return
	}
	inst.seq++
	event.Sequence = inst.seq
	event.InstanceID = inst.id
	event.Backend = agents.BackendPi
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

// runProcess drains one prompt process's stdout and stderr into the instance
// stream, reaps it, and reports a non-zero exit as an error event (unless the
// instance is stopping, in which case the kill is expected).
func (inst *instance) runProcess(proc *process, sessionID string) {
	stdoutParser := newParser(inst.id, sessionID, inst.now)
	stdoutParser.newTurnID = func() string { return inst.nextTurnID(sessionID) }
	stderrParser := newParser(inst.id, sessionID, inst.now)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_ = stdoutParser.run(proc.stdout, inst.publish)
	}()
	go func() {
		defer wg.Done()
		_ = stderrParser.runStderr(proc.stderr, inst.publish)
	}()
	wg.Wait()

	waitErr := proc.wait()
	inst.removeProcess(proc)
	if proc.interrupted.Load() {
		if turn, open := stdoutParser.endTurn(agents.TurnCancelled); open && !inst.isClosed() {
			inst.publish(turn)
		}
		return
	}
	if turn, open := stdoutParser.endTurn(agents.TurnFailed); open && !inst.isClosed() {
		inst.publish(turn)
	}
	if waitErr != nil && !inst.isClosed() {
		inst.publish(agents.NormalizedEvent{
			Type:      agents.EventError,
			Error:     &agents.ErrorPayload{Code: "process_exit", Message: redactSecrets(waitErr.Error())},
			SessionID: sessionID,
			Timestamp: inst.now(),
		})
	}
}

// nextTurnID mints a turn id unique within the instance.
func (inst *instance) nextTurnID(sessionID string) string {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	inst.turnSeq++
	return fmt.Sprintf("%s-turn-%d", inst.id, inst.turnSeq)
}

// subscriber is one consumer of an instance's stream. stash is written by
// publishers and never closed; out is owned and closed by forward, so a
// concurrent stop can never race a close against a send.
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

// piDeliverTimeout bounds how long a torn-down subscriber waits to hand an
// already-queued terminal event to its consumer before giving up.
const piDeliverTimeout = 100 * time.Millisecond

// forward copies the subscriber's stash to its output and closes the output
// when the subscription ends. On teardown it flushes events already queued so a
// terminal event is delivered rather than dropped.
func (s *subscriber) forward() {
	defer close(s.out)
	for {
		select {
		case <-s.done:
			s.flush()
			return
		case event := <-s.stash:
			select {
			case s.out <- event:
			case <-s.done:
				s.deliver(event)
				s.flush()
				return
			}
		}
	}
}

// deliver sends one event to the output, giving up after a short bound.
func (s *subscriber) deliver(event agents.NormalizedEvent) {
	select {
	case s.out <- event:
	case <-time.After(piDeliverTimeout):
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

// promptOptions describes the arguments for one `pi --mode json` invocation.
type promptOptions struct {
	sessionID      string
	sessionDir     string
	passSessionDir bool
	model          string
	name           string
	text           string
}

// promptArgs builds pi's argv for a one-shot prompt. The message is passed as a
// single argv element after `--`, so leading dashes in user text can never be
// interpreted as options and nothing is ever handed to a shell.
func promptArgs(options promptOptions) []string {
	args := []string{"--mode", "json"}
	if options.sessionDir != "" && options.passSessionDir {
		args = append(args, "--session-dir", options.sessionDir)
	}
	if options.sessionID != "" {
		args = append(args, "--session-id", options.sessionID)
	}
	if options.model != "" {
		args = append(args, "--model", options.model)
	}
	if options.name != "" {
		args = append(args, "--name", options.name)
	}
	return append(args, "--", options.text)
}

// sessionDirName reproduces pi's session-folder naming: the leading separator
// is removed and path separators and colons become dashes, wrapped in double
// dashes. For /data/projects/demo it yields --data-projects-demo--.
func sessionDirName(directory string) string {
	trimmed := strings.TrimLeft(directory, `/\`)
	replacer := strings.NewReplacer("/", "-", `\`, "-", ":", "-")
	return "--" + replacer.Replace(trimmed) + "--"
}

// readSessionRef reads a pi session file's header, display name and first-user-
// message preview without loading the whole conversation. It returns false for
// files that do not look like a session for the expected directory.
func readSessionRef(path, instanceID, expectedDirectory string) (agents.SessionRef, bool) {
	file, err := os.Open(path)
	if err != nil {
		return agents.SessionRef{}, false
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	if !scanner.Scan() {
		return agents.SessionRef{}, false
	}
	var header struct {
		Type string `json:"type"`
		ID   string `json:"id"`
		Cwd  string `json:"cwd"`
	}
	if err := json.Unmarshal(scanner.Bytes(), &header); err != nil || header.Type != "session" || header.ID == "" {
		return agents.SessionRef{}, false
	}
	if !sameDirectory(header.Cwd, expectedDirectory) {
		return agents.SessionRef{}, false
	}

	ref := agents.SessionRef{InstanceID: instanceID, Backend: agents.BackendPi, SessionID: header.ID}
	var scanned int
	for scanner.Scan() {
		scanned += len(scanner.Bytes())
		if scanned > maxSessionScanBytes {
			break
		}
		var entry struct {
			Type    string `json:"type"`
			Name    string `json:"name"`
			Message *struct {
				Role    string          `json:"role"`
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue
		}
		switch entry.Type {
		case "session_info":
			ref.Title = entry.Name
		case "message":
			// pi never writes an automatic name, so the first user
			// message is the best display text discovery can offer.
			if ref.Preview == "" && entry.Message != nil && entry.Message.Role == "user" {
				var texts []string
				for _, block := range decodeHistoryContent(entry.Message.Content) {
					if block.Type == "text" {
						texts = append(texts, block.Text)
					}
				}
				ref.Preview = agents.DeriveTitle(strings.Join(texts, " "))
			}
		}
	}
	return ref, true
}

// sameDirectory compares two directories after cleaning and symlink-resolving
// them, so discovery is not fooled by a trailing slash or a symlinked root.
func sameDirectory(a, b string) bool {
	if a == b {
		return true
	}
	resolvedA, errA := filepath.EvalSymlinks(filepath.Clean(a))
	resolvedB, errB := filepath.EvalSymlinks(filepath.Clean(b))
	if errA != nil || errB != nil {
		return filepath.Clean(a) == filepath.Clean(b)
	}
	return resolvedA == resolvedB
}

// defaultSessionDir mirrors pi's precedence for its session root.
func defaultSessionDir() string {
	if dir := os.Getenv("PI_CODING_AGENT_SESSION_DIR"); dir != "" {
		return dir
	}
	home, err := platform.New().Dirs().HomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".pi", "agent", "sessions")
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

// DetectTurn reports whether the native session is actively running a turn.
func (a *Adapter) DetectTurn(ctx context.Context, ref agents.SessionRef) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if err := ref.Validate(); err != nil {
		return false, err
	}
	if ref.Backend != agents.BackendPi {
		return false, fmt.Errorf("pi: session backend %q is not pi", ref.Backend)
	}
	inst, err := a.getInstance(ref.InstanceID)
	if err != nil {
		return false, err
	}
	child := inst.child(ref.SessionID)
	if child == nil {
		return false, nil
	}
	return child.isStreaming(), nil
}

