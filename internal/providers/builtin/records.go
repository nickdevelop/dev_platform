package builtin

import (
	"encoding/json"
	"time"

	"devplatform/internal/core/agents"
)

// The record builders turn runtime facts into stored transcript rows. Payload
// JSON keeps the full, unpruned data the LLM context needs; Content carries
// optional display text and stays empty for tool results.

// buildUserRecord stores one user message with its images (identity and the
// base64 body, bounded per image, so a rebuilt history keeps the attachments).
func buildUserRecord(seq int, message agents.SendMessageRequest, now time.Time) TranscriptRecord {
	payload := userRecord{}
	for _, image := range message.Images {
		stored := agentsImage{ID: image.ID, Name: image.Name, MediaType: image.MediaType}
		if len(image.Data) <= maxStoredImageBytes {
			stored.Data = image.Data
		}
		payload.Images = append(payload.Images, stored)
	}
	encoded, _ := json.Marshal(payload)
	return TranscriptRecord{
		Seq:       seq,
		Kind:      TranscriptUser,
		Content:   message.Text,
		Payload:   encoded,
		CreatedAt: now,
	}
}

// buildToolHandoffRecords stores the text of the assistant's pre-call note
// plus one record per tool invocation with its full arguments.
func buildToolHandoffRecords(seq int, result *assistantResult, now time.Time) []TranscriptRecord {
	records := make([]TranscriptRecord, 0, len(result.ToolCalls)+1)
	if result.Text != "" {
		records = append(records, TranscriptRecord{
			Seq:       seq,
			Kind:      TranscriptAssistant,
			Content:   result.Text,
			CreatedAt: now,
		})
		seq++
	}
	for _, call := range result.ToolCalls {
		records = append(records, buildToolCallRecord(seq, call.Function.Name, call.ID, json.RawMessage(call.Function.Arguments), now))
		seq++
	}
	return records
}

// buildToolCallRecord stores one tool invocation with its full arguments. A
// malformed model fragment has no replayable JSON form, so it is stored as an
// empty object: the call keeps its id and the matching tool result stays
// addressable when the transcript is rebuilt.
func buildToolCallRecord(seq int, toolName, callID string, input json.RawMessage, now time.Time) TranscriptRecord {
	if !json.Valid(input) {
		input = json.RawMessage("{}")
	}
	encoded, _ := json.Marshal(toolCallRecord{
		CallID:        callID,
		Name:          toolName,
		Input:         input,
		NeedsApproval: requiresApproval(toolName),
	})
	return TranscriptRecord{
		Seq:       seq,
		Kind:      TranscriptToolCall,
		Payload:   encoded,
		CreatedAt: now,
	}
}

// buildToolResultRecord stores one tool outcome: the model-facing output in
// the payload. Tool results leave Content empty; history replays Payload.Output.
func buildToolResultRecord(seq int, callID, output string, isErr bool, now time.Time) TranscriptRecord {
	encoded, _ := json.Marshal(toolResultRecord{
		CallID:  callID,
		Output:  output,
		IsError: isErr,
	})
	return TranscriptRecord{
		Seq:       seq,
		Kind:      TranscriptToolResult,
		Payload:   encoded,
		CreatedAt: now,
	}
}

// buildAssistantRecords stores the final reasoning and the final assistant
// text of one completion call.
func buildAssistantRecords(seq int, result *assistantResult, now time.Time) []TranscriptRecord {
	records := make([]TranscriptRecord, 0, 2)
	if result.Reasoning != "" {
		records = append(records, TranscriptRecord{
			Seq:       seq,
			Kind:      TranscriptThinking,
			Content:   result.Reasoning,
			CreatedAt: now,
		})
		seq++
	}
	if result.Text != "" {
		records = append(records, TranscriptRecord{
			Seq:       seq,
			Kind:      TranscriptAssistant,
			Content:   result.Text,
			CreatedAt: now,
		})
	}
	return records
}

// buildErrorRecord stores one turn error for replay and context.
func buildErrorRecord(seq int, code, message string, now time.Time) TranscriptRecord {
	return TranscriptRecord{
		Seq:       seq,
		Kind:      TranscriptError,
		Content:   message,
		CreatedAt: now,
	}
}

// userChatMessage renders the stored user message for the completer:
// a plain string message, or a multimodal parts array when images ride along.
func userChatMessage(message agents.SendMessageRequest, endpoint ModelEndpoint) chatMessage {
	if len(message.Images) == 0 {
		return textMessage("user", message.Text)
	}
	parts := make([]contentPart, 0, len(message.Images)+1)
	if message.Text != "" {
		parts = append(parts, contentPart{Type: "text", Text: message.Text})
	}
	for _, image := range message.Images {
		if !endpoint.SupportsImage {
			continue
		}
		parts = append(parts, contentPart{Type: "image_url", ImageURL: &imageURLRef{URL: image.Data}})
	}
	if len(parts) == 0 {
		return textMessage("user", message.Text)
	}
	encoded, _ := json.Marshal(parts)
	return chatMessage{Role: "user", Content: encoded}
}
