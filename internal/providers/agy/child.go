package agy

import (
	"bufio"
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
)

var (
	errChildClosed    = errors.New("agy: child process closed")
	errTurnInProgress = errors.New("agy: turn already in progress")
)

// StreamUserMessage is the NDJSON message sent to agy's stdin.
type StreamUserMessage struct {
	Event   string                 `json:"event"`
	Message StreamUserContentBlock `json:"message"`
}

// StreamUserContentBlock wraps the message content.
type StreamUserContentBlock struct {
	Content string `json:"content"`
}

// agyChild represents a long-lived agy stream-json session child process.
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
	normalizer  *EventNormalizer
	idleTimer   *time.Timer
	turnRunning bool
	closed      bool
	done        chan struct{}
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
	}
}

func (c *agyChild) start(ctx context.Context, executable string, env []string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return errChildClosed
	}

	args := buildChildArgs(c.sessionID, c.model, c.permissionMode, c.inst.directory)
	// Process is bound to the instance context so it survives individual HTTP requests.
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

	// agy is a terminal UI: in a process group of its own it is a background
	// job of the platform's terminal, so its first terminal access stops it
	// before init is printed. A new session detaches it from the terminal.
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

	reader := bufio.NewReader(stdout)

	// Read the first line synchronously to extract the conversation ID from the init event.
	type readResult struct {
		line []byte
		err  error
	}
	ch := make(chan readResult, 1)
	go func() {
		line, err := reader.ReadBytes('\n')
		ch <- readResult{line: line, err: err}
	}()

	var firstLine []byte
	select {
	case <-ctx.Done():
		c.abortStartLocked(cmd)
		return fmt.Errorf("agy: wait for child init: %w", ctx.Err())
	case res := <-ch:
		if res.err != nil {
			c.abortStartLocked(cmd)
			return fmt.Errorf("agy: read child init line: %w", res.err)
		}
		firstLine = res.line
	}

	var raw RawagyEvent
	if err := json.Unmarshal(firstLine, &raw); err == nil {
		if raw.ConversationID != "" {
			if c.sessionID == "" {
				c.sessionID = raw.ConversationID
			}
			c.normalizer.SetSessionID(c.sessionID)
		}
	}

	events, err := c.normalizer.Normalize(firstLine)
	if err == nil {
		for _, ev := range events {
			c.inst.broadcast(ev)
		}
	}

	c.resetIdleTimerLocked()
	go c.readLoop(stdout, reader)
	return nil
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

func (c *agyChild) readLoop(stdout io.ReadCloser, reader *bufio.Reader) {
	defer stdout.Close()
	defer close(c.done)
	defer func() {
		if c.cmd != nil {
			_ = c.cmd.Wait()
		}
		c.inst.removeChild(c.sessionID)
	}()

	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			events, err := c.normalizer.Normalize(line)
			if err == nil {
				for _, ev := range events {
					if ev.Type == agents.EventTurn && ev.Turn != nil {
						c.mu.Lock()
						if ev.Turn.Phase == agents.TurnStarted {
							c.turnRunning = true
							if c.idleTimer != nil {
								c.idleTimer.Stop()
							}
						} else if ev.Turn.Phase == agents.TurnEnded {
							c.turnRunning = false
							c.resetIdleTimerLocked()
						}
						c.mu.Unlock()
					}
					c.inst.broadcast(ev)
				}
			}
		}
		if err != nil {
			break
		}
	}

	c.mu.Lock()
	wasRunning := c.turnRunning
	c.closed = true
	c.turnRunning = false
	c.mu.Unlock()

	// If the process exited unexpectedly while turn was running, emit turn failed
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

func (c *agyChild) sendTurn(ctx context.Context, text string) error {
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
	c.mu.Unlock()

	msg := StreamUserMessage{
		Event: "user",
		Message: StreamUserContentBlock{
			Content: text,
		},
	}
	data, err := json.Marshal(msg)
	if err != nil {
		c.mu.Lock()
		c.turnRunning = false
		c.resetIdleTimerLocked()
		c.mu.Unlock()
		return fmt.Errorf("agy: marshal user turn: %w", err)
	}

	data = append(data, '\n')
	if _, err := c.stdin.Write(data); err != nil {
		c.mu.Lock()
		c.turnRunning = false
		c.resetIdleTimerLocked()
		c.mu.Unlock()
		return fmt.Errorf("agy: write user turn: %w", err)
	}
	return nil
}

func (c *agyChild) interrupt(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || !c.turnRunning {
		return nil
	}

	// Terminate the running process to stop generation
	if c.cmd != nil && c.cmd.Process != nil {
		_ = platform.New().Process().Terminate(c.cmd.Process.Pid)
	}
	c.turnRunning = false
	c.closed = true

	c.inst.broadcast(agents.NormalizedEvent{
		InstanceID: c.inst.id,
		SessionID:  c.sessionID,
		Backend:    agents.BackendAgy,
		Timestamp:  c.now(),
		Type:       agents.EventTurn,
		Turn: &agents.TurnPayload{
			ID:      "turn-interrupted",
			Phase:   agents.TurnEnded,
			Outcome: agents.TurnCancelled,
		},
	})
	return nil
}

// abortStartLocked kills a child whose init line never arrived and reaps it, so
// a failed start leaves no zombie behind.
func (c *agyChild) abortStartLocked(cmd *exec.Cmd) {
	_ = c.closeLocked()
	_ = cmd.Wait()
}

func (c *agyChild) closeLocked() error {
	if c.closed {
		return nil
	}
	c.closed = true
	if c.idleTimer != nil {
		c.idleTimer.Stop()
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
