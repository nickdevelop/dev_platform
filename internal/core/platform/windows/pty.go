//go:build windows

package windows

import (
	"errors"
	"os/exec"
)

// ErrPTYUnsupported indicates terminal/pty is unsupported on Windows.
var ErrPTYUnsupported = errors.New("terminal is only supported on Linux")

// PTY stub for Windows.
type PTY struct{}

// Supported reports whether pseudoterminals are supported (false on Windows).
func (PTY) Supported() bool {
	return false
}

// Start always fails on Windows.
func (PTY) Start(cmd *exec.Cmd, rows, cols uint16) (any, error) {
	return nil, ErrPTYUnsupported
}
