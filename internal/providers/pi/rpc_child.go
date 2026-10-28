package pi

import (
	"context"
	"encoding/json"
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
	parser.newTurnID = func() string { return inst.nextTurnID(sessionID) }
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
		if event.Type == agents.EventMessage && event.Message != nil && event.Message.Role == "assistant" && event.Message.Model == nil {
			if m := child.currentModel(); m != "" {
				provider, _ := splitModelRef(m)
				event.Message.Model = &agents.MessageModel{
					ID:       m,
					Provider: provider,
				}
			}
		}
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

	// A run still open when the child dies ends failed before the process
	// error explains why.
	if turn, open := c.parser.endTurn(agents.TurnFailed); open && !c.inst.isClosed() {
		c.inst.publish(turn)
	}
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
	permission := agents.PermissionRequest{
		ID:        request.ID,
		SessionID: c.sessionID,
		Action:    request.Title,
		Message:   request.Message,
	}
	c.pending.add(permission)
	c.publishInteraction(agents.InteractionPayload{
		ID:         request.ID,
		Kind:       agents.InteractionPermission,
		Phase:      agents.InteractionRequested,
		Permission: &permission,
	})
	return true
}

// publishInteraction emits one interaction boundary of the child's session.
func (c *rpcChild) publishInteraction(payload agents.InteractionPayload) {
	c.inst.publish(agents.NormalizedEvent{
		Type:        agents.EventInteraction,
		SessionID:   c.sessionID,
		Interaction: &payload,
	})
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

// observe tracks run state from turn boundaries so followUp queuing and idle
// reclamation see a run start and settle without an extra get_state round trip.
func (c *rpcChild) observe(event agents.NormalizedEvent) {
	if event.Type != agents.EventTurn || event.Turn == nil {
		return
	}
	if event.Turn.Phase == agents.TurnStarted {
		c.setStreaming(true)
		return
	}
	c.setStreaming(false)
	c.touch()
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
		// When pi rejects the model (for example, if a legacy 2-part model string
		// like "deepseek/deepseek-v4.1-flash" was supplied where the real provider is
		// "occ-commandcode-o"), inspect available models to see if the full model matches
		// an available model ID under another provider.
		if altProvider, altModelID, ok := c.resolveAvailableModel(ctx, model); ok && (altProvider != provider || altModelID != modelID) {
			retryResp, retryErr := c.conn.request(ctx, rpcCommand{Type: "set_model", Provider: altProvider, ModelID: altModelID}, rpcCommandTimeout)
			if retryErr == nil && retryResp.Success {
				c.stateMu.Lock()
				c.model = qualifyModelID(altProvider, altModelID)
				c.stateMu.Unlock()
				return nil
			}
		}
		return fmt.Errorf("pi: set_model %q rejected: %s", model, redactSecrets(response.Error))
	}
	c.stateMu.Lock()
	c.model = qualifyModelID(provider, modelID)
	c.stateMu.Unlock()
	return nil
}

