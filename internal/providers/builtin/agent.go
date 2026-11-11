package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"devplatform/internal/core/agents"
)

// turn is one in-flight conversation turn: its context cancels on interrupt
// or instance stop, and its id pairs the started and ended boundary events.
type turn struct {
	id     string
	ctx    context.Context
	cancel context.CancelFunc
}

// workspaceBrief describes the working environment for the system prompt.
// The agent is in-process, so there is no host CLI writing one for it.
func workspaceBrief(directory string) string {
	return fmt.Sprintf(
		"当前工作目录：%s\n\n你运行在 dev_platform 平台进程内，通过平台注入的工具读写该工作目录。遵循以下原则：\n"+
			"- 修改文件前先用 read_file 读取目标文件，避免盲目覆盖；edit_file 的 old_string 必须精确且唯一。\n"+
			"- 修改代码后按需运行项目自身的测试或构建命令验证。\n"+
			"- 工具长输出会被截断，重要信息请分步获取。\n"+
			"- 回复使用简体中文。", directory)
}

// systemPrompt assembles the turn system prompt: the platform's global agent
// prompt (dataDir/AGENTS.md) and the workspace brief. The prompt is re-read
// every turn so operator edits apply at once.
func (a *Adapter) systemPrompt(ctx context.Context, directory string) string {
	parts := make([]string, 0, 3)
	parts = append(parts, "你是 dev_platform 平台的内置智能编码 Agent，直接操作用户的项目工作区。")
	if a.prompt != nil {
		if prompt, err := a.prompt(ctx); err == nil && strings.TrimSpace(prompt) != "" {
			parts = append(parts, strings.TrimSpace(prompt))
		}
	}
	parts = append(parts, workspaceBrief(directory))
	return strings.Join(parts, "\n\n---\n\n")
}

// resolveEndpoint maps the session's model choice onto an endpoint, falling
// back to the first enabled model when no choice was made.
func (a *Adapter) resolveEndpoint(ctx context.Context, model string) (ModelEndpoint, error) {
	if strings.TrimSpace(model) != "" {
		return a.resolver.ResolveModel(ctx, model)
	}
	models, err := a.resolver.ListModels(ctx)
	if err != nil {
		return ModelEndpoint{}, fmt.Errorf("builtin: list models: %w", err)
	}
	if len(models) == 0 {
		return ModelEndpoint{}, ErrNoModels
	}
	return a.resolver.ResolveModel(ctx, models[0].ID)
}

