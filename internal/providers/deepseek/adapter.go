package deepseek

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"devplatform/internal/core/agents"
)

var (
	_ agents.AgentAdapter        = (*Adapter)(nil)
	_ agents.SessionTurnDetector = (*Adapter)(nil)
	_ agents.HistoryReader       = (*Adapter)(nil)
	_ agents.PermissionReplier   = (*Adapter)(nil)
	_ agents.CommandDiscoverer   = (*Adapter)(nil)
	_ agents.Diagnostician       = (*Adapter)(nil)
)

// Options configures the deepseek adapter.
type Options struct {
	// Executable is the binary name or path. Defaults to "dsh".
	Executable string
	// ResolveExecutable, when set, is consulted at each use to obtain the current binary path.
	ResolveExecutable func() string
	// WorkDir is the working directory the shared dsh host starts in.
	WorkDir string
	// Env overrides the child environment.
	Env []string
	// IdleTimeout is how long the shared host sits idle before being reclaimed.
	IdleTimeout time.Duration
	// StartupTimeout is the deadline for dsh startup.
	StartupTimeout time.Duration
	// VersionProbe overrides version detection for tests.
	VersionProbe func(ctx context.Context) (string, error)
	// ProcessLauncher overrides process spawning for tests.
	ProcessLauncher func(ctx context.Context) (*Process, error)
}

// Adapter implements agents.AgentAdapter for DeepSeek Harness.
type Adapter struct {
	executable        string
	resolveExecutable func() string
	versionProbe      func(ctx context.Context) (string, error)
	host              *SharedHost
	permissions       *PermissionsManager

	mu          sync.Mutex
	instances   map[string]*instanceState
	subscribers map[string]map[chan agents.NormalizedEvent]struct{}
}

type instanceState struct {
	id        string
	directory string
	createdAt time.Time
	updatedAt time.Time
}

// New creates an initialized deepseek adapter.
func New(opts Options) *Adapter {
	execName := opts.Executable
	if execName == "" {
		execName = "dsh"
	}

	host := NewSharedHost(SharedHostOptions{
		ResolveExecutable: opts.ResolveExecutable,
		WorkDir:           opts.WorkDir,
		Env:               opts.Env,
		IdleTimeout:       opts.IdleTimeout,
		StartupTimeout:    opts.StartupTimeout,
		ProcessLauncher:   opts.ProcessLauncher,
	})

	return &Adapter{
		executable:        execName,
		resolveExecutable: opts.ResolveExecutable,
		versionProbe:      opts.VersionProbe,
		host:              host,
		permissions:       NewPermissionsManager(),
		instances:         make(map[string]*instanceState),
		subscribers:       make(map[string]map[chan agents.NormalizedEvent]struct{}),
	}
}

func (a *Adapter) execPath() string {
	if a.resolveExecutable != nil {
		if p := a.resolveExecutable(); p != "" {
			return p
		}
	}
	return a.executable
}

// Capabilities declares what deepseek supports.
func (a *Adapter) Capabilities() agents.Capabilities {
	return agents.Capabilities{
		Streaming:        true,
		SessionDiscovery: true,
		SessionContinue:  true,
		Stop:             true,
		ModelSelection:   true,
		ToolCallEvents:   true,
		ToolConfirmation: true,
		Interrupt:        true,
		SessionHistory:   true,
		RawEvents:        false,
	}
}

