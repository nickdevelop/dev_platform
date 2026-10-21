package deepseek

import (
	"context"
	"fmt"
	"sync"
	"time"
)

const defaultIdleTimeout = 30 * time.Minute

// SharedHost manages a single shared dsh process across all instances and projects,
// reference counting active leases and reclaiming the process after an idle period.
type SharedHost struct {
	mu             sync.Mutex
	proc           *Process
	leases         int
	idleTimer      *time.Timer
	idleTimeout    time.Duration
	startupTimeout time.Duration

	resolveExecutable func() string
	workDir           string
	env               []string

	// processLauncher overrides process creation for testing.
	processLauncher func(ctx context.Context) (*Process, error)
}

// SharedHostOptions configures the SharedHost.
type SharedHostOptions struct {
	ResolveExecutable func() string
	WorkDir           string
	Env               []string
	IdleTimeout       time.Duration
	StartupTimeout    time.Duration
	ProcessLauncher   func(ctx context.Context) (*Process, error)
}

// NewSharedHost creates a SharedHost.
func NewSharedHost(opts SharedHostOptions) *SharedHost {
	idleTimeout := opts.IdleTimeout
	if idleTimeout == 0 {
		idleTimeout = defaultIdleTimeout
	}
	return &SharedHost{
		resolveExecutable: opts.ResolveExecutable,
		workDir:           opts.WorkDir,
		env:               opts.Env,
		idleTimeout:       idleTimeout,
		startupTimeout:    opts.StartupTimeout,
		processLauncher:   opts.ProcessLauncher,
	}
}

// Acquire gets a lease on the running dsh process, starting it if not running.
// The caller must call the returned release function when the lease is no longer needed.
func (h *SharedHost) Acquire(ctx context.Context) (*Process, func(), error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.idleTimer != nil {
		h.idleTimer.Stop()
		h.idleTimer = nil
	}

	// Check if current process is alive
	if h.proc != nil {
		select {
		case <-h.proc.ExitDone():
			h.proc = nil
		default:
		}
	}

	if h.proc == nil {
		var proc *Process
		var err error

		if h.processLauncher != nil {
			proc, err = h.processLauncher(ctx)
		} else {
			execPath := "dsh"
			if h.resolveExecutable != nil {
				if p := h.resolveExecutable(); p != "" {
					execPath = p
				}
			}
			proc, err = StartProcess(ctx, execPath, h.workDir, h.env, h.startupTimeout)
		}

		if err != nil {
			return nil, nil, fmt.Errorf("start shared dsh host: %w", err)
		}
		h.proc = proc

		// Watch for unexpected exit
		go func(p *Process) {
			<-p.ExitDone()
			h.mu.Lock()
			if h.proc == p {
				h.proc = nil
			}
			h.mu.Unlock()
		}(proc)
	}

	h.leases++
	currentProc := h.proc

	releaseOnce := sync.Once{}
	release := func() {
		releaseOnce.Do(func() {
			h.release(currentProc)
		})
	}

	return currentProc, release, nil
}

func (h *SharedHost) release(proc *Process) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.leases--
	if h.leases <= 0 {
		h.leases = 0
		if h.proc == proc && h.proc != nil {
			if h.idleTimeout < 0 {
				// Immediate shutdown
				p := h.proc
				h.proc = nil
				go func() { _ = p.Shutdown(context.Background()) }()
			} else {
				// Schedule idle shutdown
				h.idleTimer = time.AfterFunc(h.idleTimeout, func() {
					h.idleShutdown(proc)
				})
			}
		}
	}
}

func (h *SharedHost) idleShutdown(proc *Process) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.proc == proc && h.leases == 0 {
		p := h.proc
		h.proc = nil
		go func() { _ = p.Shutdown(context.Background()) }()
	}
}

// Stop terminates the running dsh process and releases all resources.
func (h *SharedHost) Stop(ctx context.Context) error {
	h.mu.Lock()
	if h.idleTimer != nil {
		h.idleTimer.Stop()
		h.idleTimer = nil
	}
	p := h.proc
	h.proc = nil
	h.leases = 0
	h.mu.Unlock()

	if p != nil {
		return p.Shutdown(ctx)
	}
	return nil
}
