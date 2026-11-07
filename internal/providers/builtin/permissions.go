package builtin

import (
	"context"
	"fmt"

	"devplatform/internal/core/agents"
)

// Compile-time proof the builtin adapter implements the optional approval
// extension. The confirm permission mode raises permission interactions on
// the session stream; this extension answers them.
var _ agents.PermissionReplier = (*Adapter)(nil)

// ListPermissions returns the session's pending tool confirmations.
func (a *Adapter) ListPermissions(ctx context.Context, ref agents.SessionRef) ([]agents.PermissionRequest, error) {
	if err := ref.Validate(); err != nil {
		return nil, err
	}
	state := a.getSession(ref.InstanceID, ref.SessionID)
	if state == nil {
		return []agents.PermissionRequest{}, nil
	}
	return a.pendingOf(state), nil
}

// ReplyPermission answers a pending confirmation. A reply for an id no one is
// waiting on wraps agents.ErrReplyExpired (HTTP 409 at the session layer).
// "always" is remembered by the turn loop when it consumes the decision.
func (a *Adapter) ReplyPermission(ctx context.Context, ref agents.SessionRef, requestID, decision, message string) error {
	if err := ref.Validate(); err != nil {
		return err
	}
	if requestID == "" {
		return fmt.Errorf("builtin: permission request id is required")
	}
	switch decision {
	case "once", "always", "reject":
	default:
		return fmt.Errorf("builtin: permission decision %q must be once, always or reject", decision)
	}
	a.mu.Lock()
	pending, ok := a.pending[requestID]
	if !ok {
		a.mu.Unlock()
		return fmt.Errorf("%w: builtin: permission request %q is not pending", agents.ErrReplyExpired, requestID)
	}
	// Take the entry: exactly one reply is accepted, a second one becomes
	// expired like every other backend.
	delete(a.pending, requestID)
	a.mu.Unlock()

	// The channel is buffered, so a dropped (already cancelled) wait leaves
	// the decision unread and garbage collected.
	select {
	case pending.decision <- decision:
	default:
	}
	return nil
}

// pendingOf snapshots the pending requests of a session state.
func (a *Adapter) pendingOf(state *sessionState) []agents.PermissionRequest {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]agents.PermissionRequest, 0, len(state.pending))
	for _, entry := range state.pending {
		out = append(out, entry.request)
	}
	return out
}

// ListForms returns no pending question forms: the embedded agent has no
// question tool yet, its whole interaction rides standard turns.
func (a *Adapter) ListForms(ctx context.Context, ref agents.SessionRef) ([]agents.FormInfo, error) {
	return []agents.FormInfo{}, nil
}

// ReplyForm rejects form answers: there are no forms to answer.
func (a *Adapter) ReplyForm(ctx context.Context, ref agents.SessionRef, formID string, answer map[string]any) error {
	return fmt.Errorf("%w: builtin: form %s", agents.ErrReplyExpired, formID)
}
