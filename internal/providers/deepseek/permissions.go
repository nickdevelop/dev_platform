package deepseek

import (
	"context"
	"fmt"
	"sync"

	"devplatform/internal/core/agents"
)

// PermissionsManager tracks pending tool approval and question requests across sessions.
type PermissionsManager struct {
	mu          sync.RWMutex
	permissions map[string]agents.PermissionRequest
	forms       map[string]agents.FormInfo
}

// NewPermissionsManager creates a PermissionsManager.
func NewPermissionsManager() *PermissionsManager {
	return &PermissionsManager{
		permissions: make(map[string]agents.PermissionRequest),
		forms:       make(map[string]agents.FormInfo),
	}
}

// AddPermission registers a pending permission request.
func (pm *PermissionsManager) AddPermission(req agents.PermissionRequest) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.permissions[req.ID] = req
}

// RemovePermission removes a permission request.
func (pm *PermissionsManager) RemovePermission(id string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	delete(pm.permissions, id)
}

// ListPermissions returns pending permission requests for the given session.
func (a *Adapter) ListPermissions(ctx context.Context, ref agents.SessionRef) ([]agents.PermissionRequest, error) {
	a.permissions.mu.RLock()
	defer a.permissions.mu.RUnlock()

	var out []agents.PermissionRequest
	for _, p := range a.permissions.permissions {
		if p.SessionID == ref.SessionID {
			out = append(out, p)
		}
	}
	return out, nil
}

// ReplyPermission submits an approval decision for a pending tool confirmation.
func (a *Adapter) ReplyPermission(ctx context.Context, ref agents.SessionRef, requestID, decision, message string) error {
	a.permissions.mu.Lock()
	req, exists := a.permissions.permissions[requestID]
	if !exists || req.SessionID != ref.SessionID {
		a.permissions.mu.Unlock()
		return agents.ErrReplyExpired
	}
	delete(a.permissions.permissions, requestID)
	a.permissions.mu.Unlock()

	proc, release, err := a.host.Acquire(ctx)
	if err != nil {
		return err
	}
	defer release()

	client := proc.Client()
	val := "rejected"
	if decision == "allow" || decision == "accepted" || decision == "yes" {
		val = "accepted"
	}

	payload := map[string]any{
		"clientId": client.ClientID(),
		"eventId":  requestID,
		"outcome": map[string]any{
			"kind":  "result",
			"value": val,
		},
	}

	if err := client.Call(ctx, "$events/result", payload, nil); err != nil {
		return fmt.Errorf("reply permission to dsh: %w", err)
	}
	return nil
}

// ListForms returns pending question forms for the given session.
func (a *Adapter) ListForms(ctx context.Context, ref agents.SessionRef) ([]agents.FormInfo, error) {
	a.permissions.mu.RLock()
	defer a.permissions.mu.RUnlock()

	var out []agents.FormInfo
	for _, f := range a.permissions.forms {
		if f.SessionID == ref.SessionID {
			out = append(out, f)
		}
	}
	return out, nil
}

// ReplyForm submits answers to a pending question form.
func (a *Adapter) ReplyForm(ctx context.Context, ref agents.SessionRef, formID string, answer map[string]any) error {
	a.permissions.mu.Lock()
	form, exists := a.permissions.forms[formID]
	if !exists || form.SessionID != ref.SessionID {
		a.permissions.mu.Unlock()
		return agents.ErrReplyExpired
	}
	delete(a.permissions.forms, formID)
	a.permissions.mu.Unlock()

	proc, release, err := a.host.Acquire(ctx)
	if err != nil {
		return err
	}
	defer release()

	client := proc.Client()
	payload := map[string]any{
		"clientId": client.ClientID(),
		"eventId":  formID,
		"outcome": map[string]any{
			"kind":  "result",
			"value": answer,
		},
	}

	if err := client.Call(ctx, "$events/result", payload, nil); err != nil {
		return fmt.Errorf("reply form to dsh: %w", err)
	}
	return nil
}
