//go:build unix

package pi

import (
	"context"
	"os/exec"

	"devplatform/internal/core/platform"
)

// probePath resolves the executable for pre-flight checks.
func (a *Adapter) probePath() (string, error) {
	return platform.New().Executables().Resolve(a.execPath())
}

// backendCommand builds a one-shot process to run. Unix executes the
// resolved name directly; PATH lookup happens at Start.
func (a *Adapter) backendCommand(ctx context.Context, args ...string) (*exec.Cmd, error) {
	return exec.CommandContext(ctx, a.execPath(), args...), nil
}

// resolveSpec passes the launch spec through unchanged: the executable name
// resolves via PATH at Start.
func (a *Adapter) resolveSpec(spec processSpec) (processSpec, error) {
	return spec, nil
}
