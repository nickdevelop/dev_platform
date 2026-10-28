package opencode

import (
	"context"
	"fmt"

	"devplatform/internal/core/agents"
)

var _ agents.SessionCompactor = (*Adapter)(nil)

// CompactSession requests context compaction on the OpenCode server.
// According to the platform command design, compact is a functional action command
// defined by the platform domain layer and implemented by the adapter.
func (a *Adapter) CompactSession(ctx context.Context, ref agents.SessionRef, instructions string) error {
	if ref.Backend != agents.BackendOpenCodeV2 {
		return fmt.Errorf("opencode: session backend %q is not opencode-v2", ref.Backend)
	}

	inst, err := a.getInstance(ref.InstanceID)
	if err != nil {
		return err
	}
	if err := inst.ensureRunning(); err != nil {
		return err
	}

	if err := inst.client.CompactSession(ctx, ref.SessionID); err != nil {
		return fmt.Errorf("opencode: compact session %q: %w", ref.SessionID, err)
	}

	return nil
}
