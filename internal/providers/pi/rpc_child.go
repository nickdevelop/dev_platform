package pi

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"devplatform/internal/core/agents"
)

// rpcChild is one long-lived `pi --mode rpc` process bound to a single
// session. It owns the process, the command/response connection, the pending
// tool confirmations and the session's orphan rule memory. A child is started
// lazily on the first prompt and reclaimed after it has been idle for
// rpcIdleTimeout.
type rpcChild struct {
	adapter   *Adapter
	inst      *instance
	sessionID string

	proc    *process
	conn    *rpcConn
	parser  *parser
	stderr  *parser
	pending *piPermissions

	lastUsed  atomic.Int64 // unix nanoseconds
	streaming atomic.Bool
	dead      atomic.Bool
	orderly   atomic.Bool
	// retiring is set by the idle reaper before it shuts a child down, so a
	// concurrent send starts a fresh child instead of racing the shutdown.
	retiring atomic.Bool

	done      chan struct{}
	closeOnce sync.Once

	stateMu         sync.Mutex
	model           string
	name            string
	sessionFilePath string
}

// rpc start-up and command timeouts. get_state must answer promptly; it is the
// binding proof that the child is the session we asked for. A prompt response
// only means "accepted", so it is fast; abort waits for idle, hence the larger
// bound. Both remain bounded by the caller's context.
const (
	rpcGetStateTimeout = 15 * time.Second
	rpcCommandTimeout  = 30 * time.Second
)

// defaultRPCIdleTimeout is how long a child may sit with no run and no recent
// use before its stdin is closed. It keeps the steady-state process count near
// zero without adding start latency to an active conversation.
const defaultRPCIdleTimeout = 10 * time.Minute

// defaultRPCReapInterval is how often the idle reaper checks a child.
const defaultRPCReapInterval = time.Minute

// rpcOptions describes the arguments for one `pi --mode rpc` child.
type rpcOptions struct {
	sessionID      string
	sessionDir     string
	passSessionDir bool
	model          string
	name           string
	extensions     []string
}

// rpcArgs builds pi's argv for a long-lived RPC session. The session id is the
// platform-assigned id so the child binds to the same transcript a later
// restart re-attaches to. The prompt is never passed as an argument (RPC mode
// rejects @file arguments); it goes over stdin.
func rpcArgs(options rpcOptions) []string {
	args := []string{"--mode", "rpc"}
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
	for _, extension := range options.extensions {
		if extension != "" {
			args = append(args, "--extension", extension)
		}
	}
	return args
}

// startRPCChild launches the child, wires its read loops and proves the
// session binding with get_state before returning it. A child that fails
// before the binding is proved is reaped and reported as an error.
func (a *Adapter) startRPCChild(inst *instance, sessionID, model, name string) (*rpcChild, error) {
	spec := processSpec{
		executable: a.execPath(),
		args: rpcArgs(rpcOptions{
			sessionID:      sessionID,
			sessionDir:     a.sessionDir,
			passSessionDir: a.passSessionDir,
			model:          model,
			name:           name,
			extensions:     a.extensions,
		}),
		dir:   inst.directory,
		env:   a.env,
		stdin: true,
	}
	spec, err := a.resolveSpec(spec)
	if err != nil {
		return nil, err
	}
	proc, err := startProcess(inst.ctx, spec)
	if err != nil {
		return nil, fmt.Errorf("pi: start rpc child: %w", err)
	}

	parser := newParser(inst.id, sessionID, a.now)
	child := &rpcChild{
		adapter:   a,
		inst:      inst,
		sessionID: sessionID,
		proc:      proc,
		parser:    parser,
		stderr:    newParser(inst.id, sessionID, a.now),
		pending:   newPiPermissions(),
		done:      make(chan struct{}),
		model:     model,
		name:      name,
	}
	// The connection intercepts lifecycle events so the child can track
	// whether a run is in flight (for followUp queuing and idle reclamation)
	// before the event reaches subscribers.
	conn := newRPCConn(proc.stdin, parser, func(event agents.NormalizedEvent) {
		child.observe(event)
		inst.publish(event)
	})
	child.conn = conn
	conn.extensionUI = child.handleExtensionUI
	child.touch()

	go child.run()

	if err := child.bindSession(context.Background()); err != nil {
		child.kill()
		return nil, err
	}
	return child, nil
}

