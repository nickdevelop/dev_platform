package agents

import (
	"context"
	"errors"
	"time"
)

// Optional contract for backends that expose one machine-level, long-lived
// service process which the platform talks to instead of owning. The opencode
// backend is such a backend: one `opencode serve --service` per machine, also
// used by the desktop client and by manually started servers, discovered
// through its registration file. CLI-shaped backends (pi) spawn a process per
// instance and do not implement it.
//
// The contract exists because that service is a third party: it does not serve
// the platform alone, so the platform must never start, stop or replace it from
// a business path. Only an operator, from the platform settings, decides that
// the service should start or stop — and owns that decision. Business code only
// asks whether the service is usable right now.

// ServiceState is the observed state of a backend's machine-level service.
type ServiceState string

const (
	// ServiceRunning means the service is registered, its process is alive and
	// its API answered.
	ServiceRunning ServiceState = "running"
	// ServiceStopped means no usable registration exists: the file is missing
	// or unreadable, or the process it names is gone.
	ServiceStopped ServiceState = "stopped"
	// ServiceUnreachable means the registration names a live process whose API
	// did not answer (or answered as something else).
	ServiceUnreachable ServiceState = "unreachable"
)

// ServiceStatus is one observation of a backend's machine-level service.
// Detail carries an operator-readable, already-redacted reason when the state is
// not running.
type ServiceStatus struct {
	Backend    BackendID    `json:"backend"`
	Supported  bool         `json:"supported"`
	State      ServiceState `json:"state"`
	Endpoint   string       `json:"endpoint"`
	PID        int          `json:"pid"`
	Version    string       `json:"version"`
	Executable string       `json:"executable"`
	Detail     string       `json:"detail"`
	CheckedAt  time.Time    `json:"checkedAt"`
}

// ServiceController is the optional adapter capability for backends with a
// machine-level service. Implementations never call it from a business path:
// the platform settings page is the only caller of StartService and StopService.
type ServiceController interface {
	// DetectService performs the full observation: the registration file, the
	// process liveness check and one API probe. It reports what it finds and
	// never changes the service.
	DetectService(ctx context.Context) (ServiceStatus, error)
	// ServiceAvailable is the cheap check the platform runs before it does
	// business: it reads the registration file and checks the process is alive,
	// and makes no network request. It returns ErrServiceNotRunning when the
	// service cannot serve, and nil when it can.
	ServiceAvailable() error
	// StartService starts the machine-level service. When a usable service is
	// already running it returns ErrServiceAlreadyRunning and changes nothing:
	// the platform never replaces a service it did not start.
	StartService(ctx context.Context) (ServiceStatus, error)
	// StopService stops the machine-level service, whoever started it. The
	// operator is responsible for that decision; the platform does not second
	// guess it. Stopping a service that is not running succeeds.
	StopService(ctx context.Context) (ServiceStatus, error)
}

// Sentinel errors of the service contract. They are distinct so the HTTP layer
// can map each to its own status and code.
var (
	// ErrServiceNotRunning reports a backend whose machine-level service is not
	// usable, so the work it was asked for cannot proceed.
	ErrServiceNotRunning = errors.New("agents: agent service is not running")
	// ErrServiceAlreadyRunning reports a start request for a service that is
	// already serving.
	ErrServiceAlreadyRunning = errors.New("agents: agent service is already running")
	// ErrServiceUnsupported reports a backend with no machine-level service.
	ErrServiceUnsupported = errors.New("agents: backend has no machine-level service")
)
