package deepseek

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"devplatform/internal/core/agents"
)

// ListHistory reads back previous turns of an agent-native session using dsh's projection and page APIs.
func (a *Adapter) ListHistory(ctx context.Context, request agents.ListHistoryRequest) (agents.HistoryResult, error) {
	if err := request.Validate(); err != nil {
		return agents.HistoryResult{}, err
	}

	proc, release, err := a.host.Acquire(ctx)
	if err != nil {
		return agents.HistoryResult{}, err
	}
	defer release()

	client := proc.Client()

	// 1. Get session projection to find the latest sequence number (asOfSeq)
	var projResult struct {
		AsOfSeq int `json:"asOfSeq"`
	}
	err = client.Call(ctx, "session/projections", map[string]any{
		"request": map[string]any{
			"sessionId": request.SessionID,
		},
	}, &projResult)
	if err != nil {
		return agents.HistoryResult{}, fmt.Errorf("read session projections: %w", err)
	}

	throughSeq := projResult.AsOfSeq
	if throughSeq <= 0 {
		return agents.HistoryResult{Events: nil, Truncated: false}, nil
	}

	// 2. Fetch pages backwards or forwards
	type pageRecord struct {
		Type  string          `json:"type"`
		Event json.RawMessage `json:"event"`
	}
	type pageResult struct {
		Records []pageRecord `json:"records"`
		HasMore bool         `json:"hasMore"`
	}

	const pageSize = 100
	var collectedEvents []agents.NormalizedEvent
	normalizer := NewSessionEventNormalizer("", "")

	var beforeSeq *int
	truncated := false

	for {
		pageReq := map[string]any{
			"address": map[string]any{
				"kind":      "session",
				"sessionId": request.SessionID,
			},
			"throughSeq":  throughSeq,
			"maxMessages": pageSize,
		}
		if beforeSeq != nil {
			pageReq["beforeSeq"] = *beforeSeq
		}

		var page pageResult
		err := client.Call(ctx, "session/page", map[string]any{
			"request": pageReq,
		}, &page)
		if err != nil {
			return agents.HistoryResult{}, fmt.Errorf("read session page: %w", err)
		}

		if len(page.Records) == 0 {
			break
		}

		// Parse backwards page
		for _, rec := range page.Records {
			if rec.Type == "event" && len(rec.Event) > 0 {
				frame := map[string]any{
					"type":  "event",
					"event": json.RawMessage(rec.Event),
				}
				frameBytes, _ := json.Marshal(frame)
				evs := normalizer.NormalizeFrame(frameBytes)
				// History replay carries transcript events only (message, thinking, tool_call, tool_result)
				for _, ev := range evs {
					if ev.Type == agents.EventMessage || ev.Type == agents.EventThinking ||
						ev.Type == agents.EventToolCall || ev.Type == agents.EventToolResult {
						collectedEvents = append(collectedEvents, ev)
						if len(collectedEvents) >= agents.MaxHistoryEvents {
							truncated = true
							break
						}
					}
				}
				if truncated {
					break
				}
			}
		}

		if truncated || !page.HasMore {
			break
		}

		// Find lowest seq for next page query
		var firstSeq struct {
			Seq int `json:"seq"`
		}
		if err := json.Unmarshal(page.Records[0].Event, &firstSeq); err == nil && firstSeq.Seq > 0 {
			beforeSeq = &firstSeq.Seq
		} else {
			break
		}
	}

	// Renumber sequences from 1 and ensure contract compliance (InstanceID and SessionID empty)
	for i := range collectedEvents {
		collectedEvents[i].Backend = agents.BackendDeepSeek
		collectedEvents[i].InstanceID = ""
		collectedEvents[i].SessionID = ""
		collectedEvents[i].Sequence = uint64(i + 1)
		if collectedEvents[i].Timestamp.IsZero() {
			collectedEvents[i].Timestamp = time.Now().UTC()
		}
	}

	return agents.HistoryResult{
		Events:    collectedEvents,
		Truncated: truncated,
	}, nil
}
