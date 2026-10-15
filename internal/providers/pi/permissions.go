package pi

import (
	"context"
	"fmt"

	"devplatform/internal/core/agents"
)

// Compile-time proof that the pi adapter implements the optional approval
// extension. Whether the platform actually offers the control is governed by
// Capabilities().ToolConfirmation, which RPC mode declares and CLI mode does
// not.
var _ agents.PermissionReplier = (*Adapter)(nil)

// ListPermissions returns the session's pending tool confirmations. pi has no
// native listing command, so the adapter answers from its own pending table:
// a confirm dialog the child is blocked on.
func (a *Adapter) ListPermissions(ctx context.Context, ref agents.SessionRef) ([]agents.PermissionRequest, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := ref.Validate(); err != nil {
		return nil, err
	}
	inst, err := a.getInstance(ref.InstanceID)
	if err != nil {
		return nil, err
	}
	child := inst.child(ref.SessionID)
	if child == nil {
		return []agents.PermissionRequest{}, nil
	}
	return child.pending.list(ref.SessionID), nil
}

// ReplyPermission answers a pending confirmation by writing an
// extension_ui_response with the matching id. A reply for an id the child is
// no longer waiting on wraps agents.ErrReplyExpired, which the session layer
// turns into an HTTP 409. "always" additionally remembers the approval rule
// for the session so a later identical confirmation is auto-approved.
func (a *Adapter) ReplyPermission(ctx context.Context, ref agents.SessionRef, requestID, decision, message string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := ref.Validate(); err != nil {
		return err
	}
	if requestID == "" {
		return fmt.Errorf("pi: permission request id is required")
	}
	switch decision {
	case "once", "always", "reject":
	default:
		return fmt.Errorf("pi: permission decision %q must be once, always or reject", decision)
	}
	inst, err := a.getInstance(ref.InstanceID)
	if err != nil {
		return err
	}
	child := inst.child(ref.SessionID)
	if child == nil {
		return fmt.Errorf("%w: pi: no live session %q", agents.ErrReplyExpired, ref.SessionID)
	}
	entry, ok := child.pending.take(requestID)
	if !ok {
		return fmt.Errorf("%w: pi: permission request %q is not pending", agents.ErrReplyExpired, requestID)
	}

	confirmed := decision != "reject"
	response := rpcExtensionUIResponse{Type: "extension_ui_response", ID: requestID, Confirmed: &confirmed}
	if err := child.respond(response); err != nil {
		return fmt.Errorf("pi: reply permission %q: %w", requestID, err)
	}
	child.publishInteraction(agents.InteractionPayload{ID: requestID, Kind: agents.InteractionPermission, Phase: agents.InteractionResolved})
	if decision == "always" {
		a.rememberApproval(ref.InstanceID, ref.SessionID, confirmKey(entry.request.Action, entry.request.Message))
	}
	child.touch()
	return nil
}

// ListForms returns pending question forms. pi's approval subprotocol can emit
// select/input/editor dialogs, but the platform only maps confirm onto tool
// approval today, so there are no forms to report.
func (a *Adapter) ListForms(ctx context.Context, ref agents.SessionRef) ([]agents.FormInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := ref.Validate(); err != nil {
		return nil, err
	}
	return []agents.FormInfo{}, nil
}

// ReplyForm answers a pending question form. pi never surfaces one through the
// adapter today, so every reply is expired.
func (a *Adapter) ReplyForm(ctx context.Context, ref agents.SessionRef, formID string, answer map[string]any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := ref.Validate(); err != nil {
		return err
	}
	return fmt.Errorf("%w: pi: form %q is not pending", agents.ErrReplyExpired, formID)
}
