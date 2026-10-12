package sessions

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"devplatform/internal/agents"
	"devplatform/internal/agents/instances"
	"devplatform/internal/agents/opencode"
)

// Service errors for the opencode approval replies. A reply that arrives
// after the request is gone (answered, cancelled or timed out server-side)
// is an expired reply (HTTP 409), never a generic backend failure (HTTP 502),
// so the workbench can drop the stale control with a clear message.
var (
	// ErrPermissionExpired reports a permission reply for a request the
	// server no longer holds.
	ErrPermissionExpired = errors.New("sessions: permission request expired")
	// ErrFormExpired reports a form reply for a form the server no longer
	// holds.
	ErrFormExpired = errors.New("sessions: form expired")
)

// PermissionRequest is the JSON-stable DTO for a pending tool approval. It
// aliases the opencode package's type so the service's structural assertion
// against the adapter matches exactly: a separately declared struct with the
// same shape would make the assertion silently fail (named return types must
// be identical, not merely identical in shape).
type PermissionRequest = opencode.PermissionRequest

// FormInfo is the JSON-stable DTO for a pending form, aliasing the opencode
// type for the same exact-match reason. Fields stays opaque JSON the
// workbench renders generically.
type FormInfo = opencode.FormInfo

// opencodeRef loads the session row, enforces project scope and that the
// backend is opencode-v2, and builds the native ref for adapter calls.
func (s *Service) opencodeRef(projectID uint, sessionID string) (agents.SessionRef, instances.Instance, error) {
	row, err := s.loadScoped(projectID, sessionID)
	if err != nil {
		return agents.SessionRef{}, instances.Instance{}, err
	}
	if agents.BackendID(row.Backend) != agents.BackendOpenCodeV2 {
		return agents.SessionRef{}, instances.Instance{}, fmt.Errorf("%w: not an opencode session", ErrUnsupportedCapability)
	}
	inst, err := s.scopedInstance(projectID, row.InstanceID)
	if err != nil {
		return agents.SessionRef{}, instances.Instance{}, err
	}
	if !instanceRunnable(inst) {
		return agents.SessionRef{}, instances.Instance{}, fmt.Errorf("%w: %s is %s", instances.ErrInstanceNotRunning, inst.ID, inst.Status)
	}
	return agents.SessionRef{
		InstanceID: row.InstanceID,
		Backend:    agents.BackendOpenCodeV2,
		SessionID:  row.NativeSessionID,
		Title:      row.Title,
	}, inst, nil
}

// ListOpencodePermissions returns the session's pending permission requests.
func (s *Service) ListOpencodePermissions(ctx context.Context, projectID uint, sessionID string) ([]PermissionRequest, error) {
	ref, _, err := s.opencodeRef(projectID, sessionID)
	if err != nil {
		return nil, err
	}
	if !s.capabilities(agents.BackendOpenCodeV2).ToolConfirmation {
		return nil, fmt.Errorf("%w: tool confirmation", ErrUnsupportedCapability)
	}
	adapter, err := s.adapter(agents.BackendOpenCodeV2)
	if err != nil {
		return nil, err
	}
	replier, ok := adapter.(opencode.PermissionReplier)
	if !ok {
		return nil, fmt.Errorf("%w: tool confirmation", ErrUnsupportedCapability)
	}
	requests, err := replier.ListPermissions(ctx, ref)
	if err != nil {
		return nil, fmt.Errorf("%w: list permissions: %w", ErrBackendFailed, err)
	}
	if requests == nil {
		requests = []PermissionRequest{}
	}
	return requests, nil
}

// ReplyOpencodePermission answers a pending permission request.
func (s *Service) ReplyOpencodePermission(ctx context.Context, projectID uint, sessionID, requestID, decision, message string) error {
	ref, _, err := s.opencodeRef(projectID, sessionID)
	if err != nil {
		return err
	}
	if !s.capabilities(agents.BackendOpenCodeV2).ToolConfirmation {
		return fmt.Errorf("%w: tool confirmation", ErrUnsupportedCapability)
	}
	switch decision {
	case "once", "always", "reject":
	default:
		return fmt.Errorf("%w: decision must be once, always or reject", ErrInvalidRequest)
	}
	if requestID == "" {
		return fmt.Errorf("%w: permission request id is required", ErrInvalidRequest)
	}
	adapter, err := s.adapter(agents.BackendOpenCodeV2)
	if err != nil {
		return err
	}
	replier, ok := adapter.(opencode.PermissionReplier)
	if !ok {
		return fmt.Errorf("%w: tool confirmation", ErrUnsupportedCapability)
	}
	if err := replier.ReplyPermission(ctx, ref, requestID, decision, message); err != nil {
		if opencode.IsExpiredReplyError(err) {
			return fmt.Errorf("%w: permission request %q is gone: %w", ErrPermissionExpired, requestID, err)
		}
		return fmt.Errorf("%w: reply permission: %w", ErrBackendFailed, err)
	}
	return nil
}