// runTurn drives one user turn: it records the user message, loops streaming
// completions and tool executions, publishes events on the instance stream
// and persists the transcript to the platform database. All failures settle
// into a turn-failed event; nothing panics out of the goroutine.
func (a *Adapter) runTurn(inst *instance, ref agents.SessionRef, state *sessionState, current *turn, message agents.SendMessageRequest) {
	defer func() {
		a.mu.Lock()
		if state.running == current {
			state.running = nil
			state.cancelled = nil
		}
		a.mu.Unlock()
		current.cancel()
	}()
	ctx := current.ctx
	now := a.now()

	// Snapshot the persisted transcript before recording this turn's message:
	// the rebuilt context must hold prior turns only, otherwise the fresh user
	// message would appear twice in the request.
	records, err := a.loadTranscriptRecords(ctx, ref.SessionID)
	if err != nil {
		a.failTurn(inst, ref, current, "history_load_failed", err.Error())
		return
	}

	// Record and publish the user message, then open the turn.
	a.recordSession(ref, state, func(seq int) ([]TranscriptRecord, error) {
		return []TranscriptRecord{buildUserRecord(seq, message, now)}, nil
	})
	inst.publish(agents.NormalizedEvent{
		Type:      agents.EventMessage,
		SessionID: ref.SessionID,
		Timestamp: now,
		Message:   &agents.MessagePayload{Role: "user", Text: message.Text, Images: message.Images, Final: true},
	})
	inst.publish(agents.NormalizedEvent{
		Type:      agents.EventTurn,
		SessionID: ref.SessionID,
		Timestamp: now,
		Turn:      &agents.TurnPayload{ID: current.id, Phase: agents.TurnStarted},
	})

	endpoint, err := a.resolveEndpoint(ctx, state.model)
	if err != nil {
		a.failTurn(inst, ref, current, "model_unavailable", err.Error())
		return
	}
	_ = a.store.SetModel(ctx, ref.SessionID, endpoint.ModelID)

	// Rebuild the conversation context from the pre-turn snapshot (so a
	// session survives restarts) and add the fresh user message exactly once.
	history := renderHistoryMessages(records, endpoint)
	messages := make([]chatMessage, 0, len(history)+3)
	if prompt := a.systemPrompt(ctx, inst.directory); prompt != "" {
		messages = append(messages, textMessage("system", prompt))
	}
	messages = append(messages, history...)
	messages = append(messages, userChatMessage(message, endpoint))

	schemas := buildSchemas(a.registry)
	mode := a.permMode()
	for steps := 1; ctx.Err() == nil; steps++ {
		if steps > a.maxSteps {
			a.failTurn(inst, ref, current, "step_limit",
				fmt.Sprintf("单轮工具调用步数超过上限（%d 步），回合已终止。", a.maxSteps))
			return
		}

		// Deltas are published as they arrive. Buffering them until the
		// completion ends put the step's text behind the tool calls it
		// preceded, so a client saw the card before the sentence explaining it.
		result, streamErr := a.completions.streamChat(ctx, endpoint, messages, schemas, func(delta streamDelta) {
			for _, event := range mapStreamDelta(ref, delta, a.now()) {
				inst.publish(event)
			}
		})
		if streamErr != nil {
			if ctx.Err() != nil {
				a.cancelTurn(inst, ref, current, resultTextOf(result))
				return
			}
			a.failTurn(inst, ref, current, "model_error", streamErr.Error())
			return
		}

		// Reasoning closes with a full-snapshot final per completion call.
		if result.Reasoning != "" {
			inst.publish(agents.NormalizedEvent{
				Type:      agents.EventThinking,
				SessionID: ref.SessionID,
				Timestamp: a.now(),
				Thinking:  &agents.ThinkingPayload{Text: result.Reasoning, Final: true},
			})
		}

		if len(result.ToolCalls) == 0 {
			a.finalizeTurn(inst, ref, state, current, endpoint, result)
			return
		}

		// One assistant handoff message carries every tool call of the step;
		// each tool result answers against its call id. The handoff records only
		// the assistant's note: executeToolCall is the single write point of the
		// call records, so replay never sees a duplicated invocation.
		messages = append(messages, chatMessage{Role: "assistant", ToolCalls: result.ToolCalls})
		a.recordSession(ref, state, func(seq int) ([]TranscriptRecord, error) {
			return buildAssistantNoteRecords(seq, result, a.now()), nil
		})

		for _, call := range result.ToolCalls {
			if exit := a.executeToolCall(ctx, inst, ref, state, mode, current, &messages, call); exit == toolLoopExitCancelled {
				a.cancelTurn(inst, ref, current, result.Text)
				return
			}
		}
	}
	// ctx.Err() guarded the loop: the only way out here is an interrupt.
	a.cancelTurn(inst, ref, current, "")
}

