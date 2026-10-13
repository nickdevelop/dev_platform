package opencode

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"

	"devplatform/internal/agents"
)

// Permission decision values accepted by ReplyPermission. They mirror
// Permission.Reply in the v2 protocol (packages/schema/src/permission.ts).
const (
	PermissionOnce   = "once"
	PermissionAlways = "always"
	PermissionReject = "reject"
)

// PermissionReplier and RawEventOpener are the shared agents extensions; the
// aliases keep existing opencode-package callers compiling.
type (
	PermissionReplier = agents.PermissionReplier
	RawEventOpener    = agents.RawEventOpener
)

var (
	_ PermissionReplier = (*Adapter)(nil)
	_ RawEventOpener    = (*Adapter)(nil)
)

func validDecision(decision string) bool {
	switch decision {
	case PermissionOnce, PermissionAlways, PermissionReject:
		return true
	default:
		return false
	}
}

// IsExpiredReplyError reports whether err is the sidecar's "already gone"
// answer to a permission/form reply: the request id is unknown
// (PermissionNotFoundError/FormNotFoundError) or was already answered or
// cancelled (FormAlreadySettledError). Callers translate it into an expired
// reply (HTTP 409) instead of a generic backend failure (HTTP 502) so the
// workbench can drop the stale control with a clear message.
func IsExpiredReplyError(err error) bool {
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	switch apiErr.Code {
	case "PermissionNotFoundError", "FormNotFoundError", "FormAlreadySettledError":
		return true
	default:
		return false
	}
}

// replyFailure maps a failed permission or form reply onto the agents
// contract: a reply whose target the server no longer holds wraps
// agents.ErrReplyExpired, anything else is a plain backend failure.
func replyFailure(err error, what string) error {
	if IsExpiredReplyError(err) {
		return fmt.Errorf("%w: opencode: %s: %w", agents.ErrReplyExpired, what, err)
	}
	return fmt.Errorf("opencode: %s: %w", what, err)
}

// ListPermissions returns the native session's pending permission requests.
func (a *Adapter) ListPermissions(ctx context.Context, ref agents.SessionRef) ([]PermissionRequest, error) {
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
	if err := inst.ensureRunning(); err != nil {
		return nil, err
	}
	requests, err := inst.client.ListPermissions(ctx, ref.SessionID)
	if err != nil {
		return nil, fmt.Errorf("opencode: list permissions for session %q: %w", ref.SessionID, err)
	}
	return requests, nil
}

// ReplyPermission answers a pending permission request.
func (a *Adapter) ReplyPermission(ctx context.Context, ref agents.SessionRef, requestID, decision, message string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := ref.Validate(); err != nil {
		return err
	}
	if requestID == "" {
		return fmt.Errorf("opencode: permission request id is required")
	}
	if !validDecision(decision) {
		return fmt.Errorf("opencode: permission decision %q must be once, always or reject", decision)
	}
	inst, err := a.getInstance(ref.InstanceID)
	if err != nil {
		return err
	}
	if err := inst.ensureRunning(); err != nil {
		return err
	}
	if err := inst.client.ReplyPermission(ctx, ref.SessionID, requestID, decision, message); err != nil {
		return replyFailure(err, fmt.Sprintf("reply permission %q", requestID))
	}
	return nil
}

// ListForms returns the native session's pending forms.
func (a *Adapter) ListForms(ctx context.Context, ref agents.SessionRef) ([]FormInfo, error) {
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
	if err := inst.ensureRunning(); err != nil {
		return nil, err
	}
	forms, err := inst.client.ListForms(ctx, ref.SessionID)
	if err != nil {
		return nil, fmt.Errorf("opencode: list forms for session %q: %w", ref.SessionID, err)
	}
	return forms, nil
}

// ReplyForm submits an answer to a pending form.
func (a *Adapter) ReplyForm(ctx context.Context, ref agents.SessionRef, formID string, answer map[string]any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := ref.Validate(); err != nil {
		return err
	}
	if formID == "" {
		return fmt.Errorf("opencode: form id is required")
	}
	if len(answer) == 0 {
		return fmt.Errorf("opencode: form answer is required")
	}
	inst, err := a.getInstance(ref.InstanceID)
	if err != nil {
		return err
	}
	if err := inst.ensureRunning(); err != nil {
		return err
	}
	if err := inst.client.ReplyForm(ctx, ref.SessionID, formID, answer); err != nil {
		return replyFailure(err, fmt.Sprintf("reply form %q", formID))
	}
	return nil
}

// OpenRawEvents opens the shared server's untouched SSE stream. The caller
// owns the body and must close it.
func (a *Adapter) OpenRawEvents(ctx context.Context) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	a.sharedMu.Lock()
	shared := a.shared
	a.sharedMu.Unlock()
	if shared == nil {
		return nil, fmt.Errorf("opencode: no shared server; start an instance first")
	}
	response, err := shared.client.OpenRawEventStream(ctx)
	if err != nil {
		return nil, err
	}
	return response.Body, nil
}

var _ = http.StatusUnauthorized
