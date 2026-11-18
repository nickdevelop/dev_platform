package agy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"

	"devplatform/internal/core/agents"
	"devplatform/internal/core/platform"
	"devplatform/internal/providers/agy/acp"
)

var (
	errChildClosed    = errors.New("agy: child process closed")
	errTurnInProgress = errors.New("agy: turn already in progress")
)

type pendingPermission struct {
	id          string
	permRequest agents.PermissionRequest
	replyCh     chan string
}

// agyChild represents a long-lived agy ACP stdio session child process.
type agyChild struct {
	sessionID      string
	model          string
	permissionMode string
	inst           *instance
	idleTimeout    time.Duration
	now            func() time.Time

	mu          sync.Mutex
	cmd         *exec.Cmd
	stdin       io.WriteCloser
	client      *acp.Client
	normalizer  *EventNormalizer
	idleTimer   *time.Timer
	turnRunning bool
	turnCancel  context.CancelFunc
	closed      bool
	done        chan struct{}

	permMu       sync.Mutex
	pendingPerms map[string]*pendingPermission
}

func newAgyChild(inst *instance, sessionID, model, permissionMode string, idleTimeout time.Duration, now func() time.Time) *agyChild {
	if idleTimeout <= 0 {
		idleTimeout = 10 * time.Minute
	}
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	norm := NewEventNormalizer(inst.id, sessionID, now)
	norm.SetModel(model)
	return &agyChild{
		sessionID:      sessionID,
		model:          model,
		permissionMode: permissionMode,
		inst:           inst,
		idleTimeout:    idleTimeout,
		now:            now,
		normalizer:     norm,
		done:           make(chan struct{}),
		pendingPerms:   make(map[string]*pendingPermission),
	}
}

func (c *agyChild) start(ctx context.Context, executable string, env []string, onBound func(string) error) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return errChildClosed
	}

	args := buildChildArgs(c.permissionMode, c.inst.directory)
	cmd := exec.CommandContext(c.inst.ctx, executable, args...)
	cmd.Dir = c.inst.directory
	if len(env) > 0 {
		cmd.Env = env
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("agy: stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return fmt.Errorf("agy: stdout pipe: %w", err)
	}

	if err := platform.New().Process().ConfigureDetached(cmd); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		return fmt.Errorf("agy: configure child process: %w", err)
	}

	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return platform.New().Process().Terminate(cmd.Process.Pid)
	}
	cmd.WaitDelay = 2 * time.Second

	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		return fmt.Errorf("agy: start process: %w", err)
	}
	if err := platform.New().Process().Attach(cmd.Process); err != nil {
		_ = platform.New().Process().Terminate(cmd.Process.Pid)
		_ = cmd.Wait()
		return fmt.Errorf("agy: supervise child process: %w", err)
	}

	c.cmd = cmd
	c.stdin = stdin

	c.client = acp.NewClient(stdin, stdin, c.handleRequest, c.handleNotification)

	go c.runClientLoop(stdout)

	// Step 1: initialize
	var initRes acp.InitializeResult
	if err := c.client.Call(ctx, acp.MethodInitialize, acp.InitializeParams{
		ProtocolVersion: 1,
		ClientCapabilities: acp.ClientCapabilities{
			Terminal: true,
			FS: &acp.FSCapabilities{
				ReadTextFile:  true,
				WriteTextFile: true,
			},
		},
	}, &initRes); err != nil {
		c.abortStartLocked(cmd)
		return fmt.Errorf("agy: initialize acp: %w", err)
	}

	// Step 2: session/new or session/load
	if c.sessionID == "" || isPendingSession(c.sessionID) {
		var sessRes acp.SessionResult
		if err := c.client.Call(ctx, acp.MethodSessionNew, acp.NewSessionParams{
			CWD:        c.inst.directory,
			MCPServers: []any{},
		}, &sessRes); err != nil {
			c.abortStartLocked(cmd)
			return fmt.Errorf("agy: new acp session: %w", err)
		}
		if sessRes.SessionID != "" {
			c.sessionID = sessRes.SessionID
			c.normalizer.SetSessionID(c.sessionID)
			if onBound != nil {
				if err := onBound(c.sessionID); err != nil {
					c.abortStartLocked(cmd)
					return err
				}
			}
		}
	} else {
		var sessRes acp.SessionResult
		if err := c.client.Call(ctx, acp.MethodSessionLoad, acp.LoadSessionParams{
			SessionID: c.sessionID,
			CWD:       c.inst.directory,
		}, &sessRes); err != nil {
			c.abortStartLocked(cmd)
			return fmt.Errorf("agy: load acp session: %w", err)
		}
	}

	// Step 3: set model if configured
	if c.model != "" {
		_ = c.client.Call(ctx, acp.MethodSessionSetModel, acp.SetModelParams{
			SessionID: c.sessionID,
			ModelID:   c.model,
		}, nil)
	}

	// Step 4: set mode if auto
	if c.permissionMode == PermissionModeAuto {
		_ = c.client.Call(ctx, acp.MethodSessionSetMode, acp.SetModeParams{
			SessionID: c.sessionID,
			ModeID:    "auto",
		}, nil)
	}

	// Broadcast session initialized lifecycle event
	c.inst.broadcast(agents.NormalizedEvent{
		InstanceID: c.inst.id,
		SessionID:  c.sessionID,
		Backend:    agents.BackendAgy,
		Timestamp:  c.now(),
		Type:       agents.EventLifecycle,
		Lifecycle: &agents.LifecycleEvent{
			State:  "initialized",
			Detail: "agy acp session initialized",
		},
	})

	c.resetIdleTimerLocked()
	return nil
}