// finalizeTurn settles a successful turn: the final assistant text is
// published as a full-snapshot close (the workbench replaces its accumulated
// stream prefix with it), persisted, and the turn boundary published.
func (a *Adapter) finalizeTurn(inst *instance, ref agents.SessionRef, state *sessionState, current *turn, endpoint ModelEndpoint, result *assistantResult) {
	a.recordSession(ref, state, func(seq int) ([]TranscriptRecord, error) {
		return buildAssistantRecords(seq, result, a.now()), nil
	})
	inst.publish(agents.NormalizedEvent{
		Type:      agents.EventMessage,
		SessionID: ref.SessionID,
		Timestamp: a.now(),
		Message: &agents.MessagePayload{
			Role:  "assistant",
			Text:  result.Text,
			Final: true,
			Model: &agents.MessageModel{ID: endpoint.ModelID},
		},
	})
	inst.publish(agents.NormalizedEvent{
		Type:      agents.EventTurn,
		SessionID: ref.SessionID,
		Timestamp: a.now(),
		Turn:      &agents.TurnPayload{ID: current.id, Phase: agents.TurnEnded, Outcome: agents.TurnCompleted},
	})
}

// failTurn publishes the failure, records the error and closes the turn.
func (a *Adapter) failTurn(inst *instance, ref agents.SessionRef, current *turn, code, text string) {
	inst.publish(agents.NormalizedEvent{
		Type:      agents.EventError,
		SessionID: ref.SessionID,
		Timestamp: a.now(),
		Error:     &agents.ErrorPayload{Code: code, Message: text},
	})
	inst.publish(agents.NormalizedEvent{
		Type:      agents.EventTurn,
		SessionID: ref.SessionID,
		Timestamp: a.now(),
		Turn:      &agents.TurnPayload{ID: current.id, Phase: agents.TurnEnded, Outcome: agents.TurnFailed},
	})
}

// cancelTurn closes an interrupted turn. Anything the model already streamed
// is lost to the stream but the user message and any tool records are kept,
// so a later turn still knows what happened so far.
func (a *Adapter) cancelTurn(inst *instance, ref agents.SessionRef, current *turn, streamedText string) {
	inst.publish(agents.NormalizedEvent{
		Type:      agents.EventTurn,
		SessionID: ref.SessionID,
		Timestamp: a.now(),
		Turn:      &agents.TurnPayload{ID: current.id, Phase: agents.TurnEnded, Outcome: agents.TurnCancelled},
	})
}

// loopExit reports how one tool invocation affected the turn loop.
type toolLoopExit int

const (
	toolLoopExitContinue toolLoopExit = iota
	toolLoopExitCancelled
)

// executeToolCall funnels one tool invocation through the announcement, the
// permission gate, the execution and the result recording. It appends the
// tool result message so the next completion sees the outcome.
func (a *Adapter) executeToolCall(
	ctx context.Context,
	inst *instance,
	ref agents.SessionRef,
	state *sessionState,
	mode string,
	current *turn,
	messages *[]chatMessage,
	call wireToolCall,
) toolLoopExit {
	callID := call.ID
	if callID == "" {
		callID = fmt.Sprintf("call-%d", time.Now().UnixNano())
	}
	toolName := call.Function.Name
	inputJSON := call.Function.Arguments

	// Announce the invocation with its raw arguments; the stored transcript
	// keeps the full arguments too, so both sides stay honest.
	inst.publish(agents.NormalizedEvent{
		Type:      agents.EventToolCall,
		SessionID: ref.SessionID,
		Timestamp: a.now(),
		ToolCall: &agents.ToolCallPayload{
			CallID: callID,
			Name:   toolName,
			Input:  json.RawMessage(normalizeToolArguments(inputJSON)),
		},
	})
	a.recordSession(ref, state, func(seq int) ([]TranscriptRecord, error) {
		// The record must use the same id the call event and its result carry,
		// including the generated fallback: recording the raw model id left the
		// replay unable to pair a result with its call.
		return []TranscriptRecord{buildToolCallRecord(seq, toolName, callID, json.RawMessage(inputJSON), a.now())}, nil
	})

	approved, cancelled := true, false
	if requiresApproval(toolName) && mode == PermissionModeConfirm {
		approved, cancelled = a.gateToolCall(ctx, inst, ref, state, toolName, inputJSON)
	}
	output, isErr := "", false
	switch {
	case cancelled:
		// The call was announced before the interrupt landed. Answer it anyway,
		// so the card settles instead of showing "running" forever.
		a.publishToolResult(inst, ref, state, callID, "工具调用已中断，未产生结果。", true)
		return toolLoopExitCancelled
	case !approved:
		output = "用户拒绝本次执行。如果需要继续，请先解释方案并重新请求授权。"
		isErr = true
	default:
		executed, execErr := a.registry.Execute(ctx, inst.directory, toolName, inputJSON)
		if execErr != nil && ctx.Err() != nil {
			a.publishToolResult(inst, ref, state, callID, "工具调用已中断，未产生结果。", true)
			return toolLoopExitCancelled
		}
		if execErr != nil {
			output = fmt.Sprintf("工具执行失败：%v", execErr)
			isErr = true
		} else {
			output = executed
		}
	}
	// Publish the raw tool output; the stored transcript keeps the same
	// model-facing output so later turns rebuild an honest context.
	a.publishToolResult(inst, ref, state, callID, output, isErr)

	*messages = append(*messages, chatMessage{
		Role:       "tool",
		Content:    jsonOrString(output),
		ToolCallID: callID,
	})
	return toolLoopExitContinue
}