// resolveAvailableModel queries get_available_models and looks for a configured model
// that matches either:
// 1. m.ID == target (handles legacy 2-part input where target was "deepseek/deepseek-v4.1-flash" but provider was "occ-commandcode-o")
// 2. qualifyModelID(m.Provider, m.ID) == target
func (c *rpcChild) resolveAvailableModel(ctx context.Context, target string) (provider, modelID string, ok bool) {
	resp, err := c.conn.request(ctx, newGetAvailableModelsCommand(""), rpcCommandTimeout)
	if err != nil || !resp.Success {
		return "", "", false
	}
	var payload struct {
		Models []rpcModelData `json:"models"`
	}
	if err := json.Unmarshal(resp.Data, &payload); err != nil {
		return "", "", false
	}
	// First pass: exact match on pi's raw model ID
	for _, m := range payload.Models {
		if m.ID == target {
			return m.Provider, m.ID, true
		}
	}
	// Second pass: match on fully qualified ID
	for _, m := range payload.Models {
		if qualifyModelID(m.Provider, m.ID) == target {
			return m.Provider, m.ID, true
		}
	}
	return "", "", false
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
	c.parser.markAbort()
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
	for _, id := range c.pending.drain() {
		c.publishInteraction(agents.InteractionPayload{ID: id, Kind: agents.InteractionPermission, Phase: agents.InteractionResolved})
	}
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

// getCommands queries the child's available slash commands, prompt templates, and skills.
func (c *rpcChild) getCommands(ctx context.Context) ([]rpcCommandInfo, error) {
	response, err := c.conn.request(ctx, rpcCommand{Type: rpcCommandGetCommands}, rpcCommandTimeout)
	if err != nil {
		return nil, err
	}
	if !response.Success {
		return nil, fmt.Errorf("pi: get_commands rejected: %s", redactSecrets(response.Error))
	}
	var data rpcGetCommandsData
	if err := json.Unmarshal(response.Data, &data); err != nil {
		return nil, fmt.Errorf("pi: decode get_commands response: %w", err)
	}
	return data.Commands, nil
}

// compact asks pi to compact context history with optional instructions.
func (c *rpcChild) compact(ctx context.Context, instructions string) error {
	// Context compaction may involve a full LLM summarization pass over long history,
	// so we allow up to 120 seconds rather than the default 30-second unary timeout.
	timeout := 120 * time.Second
	if deadline, ok := ctx.Deadline(); ok {
		if remain := time.Until(deadline); remain > 0 && remain < timeout {
			timeout = remain
		}
	}

	response, err := c.conn.request(ctx, rpcCommand{Type: rpcCommandCompact, CustomInstructions: instructions}, timeout)
	if err != nil {
		return err
	}
	if !response.Success {
		errStr := response.Error
		lowerErr := strings.ToLower(errStr)
		// Pi rejects compaction with an error when history is too small or already compacted:
		// "Nothing to compact (session too small)" or "Already compacted".
		// In platform semantics, these are benign no-op outcomes indicating the context
		// is already compact, not an execution failure.
		if strings.Contains(lowerErr, "nothing to compact") ||
			strings.Contains(lowerErr, "session too small") ||
			strings.Contains(lowerErr, "already compacted") {
			return nil
		}

		return fmt.Errorf("pi: compact rejected: %s", redactSecrets(errStr))
	}
	return nil
}

// executeCommand runs a slash command or thinking command directly.
func (c *rpcChild) executeCommand(ctx context.Context, name, rawInput, arguments string) (*agents.CommandExecutionResult, error) {
	if name == "thinking" {
		level := strings.TrimSpace(arguments)
		if level == "" {
			level = "medium"
		}
		response, err := c.conn.request(ctx, rpcCommand{Type: rpcCommandSetThinkingLevel, Level: level}, rpcCommandTimeout)
		if err != nil {
			return nil, err
		}
		if !response.Success {
			return nil, fmt.Errorf("pi: set_thinking_level rejected: %s", redactSecrets(response.Error))
		}
		return &agents.CommandExecutionResult{
			Handled: true,
			Output:  fmt.Sprintf("Thinking level set to %s", level),
		}, nil
	}

	promptMsg := rawInput
	if promptMsg == "" {
		promptMsg = "/" + name
		if arguments != "" {
			promptMsg += " " + arguments
		}
	}

	response, err := c.conn.request(ctx, rpcCommand{
		Type:    rpcCommandPrompt,
		Message: promptMsg,
	}, rpcCommandTimeout)
	if err != nil {
		return nil, err
	}
	if !response.Success {
		return nil, fmt.Errorf("pi: command prompt rejected: %s", redactSecrets(response.Error))
	}

	var data struct {
		Disposition string `json:"disposition"`
	}
	_ = json.Unmarshal(response.Data, &data)

	return &agents.CommandExecutionResult{
		Handled: data.Disposition == "handled",
	}, nil
}
