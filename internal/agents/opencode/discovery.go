package opencode

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// serviceRegistration mirrors the desktop discovery contract
// (packages/client/src/service.ts Info): the registration file is the
// complete discovery contract for the machine-wide `opencode serve --service`.
type serviceRegistration struct {
	ID       string `json:"id"`
	Version  string `json:"version"`
	URL      string `json:"url"`
	PID      int    `json:"pid"`
	Password string `json:"password"`
}

// serviceFile returns the registration file path. ServiceFile in Options
// overrides it (tests); otherwise XDG_STATE_HOME/opencode/service.json,
// falling back to ~/.local/state like the desktop fallback().
func (a *Adapter) serviceFile() string {
	if a.serviceFilePath != "" {
		return a.serviceFilePath
	}
	return defaultServiceFile()
}

func defaultServiceFile() string {
	state := os.Getenv("XDG_STATE_HOME")
	if state == "" {
		home, err := os.UserHomeDir()
		if err != nil || home == "" {
			return ""
		}
		state = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(state, "opencode", "service.json")
}

// readRegistration reads the machine-wide registration file and applies the
// minimal validation both adoption and detection need: the file must parse and
// name a url and a pid. Process liveness and API health are separate questions,
// answered by the callers.
func (a *Adapter) readRegistration() (serviceRegistration, bool) {
	path := a.serviceFile()
	if path == "" {
		return serviceRegistration{}, false
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return serviceRegistration{}, false
	}
	var reg serviceRegistration
	if err := json.Unmarshal(raw, &reg); err != nil {
		return serviceRegistration{}, false
	}
	if reg.URL == "" || reg.PID <= 0 {
		return serviceRegistration{}, false
	}
	if parsed, err := url.Parse(reg.URL); err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return serviceRegistration{}, false
	}
	return reg, true
}

// discoverExistingService adopts a healthy machine-wide service without
// starting one (desktop Service.discover semantics). It returns nil when no
// usable registration exists: missing/unparseable file, unreachable server,
// pid mismatch, dead process, or an unready/failed server. A version outside
// the tested range is still adopted — diagnostics already warns about it —
// because killing a user's running service to replace it would be worse.
//
// The adopted server is external: the returned stop hook is nil so dropping the
// reference never kills a process the platform did not start.
func (a *Adapter) discoverExistingService(ctx context.Context) (*sharedServer, error) {
	reg, ok := a.readRegistration()
	if !ok {
		return nil, nil
	}
	if !processAlive(reg.PID) {
		return nil, nil
	}
	client, err := NewClient(ClientOptions{BaseURL: reg.URL, Username: defaultUsername, Password: reg.Password})
	if err != nil {
		return nil, nil
	}
	probeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	info, err := client.Info(probeCtx)
	cancel()
	if err != nil {
		return nil, nil
	}
	// The registration must describe the process that answered: a pid change
	// means the file is stale (same check as desktop probeResult).
	if info.PID != reg.PID {
		return nil, nil
	}
	if reg.Version != "" && info.Version != reg.Version {
		return nil, nil
	}
	return &sharedServer{client: client, endpoint: reg.URL, done: nil}, nil
}

// processAlive reports whether pid refers to a live process.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	if err := syscall.Kill(pid, 0); err != nil {
		return false
	}
	return true
}

var _ = fmt.Sprint