// publishToolResult publishes one tool outcome and records it. The event and
// the record always carry the same call id, so a replay pairs them exactly as
// the live stream did.
func (a *Adapter) publishToolResult(inst *instance, ref agents.SessionRef, state *sessionState, callID, output string, isErr bool) {
	inst.publish(agents.NormalizedEvent{
		Type:      agents.EventToolResult,
		SessionID: ref.SessionID,
		Timestamp: a.now(),
		ToolResult: &agents.ToolResultPayload{
			CallID:  callID,
			Output:  output,
			IsError: isErr,
		},
	})
	a.recordSession(ref, state, func(seq int) ([]TranscriptRecord, error) {
		return []TranscriptRecord{buildToolResultRecord(seq, callID, output, isErr, a.now())}, nil
	})
}

// gateToolCall blocks for a permission decision in confirm mode. It reports
// whether the call is approved and whether the turn was cancelled instead of
// answered. An "always" decision remembers the invocation class for the rest
// of the runtime session; approvals do not survive a restart, like the
// approvals of every other backend.
func (a *Adapter) gateToolCall(ctx context.Context, inst *instance, ref agents.SessionRef, state *sessionState, toolName, inputJSON string) (bool, bool) {
	class := approvalClass(toolName, inputJSON)
	if a.sessionApproved(ref.InstanceID, ref.SessionID, class) {
		return true, false
	}

	requestID := ""
	if id, err := a.newID(); err == nil {
		requestID = id
	} else {
		requestID = fmt.Sprintf("perm-%d", time.Now().UnixNano())
	}
	var resources []string
	if target := approvalTarget(inputJSON); target != "" {
		resources = []string{target}
	} else {
		resources = []string{inst.directory}
	}
	request := agents.PermissionRequest{
		ID:        requestID,
		SessionID: ref.SessionID,
		Action:    toolName,
		Resources: resources,
		Message:   "内置 Agent 请求执行 " + toolName + " 操作",
	}
	pending := &permissionPending{request: request, decision: make(chan string, 1)}

	inst.publish(agents.NormalizedEvent{
		Type:      agents.EventInteraction,
		SessionID: ref.SessionID,
		Timestamp: a.now(),
		Interaction: &agents.InteractionPayload{
			ID:         requestID,
			Kind:       agents.InteractionPermission,
			Phase:      agents.InteractionRequested,
			Permission: &request,
		},
	})

	a.mu.Lock()
	a.pending[requestID] = pending
	state.pending[requestID] = pending
	a.mu.Unlock()

	defer func() {
		a.mu.Lock()
		delete(a.pending, requestID)
		delete(state.pending, requestID)
		a.mu.Unlock()
		inst.publish(agents.NormalizedEvent{
			Type:      agents.EventInteraction,
			SessionID: ref.SessionID,
			Timestamp: a.now(),
			Interaction: &agents.InteractionPayload{
				ID:    requestID,
				Kind:  agents.InteractionPermission,
				Phase: agents.InteractionResolved,
			},
		})
	}()

	select {
	case <-ctx.Done():
		return false, true
	case decision := <-pending.decision:
		switch decision {
		case "always":
			a.rememberApproval(ref.InstanceID, ref.SessionID, class)
			return true, false
		case "reject":
			return false, false
		default:
			return true, false
		}
	}
}