func (c *agyChild) runClientLoop(stdout io.ReadCloser) {
	defer stdout.Close()
	defer close(c.done)
	defer func() {
		if c.cmd != nil {
			_ = c.cmd.Wait()
		}
		c.inst.removeChild(c.sessionID)
	}()

	_ = c.client.ReadLoop(stdout)

	c.mu.Lock()
	wasRunning := c.turnRunning
	c.closed = true
	c.turnRunning = false
	c.mu.Unlock()

	if wasRunning {
		c.inst.broadcast(agents.NormalizedEvent{
			InstanceID: c.inst.id,
			SessionID:  c.sessionID,
			Backend:    agents.BackendAgy,
			Timestamp:  c.now(),
			Type:       agents.EventTurn,
			Turn: &agents.TurnPayload{
				ID:      "turn-abrupt",
				Phase:   agents.TurnEnded,
				Outcome: agents.TurnFailed,
			},
		})
	}
}

func (c *agyChild) handleNotification(method string, params json.RawMessage) {
	if method != acp.MethodSessionUpdate {
		return
	}
	var notif acp.SessionUpdateNotification
	if err := json.Unmarshal(params, &notif); err != nil {
		return
	}

	var header acp.SessionUpdateHeader
	if err := json.Unmarshal(notif.Update, &header); err != nil {
		return
	}

	events, err := c.normalizer.NormalizeACPUpdate(header.SessionUpdate, notif.Update)
	if err == nil {
		for _, ev := range events {
			c.inst.broadcast(ev)
		}
	}
}

