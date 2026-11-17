package agy

import (
	"context"
	"testing"
	"time"

	"devplatform/internal/core/agents"
)

const sampleUsageJSON = `{
  "conversation_id": "",
  "status": "SUCCESS",
  "response": "Gemini Models\tWeekly Limit Remaining\t81%\t2026-11-21T01:13:16Z\nClaude and GPT models\tWeekly Limit Remaining\t26%\t2026-11-22T06:55:54Z\n",
  "command": {
    "name": "usage",
    "data": {
      "description": "Quota is consumed proportionally to the cost of the tokens.",
      "groups": [
        {
          "name": "Gemini Models",
          "description": "Models within this group: Gemini 3.8 Flash",
          "buckets": [
            {
              "id": "gemini-weekly",
              "name": "Weekly Limit Remaining",
              "description": "You have used some of your weekly limit.",
              "window": "weekly",
              "remaining_fraction": 0.81,
              "reset_time": "2026-11-21T01:13:16Z"
            },
            {
              "id": "gemini-5h",
              "name": "Five Hour Limit Remaining",
              "window": "5h",
              "remaining_fraction": 0.93,
              "reset_time": "2026-11-17T04:34:46Z"
            }
          ]
        },
        {
          "name": "Claude and GPT models",
          "description": "Models within this group: Claude Sonnet 4.6",
          "buckets": [
            {
              "id": "3p-weekly",
              "name": "Weekly Limit Remaining",
              "window": "weekly",
              "remaining_fraction": 0.26,
              "reset_time": "2026-11-22T06:55:54Z"
            }
          ]
        }
      ]
    }
  }
}`

func TestParseUsageOutput(t *testing.T) {
	report, err := ParseUsageOutput([]byte(sampleUsageJSON))
	if err != nil {
		t.Fatalf("ParseUsageOutput failed: %v", err)
	}

	if report.Backend != agents.BackendAgy {
		t.Errorf("Backend = %q, want %q", report.Backend, agents.BackendAgy)
	}
	if report.Description != "Quota is consumed proportionally to the cost of the tokens." {
		t.Errorf("Description = %q", report.Description)
	}
	if len(report.Groups) != 2 {
		t.Fatalf("len(Groups) = %d, want 2", len(report.Groups))
	}

	geminiGroup := report.Groups[0]
	if geminiGroup.Name != "Gemini Models" {
		t.Errorf("Group 0 name = %q, want Gemini Models", geminiGroup.Name)
	}
	if len(geminiGroup.Buckets) != 2 {
		t.Fatalf("Gemini buckets len = %d, want 2", len(geminiGroup.Buckets))
	}
	if geminiGroup.Buckets[0].RemainingFraction != 0.81 {
		t.Errorf("Bucket remaining = %v, want 0.81", geminiGroup.Buckets[0].RemainingFraction)
	}
	if geminiGroup.Buckets[0].ResetTime != "2026-11-21T01:13:16Z" {
		t.Errorf("Bucket reset_time = %v", geminiGroup.Buckets[0].ResetTime)
	}
}

func TestGetUsageWithOverride(t *testing.T) {
	expected := &agents.UsageReport{
		Backend:     agents.BackendAgy,
		Description: "test quota",
		UpdatedAt:   time.Now().UTC(),
	}
	adapter := New(Options{
		GetUsageOverride: func(ctx context.Context) (*agents.UsageReport, error) {
			return expected, nil
		},
	})

	got, err := adapter.GetUsage(context.Background())
	if err != nil {
		t.Fatalf("GetUsage failed: %v", err)
	}
	if got.Description != expected.Description {
		t.Errorf("Description = %q, want %q", got.Description, expected.Description)
	}
}