// approvalClass builds the "always"-approval key of one invocation. Paths
// collapse to their requested form so repeated edits of one file share a
// rule; commands collapse to the first line so repeated identical commands
// share a rule.
func approvalClass(toolName, inputJSON string) string {
	var args map[string]any
	_ = json.Unmarshal([]byte(inputJSON), &args)
	if path, ok := args["path"].(string); ok && path != "" {
		return toolName + "|" + path
	}
	if command, ok := args["command"].(string); ok && command != "" {
		return toolName + "|" + firstLine(command)
	}
	return toolName
}

// approvalTarget extracts the confirmation card's human-readable resource.
func approvalTarget(inputJSON string) string {
	var args map[string]any
	if err := json.Unmarshal([]byte(inputJSON), &args); err == nil {
		if path, ok := args["path"].(string); ok && path != "" {
			return path
		}
		if command, ok := args["command"].(string); ok && command != "" {
			return firstLine(command)
		}
	}
	return ""
}

func firstLine(value string) string {
	if index := strings.IndexAny(value, "\r\n"); index >= 0 {
		return value[:index]
	}
	return value
}

// jsonOrString encodes plain text content as a JSON string for the wire.
func jsonOrString(text string) json.RawMessage {
	encoded, _ := json.Marshal(text)
	return encoded
}

// resultTextOf extracts the aggregated text of a possibly nil result.
func resultTextOf(result *assistantResult) string {
	if result == nil {
		return ""
	}
	return result.Text
}

// mapStreamDelta maps one model stream delta onto normalized events:
// reasoning becomes thinking chunks and text becomes message chunks. Chunk
// events are not persisted; only the final aggregates are.
func mapStreamDelta(ref agents.SessionRef, delta streamDelta, now time.Time) []agents.NormalizedEvent {
	events := make([]agents.NormalizedEvent, 0, 2)
	if delta.Reasoning != "" {
		events = append(events, agents.NormalizedEvent{
			Type:      agents.EventThinking,
			SessionID: ref.SessionID,
			Timestamp: now,
			Thinking:  &agents.ThinkingPayload{Text: delta.Reasoning},
		})
	}
	if delta.Text != "" {
		events = append(events, agents.NormalizedEvent{
			Type:      agents.EventMessage,
			SessionID: ref.SessionID,
			Timestamp: now,
			Message:   &agents.MessagePayload{Role: "assistant", Text: delta.Text},
		})
	}
	return events
}

// recordSession persists a batch of transcript records, allocating sequence
// numbers from the session's counter, seeded from the store after a restart.
func (a *Adapter) recordSession(ref agents.SessionRef, state *sessionState, build func(seq int) ([]TranscriptRecord, error)) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if state.recordSeq == 0 {
		state.recordSeq = a.seededRecordCount(ref.SessionID)
	}
	records, err := build(state.recordSeq + 1)
	if err != nil {
		return
	}
	state.recordSeq += len(records)
	_ = a.store.Append(context.Background(), ref.SessionID, records)
}

// seededRecordCount recovers the record counter for a session restored after
// a restart by counting its stored transcript. The adapter lock is held.
func (a *Adapter) seededRecordCount(sessionID string) int {
	if sessionID == "" {
		return 0
	}
	records, err := a.store.Load(context.Background(), sessionID)
	if err != nil {
		return 0
	}
	return len(records)
}
