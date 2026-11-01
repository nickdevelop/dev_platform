package deepseek

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
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
		Values  struct {
			ContextPressure *struct {
				ContextWindow   int `json:"contextWindow"`
				PressureTokens  int `json:"pressureTokens"`
				ProjectedTokens int `json:"projectedTokens"`
			} `json:"contextPressure"`
			TokenUsage *struct {
				UncachedInputTokens int `json:"uncachedInputTokens"`
				OutputTokens        int `json:"outputTokens"`
				CacheReadTokens     int `json:"cacheReadTokens"`
				CacheWriteTokens    int `json:"cacheWriteTokens"`
			} `json:"tokenUsage"`
		} `json:"values"`
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
	normalizer := NewSessionEventNormalizer("", "")
	if m := a.sessionModel(request.SessionID); m != nil {
		normalizer.SetCurrentModel(m)
	}

	type seqRecord struct {
		seq   int
		event json.RawMessage
	}
	var allRecords []seqRecord

	var beforeSeq *int
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

		minSeqInPage := 0
		for _, rec := range page.Records {
			if rec.Type == "event" && len(rec.Event) > 0 {
				var header struct {
					Seq int `json:"seq"`
				}
				_ = json.Unmarshal(rec.Event, &header)
				if header.Seq > 0 && (minSeqInPage == 0 || header.Seq < minSeqInPage) {
					minSeqInPage = header.Seq
				}
				allRecords = append(allRecords, seqRecord{
					seq:   header.Seq,
					event: rec.Event,
				})
			}
		}

		if !page.HasMore || minSeqInPage <= 0 {
			break
		}
		beforeSeq = &minSeqInPage
	}

	// Sort records strictly by chronological sequence number
	sort.SliceStable(allRecords, func(i, j int) bool {
		return allRecords[i].seq < allRecords[j].seq
	})

	var collectedEvents []agents.NormalizedEvent
	truncated := false
	for _, rec := range allRecords {
		frame := map[string]any{
			"type":  "event",
			"event": rec.event,
		}
		frameBytes, _ := json.Marshal(frame)
		evs := normalizer.NormalizeFrame(frameBytes)
		for _, ev := range evs {
			if ev.Type == agents.EventMessage || ev.Type == agents.EventThinking ||
				ev.Type == agents.EventToolCall || ev.Type == agents.EventToolResult ||
				(ev.Type == agents.EventLifecycle && (ev.Lifecycle.Usage != nil || ev.Lifecycle.ContextLimit != nil)) {
				collectedEvents = append(collectedEvents, ev)
			}
		}
	}

	if len(collectedEvents) > agents.MaxHistoryEvents {
		collectedEvents = collectedEvents[len(collectedEvents)-agents.MaxHistoryEvents:]
		truncated = true
	}

	// 3. If projections carry context/usage anchor and none was in history, attach an anchor event
	var initialLimit *int
	var initialUsage *agents.MessageUsage
	if cp := projResult.Values.ContextPressure; cp != nil {
		if cp.ContextWindow > 0 {
			w := cp.ContextWindow
			initialLimit = &w
		}
		if cp.PressureTokens > 0 {
			initialUsage = &agents.MessageUsage{Input: cp.PressureTokens}
		}
	}
	if tu := projResult.Values.TokenUsage; tu != nil {
		u := &agents.MessageUsage{
			Input:      tu.UncachedInputTokens,
			Output:     tu.OutputTokens,
			CacheRead:  tu.CacheReadTokens,
			CacheWrite: tu.CacheWriteTokens,
		}
		if u.Total() > 0 {
			initialUsage = u
		}
	}
	if initialLimit != nil || initialUsage != nil {
		hasAnchor := false
		for _, ev := range collectedEvents {
			if ev.Type == agents.EventLifecycle && (ev.Lifecycle.ContextLimit != nil || ev.Lifecycle.Usage != nil) {
				hasAnchor = true
				break
			}
		}
		if !hasAnchor && len(collectedEvents) < agents.MaxHistoryEvents {
			collectedEvents = append(collectedEvents, agents.NormalizedEvent{
				Type: agents.EventLifecycle,
				Lifecycle: &agents.LifecycleEvent{
					State:        "context_anchor",
					Usage:        initialUsage,
					ContextLimit: initialLimit,
				},
			})
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
