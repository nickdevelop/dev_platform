package builtin

import (
	"context"
	"fmt"
	"strings"

	"devplatform/internal/core/agents"
)

var _ agents.SessionCompactor = (*Adapter)(nil)

// CompactSession implements agents.SessionCompactor for the embedded agent.
// It summarizes earlier conversation turns to relieve context window pressure.
func (a *Adapter) CompactSession(ctx context.Context, ref agents.SessionRef, instructions string) error {
	if ref.Backend != agents.BackendBuiltin {
		return fmt.Errorf("builtin: session backend %q is not builtin", ref.Backend)
	}

	inst, err := a.getInstance(ref.InstanceID)
	if err != nil {
		return err
	}

	state := a.getSession(ref.InstanceID, ref.SessionID)
	if state == nil {
		return fmt.Errorf("builtin: session %q not found", ref.SessionID)
	}

	if a.SessionBusy(ref.SessionID) {
		return fmt.Errorf("builtin: session %q is busy", ref.SessionID)
	}

	records, err := a.loadTranscriptRecords(ctx, ref.SessionID)
	if err != nil {
		return fmt.Errorf("builtin: load transcript for compaction: %w", err)
	}

	// If there are very few records, compaction is unnecessary.
	if len(records) <= 4 {
		return nil
	}

	modelName := ""
	if state != nil {
		modelName = state.model
	}
	endpoint, err := a.resolveEndpoint(ctx, modelName)
	if err != nil {
		return fmt.Errorf("builtin: resolve model for compaction: %w", err)
	}

	// Prepare history text for summarization
	var historyBuilder strings.Builder
	for _, rec := range records {
		switch rec.Kind {
		case TranscriptUser:
			historyBuilder.WriteString("User: " + rec.Content + "\n")
		case TranscriptAssistant:
			if rec.Content != "" {
				historyBuilder.WriteString("Assistant: " + rec.Content + "\n")
			}
		case TranscriptToolCall:
			// Summarize that a tool was called
			historyBuilder.WriteString("[Tool Call]\n")
		case TranscriptToolResult:
			// Summarize that a tool returned
			historyBuilder.WriteString("[Tool Output]\n")
		}
	}

	prompt := "请将以下会话历史总结为一段简明扼要的上下文摘要。总结必须包含：\n" +
		"1. 用户的主要意图与目标\n" +
		"2. 已经完成的关键任务、修改或创建的重要文件\n" +
		"3. 当前进行中的任务及关键上下文\n\n"
	if strings.TrimSpace(instructions) != "" {
		prompt += "用户附加指示：" + strings.TrimSpace(instructions) + "\n\n"
	}
	prompt += "会话历史如下：\n" + historyBuilder.String()

	messages := []chatMessage{
		textMessage("system", "你是一个专业的对话上下文提炼助手，负责将冗长的会话历史浓缩为紧凑、无遗漏的高信息密度摘要。"),
		textMessage("user", prompt),
	}

	result, err := a.completions.streamChat(ctx, endpoint, messages, nil, nil)
	if err != nil {
		return fmt.Errorf("builtin: compaction completion: %w", err)
	}

	summaryText := strings.TrimSpace(result.Text)
	if summaryText == "" {
		summaryText = "会话历史上下文已压缩。"
	}

	finalMessage := fmt.Sprintf("【上下文已压缩】\n%s", summaryText)
	now := a.now()

	// Persist the compaction summary as an assistant record
	a.recordSession(ref, state, func(seq int) ([]TranscriptRecord, error) {
		return []TranscriptRecord{{
			Seq:       seq,
			Kind:      TranscriptAssistant,
			Content:   finalMessage,
			CreatedAt: now,
		}}, nil
	})

	// Publish the event to the instance stream so clients immediately update
	inst.publish(agents.NormalizedEvent{
		Type:      agents.EventMessage,
		SessionID: ref.SessionID,
		Timestamp: now,
		Message: &agents.MessagePayload{
			Role:  "assistant",
			Text:  finalMessage,
			Final: true,
			Model: &agents.MessageModel{ID: endpoint.ModelID},
		},
	})

	return nil
}
