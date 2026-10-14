package opencode

import (
	"context"
	"fmt"
	"time"

	"devplatform/internal/agents"
	"devplatform/internal/platform"
)

// The operator-facing half of the adapter: the platform settings page is the
// only place that may start or stop the machine-wide opencode service, and the
// only place that shows what the service looks like right now. Everything the
// workbench does goes through ensureShared, which observes and adopts but never
// acts.

var _ agents.ServiceController = (*Adapter)(nil)

// DetectService observes the machine-wide service: its registration file, the
// liveness of the process it names, and one API probe. It changes nothing —
// neither the service nor the platform's reference to it.
func (a *Adapter) DetectService(ctx context.Context) (agents.ServiceStatus, error) {
	status := a.baseStatus()

	reg, ok := a.readRegistration()
	if !ok {
		return status, nil // stopped: nothing is registered
	}
	status.Endpoint = reg.URL
	status.PID = reg.PID
	status.Version = reg.Version
	if !processAlive(reg.PID) {
		status.Detail = "the registration names a process that is gone"
		status.State = agents.ServiceStopped
		return status, nil
	}

	client, err := NewClient(ClientOptions{BaseURL: reg.URL, Username: defaultUsername, Password: reg.Password})
	if err != nil {
		status.State = agents.ServiceUnreachable
		status.Detail = redactSecrets(err.Error())
		return status, nil
	}
	probeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	info, err := client.Info(probeCtx)
	cancel()
	if err != nil {
		status.State = agents.ServiceUnreachable
		status.Detail = redactSecrets(err.Error())
		return status, nil
	}
	if info.PID != 0 && info.PID != reg.PID {
		status.State = agents.ServiceUnreachable
		status.Detail = fmt.Sprintf("the registration names pid %d but the service answers as pid %d; the registration is stale", reg.PID, info.PID)
		return status, nil
	}
	status.State = agents.ServiceRunning
	status.Version = info.Version
	return status, nil
}

// ServiceAvailable is the cheap gate the workbench asks before it does any
// work: a registered service whose process is alive. It deliberately makes no
// network request — a task must not wait on an HTTP round trip to learn
// something the registration file already answers, and a slow service is not an
// absent one.
func (a *Adapter) ServiceAvailable() error {
	reg, ok := a.readRegistration()
	if !ok {
		return fmt.Errorf("%w: no `opencode serve --service` is registered in %s", agents.ErrServiceNotRunning, a.serviceFile())
	}
	if !processAlive(reg.PID) {
		return fmt.Errorf("%w: the service registered in %s (pid %d) is not running", agents.ErrServiceNotRunning, a.serviceFile(), reg.PID)
	}
	return nil
}

// StartService starts the machine-wide service because an operator asked for it.
// A service that is already serving is left exactly as it is: the platform does
// not replace a process that may be serving other clients.
func (a *Adapter) StartService(ctx context.Context) (agents.ServiceStatus, error) {
	current, err := a.DetectService(ctx)
	if err == nil && current.State == agents.ServiceRunning {
		return current, fmt.Errorf("%w at %s", agents.ErrServiceAlreadyRunning, current.Endpoint)
	}

	result, err := a.launch(ctx, a.launchDirectory())
	if err != nil {
		return a.baseStatus(), fmt.Errorf("opencode: start service: %w", err)
	}
	if result == nil || result.client == nil || result.endpoint == "" {
		return a.baseStatus(), fmt.Errorf("opencode: start service: the launcher returned no usable server")
	}

	next := &sharedServer{client: result.client, endpoint: result.endpoint, done: result.done}
	a.sharedMu.Lock()
	previousStop := a.sharedStop
	a.shared = next
	a.sharedStop = result.stop
	a.sharedMu.Unlock()
	// A service the platform started earlier in this process is the platform's
	// to reap; that stays true across a restart from the settings page.
	if previousStop != nil {
		_ = previousStop(ctx)
	}
	go a.watchShared(next)

	status := a.baseStatus()
	status.State = agents.ServiceRunning
	status.Endpoint = result.endpoint
	status.PID = result.pid
	if info, err := a.probeClient(ctx, result.client); err == nil {
		status.Version = info.Version
	} else {
		status.Detail = redactSecrets(err.Error())
	}
	return status, nil
}

// StopService stops the machine-wide service because an operator asked for it,
// whoever started it. Confirming that decision is the settings page's job; the
// operator owns its consequences, including for the desktop client.
func (a *Adapter) StopService(ctx context.Context) (agents.ServiceStatus, error) {
	status := a.baseStatus()

	reg, ok := a.readRegistration()
	if !ok || !processAlive(reg.PID) {
		a.releaseShared(ctx, "opencode service stopped from platform settings")
		status.State = agents.ServiceStopped
		return status, nil
	}

	stopErr := platform.New().Process().Terminate(reg.PID)
	// The platform may have started this very process; reap it through the hook
	// it holds so nothing is left behind as a zombie.
	a.releaseShared(ctx, "opencode service stopped from platform settings")

	if stopErr != nil {
		status.State = agents.ServiceUnreachable
		status.Endpoint = reg.URL
		status.PID = reg.PID
		status.Detail = redactSecrets(stopErr.Error())
		return status, stopErr
	}
	status.State = agents.ServiceStopped
	status.Endpoint = reg.URL
	status.PID = reg.PID
	status.Detail = fmt.Sprintf("stopped pid %d", reg.PID)
	return status, nil
}

// releaseShared drops the platform's reference to the service and reports the
// instances that were bound to it. The process itself is stopped by the caller.
func (a *Adapter) releaseShared(_ context.Context, reason string) {
	a.sharedMu.Lock()
	stop := a.sharedStop
	a.shared = nil
	a.sharedStop = nil
	a.sharedMu.Unlock()
	a.failAllInstances(reason)
	if stop != nil {
		_ = stop(context.Background())
	}
}

// probeClient asks a service for its identity with a bounded timeout.
func (a *Adapter) probeClient(ctx context.Context, client *Client) (ServerInfo, error) {
	probeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return client.Info(probeCtx)
}

// baseStatus is an observation shell: the backend is supported, nothing is
// registered, and the executable the platform would use is known.
func (a *Adapter) baseStatus() agents.ServiceStatus {
	return agents.ServiceStatus{
		Backend:    agents.BackendOpenCodeV2,
		Supported:  true,
		State:      agents.ServiceStopped,
		Executable: a.execPath(),
		CheckedAt:  a.now(),
	}
}

// launchDirectory is the neutral working directory for a service the operator
// starts: machine-wide, serving no project of its own.
func (a *Adapter) launchDirectory() string {
	if a.serviceDirectory != "" {
		return a.serviceDirectory
	}
	return ""
}
