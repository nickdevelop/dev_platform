package opencode

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"devplatform/internal/core/agents"
)

type toolCallInfo struct {
	name  string
	input []byte
}

// rememberTool records a tool's name and input until its result is emitted.
func (inst *instance) rememberTool(sessionID, messageID, toolID, name string, input []byte) {
	if toolID == "" {
		return
	}
	inst.mu.Lock()
	defer inst.mu.Unlock()
	key := toolKey(sessionID, messageID, toolID)
	existing := inst.toolCalls[key]
	if name == "" {
		name = existing.name
	}
	if len(input) == 0 {
		input = existing.input
	}
	inst.toolCalls[key] = toolCallInfo{name: name, input: input}
}

// getTool returns a recorded tool info, if any.
func (inst *instance) getTool(sessionID, messageID, toolID string) toolCallInfo {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	return inst.toolCalls[toolKey(sessionID, messageID, toolID)]
}

// forgetTool drops a tool's recorded info once it finishes and returns it.
func (inst *instance) forgetTool(sessionID, messageID, toolID string) toolCallInfo {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	key := toolKey(sessionID, messageID, toolID)
	info := inst.toolCalls[key]
	delete(inst.toolCalls, key)
	return info
}

// toolKey scopes adapter-side tool bookkeeping to one message's tool.
func toolKey(sessionID, messageID, toolID string) string {
	return sessionID + "|" + messageID + "|" + toolID
}

// eventData is the union of the data bodies the adapter inspects. Unknown
// fields are ignored.
type eventData struct {
	SessionID          string           `json:"sessionID"`
	AssistantMessageID string           `json:"assistantMessageID"`
	Delta              string           `json:"delta"`
	Text               string           `json:"text"`
	ID                 string           `json:"id"`
	Name               string           `json:"name"`
	Input              json.RawMessage  `json:"input"`
	Content            json.RawMessage  `json:"content"`
	Metadata           json.RawMessage  `json:"metadata"`
	Error              *structuredError `json:"error"`
	Title              string           `json:"title"`
	Reason             string           `json:"reason"`
	Attempt            int              `json:"attempt"`
	Tokens             *WireTokens      `json:"tokens"`
	RequestID          string           `json:"requestID"`
	Action             string           `json:"action"`
	Resources          json.RawMessage  `json:"resources"`
	Message            string           `json:"message"`
	Form               *nativeFormInfo  `json:"form"`
}

// nativeFormInfo mirrors the form object of form.created.
type nativeFormInfo struct {
	ID        string          `json:"id"`
	SessionID string          `json:"sessionID"`
	Title     string          `json:"title"`
	Fields    json.RawMessage `json:"fields"`
}

// structuredError mirrors Session.StructuredError.
type structuredError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
	Status  int    `json:"status"`
}

// decodeEventData parses an event body into the union; a malformed body yields
// the zero value and is later preserved as EventUnknown.
func decodeEventData(raw json.RawMessage) eventData {
	var data eventData
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &data)
	}
	return data
}

// errorCode classifies a failure event type.
func errorCode(eventType string) string {
	switch eventType {
	case "session.error":
		return "session_error"
	case "session.step.failed":
		return "step_failed"
	default:
		return "execution_failed"
	}
}

// lifecycleDetail chooses human-readable context for a lifecycle event.
func lifecycleDetail(data eventData) string {
	if data.Title != "" {
		return data.Title
	}
	return data.Reason
}

// structuredErrorMessage extracts a failure's message, falling back to content.
func structuredErrorMessage(data eventData, content json.RawMessage) string {
	if data.Error != nil {
		if data.Error.Message != "" {
			return data.Error.Message
		}
		if data.Error.Type != "" {
			return data.Error.Type
		}
	}
	if len(content) > 0 {
		if text := flattenRawContent(content); text != "" {
			return text
		}
	}
	return "opencode reported a failure"
}

// retryMessage renders a scheduled-retry event.
func retryMessage(data eventData) string {
	message := structuredErrorMessage(data, nil)
	if data.Attempt > 0 {
		return fmt.Sprintf("retry attempt %d: %s", data.Attempt, message)
	}
	return message
}

// lifecycleState converts a native event type into a lifecycle state name.
func lifecycleState(eventType string) string {
	return strings.ReplaceAll(strings.TrimPrefix(eventType, "session."), ".", "_")
}