// bindSession sends get_state and checks that the child is running the session
// the platform asked for. It records the on-disk session file for diagnostics.
func (c *rpcChild) bindSession(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, rpcGetStateTimeout)
	defer cancel()
	response, err := c.conn.request(ctx, newGetStateCommand(""), rpcGetStateTimeout)
	if err != nil {
		return fmt.Errorf("pi: rpc child get_state: %w", err)
	}
	if !response.Success {
		return fmt.Errorf("pi: rpc child get_state rejected: %s", redactSecrets(response.Error))
	}
	state, err := decodeResponseData[rpcStateData](response)
	if err != nil {
		return err
	}
	if state.SessionID != "" && state.SessionID != c.sessionID {
		return fmt.Errorf("pi: rpc child bound to session %q, want %q", state.SessionID, c.sessionID)
	}
	c.stateMu.Lock()
	c.sessionFilePath = state.SessionFile
	// Only learn the model from get_state when the platform did not request
	// one; otherwise keep the requested value so a later send does not
	// re-issue set_model every turn just because pi reports a canonical id.
	if state.Model != nil && c.model == "" {
		c.model = rpcModelID(state.Model)
	}
	c.stateMu.Unlock()
	c.setStreaming(state.IsStreaming)
	return nil
}

// run drains the child's stdout and stderr, reaps it and marks it dead. A
// non-orderly exit publishes a process_exit error so the workbench can explain
// the interrupted turn; the next prompt re-attaches with the same session id.
func (c *rpcChild) run() {
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_ = c.conn.readLoop(c.proc.stdout)
	}()
	go func() {
		defer wg.Done()
		_ = c.stderr.runStderr(c.proc.stderr, c.inst.publish)
	}()
	wg.Wait()

	waitErr := c.proc.wait()
	c.conn.close()
	c.dead.Store(true)
	c.inst.removeChild(c)
	close(c.done)

	if !c.orderly.Load() && !c.inst.isClosed() {
		message := "pi rpc child exited"
		if waitErr != nil {
			message = waitErr.Error()
		}
		c.inst.publish(agents.NormalizedEvent{
			Type:      agents.EventError,
			Error:     &agents.ErrorPayload{Code: "process_exit", Message: redactSecrets(message)},
			SessionID: c.sessionID,
			Timestamp: c.inst.now(),
		})
	}
}

// handleExtensionUI answers the extension UI subprotocol. Only `confirm`
// maps onto the platform's tool approval contract; other dialogs are left
// unanswered here and auto-cancelled by the connection so pi never blocks.
func (c *rpcChild) handleExtensionUI(request *rpcExtensionUIRequest) bool {
	if request.Method != "confirm" {
		return false
	}
	key := confirmKey(request.Title, request.Message)
	if c.adapter.sessionApproved(c.inst.id, c.sessionID, key) {
		// A previous "always" already covers this request: answer without
		// surfacing a card.
		confirmed := true
		_ = c.conn.respondExtensionUI(rpcExtensionUIResponse{Type: "extension_ui_response", ID: request.ID, Confirmed: &confirmed})
		return true
	}
	c.pending.add(agents.PermissionRequest{
		ID:        request.ID,
		SessionID: c.sessionID,
		Action:    request.Title,
		Message:   request.Message,
	})
	return true
}

// confirmKey derives a stable rule key for an approval so "always" can be
// remembered per session. It uses the title and message the approval extension
// supplies; the tool name is expected in the title.
func confirmKey(title, message string) string {
	return strings.TrimSpace(title) + "\x00" + strings.TrimSpace(message)
}

// respond sends one extension UI response for a pending id.
func (c *rpcChild) respond(response rpcExtensionUIResponse) error {
	response.Type = "extension_ui_response"
	return c.conn.respondExtensionUI(response)
}

// touch records that the child was just used, delaying idle reclamation.
func (c *rpcChild) touch() {
	c.lastUsed.Store(time.Now().UnixNano())
}

// lastUsedAt returns the last-use time.
func (c *rpcChild) lastUsedAt() time.Time {
	return time.Unix(0, c.lastUsed.Load())
}

// setStreaming records whether a run is in flight, learned from lifecycle
// events and get_state. It lets SendMessage choose the followUp queuing
// behavior without a round trip.
func (c *rpcChild) setStreaming(streaming bool) {
	c.streaming.Store(streaming)
}

