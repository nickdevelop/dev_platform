//go:build windows

package tools

import (
	"context"
	"os/exec"
)

func buildShellCommand(ctx context.Context, cmdStr string) *exec.Cmd {
	return exec.CommandContext(ctx, "cmd.exe", "/c", cmdStr)
}
