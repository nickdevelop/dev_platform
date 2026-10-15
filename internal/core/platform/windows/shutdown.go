//go:build windows

package windows

import (
	"os"
	"syscall"
)

// Shutdown is the Windows termination story: os.Interrupt is Ctrl+C, and
// the Go runtime delivers console-close and logoff events as SIGTERM, so
// both arrive through signal.NotifyContext. (Close-event delivery needs
// real-machine confirmation; see the plan's 4.6 notes.)
type Shutdown struct{}

// Signals returns the termination signals for signal.NotifyContext.
func (Shutdown) Signals() []os.Signal {
	return []os.Signal{os.Interrupt, syscall.SIGTERM}
}

// Interrupt asks a test helper process to stop via Ctrl+C. It only reaches
// processes sharing the platform console; headless callers fall back to
// harder stops (the test helper kills after its grace period).
func (Shutdown) Interrupt(p *os.Process) error {
	return p.Signal(os.Interrupt)
}
