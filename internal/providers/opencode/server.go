package opencode

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"devplatform/internal/core/agents"
	"devplatform/internal/core/platform"
)

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

// isMultimodalModel reports whether a model is known to accept image inputs.
func isMultimodalModel(modelID string) bool {
	id := strings.ToLower(modelID)
	indicators := []string{
		"gpt-4o", "gpt-4-turbo", "gpt-4-vision",
		"claude-3", "claude-3.5", "claude-3-5", "claude-3.7", "claude-3-7",
		"gemini", "vision", "vl", "pixtral", "llava", "omni",
	}
	for _, ind := range indicators {
		if strings.Contains(id, ind) {
			return true
		}
	}
	return false
}