func (c *agyChild) handleRequest(ctx context.Context, id any, method string, params json.RawMessage) (any, error) {
	if method != acp.MethodRequestPermission {
		return nil, fmt.Errorf("unknown method: %s", method)
	}

	var permReq acp.RequestPermissionParams
	if err := json.Unmarshal(params, &permReq); err != nil {
		return nil, err
	}

	reqIDStr := fmt.Sprintf("%v", id)
	if c.permissionMode == PermissionModeAuto {
		selected := "allow_once"
		for _, opt := range permReq.Options {
			if opt.Kind == "allow_once" || opt.Kind == "allow_always" {
				selected = opt.OptionID
				break
			}
		}
		return acp.RequestPermissionResult{
			Outcome: acp.PermissionOutcome{
				Outcome:  "selected",
				OptionID: selected,
			},
		}, nil
	}

	replyCh := make(chan string, 1)
	perm := agents.PermissionRequest{
		ID:        reqIDStr,
		SessionID: c.sessionID,
		Action:    permReq.ToolCall.Title,
		Message:   permReq.ToolCall.Title,
	}

	c.permMu.Lock()
	c.pendingPerms[reqIDStr] = &pendingPermission{
		id:          reqIDStr,
		permRequest: perm,
		replyCh:     replyCh,
	}
	c.permMu.Unlock()

	c.inst.broadcast(agents.NormalizedEvent{
		InstanceID: c.inst.id,
		SessionID:  c.sessionID,
		Backend:    agents.BackendAgy,
		Timestamp:  c.now(),
		Type:       agents.EventInteraction,
		Interaction: &agents.InteractionPayload{
			ID:         reqIDStr,
			Kind:       agents.InteractionPermission,
			Phase:      agents.InteractionRequested,
			Permission: &perm,
		},
	})

	select {
	case decision := <-replyCh:
		if decision == "reject" {
			for _, opt := range permReq.Options {
				if opt.Kind == "reject_once" || opt.Kind == "reject_always" {
					return acp.RequestPermissionResult{
						Outcome: acp.PermissionOutcome{
							Outcome:  "selected",
							OptionID: opt.OptionID,
						},
					}, nil
				}
			}
			return acp.RequestPermissionResult{
				Outcome: acp.PermissionOutcome{
					Outcome: "cancelled",
				},
			}, nil
		}

		for _, opt := range permReq.Options {
			if opt.Kind == "allow_once" || opt.Kind == "allow_always" {
				return acp.RequestPermissionResult{
					Outcome: acp.PermissionOutcome{
						Outcome:  "selected",
						OptionID: opt.OptionID,
					},
				}, nil
			}
		}
		if len(permReq.Options) > 0 {
			return acp.RequestPermissionResult{
				Outcome: acp.PermissionOutcome{
					Outcome:  "selected",
					OptionID: permReq.Options[0].OptionID,
				},
			}, nil
		}
		return acp.RequestPermissionResult{
			Outcome: acp.PermissionOutcome{
				Outcome: "cancelled",
			},
		}, nil
	case <-c.done:
		return nil, errChildClosed
	}
}

func (c *agyChild) replyPermission(requestID, decision string) error {
	c.permMu.Lock()
	pending, ok := c.pendingPerms[requestID]
	if ok {
		delete(c.pendingPerms, requestID)
	}
	c.permMu.Unlock()

	if !ok {
		return fmt.Errorf("%w: permission %q not pending", agents.ErrReplyExpired, requestID)
	}

	pending.replyCh <- decision

	c.inst.broadcast(agents.NormalizedEvent{
		InstanceID: c.inst.id,
		SessionID:  c.sessionID,
		Backend:    agents.BackendAgy,
		Timestamp:  c.now(),
		Type:       agents.EventInteraction,
		Interaction: &agents.InteractionPayload{
			ID:    requestID,
			Kind:  agents.InteractionPermission,
			Phase: agents.InteractionResolved,
		},
	})
	return nil
}

func (c *agyChild) listPermissions() []agents.PermissionRequest {
	c.permMu.Lock()
	defer c.permMu.Unlock()

	res := make([]agents.PermissionRequest, 0, len(c.pendingPerms))
	for _, p := range c.pendingPerms {
		res = append(res, p.permRequest)
	}
	return res
}

func (c *agyChild) resetIdleTimerLocked() {
	if c.idleTimer != nil {
		c.idleTimer.Stop()
	}
	c.idleTimer = time.AfterFunc(c.idleTimeout, func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		if !c.closed && !c.turnRunning {
			_ = c.closeLocked()
		}
	})
}

func (c *agyChild) isTurnRunning() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return !c.closed && c.turnRunning
}

