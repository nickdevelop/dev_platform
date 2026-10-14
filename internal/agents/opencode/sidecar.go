package opencode

import (
	"context"
	"fmt"
	"net"
	"os"
	"strconv"
	"time"

	"devplatform/internal/agents"
	"devplatform/internal/platform"
)

// sharedServer is the single machine-wide opencode server all adapter
// instances reuse. It mirrors the desktop background-service model: one
// `serve --service` process, adopted when healthy and never started by a
// business path.
//
// The password never leaves this package except inside the bound Client.
type sharedServer struct {
	client   *Client
	endpoint string
	done     <-chan struct{}
}

// ensureShared returns the machine-wide opencode service the platform talks
// to, adopting the one that is already registered when it is healthy.
//
// It never starts a service. The service is a third party — the desktop
// client and manually started `opencode serve --service` processes use the
// same registration — so starting, stopping or replacing it is an operator's
// decision, taken on the platform settings page and nowhere else. When no
// usable service is registered, ensureShared reports
// agents.ErrServiceNotRunning and the caller refuses the work.
//
// Safe for concurrent use.
func (a *Adapter) ensureShared(ctx context.Context) (*sharedServer, error) {
	a.sharedMu.Lock()
	shared := a.shared
	a.sharedMu.Unlock()
	if shared != nil {
		if err := a.probeShared(ctx, shared); err == nil {
			return shared, nil
		}
		// The reference we hold is stale. Dropping it is not an operation on
		// somebody else's process: it only stops the platform from using it.
		a.detachShared()
	}

	// Prefer the machine-wide service that is already there — a desktop
	// sidecar, another platform instance, or a manually started
	// `opencode serve --service` — over refusing the work.
	discovered, err := a.discoverExistingService(ctx)
	if err == nil && discovered != nil {
		a.sharedMu.Lock()
		if a.shared != nil {
			existing := a.shared
			a.sharedMu.Unlock()
			if err := a.probeShared(ctx, existing); err == nil {
				return existing, nil
			}
			a.detachShared()
		}
		a.shared = discovered
		a.sharedStop = nil
		a.sharedMu.Unlock()
		go a.watchShared(discovered)
		return discovered, nil
	}

	return nil, fmt.Errorf(
		"%w: no healthy `opencode serve --service` is registered in %s; start it from the platform settings (Agent page)",
		agents.ErrServiceNotRunning, a.serviceFile())
}

// probeShared reports whether the shared server answers /api/info.
func (a *Adapter) probeShared(ctx context.Context, shared *sharedServer) error {
	probeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	_, err := shared.client.Info(probeCtx)
	return err
}

// detachShared drops the platform's reference to the current service. It never
// touches the process: the service may be serving other clients, and only the
// settings page (StopService) may stop it.
func (a *Adapter) detachShared() {
	a.sharedMu.Lock()
	a.shared = nil
	a.sharedStop = nil
	a.sharedMu.Unlock()
}

// teardownShared stops the current service and drops the reference. It is the
// operator's action (platform settings), not a business path: the platform
// kills only the process the settings page started or was told to stop.
func (a *Adapter) teardownShared() {
	a.sharedMu.Lock()
	shared := a.shared
	stop := a.sharedStop
	a.shared = nil
	a.sharedStop = nil
	a.sharedMu.Unlock()
	if shared == nil {
		return
	}
	if stop != nil {
		_ = stop(context.Background())
	}
}

// watchShared observes the shared server: an explicit process exit (the
// launcher's done channel, used by tests and the real launcher) fails all
// instances at once; otherwise periodic probing catches a silent death.
func (a *Adapter) watchShared(shared *sharedServer) {
	ticker := time.NewTicker(a.watchInterval)
	defer ticker.Stop()
	var done <-chan struct{}
	if shared.done != nil {
		done = shared.done
	}
	for {
		select {
		case <-done:
			if done == nil {
				continue
			}
			a.sharedMu.Lock()
			current := a.shared
			a.sharedMu.Unlock()
			if current != shared {
				return
			}
			a.failAllInstances("opencode shared server process exited unexpectedly")
			a.detachShared()
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			err := a.probeShared(ctx, shared)
			cancel()
			if err == nil {
				continue
			}
			a.sharedMu.Lock()
			current := a.shared
			a.sharedMu.Unlock()
			if current != shared {
				return
			}
			ctx2, cancel2 := context.WithTimeout(context.Background(), 3*time.Second)
			err2 := a.probeShared(ctx2, shared)
			cancel2()
			if err2 == nil {
				continue
			}
			a.failAllInstances("opencode shared server became unreachable")
			a.detachShared()
			return
		}
	}
}

// failAllInstances marks every registered instance failed so the platform
// record stops reporting usable instances whose server is gone.
func (a *Adapter) failAllInstances(detail string) {
	a.mu.Lock()
	instances := make([]*instance, 0, len(a.instances))
	for _, inst := range a.instances {
		instances = append(instances, inst)
	}
	a.mu.Unlock()
	for _, inst := range instances {
		inst.fail(detail)
	}
}

// launchSharedServer starts `opencode serve --service` on a loopback port.
// It is the default launcher used for the shared server; tests replace
// Adapter.launch with a fake.
func (a *Adapter) launchSharedServer(ctx context.Context, directory string) (*launchResult, error) {
	port, err := freePort()
	if err != nil {
		return nil, err
	}
	password, err := randomPassword()
	if err != nil {
		return nil, err
	}

	command, err := a.backendCommand(context.Background(), "serve", "--service", "--hostname", a.hostname, "--port", strconv.Itoa(port))
	if err != nil {
		return nil, err
	}
	if directory != "" {
		command.Dir = directory
	}
	command.Env = append(os.Environ(), "OPENCODE_SERVER_PASSWORD="+password)
	output := &limitedWriter{limit: maxStartupOutputBytes}
	command.Stdout = output
	command.Stderr = output
	if err := platform.New().Process().Configure(command); err != nil {
		return nil, fmt.Errorf("opencode: configure shared server process: %w", err)
	}
	command.WaitDelay = 2 * time.Second

	if err := command.Start(); err != nil {
		return nil, fmt.Errorf("opencode: start shared server process: %w", err)
	}
	// Enroll the server in platform supervision (the Windows Job Object);
	// a server the platform cannot supervise must not be left behind.
	if err := platform.New().Process().Attach(command.Process); err != nil {
		_ = platform.New().Process().Terminate(command.Process.Pid)
		_ = command.Wait()
		return nil, fmt.Errorf("opencode: supervise shared server process: %w", err)
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
		pid:      command.Process.Pid,
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

// sharedEndpoint reports the shared server endpoint, if any.
func (a *Adapter) sharedEndpoint() string {
	a.sharedMu.Lock()
	defer a.sharedMu.Unlock()
	if a.shared == nil {
		return ""
	}
	return a.shared.endpoint
}
