package deepseek

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"devplatform/internal/core/agents"
)

// dsh forwards pending human interactions as waterfall frames on $events.
// This file translates those frames into the platform's interaction contract
// and converts the workbench's answers back into the native result shapes.

type waterfallFrame struct {
	Type    string          `json:"type"`
	Event   string          `json:"event"`
	EventID string          `json:"eventId"`
	AgentID string          `json:"agentId"`
	Request json.RawMessage `json:"request"`
}

type dshQuestionOption struct {
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

type dshQuestion struct {
	ID               string              `json:"id"`
	Question         string              `json:"question"`
	Detail           string              `json:"detail,omitempty"`
	Header           string              `json:"header,omitempty"`
	Options          []dshQuestionOption `json:"options,omitempty"`
	MultiSelect      bool                `json:"multiSelect,omitempty"`
	MultiSelectSnake bool                `json:"multi_select,omitempty"`
}

func (q dshQuestion) multiSelect() bool {
	return q.MultiSelect || q.MultiSelectSnake
}

type dshQuestionRequest struct {
	Questions []dshQuestion `json:"questions"`
}

type dshApprovalRequest struct {
	ToolName      string            `json:"toolName"`
	CallID        string            `json:"callId,omitempty"`
	Reason        any               `json:"reason,omitempty"`
	DisplayReason map[string]string `json:"displayReason,omitempty"`
}

// formField/formOption mirror the opaque field JSON the workbench renders
// (web/src/features/chat/interactions/types.ts).
type formOption struct {
	Value       string `json:"value"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

type formField struct {
	Key         string       `json:"key"`
	Type        string       `json:"type"`
	Title       string       `json:"title,omitempty"`
	Description string       `json:"description,omitempty"`
	Required    bool         `json:"required,omitempty"`
	Options     []formOption `json:"options,omitempty"`
	Custom      bool         `json:"custom,omitempty"`
}

type dshAnswer struct {
	ID       string   `json:"id"`
	Selected []string `json:"selected"`
	Custom   string   `json:"custom,omitempty"`
}

type dshAnswerBatch struct {
	Answers []dshAnswer `json:"answers"`
}

// bindEvents attaches this adapter to a process's $events stream exactly once.
// A restarted dsh host produces a new client, so the old listeners are removed
// and the new ones registered.
func (a *Adapter) bindEvents(client *Client) {
	if client == nil {
		return
	}

	a.eventsMu.Lock()
	defer a.eventsMu.Unlock()
	if a.eventsClient == client {
		return
	}
	if a.eventsOff != nil {
		a.eventsOff()
	}
	if a.eventsClosedOff != nil {
		a.eventsClosedOff()
	}
	a.eventsClient = client
	a.eventsOff = client.OnEvent(func(raw json.RawMessage) {
		a.handleClientEvent(client, raw)
	})
	a.eventsClosedOff = client.OnEventClosed(func() {
		a.handleClientClosed(client)
	})
}

func (a *Adapter) handleClientEvent(client *Client, raw json.RawMessage) {
	a.eventsMu.Lock()
	current := a.eventsClient == client
	a.eventsMu.Unlock()
	if !current {
		return
	}

	var frame waterfallFrame
	if err := json.Unmarshal(raw, &frame); err != nil {
		return
	}

	switch frame.Type {
	case "waterfall":
		if frame.EventID == "" || frame.AgentID == "" {
			return
		}
		sessionID, instanceID := a.rootAndInstance(frame.AgentID)
		switch frame.Event {
		case "user-questions/request":
			a.handleUserQuestions(instanceID, sessionID, frame.EventID, frame.Request)
		case "approval/request":
			a.handleApproval(instanceID, sessionID, frame.EventID, frame.Request)
		}
	case "cancel":
		if frame.EventID != "" {
			a.cancelPendingInteraction(frame.EventID)
		}
	}
}

func (a *Adapter) handleClientClosed(client *Client) {
	a.eventsMu.Lock()
	current := a.eventsClient == client
	a.eventsMu.Unlock()
	if !current {
		return
	}

	for _, form := range a.permissions.ClearForms() {
		a.emitInteraction(a.instanceForSession(form.info.SessionID), form.info.SessionID, agents.InteractionPayload{
			ID:    form.info.ID,
			Kind:  agents.InteractionForm,
			Phase: agents.InteractionResolved,
		})
	}
	for _, permission := range a.permissions.ClearPermissions() {
		a.emitInteraction(a.instanceForSession(permission.SessionID), permission.SessionID, agents.InteractionPayload{
			ID:    permission.ID,
			Kind:  agents.InteractionPermission,
			Phase: agents.InteractionResolved,
		})
	}
}

func (a *Adapter) handleUserQuestions(instanceID, sessionID, eventID string, raw json.RawMessage) {
	var request dshQuestionRequest
	if err := json.Unmarshal(raw, &request); err != nil || len(request.Questions) == 0 {
		return
	}

	fields, title := buildFormFields(request.Questions)
	encoded, err := json.Marshal(fields)
	if err != nil {
		return
	}
	info := agents.FormInfo{
		ID:        eventID,
		SessionID: sessionID,
		Title:     title,
		Fields:    encoded,
	}
	a.permissions.AddForm(info, request.Questions)
	a.emitInteraction(instanceID, sessionID, agents.InteractionPayload{
		ID:    eventID,
		Kind:  agents.InteractionForm,
		Phase: agents.InteractionRequested,
		Form:  &info,
	})
}

func (a *Adapter) handleApproval(instanceID, sessionID, eventID string, raw json.RawMessage) {
	var request dshApprovalRequest
	if err := json.Unmarshal(raw, &request); err != nil {
		return
	}

	action := request.ToolName
	if action == "" {
		action = "tool"
	}
	var resources []string
	if request.CallID != "" {
		resources = append(resources, request.CallID)
	}
	permission := agents.PermissionRequest{
		ID:        eventID,
		SessionID: sessionID,
		Action:    action,
		Resources: resources,
		Message:   approvalMessage(request),
	}
	a.permissions.AddPermission(permission)
	a.emitInteraction(instanceID, sessionID, agents.InteractionPayload{
		ID:         eventID,
		Kind:       agents.InteractionPermission,
		Phase:      agents.InteractionRequested,
		Permission: &permission,
	})
}

func (a *Adapter) cancelPendingInteraction(eventID string) {
	if form, ok := a.permissions.TakeForm(eventID); ok {
		a.emitResolved(form.info.SessionID, agents.InteractionForm, eventID)
		return
	}
	if permission, ok := a.permissions.TakePermission(eventID); ok {
		a.emitResolved(permission.SessionID, agents.InteractionPermission, eventID)
	}
}

func (a *Adapter) emitResolved(sessionID string, kind agents.InteractionKind, eventID string) {
	a.emitInteraction(a.instanceForSession(sessionID), sessionID, agents.InteractionPayload{
		ID:    eventID,
		Kind:  kind,
		Phase: agents.InteractionResolved,
	})
}

func (a *Adapter) emitInteraction(instanceID, sessionID string, payload agents.InteractionPayload) {
	if sessionID == "" {
		return
	}
	event := agents.NormalizedEvent{
		Backend:     agents.BackendDeepSeek,
		InstanceID:  instanceID,
		SessionID:   sessionID,
		Timestamp:   time.Now().UTC(),
		Type:        agents.EventInteraction,
		Interaction: &payload,
	}
	a.deliver(instanceID, event)
}

// deliver fans one adapter event out to an instance's subscribers. Interaction
// events are not part of a replay buffer, so they carry sequence 0.
func (a *Adapter) deliver(instanceID string, event agents.NormalizedEvent) {
	if instanceID == "" {
		return
	}
	a.mu.Lock()
	subs := make([]*subscription, 0, len(a.subscribers[instanceID]))
	for sub := range a.subscribers[instanceID] {
		subs = append(subs, sub)
	}
	a.mu.Unlock()

	for _, sub := range subs {
		sub.send(event)
	}
}

// recordSession remembers which platform instance owns a native session and,
// for subagent sessions, its native parent.
func (a *Adapter) recordSession(instanceID, sessionID, parent string) {
	if sessionID == "" {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if instanceID != "" {
		a.sessionInstances[sessionID] = instanceID
	}
	if parent != "" {
		a.sessionParents[sessionID] = parent
	}
}

// rootAndInstance resolves a native agent id (a root session id or a subagent
// session id) to the root conversation and the platform instance that owns it.
func (a *Adapter) rootAndInstance(sessionID string) (string, string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	seen := make(map[string]struct{})
	current := sessionID
	for current != "" {
		if _, ok := seen[current]; ok {
			break
		}
		seen[current] = struct{}{}
		parent := a.sessionParents[current]
		if parent == "" {
			break
		}
		current = parent
	}

	if instanceID := a.sessionInstances[current]; instanceID != "" {
		return current, instanceID
	}
	// The ancestry may be incomplete for a session discovered late; fall back
	// to the id that is known to belong to an instance.
	if instanceID := a.sessionInstances[sessionID]; instanceID != "" {
		return sessionID, instanceID
	}
	return current, ""
}

func (a *Adapter) instanceForSession(sessionID string) string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.sessionInstances[sessionID]
}

func buildFormFields(questions []dshQuestion) ([]formField, string) {
	fields := make([]formField, 0, len(questions))
	title := "问题"
	for index, question := range questions {
		if index == 0 {
			if header := strings.TrimSpace(question.Header); header != "" {
				title = header
			}
		}

		field := formField{
			Key:      question.ID,
			Type:     "string",
			Title:    strings.TrimSpace(question.Header),
			Required: true,
		}
		if field.Title == "" {
			field.Title = question.Question
			field.Description = strings.TrimSpace(question.Detail)
		} else {
			field.Description = strings.TrimSpace(question.Question)
			if detail := strings.TrimSpace(question.Detail); detail != "" {
				if field.Description != "" {
					field.Description += "\n\n"
				}
				field.Description += detail
			}
		}
		if field.Title == "" {
			field.Title = question.ID
		}

		if len(question.Options) > 0 {
			field.Options = make([]formOption, 0, len(question.Options))
			for _, option := range question.Options {
				field.Options = append(field.Options, formOption{
					Value:       option.Label,
					Label:       option.Label,
					Description: option.Description,
				})
			}
			if question.multiSelect() {
				field.Type = "multiselect"
			} else {
				field.Custom = true
			}
		}
		fields = append(fields, field)
	}
	return fields, title
}

func approvalMessage(request dshApprovalRequest) string {
	for _, locale := range []string{"zh", "zh-Hans", "zh-CN", "en"} {
		if text := strings.TrimSpace(request.DisplayReason[locale]); text != "" {
			return text
		}
	}
	switch reason := request.Reason.(type) {
	case string:
		return strings.TrimSpace(reason)
	case map[string]any:
		for _, key := range []string{"zh", "en", "message", "text"} {
			if text, ok := reason[key].(string); ok && strings.TrimSpace(text) != "" {
				return strings.TrimSpace(text)
			}
		}
	}
	return ""
}

// buildAnswerBatch converts the workbench's opaque answer map into the batch
// shape dsh expects: one answer per question, selected labels separate from
// custom text.
func buildAnswerBatch(questions []dshQuestion, answer map[string]any) map[string]any {
	batch := dshAnswerBatch{Answers: make([]dshAnswer, 0, len(questions))}
	for _, question := range questions {
		labels := make(map[string]struct{}, len(question.Options))
		for _, option := range question.Options {
			labels[option.Label] = struct{}{}
		}

		selected := []string{}
		custom := []string{}
		for _, value := range answerValues(answer[question.ID]) {
			if _, ok := labels[value]; ok {
				selected = append(selected, value)
			} else {
				custom = append(custom, value)
			}
		}
		batch.Answers = append(batch.Answers, dshAnswer{
			ID:       question.ID,
			Selected: selected,
			Custom:   strings.Join(custom, "\n"),
		})
	}
	return map[string]any{"answers": batch.Answers}
}

func answerValues(value any) []string {
	switch typed := value.(type) {
	case nil:
		return nil
	case string:
		if typed == "" {
			return nil
		}
		return []string{typed}
	case []string:
		return typed
	case []any:
		values := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok && text != "" {
				values = append(values, text)
			}
		}
		return values
	default:
		return []string{fmt.Sprint(typed)}
	}
}

func approvalOutcome(decision string) string {
	switch strings.ToLower(strings.TrimSpace(decision)) {
	case "allow", "allowed", "accepted", "yes", "once", "always", "approve", "approved":
		return "allowed-once"
	case "cancel", "cancelled", "canceled":
		return "cancelled"
	default:
		return "rejected"
	}
}