func (c *agyChild) sendTurn(text, model string, images []agents.ImageAttachment) error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return errChildClosed
	}
	if c.turnRunning {
		c.mu.Unlock()
		return errTurnInProgress
	}
	c.turnRunning = true
	if c.idleTimer != nil {
		c.idleTimer.Stop()
	}

	turnCtx, turnCancel := context.WithCancel(c.inst.ctx)
	c.turnCancel = turnCancel
	c.mu.Unlock()

	turnID := fmt.Sprintf("turn-%d", c.now().UnixNano())
	c.normalizer.StartTurn(turnID)

	// 广播轮次开始事件
	c.inst.broadcast(agents.NormalizedEvent{
		InstanceID: c.inst.id,
		SessionID:  c.sessionID,
		Backend:    agents.BackendAgy,
		Timestamp:  c.now(),
		Type:       agents.EventTurn,
		Turn: &agents.TurnPayload{
			ID:    turnID,
			Phase: agents.TurnStarted,
		},
	})

	go func() {
		defer func() {
			c.mu.Lock()
			if c.turnCancel != nil {
				c.turnCancel()
				c.turnCancel = nil
			}
			c.turnRunning = false
			c.resetIdleTimerLocked()
			c.mu.Unlock()
		}()

		if model != "" && model != c.model {
			c.model = model
			c.normalizer.SetModel(model)
			_ = c.client.Call(turnCtx, acp.MethodSessionSetModel, acp.SetModelParams{
				SessionID: c.sessionID,
				ModelID:   model,
			}, nil)
		}

		promptContents := make([]acp.PromptContent, 0, 1+len(images))
		if text != "" || len(images) == 0 {
			promptContents = append(promptContents, acp.PromptContent{
				Type: "text",
				Text: text,
			})
		}
		for _, img := range images {
			promptContents = append(promptContents, acp.PromptContent{
				Type:     "image",
				Data:     img.Data,
				MimeType: img.MediaType,
				URI:      img.URL,
			})
		}

		var result acp.PromptResult
		err := c.client.Call(turnCtx, acp.MethodSessionPrompt, acp.PromptParams{
			SessionID: c.sessionID,
			Prompt:    promptContents,
		}, &result)

		outcome := agents.TurnCompleted
		if err != nil {
			if errors.Is(err, context.Canceled) {
				outcome = agents.TurnCancelled
			} else {
				outcome = agents.TurnFailed
			}
		} else if result.StopReason == "cancelled" {
			outcome = agents.TurnCancelled
		}

		endEvents := c.normalizer.EndTurn(outcome, result.Usage)
		for _, ev := range endEvents {
			c.inst.broadcast(ev)
		}
	}()

	return nil
}

func (c *agyChild) interrupt(ctx context.Context) error {
	c.mu.Lock()
	if c.closed || !c.turnRunning {
		c.mu.Unlock()
		return nil
	}
	sessionID := c.sessionID
	turnCancel := c.turnCancel
	client := c.client
	c.mu.Unlock()

	// 优先发送 ACP session/cancel
	cancelCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	err := client.Call(cancelCtx, acp.MethodSessionCancel, acp.CancelParams{
		SessionID: sessionID,
	}, nil)

	if err != nil && turnCancel != nil {
		turnCancel()
	}
	return err
}

func (c *agyChild) abortStartLocked(cmd *exec.Cmd) {
	_ = c.closeLocked()
	_ = cmd.Wait()
}

func (c *agyChild) closeLocked() error {
	if c.closed {
		return nil
	}
	c.closed = true
	if c.turnCancel != nil {
		c.turnCancel()
		c.turnCancel = nil
	}
	if c.idleTimer != nil {
		c.idleTimer.Stop()
	}
	if c.client != nil {
		_ = c.client.Close()
	}
	if c.stdin != nil {
		_ = c.stdin.Close()
	}
	if c.cmd != nil && c.cmd.Process != nil {
		_ = platform.New().Process().Terminate(c.cmd.Process.Pid)
	}
	return nil
}

func (c *agyChild) close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closeLocked()
}
