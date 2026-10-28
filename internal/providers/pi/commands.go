package pi

import (
	"context"
	"fmt"

	"devplatform/internal/core/agents"
)

var _ agents.SessionCompactor = (*Adapter)(nil)

// CompactSession requests context compaction via Pi's RPC compact command.
// According to the platform command design, compact is a functional action command
// defined by the platform domain layer and implemented by the adapter.
func (a *Adapter) CompactSession(ctx context.Context, ref agents.SessionRef, instructions string) error {
	if ref.Backend != agents.BackendPi {
		return fmt.Errorf("pi: session backend %q is not pi", ref.Backend)
	}

	inst, err := a.getInstance(ref.InstanceID)
	if err != nil {
		return err
	}
	if err := inst.ensureRunning(); err != nil {
		return err
	}

	state, _ := a.lookupSession(ref.InstanceID, ref.SessionID)
	model := ""
	if state != nil {
		model = state.model
	}
	name := a.sessionTitle(ref.InstanceID, ref.SessionID)

	child, err := inst.ensureChild(ctx, ref.SessionID, func() (*rpcChild, error) {
		return a.startRPCChild(inst, ref.SessionID, model, name)
	})
	if err != nil {
		return fmt.Errorf("pi: ensure child for compact: %w", err)
	}
	defer child.touch()

	return child.compact(ctx, instructions)
}
