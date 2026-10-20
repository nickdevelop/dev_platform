//go:build windows

package deepseek

import (
	"context"
	"os/exec"

	"devplatform/internal/core/platform"
)

func init() {
	Descriptor.ResolveExecutable = func(name string) (string, error) {
		return platform.New().Executables().Resolve(name)
	}
}

func (a *Adapter) probePath() (string, error) {
	if a.resolveExecutable != nil {
		if path := a.resolveExecutable(); path != "" {
			return path, nil
		}
	}
	return platform.New().Executables().Resolve(a.execPath())
}

func (a *Adapter) backendCommand(ctx context.Context, args ...string) (*exec.Cmd, error) {
	path, err := a.probePath()
	if err != nil {
		return nil, err
	}
	return exec.CommandContext(ctx, path, args...), nil
}