// boundedMetadata keeps only a size-bounded metadata body carrying edit file
// diffs; anything larger or without files is dropped so the event stays small.
func boundedMetadata(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 || len(raw) > agents.MaxRawEventBytes {
		return nil
	}
	meta := agents.ParseToolResultMetadata(raw)
	if meta == nil || len(meta.Files) == 0 {
		return nil
	}
	return append(json.RawMessage(nil), raw...)
}

// boundRaw copies a backend event body, capped at the contract's raw-blob bound
// (MaxRawEventBytes). It backs off to a UTF-8 rune boundary so the trim never
// splits a multi-byte character.
func boundRaw(raw []byte) json.RawMessage {
	if len(raw) <= agents.MaxRawEventBytes {
		return append(json.RawMessage(nil), raw...)
	}
	cut := agents.MaxRawEventBytes
	for cut > 0 && !utf8.RuneStart(raw[cut]) {
		cut--
	}
	return append(json.RawMessage(nil), raw[:cut]...)
}

func flattenRawContent(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text
	}
	var blocks []json.RawMessage
	if err := json.Unmarshal(raw, &blocks); err == nil {
		parts := make([]string, 0, len(blocks))
		for _, block := range blocks {
			if rendered := flattenRawContent(block); rendered != "" {
				parts = append(parts, rendered)
			}
		}
		return strings.Join(parts, "\n")
	}
	var object struct {
		Text    string          `json:"text"`
		Output  string          `json:"output"`
		Message string          `json:"message"`
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(raw, &object); err == nil {
		if object.Text != "" {
			return object.Text
		}
		if len(object.Content) > 0 {
			if rendered := flattenRawContent(object.Content); rendered != "" {
				return rendered
			}
		}
		if object.Output != "" {
			return object.Output
		}
		if object.Message != "" {
			return object.Message
		}
	}
	return string(raw)
}

// event builds an envelope populated with the instance identity. Sequence is
// assigned later, centrally, at publish time.
func (inst *instance) event(sessionID string) agents.NormalizedEvent {
	return agents.NormalizedEvent{
		InstanceID: inst.id,
		SessionID:  sessionID,
		Backend:    agents.BackendOpenCodeV2,
		Timestamp:  inst.now(),
	}
}

// turnEvent builds a turn boundary of the session.
func (inst *instance) turnEvent(sessionID string, payload agents.TurnPayload) agents.NormalizedEvent {
	event := inst.event(sessionID)
	event.Type = agents.EventTurn
	event.Turn = &payload
	return event
}

// turnStarted opens a new turn of the session and remembers its id so the
// matching end event carries it.
func (inst *instance) turnStarted(sessionID string) agents.NormalizedEvent {
	inst.mu.Lock()
	inst.turnSeq++
	id := fmt.Sprintf("%s-turn-%d", inst.id, inst.turnSeq)
	inst.turns[sessionID] = id
	inst.mu.Unlock()
	return inst.turnEvent(sessionID, agents.TurnPayload{ID: id, Phase: agents.TurnStarted})
}

// turnEnded closes the session's open turn. It reports false when no turn is
// open: the stream joined mid-turn, so there is no running state to settle.
func (inst *instance) turnEnded(sessionID string, outcome agents.TurnOutcome) ([]agents.NormalizedEvent, bool) {
	inst.mu.Lock()
	id, open := inst.turns[sessionID]
	if open {
		delete(inst.turns, sessionID)
	}
	inst.mu.Unlock()
	if !open {
		return nil, false
	}
	return []agents.NormalizedEvent{inst.turnEvent(sessionID, agents.TurnPayload{ID: id, Phase: agents.TurnEnded, Outcome: outcome})}, true
}

// interactionEvent builds an interaction boundary of the session.
func (inst *instance) interactionEvent(sessionID string, payload agents.InteractionPayload) agents.NormalizedEvent {
	event := inst.event(sessionID)
	event.Type = agents.EventInteraction
	event.Interaction = &payload
	return event
}

// resolvedInteraction settles one pending interaction. An event without an id
// cannot name what it settles, so it is preserved as unknown instead.
func (inst *instance) resolvedInteraction(sessionID string, kind agents.InteractionKind, id string, event *Event) []agents.NormalizedEvent {
	if id == "" {
		return []agents.NormalizedEvent{inst.unknown(event, sessionID)}
	}
	return []agents.NormalizedEvent{inst.interactionEvent(sessionID, agents.InteractionPayload{
		ID:    id,
		Kind:  kind,
		Phase: agents.InteractionResolved,
	})}
}

