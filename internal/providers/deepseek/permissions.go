package deepseek

import (
	"context"
	"fmt"
	"sync"

	"devplatform/internal/core/agents"
)

// pendingForm keeps the platform-facing FormInfo together with the native
// question batch needed to convert an answer back into dsh's result shape.
type pendingForm struct {
	info      agents.FormInfo
	questions []dshQuestion
}

// PermissionsManager tracks pending tool approval and question requests across sessions.
type PermissionsManager struct {
	mu          sync.RWMutex
	permissions map[string]agents.PermissionRequest
	forms       map[string]pendingForm
}

// NewPermissionsManager creates a PermissionsManager.
func NewPermissionsManager() *PermissionsManager {
	return &PermissionsManager{
		permissions: make(map[string]agents.PermissionRequest),
		forms:       make(map[string]pendingForm),
	}
}

// AddPermission registers a pending permission request.
func (pm *PermissionsManager) AddPermission(req agents.PermissionRequest) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.permissions[req.ID] = req
}

// TakePermission removes and returns one pending permission request.
func (pm *PermissionsManager) TakePermission(id string) (agents.PermissionRequest, bool) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	req, ok := pm.permissions[id]
	if ok {
		delete(pm.permissions, id)
	}
	return req, ok
}

// RemovePermission removes a permission request.
func (pm *PermissionsManager) RemovePermission(id string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	delete(pm.permissions, id)
}

// AddForm registers a pending question form and the native questions behind it.
func (pm *PermissionsManager) AddForm(info agents.FormInfo, questions []dshQuestion) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.forms[info.ID] = pendingForm{info: info, questions: questions}
}

// TakeForm removes and returns one pending form.
func (pm *PermissionsManager) TakeForm(id string) (pendingForm, bool) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	form, ok := pm.forms[id]
	if ok {
		delete(pm.forms, id)
	}
	return form, ok
}

// ClearForms removes every pending form and returns them.
func (pm *PermissionsManager) ClearForms() []pendingForm {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	forms := make([]pendingForm, 0, len(pm.forms))
	for id, form := range pm.forms {
		forms = append(forms, form)
		delete(pm.forms, id)
	}
	return forms
}

// ClearPermissions removes every pending permission and returns them.
func (pm *PermissionsManager) ClearPermissions() []agents.PermissionRequest {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	permissions := make([]agents.PermissionRequest, 0, len(pm.permissions))
	for id, permission := range pm.permissions {
		permissions = append(permissions, permission)
		delete(pm.permissions, id)
	}
	return permissions
}

func (pm *PermissionsManager) permissionsFor(sessionID string) []agents.PermissionRequest {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	var out []agents.PermissionRequest
	for _, permission := range pm.permissions {
		if permission.SessionID == sessionID {
			out = append(out, permission)
		}
	}
	return out
}

func (pm *PermissionsManager) formsFor(sessionID string) []agents.FormInfo {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	var out []agents.FormInfo
	for _, form := range pm.forms {
		if form.info.SessionID == sessionID {
			out = append(out, form.info)
		}
	}
	return out
}

// ListPermissions returns pending permission requests for the given session.
func (a *Adapter) ListPermissions(ctx context.Context, ref agents.SessionRef) ([]agents.PermissionRequest, error) {
	return a.permissions.permissionsFor(ref.SessionID), nil
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

	proc, release, err := a.acquire(ctx)
	if err != nil {
		a.permissions.AddPermission(req)
		return err
	}
	defer release()

	client := proc.Client()
	payload := map[string]any{
		"clientId": client.ClientID(),
		"eventId":  requestID,
		"outcome": map[string]any{
			"kind":  "result",
			"value": approvalOutcome(decision),
		},
	}

	if err := client.Call(ctx, "$events/result", payload, nil); err != nil {
		a.permissions.AddPermission(req)
		return fmt.Errorf("reply permission to dsh: %w", err)
	}
	a.emitResolved(req.SessionID, agents.InteractionPermission, requestID)
	return nil
}

// ListForms returns pending question forms for the given session.
func (a *Adapter) ListForms(ctx context.Context, ref agents.SessionRef) ([]agents.FormInfo, error) {
	return a.permissions.formsFor(ref.SessionID), nil
}

// ReplyForm submits answers to a pending question form.
func (a *Adapter) ReplyForm(ctx context.Context, ref agents.SessionRef, formID string, answer map[string]any) error {
	a.permissions.mu.Lock()
	form, exists := a.permissions.forms[formID]
	if !exists || form.info.SessionID != ref.SessionID {
		a.permissions.mu.Unlock()
		return agents.ErrReplyExpired
	}
	delete(a.permissions.forms, formID)
	a.permissions.mu.Unlock()

	proc, release, err := a.acquire(ctx)
	if err != nil {
		a.permissions.AddForm(form.info, form.questions)
		return err
	}
	defer release()

	client := proc.Client()
	payload := map[string]any{
		"clientId": client.ClientID(),
		"eventId":  formID,
		"outcome": map[string]any{
			"kind":  "result",
			"value": buildAnswerBatch(form.questions, answer),
		},
	}

	if err := client.Call(ctx, "$events/result", payload, nil); err != nil {
		a.permissions.AddForm(form.info, form.questions)
		return fmt.Errorf("reply form to dsh: %w", err)
	}
	a.emitResolved(form.info.SessionID, agents.InteractionForm, formID)
	return nil
}
