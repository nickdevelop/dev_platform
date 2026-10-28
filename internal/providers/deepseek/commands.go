package deepseek

import (
	"context"
	"fmt"
	"strings"

	"devplatform/internal/core/agents"
)

var _ agents.SessionCompactor = (*Adapter)(nil)

// CompactSession requests context compaction via dsh RPC.
// According to the platform command design, compact is a functional action command
// defined by the platform domain layer and implemented by the adapter.
//
// In dsh, compaction is exposed via the standard slash command system over the
// TypertRemoteService "commands/execute" RPC method with line "/compact".
func (a *Adapter) CompactSession(ctx context.Context, ref agents.SessionRef, instructions string) error {
	proc, release, err := a.host.Acquire(ctx)
	if err != nil {
		return err
	}
	defer release()

	client := proc.Client()

	line := "/compact"
	if trimmed := strings.TrimSpace(instructions); trimmed != "" {
		line = "/compact " + trimmed
	}

	var res struct {
		CommandID string `json:"commandId"`
		Result    struct {
			Kind           string `json:"kind"` // "success" or "error"
			Text           string `json:"text,omitempty"`
			SourceEventSeq int64  `json:"sourceEventSeq,omitempty"`
		} `json:"result"`
	}

	payload := map[string]any{
		"agentId":              ref.SessionID,
		"line":                 line,
		"submittedAttachments": []any{},
	}

	err = client.Call(ctx, "commands/execute", payload, &res)
	if err != nil {
		// Fallback for older or customized dsh host configurations
		var fallbackRes struct {
			Success bool   `json:"success"`
			Error   string `json:"error,omitempty"`
		}
		if err2 := client.Call(ctx, "compaction/compact", map[string]any{
			"agentId": ref.SessionID,
		}, &fallbackRes); err2 == nil {
			if fallbackRes.Error != "" {
				return fmt.Errorf("deepseek: compact rejected: %s", fallbackRes.Error)
			}
			return nil
		}

		return fmt.Errorf("deepseek: execute /compact: %w", err)
	}

	if res.Result.Kind == "error" {
		// If dsh reports "no arguments", retry without arguments
		if strings.Contains(res.Result.Text, "no arguments") && line != "/compact" {
			payload["line"] = "/compact"
			if errRetry := client.Call(ctx, "commands/execute", payload, &res); errRetry == nil {
				if res.Result.Kind == "success" {
					return nil
				}
			}
		}
		return fmt.Errorf("deepseek: compact rejected: %s", res.Result.Text)
	}

	return nil
}