// requestID returns the native id of a permission or form event: requestID when
// present, otherwise the generic id field.
func (data eventData) requestID() string {
	if data.RequestID != "" {
		return data.RequestID
	}
	return data.ID
}

// errorEvent builds a typed error event.
func (inst *instance) errorEvent(sessionID, code, message string) agents.NormalizedEvent {
	event := inst.event(sessionID)
	event.Type = agents.EventError
	event.Error = &agents.ErrorPayload{Code: code, Message: truncate(redactSecrets(message))}
	return event
}

// lifecycleEvent builds a session-scoped lifecycle event.
func (inst *instance) lifecycleEvent(sessionID, state, detail string) agents.NormalizedEvent {
	event := inst.event(sessionID)
	event.Type = agents.EventLifecycle
	event.Lifecycle = &agents.LifecycleEvent{State: state, Detail: truncate(detail)}
	return event
}

// unknown preserves an event the adapter could not normalize, byte for byte, up
// to the contract's raw-blob bound.
func (inst *instance) unknown(event *Event, sessionID string) agents.NormalizedEvent {
	out := inst.event(sessionID)
	out.Type = agents.EventUnknown
	out.Raw = boundRaw(event.Raw())
	return out
}

// owns reports whether an event belongs to this instance: its session was seen
// before, or its location matches the instance directory (and the session is
// then learned).
func (inst *instance) owns(event *Event, sessionID string) bool {
	if sessionID == "" {
		return false
	}
	if inst.knownSession(sessionID) {
		return true
	}
	if event.Location != nil && sameDirectory(event.Location.Directory, inst.directory) {
		inst.registerSession(sessionID, "")
		return true
	}
	return false
}

