package pi

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"time"

	"devplatform/internal/agents"
)

// processSpec is a fully-resolved, shell-free command description. Arguments
// are passed as distinct argv elements to exec.CommandContext; the adapter
// never builds a command string and never invokes a shell.
type processSpec struct {
	executable string
	args       []string
	dir        string
	env        []string
}

// process is a started pi child process with its protocol pipes.
type process struct {
	cmd    *exec.Cmd
	stdout io.ReadCloser
	stderr io.ReadCloser

	// done is closed after the child has been reaped. StopInstance waits on
	// it (bounded) so a stopped instance does not leave a zombie.
	done chan struct{}
}

// startProcess launches the child with the instance's context. The executable
// and arguments are validated by the adapter before this point; the working
// directory is the instance's authorized project directory. On unix the child
// leads its own process group so a stop can reap the whole tree.
func startProcess(ctx context.Context, spec processSpec) (*process, error) {
	if spec.executable == "" {
		return nil, errors.New("pi: executable is required")
	}
	if spec.dir == "" {
		return nil, errors.New("pi: working directory is required")
	}

	cmd := exec.CommandContext(ctx, spec.executable, spec.args...)
	cmd.Dir = spec.dir
	if len(spec.env) > 0 {
		cmd.Env = spec.env
	}
	setProcessGroup(cmd)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("pi: stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("pi: stderr pipe: %w", err)
	}

	// CommandContext's default cancel kills only the leader. Override it so
	// context cancellation (instance stop) tears down the process group.
	cmd.Cancel = func() error { return killProcessGroup(cmd.Process) }
	cmd.WaitDelay = 2 * time.Second

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("pi: start process: %w", err)
	}
	return &process{cmd: cmd, stdout: stdout, stderr: stderr, done: make(chan struct{})}, nil
}

// wait reaps the child and closes the process's done channel. Callers must
// finish draining stdout and stderr first.
func (p *process) wait() error {
	err := p.cmd.Wait()
	close(p.done)
	return err
}

// kill force-terminates the process group. It is safe to call more than once.
func (p *process) kill() {
	_ = killProcessGroup(p.cmd.Process)
}

// pid reports the child's process id.
func (p *process) pid() int {
	if p.cmd.Process == nil {
		return 0
	}
	return p.cmd.Process.Pid
}

// redactSecrets masks the value of anything that looks like a credential. It
// delegates to the shared agents helper so every diagnostic path — the
// adapters, the collector's crash report and raw CLI output — uses one
// definition. Diagnostics and error payloads are passed through it before
// they leave the adapter.
func redactSecrets(value string) string {
	return agents.RedactSecrets(value)
}