// Probe reports whether the dsh binary is usable on this machine and meets version requirements.
// Probe reports whether the dsh binary is resolvable. It never starts dsh.
func (a *Adapter) Probe(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	executable := a.execPath()
	path, err := a.probePath()
	if err != nil {
		return fmt.Errorf("deepseek: executable %q not found: %w", executable, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("deepseek: stat executable %q: %w", path, err)
	}
	if info.IsDir() {
		return fmt.Errorf("deepseek: executable %q is a directory", path)
	}
	return nil
}

// Diagnostics reports whether dsh is installed and its version satisfies the tested range.
func (a *Adapter) Diagnostics(ctx context.Context) agents.BackendDiagnostics {
	supported := Descriptor.SupportedVersion()
	report := agents.BackendDiagnostics{
		Backend:       agents.BackendDeepSeek,
		TestedVersion: supported.Tested,
		Checks:        []agents.Diagnostic{},
		Capabilities:  capabilitiesOf(a.Capabilities()),
	}

	if err := a.Probe(ctx); err != nil {
		report.Checks = append(report.Checks, agents.Diagnostic{
			Kind:    agents.DiagnosticExecutable,
			Status:  agents.DiagnosticError,
			Message: fmt.Sprintf("%s; install dsh CLI (npm install -g @deepseek-ai/dsh) or point the adapter at the correct binary", redactSecrets(err.Error())),
		})
		report.Status = agents.DiagnosticError
		return report
	}
	report.Checks = append(report.Checks, agents.Diagnostic{
		Kind:    agents.DiagnosticExecutable,
		Status:  agents.DiagnosticOK,
		Message: fmt.Sprintf("dsh executable %q found", a.execPath()),
	})

	version, versionCheck := a.checkVersion(ctx, supported)
	report.Version = version
	report.Checks = append(report.Checks, versionCheck)
	report.Status = agents.AggregateStatus(report.Checks)
	return report
}

func (a *Adapter) checkVersion(ctx context.Context, supported agents.SupportedVersion) (string, agents.Diagnostic) {
	raw, err := a.runVersion(ctx)
	if err != nil {
		return "", agents.Diagnostic{
			Kind:    agents.DiagnosticVersion,
			Status:  agents.DiagnosticWarning,
			Message: fmt.Sprintf("could not determine the dsh CLI version (%s); the tested version is %s", redactSecrets(err.Error()), supported.Tested),
		}
	}
	parsed, ok := agents.ParseVersion(raw)
	if !ok {
		return "", agents.Diagnostic{
			Kind:    agents.DiagnosticVersion,
			Status:  agents.DiagnosticWarning,
			Message: fmt.Sprintf("dsh reported an unparseable version %q; the tested version is %s", truncate(redactSecrets(raw)), supported.Tested),
		}
	}
	if !parsed.InRange(supported) {
		return fmt.Sprintf("%d.%d.%d", parsed.Major, parsed.Minor, parsed.Patch), agents.Diagnostic{
			Kind:    agents.DiagnosticVersion,
			Status:  agents.DiagnosticWarning,
			Message: fmt.Sprintf("dsh CLI %d.%d.%d is outside the tested range [%s, %s); the tested version is %s", parsed.Major, parsed.Minor, parsed.Patch, supported.Minimum, supported.Maximum, supported.Tested),
			Detail:  truncate(redactSecrets(raw)),
		}
	}
	return fmt.Sprintf("%d.%d.%d", parsed.Major, parsed.Minor, parsed.Patch), agents.Diagnostic{
		Kind:    agents.DiagnosticVersion,
		Status:  agents.DiagnosticOK,
		Message: fmt.Sprintf("dsh CLI %d.%d.%d is within the tested range [%s, %s)", parsed.Major, parsed.Minor, parsed.Patch, supported.Minimum, supported.Maximum),
		Detail:  truncate(redactSecrets(raw)),
	}
}

func (a *Adapter) runVersion(ctx context.Context) (string, error) {
	if a.versionProbe != nil {
		return a.versionProbe(ctx)
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	cmd, err := a.backendCommand(ctx, "--version")
	if err != nil {
		return "", err
	}
	out, err := cmd.Output()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && len(exitErr.Stderr) > 0 {
		return "", fmt.Errorf("%w: %s", err, truncate(string(exitErr.Stderr)))
	}
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func capabilitiesOf(c agents.Capabilities) *agents.Capabilities {
	copy := c
	return &copy
}

func redactSecrets(s string) string {
	return agents.RedactSecrets(s)
}

func truncate(s string) string {
	const maxLen = 200
	if len(s) > maxLen {
		return s[:maxLen] + "..."
	}
	return s
}

// StartInstance starts a logical instance scoped to the requested working directory.
func (a *Adapter) StartInstance(ctx context.Context, request agents.StartInstanceRequest) (*agents.Instance, error) {
	if request.Directory == "" {
		return nil, fmt.Errorf("%w: directory is empty", agents.ErrInvalidInstance)
	}

	// Verify that shared host can start and accept connections
	proc, release, err := a.host.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("start deepseek instance: %w", err)
	}
	defer release()

	now := time.Now().UTC()
	id := fmt.Sprintf("inst-dsh-%s", newRandomID())

	inst := &instanceState{
		id:        id,
		directory: request.Directory,
		createdAt: now,
		updatedAt: now,
	}

	a.mu.Lock()
	a.instances[id] = inst
	a.subscribers[id] = make(map[chan agents.NormalizedEvent]struct{})
	a.mu.Unlock()

	return &agents.Instance{
		ID:        id,
		Backend:   agents.BackendDeepSeek,
		Directory: request.Directory,
		Status:    agents.InstanceRunning,
		Endpoint:  proc.Client().Endpoint(),
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

// StopInstance stops the instance record and closes active subscriptions.
func (a *Adapter) StopInstance(ctx context.Context, instanceID string) error {
	a.mu.Lock()
	inst, exists := a.instances[instanceID]
	if !exists {
		a.mu.Unlock()
		return fmt.Errorf("%w: instance %q not found", agents.ErrInvalidInstance, instanceID)
	}
	delete(a.instances, instanceID)
	subs := a.subscribers[instanceID]
	delete(a.subscribers, instanceID)
	a.mu.Unlock()

	for ch := range subs {
		close(ch)
	}

	_ = inst
	return nil
}

// ListSessions returns existing native sessions whose cwd matches the instance directory.
func (a *Adapter) ListSessions(ctx context.Context, instanceID string) ([]agents.SessionRef, error) {
	a.mu.Lock()
	inst, exists := a.instances[instanceID]
	a.mu.Unlock()
	if !exists {
		return nil, fmt.Errorf("%w: instance %q not found", agents.ErrInvalidInstance, instanceID)
	}

	proc, release, err := a.host.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer release()

	client := proc.Client()

	var res struct {
		Items []struct {
			SessionID   string `json:"sessionId"`
			CWD         string `json:"cwd"`
			Blank       bool   `json:"blank"`
			Projections *struct {
				Values struct {
					Title *string `json:"title"`
				} `json:"values"`
			} `json:"projections,omitempty"`
		} `json:"items"`
	}

	if err := client.Call(ctx, "session/list", map[string]any{"_request": map[string]any{}}, &res); err != nil {
		return nil, fmt.Errorf("list dsh sessions: %w", err)
	}

	var refs []agents.SessionRef
	for _, item := range res.Items {
		if item.Blank {
			continue
		}
		// Match session working directory to instance directory
		if inst.directory != "" && item.CWD != "" && item.CWD != inst.directory {
			continue
		}

		title := ""
		if item.Projections != nil && item.Projections.Values.Title != nil {
			title = *item.Projections.Values.Title
		}

		refs = append(refs, agents.SessionRef{
			InstanceID: instanceID,
			Backend:    agents.BackendDeepSeek,
			SessionID:  item.SessionID,
			Title:      title,
		})
	}

	return refs, nil
}

// CreateSession creates a new agent-native session in dsh.
func (a *Adapter) CreateSession(ctx context.Context, instanceID string, options agents.CreateSessionOptions) (*agents.SessionRef, error) {
	a.mu.Lock()
	inst, exists := a.instances[instanceID]
	a.mu.Unlock()
	if !exists {
		return nil, fmt.Errorf("%w: instance %q not found", agents.ErrInvalidInstance, instanceID)
	}

	proc, release, err := a.host.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer release()

	client := proc.Client()

	req := map[string]any{
		"cwd":         inst.directory,
		"agentPreset": "standard",
	}

	var res struct {
		SessionID string `json:"sessionId"`
	}
	if err := client.Call(ctx, "session/create", map[string]any{"request": req}, &res); err != nil {
		return nil, fmt.Errorf("create dsh session: %w", err)
	}

	sessionID := res.SessionID

	// Select model if requested
	if options.Model != "" {
		_ = a.selectModel(ctx, client, sessionID, options.Model)
	}

	// Rename session if title requested
	if options.Title != "" {
		_ = client.Call(ctx, "session/rename", map[string]any{
			"sessionId": sessionID,
			"title":     options.Title,
		}, nil)
	}

	return &agents.SessionRef{
		InstanceID: instanceID,
		Backend:    agents.BackendDeepSeek,
		SessionID:  sessionID,
		Title:      options.Title,
	}, nil
}

// ContinueSession verifies connection to an existing session and returns its reference.
func (a *Adapter) ContinueSession(ctx context.Context, ref agents.SessionRef) (*agents.SessionRef, error) {
	if err := ref.Validate(); err != nil {
		return nil, err
	}

	proc, release, err := a.host.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer release()

	client := proc.Client()

	var proj struct {
		Values struct {
			Title *string `json:"title"`
		} `json:"values"`
	}
	if err := client.Call(ctx, "session/projections", map[string]any{
		"request": map[string]any{"sessionId": ref.SessionID},
	}, &proj); err != nil {
		return nil, fmt.Errorf("continue dsh session: %w", err)
	}

	title := ref.Title
	if proj.Values.Title != nil && *proj.Values.Title != "" {
		title = *proj.Values.Title
	}

	return &agents.SessionRef{
		InstanceID: ref.InstanceID,
		Backend:    agents.BackendDeepSeek,
		SessionID:  ref.SessionID,
		Title:      title,
	}, nil
}

// SendMessage delivers one user turn to the native session.
func (a *Adapter) SendMessage(ctx context.Context, ref agents.SessionRef, message agents.SendMessageRequest) error {
	if err := ref.Validate(); err != nil {
		return err
	}

	proc, release, err := a.host.Acquire(ctx)
	if err != nil {
		return err
	}
	defer release()

	client := proc.Client()

	if message.Model != "" {
		if err := a.selectModel(ctx, client, ref.SessionID, message.Model); err != nil {
			return fmt.Errorf("select model before prompt: %w", err)
		}
	}

	var contentBlocks []any
	if message.Text != "" {
		contentBlocks = append(contentBlocks, map[string]any{
			"type": "text",
			"text": message.Text,
		})
	}
	for _, img := range message.Images {
		contentBlocks = append(contentBlocks, map[string]any{
			"type":      "image",
			"data":      img.Data,
			"mimeType":  img.MediaType,
		})
	}

	req := map[string]any{
		"sessionId": ref.SessionID,
		"requestId": fmt.Sprintf("req-%s", newRandomID()),
		"mode":      "queue",
		"content":   contentBlocks,
	}

	var res struct {
		Accepted bool `json:"accepted"`
	}
	if err := client.Call(ctx, "session/prompt", map[string]any{"request": req}, &res); err != nil {
		return fmt.Errorf("send message to dsh: %w", err)
	}
	if !res.Accepted {
		return errors.New("dsh did not accept the prompt")
	}

	return nil
}

func (a *Adapter) selectModel(ctx context.Context, client *Client, sessionID, model string) error {
	parts := strings.SplitN(model, "/", 2)
	provider := "commanccode"
	modelName := model
	if len(parts) == 2 {
		provider = parts[0]
		modelName = parts[1]
	}
	if provider == "deepseek" {
		provider = "commanccode"
		modelName = model
	}

	return client.Call(ctx, "session/selectModel", map[string]any{
		"request": map[string]any{
			"sessionId": sessionID,
			"provider":  provider,
			"model":     modelName,
		},
	}, nil)
}

// Subscribe returns a channel delivering normalized events for the given instance.
func (a *Adapter) Subscribe(ctx context.Context, instanceID string) (<-chan agents.NormalizedEvent, error) {
	a.mu.Lock()
	_, exists := a.instances[instanceID]
	if !exists {
		a.mu.Unlock()
		return nil, fmt.Errorf("%w: instance %q not found", agents.ErrInvalidInstance, instanceID)
	}

	outCh := make(chan agents.NormalizedEvent, 100)
	a.subscribers[instanceID][outCh] = struct{}{}
	a.mu.Unlock()

	proc, release, err := a.host.Acquire(ctx)
	if err != nil {
		a.mu.Lock()
		delete(a.subscribers[instanceID], outCh)
		close(outCh)
		a.mu.Unlock()
		return nil, err
	}

	client := proc.Client()

	// In dsh, we listen to control streams and session follow
	// We dynamically listen to session events via $events and session/control
	go func() {
		defer release()
		defer func() {
			a.mu.Lock()
			if subs, ok := a.subscribers[instanceID]; ok {
				delete(subs, outCh)
			}
			close(outCh)
			a.mu.Unlock()
		}()

		// Subscribe to session/control
		_, controlCh, cancelControl, err := client.Subscribe(ctx, "session/control", map[string]any{})
		if err != nil {
			return
		}
		defer cancelControl()

		// Stream multiplexer loop
		type followHandle struct {
			cancel     func()
			normalizer *SessionEventNormalizer
		}
		activeFollows := make(map[string]followHandle)
		defer func() {
			for _, h := range activeFollows {
				h.cancel()
			}
		}()

		followSession := func(sessionID string) {
			if _, exists := activeFollows[sessionID]; exists {
				return
			}
			normalizer := NewSessionEventNormalizer(instanceID, sessionID)
			_, followCh, cancelFollow, fErr := client.Subscribe(ctx, "session/follow", map[string]any{
				"request": map[string]any{
					"address": map[string]any{
						"kind":      "session",
						"sessionId": sessionID,
					},
					"assistantStream": true,
				},
			})
			if fErr != nil {
				return
			}
			activeFollows[sessionID] = followHandle{cancel: cancelFollow, normalizer: normalizer}

			go func() {
				for frameBytes := range followCh {
					evs := normalizer.NormalizeFrame(frameBytes)
					for _, ev := range evs {
						select {
						case outCh <- ev:
						case <-ctx.Done():
							return
						}
					}
				}
			}()
		}

		for {
			select {
			case <-ctx.Done():
				return
			case controlMsg, ok := <-controlCh:
				if !ok {
					return
				}
				var ctrl struct {
					Type      string `json:"type"`
					SessionID string `json:"sessionId"`
					Key       string `json:"key"`
					Value     any    `json:"value"`
				}
				if err := json.Unmarshal(controlMsg, &ctrl); err == nil {
					if ctrl.SessionID != "" {
						followSession(ctrl.SessionID)
					}
					if ctrl.Type == "projection" && ctrl.SessionID != "" {
						if h, exists := activeFollows[ctrl.SessionID]; exists {
							evs := h.normalizer.NormalizeProjection(ctrl.Key, ctrl.Value)
							for _, ev := range evs {
								select {
								case outCh <- ev:
								case <-ctx.Done():
									return
								}
							}
						}
					}
				}
			}
		}
	}()

	return outCh, nil
}

// Interrupt cancels the current turn of the session.
func (a *Adapter) Interrupt(ctx context.Context, ref agents.SessionRef) error {
	if err := ref.Validate(); err != nil {
		return err
	}

	proc, release, err := a.host.Acquire(ctx)
	if err != nil {
		return err
	}
	defer release()

	client := proc.Client()
	return client.Call(ctx, "session/cancel", map[string]any{
		"request": map[string]any{
			"sessionId": ref.SessionID,
		},
	}, nil)
}

// ListModels returns the model catalog offered by dsh.
func (a *Adapter) ListModels(ctx context.Context, directory string) ([]agents.ModelInfo, error) {
	proc, release, err := a.host.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer release()

	return FetchModels(ctx, proc.Client())
}

// DetectTurn checks whether an agent-native session is currently executing a turn.
func (a *Adapter) DetectTurn(ctx context.Context, ref agents.SessionRef) (bool, error) {
	if err := ref.Validate(); err != nil {
		return false, err
	}

	proc, release, err := a.host.Acquire(ctx)
	if err != nil {
		return false, err
	}
	defer release()

	var res struct {
		Items []struct {
			SessionID string `json:"sessionId"`
			Running   bool   `json:"running"`
		} `json:"items"`
	}

	if err := proc.Client().Call(ctx, "session/list", map[string]any{"_request": map[string]any{}}, &res); err != nil {
		return false, fmt.Errorf("detect turn on dsh: %w", err)
	}

	for _, item := range res.Items {
		if item.SessionID == ref.SessionID {
			return item.Running, nil
		}
	}
	return false, nil
}

func newRandomID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x", b)
}