// ListOpencodeForms returns the session's pending forms.
func (s *Service) ListOpencodeForms(ctx context.Context, projectID uint, sessionID string) ([]FormInfo, error) {
	ref, _, err := s.opencodeRef(projectID, sessionID)
	if err != nil {
		return nil, err
	}
	adapter, err := s.adapter(agents.BackendOpenCodeV2)
	if err != nil {
		return nil, err
	}
	replier, ok := adapter.(opencode.PermissionReplier)
	if !ok {
		return nil, fmt.Errorf("%w: forms", ErrUnsupportedCapability)
	}
	forms, err := replier.ListForms(ctx, ref)
	if err != nil {
		return nil, fmt.Errorf("%w: list forms: %w", ErrBackendFailed, err)
	}
	if forms == nil {
		forms = []FormInfo{}
	}
	return forms, nil
}

// ReplyOpencodeForm submits an answer to a pending form.
func (s *Service) ReplyOpencodeForm(ctx context.Context, projectID uint, sessionID, formID string, answer map[string]any) error {
	ref, _, err := s.opencodeRef(projectID, sessionID)
	if err != nil {
		return err
	}
	if formID == "" || len(answer) == 0 {
		return fmt.Errorf("%w: form id and answer are required", ErrInvalidRequest)
	}
	adapter, err := s.adapter(agents.BackendOpenCodeV2)
	if err != nil {
		return err
	}
	replier, ok := adapter.(opencode.PermissionReplier)
	if !ok {
		return fmt.Errorf("%w: forms", ErrUnsupportedCapability)
	}
	if err := replier.ReplyForm(ctx, ref, formID, answer); err != nil {
		if opencode.IsExpiredReplyError(err) {
			return fmt.Errorf("%w: form %q is gone: %w", ErrFormExpired, formID, err)
		}
		return fmt.Errorf("%w: reply form: %w", ErrBackendFailed, err)
	}
	return nil
}

// StreamOpencodeRaw proxies the shared server's untouched SSE bytes for one
// session (authorized by project-scoped session id). No normalization or
// filtering happens here: the frontend projects OpenCodeEvent itself.
func (s *Service) StreamOpencodeRaw(ctx context.Context, projectID uint, sessionID string, w io.Writer, flush func()) error {
	row, err := s.loadScoped(projectID, sessionID)
	if err != nil {
		return err
	}
	if row.Status == string(SessionStopped) {
		return fmt.Errorf("%w: session is stopped", ErrInvalidRequest)
	}
	if agents.BackendID(row.Backend) != agents.BackendOpenCodeV2 {
		return fmt.Errorf("%w: not an opencode session", ErrUnsupportedCapability)
	}
	inst, err := s.scopedInstance(projectID, row.InstanceID)
	if err != nil {
		return err
	}
	if !instanceRunnable(inst) {
		return fmt.Errorf("%w: %s is %s", instances.ErrInstanceNotRunning, inst.ID, inst.Status)
	}
	if !s.capabilities(agents.BackendOpenCodeV2).Streaming {
		return fmt.Errorf("%w: streaming", ErrUnsupportedCapability)
	}
	adapter, err := s.adapter(agents.BackendOpenCodeV2)
	if err != nil {
		return err
	}
	opener, ok := adapter.(opencode.RawEventOpener)
	if !ok {
		return fmt.Errorf("%w: streaming", ErrUnsupportedCapability)
	}
	body, err := opener.OpenRawEvents(ctx)
	if err != nil {
		return fmt.Errorf("%w: open raw events: %w", ErrBackendFailed, err)
	}
	defer body.Close()
	return copyRawSSE(ctx, body, w, flush)
}

// copyRawSSE relays SSE bytes line by line so each event is flushed
// promptly, with a comment heartbeat keeping idle streams alive.
func copyRawSSE(ctx context.Context, src io.Reader, dst io.Writer, flush func()) error {
	reader := bufio.NewReader(src)
	heartbeat := time.NewTicker(heartbeatInterval)
	defer heartbeat.Stop()
	type lineResult struct {
		line []byte
		err  error
	}
	lines := make(chan lineResult)
	go func() {
		defer close(lines)
		for {
			line, err := reader.ReadBytes('\n')
			if len(line) > 0 {
				// Copy before reporting an error so a final partial line is
				// still delivered.
				cp := make([]byte, len(line))
				copy(cp, line)
				select {
				case lines <- lineResult{line: cp}:
				case <-ctx.Done():
					return
				}
			}
			if err != nil {
				select {
				case lines <- lineResult{err: err}:
				case <-ctx.Done():
				}
				return
			}
		}
	}()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-heartbeat.C:
			if _, err := io.WriteString(dst, ": keep-alive\n\n"); err != nil {
				return err
			}
			if flush != nil {
				flush()
			}
		case result, ok := <-lines:
			if !ok {
				return nil
			}
			if len(result.line) > 0 {
				if _, err := dst.Write(result.line); err != nil {
					return err
				}
				if flush != nil {
					flush()
				}
			}
			if result.err != nil {
				if result.err == io.EOF {
					return nil
				}
				return result.err
			}
		}
	}
}
