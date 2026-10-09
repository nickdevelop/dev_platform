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

	"devplatform/internal/agents"
)

// Compile-time proof that the pi adapter satisfies the platform contract.
var _ agents.AgentAdapter = (*Adapter)(nil)

// maxSessionScanBytes bounds how much of a session file is scanned for its
// display name. Discovery reads the header plus a bounded prefix; it never
// loads a whole conversation into memory.
const maxSessionScanBytes = 4 * 1024 * 1024

// Options configures the pi adapter. The zero value is usable when the `pi`
// binary is on PATH: it uses pi's own default session directory.
type Options struct {
	// Executable is the pi binary name or path. Defaults to "pi".
	Executable string
	// SessionDir overrides pi's session storage directory. When set it is
	// passed to pi as --session-dir and scanned by ListSessions, so writes and
	// reads agree. When empty the adapter uses PI_CODING_AGENT_SESSION_DIR or
	// pi's default location for discovery and passes nothing to pi.
	SessionDir string
	// Env overrides the child environment. The zero value inherits the parent
	// process environment.
	Env []string
	// VersionProbe overrides how the CLI version is obtained. When nil the
	// adapter runs `<executable> --version`. Tests inject a stub so they do
	// not have to execute a real binary.
	VersionProbe func(ctx context.Context) (string, error)
	// Now overrides the clock; tests set it for deterministic timestamps.
	Now func() time.Time
	// NewID overrides identifier generation; tests may make it deterministic.
	NewID func() (string, error)
}

// Adapter implements agents.AgentAdapter for the pi CLI. Each instance is a
// logical handle (id + authorized directory): pi itself is a one-shot process
// per prompt, so an instance owns the directory, the live prompt processes and
// the normalized event fan-out rather than a long-lived daemon.
type Adapter struct {
	executable     string
	sessionDir     string
	passSessionDir bool
	env            []string
	now            func() time.Time
	newID          func() (string, error)
	versionProbe   func(ctx context.Context) (string, error)

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
// for a session (a display title and a default model). It is not conversation
// state: the transcript stays inside pi.
type sessionState struct {
	title string
	model string
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

// New builds a pi adapter.
func New(options Options) *Adapter {
	adapter := &Adapter{
		executable:   options.Executable,
		sessionDir:   options.SessionDir,
		env:          options.Env,
		now:          options.Now,
		newID:        options.NewID,
		versionProbe: options.VersionProbe,
		instances:    make(map[string]*instance),
		sessions:     make(map[sessionKey]*sessionState),
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
	if adapter.sessionDir != "" {
		adapter.passSessionDir = true
	} else {
		adapter.sessionDir = defaultSessionDir()
	}
	return adapter
}

// Capabilities declares what this adapter supports. It targets pi's documented
// `--mode json` protocol: streaming text deltas, tool call/result records and
// on-disk session files are all part of that protocol, so the adapter can
// truthfully expose them. Print mode cannot pause a tool call for confirmation,
// so ToolConfirmation stays false.
func (a *Adapter) Capabilities() agents.Capabilities {
	return agents.Capabilities{
		Streaming:        true,
		SessionDiscovery: a.sessionDir != "",
		SessionContinue:  true,
		Stop:             true,
		ModelSelection:   true,
		ToolCallEvents:   true,
		ToolConfirmation: false,
	}
}

// Probe reports whether the pi binary is resolvable. It never starts pi.
func (a *Adapter) Probe(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	path, err := exec.LookPath(a.executable)
	if err != nil {
		return fmt.Errorf("pi: executable %q not found: %w", a.executable, err)
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
	out, err := exec.CommandContext(ctx, a.executable, "--version").Output()
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
	supported, _ := agents.SupportedVersionFor(agents.BackendPi)
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
		Message: fmt.Sprintf("pi executable %q found", a.executable),
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
	}

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
	a.sessions[sessionKey{instanceID: instanceID, sessionID: id}] = &sessionState{title: options.Title, model: options.Model}
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
		if state, ok := a.lookupSession(ref.InstanceID, ref.SessionID); ok {
			ref.Title = state.title
		}
	}
	return &ref, nil
}

// SendMessage launches one `pi --mode json` process for the turn. It returns
// once the process is accepted; the reply, tool activity and diagnostics arrive
// on the subscription stream. The process outlives the call and is bound to the
// instance's lifetime, so StopInstance terminates it.
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
	name := ""
	if state != nil {
		name = state.title
	}

	spec := processSpec{
		executable: a.executable,
		args: promptArgs(promptOptions{
			sessionID:      ref.SessionID,
			sessionDir:     a.sessionDir,
			passSessionDir: a.passSessionDir,
			model:          model,
			name:           name,
			text:           message.Text,
		}),
		dir: inst.directory,
		env: a.env,
	}
	proc, err := startProcess(inst.ctx, spec)
	if err != nil {
		return fmt.Errorf("pi: send message on instance %q: %w", ref.InstanceID, err)
	}
	if !inst.attachProcess(proc, ref.SessionID) {
		return fmt.Errorf("pi: instance %q is stopping", ref.InstanceID)
	}
	return nil
}

// ListSessions reads pi's on-disk JSONL sessions for the instance directory.
// pi groups session files by working directory, so a project's sessions live in
// a single folder under the session root. Missing folders are not an error.
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

// instance is a logical pi instance: a directory, its live prompt processes and
// its subscriber set. Sequence allocation and status are guarded by mu, which
// is never held while blocking on a subscriber or a process.
type instance struct {
	id        string
	directory string
	ctx       context.Context
	cancel    context.CancelFunc
	now       func() time.Time
	done      chan struct{}

	mu     sync.Mutex
	status agents.InstanceStatus
	closed bool
	seq    uint64
	subs   map[*subscriber]struct{}
	procs  map[*process]struct{}
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

// stop closes the instance's stream, cancels its prompt processes and waits
// briefly for them to be reaped. It is idempotent.
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
	inst.mu.Unlock()

	// Cancel the shared context so CommandContext kills every live prompt
	// process group, then reinforce with an explicit kill.
	inst.cancel()
	for _, proc := range procs {
		proc.kill()
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
	registered := inst.addProcess(proc)
	if !registered {
		proc.kill()
	}
	go inst.runProcess(proc, sessionID)
	return registered
}

// removeProcess drops a reaped process.
func (inst *instance) removeProcess(proc *process) {
	inst.mu.Lock()
	delete(inst.procs, proc)
	inst.mu.Unlock()
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
	if waitErr != nil && !inst.isClosed() {
		inst.publish(agents.NormalizedEvent{
			Type:      agents.EventError,
			Error:     &agents.ErrorPayload{Code: "process_exit", Message: redactSecrets(waitErr.Error())},
			SessionID: sessionID,
			Timestamp: inst.now(),
		})
	}
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

// forward copies the subscriber's stash to its output and closes the output
// when the subscription ends.
func (s *subscriber) forward() {
	defer close(s.out)
	for {
		select {
		case <-s.done:
			return
		case event := <-s.stash:
			select {
			case s.out <- event:
			case <-s.done:
				return
			}
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

// readSessionRef reads a pi session file's header and display name without
// loading the whole conversation. It returns false for files that do not look
// like a session for the expected directory.
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
			Type string `json:"type"`
			Name string `json:"name"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &entry); err == nil && entry.Type == "session_info" {
			ref.Title = entry.Name
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
	home, err := os.UserHomeDir()
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
