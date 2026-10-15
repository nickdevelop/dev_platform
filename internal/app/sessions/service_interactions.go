package sessions

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"devplatform/internal/core/agents"
	"devplatform/internal/app/instances"
)

// Service errors for the approval and form replies. A reply that arrives
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
// aliases the agents contract type so the session layer and the adapters
// share one declaration.
type PermissionRequest = agents.PermissionRequest

// FormInfo is the JSON-stable DTO for a pending form. Fields stays opaque
// JSON the workbench renders generically.
type FormInfo = agents.FormInfo

// nativeRef loads the session row, enforces project scope and builds the
// native ref for adapter calls. Backend-specific gating happens in the callers
// through declared capabilities or optional extensions, never by backend id.
func (s *Service) nativeRef(projectID uint, sessionID string) (agents.SessionRef, instances.Instance, error) {
	row, err := s.loadScoped(projectID, sessionID)
	if err != nil {
		return agents.SessionRef{}, instances.Instance{}, err
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
		Backend:    agents.BackendID(row.Backend),
		SessionID:  row.NativeSessionID,
		Title:      row.Title,
	}, inst, nil
}

// replierFor returns the backend's permission and form extension. A backend
// that does not implement it reports ErrUnsupportedCapability for what was
// asked, which is the same answer as a backend that declares no capability.
func (s *Service) replierFor(backend agents.BackendID, operation string) (agents.PermissionReplier, error) {
	adapter, err := s.adapter(backend)
	if err != nil {
		return nil, err
	}
	replier, ok := adapter.(agents.PermissionReplier)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedCapability, operation)
	}
	return replier, nil
}

// ListPermissionRequests returns the session's pending permission requests.
func (s *Service) ListPermissionRequests(ctx context.Context, projectID uint, sessionID string) ([]PermissionRequest, error) {
	ref, _, err := s.nativeRef(projectID, sessionID)
	if err != nil {
		return nil, err
	}
	if !s.capabilities(ref.Backend).ToolConfirmation {
		return nil, fmt.Errorf("%w: tool confirmation", ErrUnsupportedCapability)
	}
	replier, err := s.replierFor(ref.Backend, "tool confirmation")
	if err != nil {
		return nil, err
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

// ReplyPermissionRequest answers a pending permission request.
func (s *Service) ReplyPermissionRequest(ctx context.Context, projectID uint, sessionID, requestID, decision, message string) error {
	ref, _, err := s.nativeRef(projectID, sessionID)
	if err != nil {
		return err
	}
	if !s.capabilities(ref.Backend).ToolConfirmation {
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
	replier, err := s.replierFor(ref.Backend, "tool confirmation")
	if err != nil {
		return err
	}
	if err := replier.ReplyPermission(ctx, ref, requestID, decision, message); err != nil {
		if errors.Is(err, agents.ErrReplyExpired) {
			return fmt.Errorf("%w: permission request %q is gone: %w", ErrPermissionExpired, requestID, err)
		}
		return fmt.Errorf("%w: reply permission: %w", ErrBackendFailed, err)
	}
	return nil
}

// ListForms returns the session's pending forms.
func (s *Service) ListForms(ctx context.Context, projectID uint, sessionID string) ([]FormInfo, error) {
	ref, _, err := s.nativeRef(projectID, sessionID)
	if err != nil {
		return nil, err
	}
	replier, err := s.replierFor(ref.Backend, "forms")
	if err != nil {
		return nil, err
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

// ReplyForm submits an answer to a pending form.
func (s *Service) ReplyForm(ctx context.Context, projectID uint, sessionID, formID string, answer map[string]any) error {
	ref, _, err := s.nativeRef(projectID, sessionID)
	if err != nil {
		return err
	}
	if formID == "" || len(answer) == 0 {
		return fmt.Errorf("%w: form id and answer are required", ErrInvalidRequest)
	}
	replier, err := s.replierFor(ref.Backend, "forms")
	if err != nil {
		return err
	}
	if err := replier.ReplyForm(ctx, ref, formID, answer); err != nil {
		if errors.Is(err, agents.ErrReplyExpired) {
			return fmt.Errorf("%w: form %q is gone: %w", ErrFormExpired, formID, err)
		}
		return fmt.Errorf("%w: reply form: %w", ErrBackendFailed, err)
	}
	return nil
}

// StreamRawEvents proxies the backend's untouched event bytes for one
// session (authorized by project-scoped session id). No normalization or
// filtering happens here: the frontend projects the backend's own events.
func (s *Service) StreamRawEvents(ctx context.Context, projectID uint, sessionID string, w io.Writer, flush func()) error {
	row, err := s.loadScoped(projectID, sessionID)
	if err != nil {
		return err
	}
	if row.Status == string(SessionStopped) {
		return fmt.Errorf("%w: session is stopped", ErrInvalidRequest)
	}
	backend := agents.BackendID(row.Backend)
	if !s.capabilities(backend).RawEvents {
		return fmt.Errorf("%w: raw events", ErrUnsupportedCapability)
	}
	inst, err := s.scopedInstance(projectID, row.InstanceID)
	if err != nil {
		return err
	}
	if !instanceRunnable(inst) {
		return fmt.Errorf("%w: %s is %s", instances.ErrInstanceNotRunning, inst.ID, inst.Status)
	}
	adapter, err := s.adapter(backend)
	if err != nil {
		return err
	}
	opener, ok := adapter.(agents.RawEventOpener)
	if !ok {
		return fmt.Errorf("%w: raw events", ErrUnsupportedCapability)
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