// isStreaming reports whether a run is in flight.
func (c *rpcChild) isStreaming() bool { return c.streaming.Load() }

// observe tracks run state from lifecycle events so followUp queuing and idle
// reclamation see a run start and settle without an extra get_state round trip.
func (c *rpcChild) observe(event agents.NormalizedEvent) {
	if event.Type != agents.EventLifecycle || event.Lifecycle == nil {
		return
	}
	switch event.Lifecycle.State {
	case "agent_start", "turn_start":
		c.setStreaming(true)
	case "agent_settled":
		c.setStreaming(false)
		c.touch()
	}
}

// beginRetire claims a child for reclamation, reporting false when it is
// already retiring or dead. Only the winner shuts it down.
func (c *rpcChild) beginRetire() bool {
	if c.dead.Load() {
		return false
	}
	return c.retiring.CompareAndSwap(false, true)
}

// idle reports whether the child has been unused past the idle timeout and is
// not mid-run, so it can be reclaimed.
func (c *rpcChild) idle(timeout time.Duration) bool {
	if c.streaming.Load() || c.dead.Load() || c.retiring.Load() {
		return false
	}
	return time.Since(c.lastUsedAt()) >= timeout
}

// sessionFile returns the on-disk session file get_state reported.
func (c *rpcChild) sessionFile() string {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	return c.sessionFilePath
}

// currentModel returns the model the child is running.
func (c *rpcChild) currentModel() string {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	return c.model
}

// currentName returns the name the child was started with.
func (c *rpcChild) currentName() string {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	return c.name
}

// setModel switches the child's model through set_model. A failure is
// reported to the caller, which decides whether the prompt should proceed.
func (c *rpcChild) setModel(ctx context.Context, model string) error {
	provider, modelID := splitModelRef(model)
	if modelID == "" {
		return nil
	}
	response, err := c.conn.request(ctx, rpcCommand{Type: "set_model", Provider: provider, ModelID: modelID}, rpcCommandTimeout)
	if err != nil {
		return err
	}
	if !response.Success {
		return fmt.Errorf("pi: set_model %q rejected: %s", model, redactSecrets(response.Error))
	}
	c.stateMu.Lock()
	c.model = model
	c.stateMu.Unlock()
	return nil
}

// setName switches the child's session display name through set_session_name.
func (c *rpcChild) setName(ctx context.Context, name string) error {
	response, err := c.conn.request(ctx, newSetSessionNameCommand("", name), rpcCommandTimeout)
	if err != nil {
		return err
	}
	if !response.Success {
		return fmt.Errorf("pi: set_session_name rejected: %s", redactSecrets(response.Error))
	}
	c.stateMu.Lock()
	c.name = name
	c.stateMu.Unlock()
	return nil
}

// abort asks pi to cancel the running turn. The response arrives once the
// session is idle. A child without a run answers immediately.
func (c *rpcChild) abort(ctx context.Context) error {
	response, err := c.conn.request(ctx, newAbortCommand(""), rpcCommandTimeout)
	if err != nil {
		return err
	}
	if !response.Success {
		return fmt.Errorf("pi: abort rejected: %s", redactSecrets(response.Error))
	}
	return nil
}

// shutdown stops the child in an orderly way: it closes stdin, which asks pi
// to dispose its runtime and exit, and waits for the process to be reaped. On
// timeout it escalates to killing the process group. It is idempotent.
func (c *rpcChild) shutdown(ctx context.Context) {
	c.orderly.Store(true)
	c.closeOnce.Do(func() {
		if c.proc.stdin != nil {
			_ = c.proc.stdin.Close()
		}
	})
	select {
	case <-c.done:
	case <-ctx.Done():
		c.kill()
	case <-time.After(2 * time.Second):
		c.kill()
	}
}

// kill force-terminates the child's process group.
func (c *rpcChild) kill() {
	c.pending.clear()
	c.proc.kill()
}

// splitModelRef splits the platform's "provider/model" selection into pi's
// set_model provider and model id. A value without a slash is treated as a
// bare model id, which pi resolves against its configured providers.
func splitModelRef(model string) (provider, modelID string) {
	if index := strings.Index(model, "/"); index > 0 {
		return model[:index], model[index+1:]
	}
	return "", model
}
