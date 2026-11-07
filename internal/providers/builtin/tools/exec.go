package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"devplatform/internal/core/platform"
)

type execCommandArgs struct {
	Command   string `json:"command"`
	TimeoutMS int    `json:"timeout_ms,omitempty"`
}

var forbiddenCommands = []string{
	"rm -rf /",
	"rm -rf /*",
	"mkfs",
	":(){ :|:& };:",
	"dd if=/dev/zero",
	"dd if=/dev/null",
	"shutdown",
	"reboot",
	"init 0",
	"init 6",
}

func handleExecuteCommand(ctx context.Context, workDir string, argsJSON string) (string, error) {
	var args execCommandArgs
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("invalid execute_command arguments: %w", err)
	}
	cmdStr := strings.TrimSpace(args.Command)
	if cmdStr == "" {
		return "", errors.New("execute_command: command is required")
	}

	// Security check for catastrophic commands
	lowerCmd := strings.ToLower(cmdStr)
	for _, f := range forbiddenCommands {
		if strings.Contains(lowerCmd, f) {
			return "", fmt.Errorf("security policy: command contains forbidden pattern %q", f)
		}
	}

	timeoutMs := args.TimeoutMS
	if timeoutMs <= 0 {
		timeoutMs = 30000 // 30s
	} else if timeoutMs > 120000 {
		timeoutMs = 120000 // 120s max
	}

	execCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()

	cmd := buildShellCommand(execCtx, cmdStr)
	cmd.Dir = workDir

	// Configure process group via core platform abstraction
	osPlatform := platform.New()
	_ = osPlatform.Process().Configure(cmd)

	var stdoutBuf, stderrBuf limitedBuffer
	stdoutBuf.limit = 100 * 1024 // 100 KB
	stderrBuf.limit = 100 * 1024

	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	startErr := cmd.Start()
	if startErr != nil {
		return "", fmt.Errorf("start command: %w", startErr)
	}

	// Attach child to platform supervision so it's reaped if platform dies
	_ = osPlatform.Process().Attach(cmd.Process)

	waitErr := cmd.Wait()

	exitCode := 0
	if waitErr != nil {
		var exitErr *exec.ExitError
		if errors.As(waitErr, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else if errors.Is(execCtx.Err(), context.DeadlineExceeded) {
			return fmt.Sprintf("Command timed out after %d ms:\nStdout:\n%s\nStderr:\n%s", timeoutMs, stdoutBuf.String(), stderrBuf.String()), nil
		} else {
			return "", fmt.Errorf("command execution failed: %w", waitErr)
		}
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Exit Code: %d\n", exitCode))
	if stdoutBuf.Len() > 0 {
		sb.WriteString("Stdout:\n")
		sb.WriteString(stdoutBuf.String())
		if stdoutBuf.truncated {
			sb.WriteString("\n[... stdout truncated at 100KB ...]\n")
		}
	}
	if stderrBuf.Len() > 0 {
		if stdoutBuf.Len() > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString("Stderr:\n")
		sb.WriteString(stderrBuf.String())
		if stderrBuf.truncated {
			sb.WriteString("\n[... stderr truncated at 100KB ...]\n")
		}
	}
	if stdoutBuf.Len() == 0 && stderrBuf.Len() == 0 {
		sb.WriteString("(No output)")
	}

	return sb.String(), nil
}

type limitedBuffer struct {
	bytes.Buffer
	limit     int
	truncated bool
}

func (b *limitedBuffer) Write(p []byte) (n int, err error) {
	if b.truncated {
		return len(p), nil
	}
	remaining := b.limit - b.Buffer.Len()
	if remaining <= 0 {
		b.truncated = true
		return len(p), nil
	}
	if len(p) > remaining {
		b.truncated = true
		n, err = b.Buffer.Write(p[:remaining])
		if err != nil {
			return n, err
		}
		return len(p), nil
	}
	return b.Buffer.Write(p)
}
