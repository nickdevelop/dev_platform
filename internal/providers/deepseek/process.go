package deepseek

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"sync"
	"sync/atomic"
	"time"
)

var launchURLRegex = regexp.MustCompile(`dsh web:\s+(https?://127\.0\.0\.1:\d+/\?token=[A-Za-z0-9_%.-]+)`)

const (
	defaultStartupTimeout = 30 * time.Second
	gracefulExitTimeout   = 5 * time.Second
)

// Process represents one running `dsh --profile web` child process and its authenticated Typert client.
type Process struct {
	cmd       *exec.Cmd
	stdinPipe io.WriteCloser
	client    *Client
	launchURL string

	exitDone  chan struct{}
	exitErr   error
	closeOnce sync.Once
	stopping  atomic.Bool
}

// StartProcess spawns `dsh --profile web --port 0 --no-open`, parses the launch URL from stdout,
// authenticates, and connects the WebSocket mux client.
func StartProcess(ctx context.Context, executable string, workDir string, env []string, startupTimeout time.Duration) (*Process, error) {
	if startupTimeout <= 0 {
		startupTimeout = defaultStartupTimeout
	}

	startupCtx, cancelStartup := context.WithTimeout(ctx, startupTimeout)
	defer cancelStartup()

	cmd := exec.Command(executable, "--profile", "web", "--port", "0", "--no-open")
	if workDir != "" {
		cmd.Dir = workDir
	}
	if len(env) > 0 {
		cmd.Env = env
	}

	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("open stdin pipe: %w", err)
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdinPipe.Close()
		return nil, fmt.Errorf("open stdout pipe: %w", err)
	}

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		_ = stdinPipe.Close()
		_ = stdoutPipe.Close()
		return nil, fmt.Errorf("open stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		_ = stdinPipe.Close()
		_ = stdoutPipe.Close()
		_ = stderrPipe.Close()
		return nil, fmt.Errorf("start dsh process: %w", err)
	}

	proc := &Process{
		cmd:       cmd,
		stdinPipe: stdinPipe,
		exitDone:  make(chan struct{}),
	}

	// Drain stderr in background to prevent blocking
	go func() {
		r := bufio.NewReader(stderrPipe)
		for {
			_, err := r.ReadString('\n')
			if err != nil {
				return
			}
		}
	}()

	// Wait for process exit in background
	go func() {
		err := cmd.Wait()
		proc.exitErr = err
		close(proc.exitDone)
		if proc.client != nil {
			_ = proc.client.Close()
		}
	}()

	// Read stdout to find launch URL
	urlCh := make(chan string, 1)
	errCh := make(chan error, 1)

	go func() {
		scanner := bufio.NewScanner(stdoutPipe)
		for scanner.Scan() {
			line := scanner.Text()
			if m := launchURLRegex.FindStringSubmatch(line); len(m) > 1 {
				urlCh <- m[1]
				// Keep draining stdout
				for scanner.Scan() {
				}
				return
			}
		}
		if err := scanner.Err(); err != nil {
			errCh <- fmt.Errorf("read dsh stdout: %w", err)
			return
		}
		errCh <- errors.New("dsh process exited before printing launch url")
	}()

	var launchURL string
	select {
	case launchURL = <-urlCh:
	case err := <-errCh:
		_ = proc.Shutdown(context.Background())
		return nil, err
	case <-proc.exitDone:
		_ = proc.Shutdown(context.Background())
		return nil, fmt.Errorf("dsh process terminated prematurely: %v", proc.exitErr)
	case <-startupCtx.Done():
		_ = proc.Shutdown(context.Background())
		return nil, fmt.Errorf("dsh startup timed out waiting for launch url: %w", startupCtx.Err())
	}

	proc.launchURL = launchURL

	// Exchange launch token for API cookie
	endpoint, cookie, err := ExchangeLaunchToken(startupCtx, launchURL)
	if err != nil {
		_ = proc.Shutdown(context.Background())
		return nil, fmt.Errorf("exchange dsh launch token: %w", err)
	}

	client := NewClient(endpoint, cookie)
	if err := client.ConnectMux(startupCtx); err != nil {
		client.Close()
		_ = proc.Shutdown(context.Background())
		return nil, fmt.Errorf("connect dsh websocket mux: %w", err)
	}

	proc.client = client
	return proc, nil
}

// Client returns the connected Typert client.
func (p *Process) Client() *Client {
	return p.client
}

// LaunchURL returns the captured launch URL.
func (p *Process) LaunchURL() string {
	return p.launchURL
}

// ExitDone returns a channel that closes when the process terminates.
func (p *Process) ExitDone() <-chan struct{} {
	return p.exitDone
}

// Shutdown gracefully terminates the dsh process and closes client connections.
func (p *Process) Shutdown(ctx context.Context) error {
	var shutdownErr error
	p.closeOnce.Do(func() {
		p.stopping.Store(true)

		if p.client != nil {
			_ = p.client.Close()
		}

		// 1. Closing stdin pipe signals dsh to exit gracefully
		if p.stdinPipe != nil {
			_ = p.stdinPipe.Close()
		}

		// Wait for graceful exit
		select {
		case <-p.exitDone:
			return
		case <-time.After(gracefulExitTimeout):
		case <-ctx.Done():
		}

		// 2. Escalate to process kill if not exited
		if p.cmd != nil && p.cmd.Process != nil {
			_ = p.cmd.Process.Kill()
		}

		select {
		case <-p.exitDone:
		case <-time.After(2 * time.Second):
			shutdownErr = errors.New("timeout waiting for dsh process termination")
		}
	})
	return shutdownErr
}
