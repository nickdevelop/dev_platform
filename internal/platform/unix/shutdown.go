//go:build unix

package unix

import (
	"os"
	"syscall"
)

// Shutdown is the unix termination story: SIGINT (Ctrl+C) and SIGTERM
// (stop, close, orchestration) both trigger the graceful shutdown path.
type Shutdown struct{}

// Signals returns the termination signals for signal.NotifyContext.
func (Shutdown) Signals() []os.Signal {
	return []os.Signal{syscall.SIGINT, syscall.SIGTERM}
}

// Interrupt asks a process to stop gracefully via SIGTERM.
func (Shutdown) Interrupt(p *os.Process) error {
	return p.Signal(syscall.SIGTERM)
}
