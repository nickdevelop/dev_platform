package pi

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"devplatform/internal/core/agents"
)

// listModelsViaRPC reads pi's catalog from a short-lived `--mode rpc` child.
// The CLI's `--list-models` table has no display-name column, while the RPC
// get_available_models payload carries each model's configured name (including
// names from models.json). A one-shot child keeps the adapter's catalog
// authoritative without requiring an instance or an active session, and the
// caller falls back to `--list-models` when RPC is unavailable.
func (a *Adapter) listModelsViaRPC(ctx context.Context) ([]agents.ModelInfo, error) {
	spec := processSpec{
		executable: a.execPath(),
		// --no-session keeps this catalog probe out of pi's session store. The
		// catalog itself is machine-wide and independent of the directory.
		args:  []string{"--mode", "rpc", "--no-session"},
		dir:   os.TempDir(),
		env:   a.env,
		stdin: true,
	}
	spec, err := a.resolveSpec(spec)
	if err != nil {
		return nil, err
	}
	proc, err := startProcess(ctx, spec)
	if err != nil {
		return nil, fmt.Errorf("pi: start model catalog child: %w", err)
	}

	readDone := make(chan struct{})
	stderrDone := make(chan struct{})
	conn := newRPCConn(proc.stdin, newParser("models", "", a.now), func(agents.NormalizedEvent) {})
	go func() {
		defer close(readDone)
		_ = conn.readLoop(proc.stdout)
	}()
	go func() {
		defer close(stderrDone)
		_, _ = io.Copy(io.Discard, proc.stderr)
	}()
	defer func() {
		_ = proc.stdin.Close()
		proc.kill()
		<-readDone
		<-stderrDone
		_ = proc.wait()
	}()

	queryCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	response, err := conn.request(queryCtx, newGetAvailableModelsCommand(""), rpcCommandTimeout)
	if err != nil {
		return nil, fmt.Errorf("pi: model catalog request: %w", err)
	}
	if !response.Success {
		return nil, fmt.Errorf("pi: model catalog rejected: %s", redactSecrets(firstNonEmpty(response.Error, "unknown error")))
	}
	return parseAvailableModels(response.Data)
}
