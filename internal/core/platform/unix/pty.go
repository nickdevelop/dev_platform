//go:build unix

package unix

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"

	"github.com/creack/pty"
)

// PTY allocates pseudoterminals for interactive shell sessions.
type PTY struct{}

// Supported reports whether pseudoterminals are supported on this operating system.
func (PTY) Supported() bool {
	return runtime.GOOS == "linux"
}

// PTYSession wraps the master pty file descriptor and child process.
type PTYSession struct {
	ptmx *os.File
	cmd  *exec.Cmd
}

func (s *PTYSession) Read(p []byte) (int, error) {
	if s.ptmx == nil {
		return 0, errors.New("pty is closed")
	}
	return s.ptmx.Read(p)
}

func (s *PTYSession) Write(p []byte) (int, error) {
	if s.ptmx == nil {
		return 0, errors.New("pty is closed")
	}
	return s.ptmx.Write(p)
}

func (s *PTYSession) Close() error {
	var errs []error
	if s.ptmx != nil {
		if err := s.ptmx.Close(); err != nil {
			errs = append(errs, err)
		}
		s.ptmx = nil
	}
	if s.cmd != nil && s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
	}
	if len(errs) > 0 {
		return errs[0]
	}
	return nil
}

func (s *PTYSession) Resize(rows, cols uint16) error {
	if s.ptmx == nil {
		return errors.New("pty is closed")
	}
	return pty.Setsize(s.ptmx, &pty.Winsize{
		Rows: rows,
		Cols: cols,
	})
}

func (s *PTYSession) Wait() error {
	if s.cmd == nil {
		return nil
	}
	return s.cmd.Wait()
}

func (s *PTYSession) Pid() int {
	if s.cmd != nil && s.cmd.Process != nil {
		return s.cmd.Process.Pid
	}
	return 0
}

// Start launches cmd in a new pty with the given dimensions.
func (PTY) Start(cmd *exec.Cmd, rows, cols uint16) (*PTYSession, error) {
	if runtime.GOOS != "linux" {
		return nil, fmt.Errorf("terminal is only supported on Linux (current: %s)", runtime.GOOS)
	}
	if rows == 0 {
		rows = 24
	}
	if cols == 0 {
		cols = 80
	}
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{
		Rows: rows,
		Cols: cols,
	})
	if err != nil {
		return nil, fmt.Errorf("pty start: %w", err)
	}
	return &PTYSession{
		ptmx: ptmx,
		cmd:  cmd,
	}, nil
}