// normalize maps one opencode event onto the normalized union. Events for other
// locations or sessions are ignored; a known session's unmapped event is
// preserved as EventUnknown.
func (inst *instance) normalize(event *Event) []agents.NormalizedEvent {
	data := decodeEventData(event.Data)
	sessionID := data.SessionID
	if sessionID == "" && data.Form != nil && data.Form.SessionID != "" {
		sessionID = data.Form.SessionID
	}
	if !inst.owns(event, sessionID) {
		return nil
	}

	switch event.Type {
	case "session.text.delta":
		out := inst.event(sessionID)
		out.Type = agents.EventMessage
		out.Message = &agents.MessagePayload{Role: "assistant", Text: data.Delta, Final: false, Model: inst.sessionModel(sessionID)}
		return []agents.NormalizedEvent{out}

	case "session.text.ended":
		out := inst.event(sessionID)
		out.Type = agents.EventMessage
		out.Message = &agents.MessagePayload{Role: "assistant", Text: data.Text, Final: true, Model: inst.sessionModel(sessionID)}
		return []agents.NormalizedEvent{out}

	case "session.text.started":
		return []agents.NormalizedEvent{inst.lifecycleEvent(sessionID, "text_started", "")}

	case "session.reasoning.started":
		return nil

	case "session.reasoning.delta":
		out := inst.event(sessionID)
		out.Type = agents.EventThinking
		out.Thinking = &agents.ThinkingPayload{Text: data.Delta, Final: false}
		return []agents.NormalizedEvent{out}

	case "session.reasoning.ended":
		out := inst.event(sessionID)
		out.Type = agents.EventThinking
		out.Thinking = &agents.ThinkingPayload{Text: data.Text, Final: true}
		return []agents.NormalizedEvent{out}

	case "session.tool.input.started":
		inst.rememberTool(sessionID, data.AssistantMessageID, data.ID, data.Name, nil)
		return nil

	case "session.tool.called":
		out := inst.event(sessionID)
		out.Type = agents.EventToolCall
		tool := inst.getTool(sessionID, data.AssistantMessageID, data.ID)
		name := tool.name
		if name == "" {
			name = data.Name
		}
		inst.rememberTool(sessionID, data.AssistantMessageID, data.ID, name, data.Input)
		out.ToolCall = &agents.ToolCallPayload{
			CallID: data.ID,
			Name:   name,
			Input:  data.Input,
		}
		return []agents.NormalizedEvent{out}

	case "session.tool.success":
		inst.forgetTool(sessionID, data.AssistantMessageID, data.ID)
		out := inst.event(sessionID)
		out.Type = agents.EventToolResult
		out.ToolResult = &agents.ToolResultPayload{
			CallID:   data.ID,
			Output:   flattenRawContent(data.Content),
			IsError:  false,
			Metadata: boundedMetadata(data.Metadata),
		}
		return []agents.NormalizedEvent{out}

	case "session.tool.failed":
		inst.forgetTool(sessionID, data.AssistantMessageID, data.ID)
		out := inst.event(sessionID)
		out.Type = agents.EventToolResult
		output := flattenRawContent(data.Content)
		if output == "" {
			output = structuredErrorMessage(data, nil)
		}
		out.ToolResult = &agents.ToolResultPayload{
			CallID:   data.ID,
			Output:   output,
			IsError:  true,
			Metadata: boundedMetadata(data.Metadata),
		}
		return []agents.NormalizedEvent{out}

	case "session.execution.started":
		return []agents.NormalizedEvent{inst.turnStarted(sessionID)}

	case "session.execution.succeeded":
		turn, _ := inst.turnEnded(sessionID, agents.TurnCompleted)
		return turn

	case "session.execution.interrupted":
		turn, _ := inst.turnEnded(sessionID, agents.TurnCancelled)
		return turn

	case "session.execution.failed":
		turn, _ := inst.turnEnded(sessionID, agents.TurnFailed)
		return append(turn, inst.errorEvent(sessionID, errorCode(event.Type), structuredErrorMessage(data, nil)))

	case "session.step.failed", "session.error":
		return []agents.NormalizedEvent{inst.errorEvent(sessionID, errorCode(event.Type), structuredErrorMessage(data, nil))}

	case "permission.asked":
		id := data.requestID()
		if id == "" {
			return []agents.NormalizedEvent{inst.unknown(event, sessionID)}
		}
		var resources []string
		_ = json.Unmarshal(data.Resources, &resources)
		if resources == nil {
			resources = []string{}
		}
		return []agents.NormalizedEvent{inst.interactionEvent(sessionID, agents.InteractionPayload{
			ID:    id,
			Kind:  agents.InteractionPermission,
			Phase: agents.InteractionRequested,
			Permission: &agents.PermissionRequest{
				ID:        id,
				SessionID: sessionID,
				Action:    data.Action,
				Resources: resources,
				Message:   data.Message,
			},
		})}

	case "permission.replied":
		return inst.resolvedInteraction(sessionID, agents.InteractionPermission, data.requestID(), event)

	case "form.created":
		if data.Form == nil || data.Form.ID == "" {
			return []agents.NormalizedEvent{inst.unknown(event, sessionID)}
		}
		return []agents.NormalizedEvent{inst.interactionEvent(sessionID, agents.InteractionPayload{
			ID:    data.Form.ID,
			Kind:  agents.InteractionForm,
			Phase: agents.InteractionRequested,
			Form: &agents.FormInfo{
				ID:        data.Form.ID,
				SessionID: sessionID,
				Title:     data.Form.Title,
				Fields:    data.Form.Fields,
			},
		})}

	case "form.replied", "form.cancelled":
		id := data.requestID()
		if id == "" && data.Form != nil {
			id = data.Form.ID
		}
		return inst.resolvedInteraction(sessionID, agents.InteractionForm, id, event)

	case "session.retry.scheduled":
		return []agents.NormalizedEvent{inst.errorEvent(sessionID, "retry_scheduled", retryMessage(data))}

	case "session.created":
		inst.registerSession(sessionID, data.Title)
		return []agents.NormalizedEvent{inst.lifecycleEvent(sessionID, "created", data.Title)}

	case "session.updated", "session.renamed", "session.moved", "session.deleted",
		"session.forked", "session.instructions.updated", "session.skill.activated",
		"session.step.started", "session.step.streamed",
		"session.compaction.started", "session.compaction.ended",
		"session.revert.staged", "session.revert.cleared", "session.revert.committed":
		return []agents.NormalizedEvent{inst.lifecycleEvent(sessionID, lifecycleState(event.Type), lifecycleDetail(data))}

	case "session.step.ended":
		lifecycle := inst.lifecycleEvent(sessionID, lifecycleState(event.Type), lifecycleDetail(data))
		lifecycle.Lifecycle.Usage = data.Tokens.Usage()
		return []agents.NormalizedEvent{lifecycle}

	default:
		return []agents.NormalizedEvent{inst.unknown(event, sessionID)}
	}
}
