package agy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"devplatform/internal/core/agents"
)

// usageOutput matches the JSON output produced by `agy -p "/usage" --output-format json`.
type usageOutput struct {
	Status   string `json:"status"`
	Response string `json:"response"`
	Command  *struct {
		Name string `json:"name"`
		Data *struct {
			Description string `json:"description"`
			Groups      []struct {
				Name        string `json:"name"`
				Description string `json:"description"`
				Buckets     []struct {
					ID                string  `json:"id"`
					Name              string  `json:"name"`
					Description       string  `json:"description"`
					Window            string  `json:"window"`
					RemainingFraction float64 `json:"remaining_fraction"`
					ResetTime         string  `json:"reset_time"`
				} `json:"buckets"`
			} `json:"groups"`
		} `json:"data"`
	} `json:"command"`
}

// GetUsage executes `agy -p "/usage" --output-format json` and returns the normalized UsageReport.
func (a *Adapter) GetUsage(ctx context.Context) (*agents.UsageReport, error) {
	if a.getUsageOverride != nil {
		return a.getUsageOverride(ctx)
	}

	a.usageMu.Lock()
	defer a.usageMu.Unlock()

	timeoutCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	execPath := a.execPath()
	cmd := exec.CommandContext(timeoutCtx, execPath, "-p", "/usage", "--output-format", "json")
	if len(a.env) > 0 {
		cmd.Env = a.env
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		errOutput := strings.TrimSpace(stderr.String())
		if errOutput == "" {
			errOutput = strings.TrimSpace(stdout.String())
		}
		if errOutput != "" {
			return nil, fmt.Errorf("agy: usage command failed: %w (output: %s)", err, errOutput)
		}
		return nil, fmt.Errorf("agy: usage command failed: %w", err)
	}

	report, err := ParseUsageOutput(stdout.Bytes())
	if err != nil {
		// Fallback: If not parseable as structured JSON, treat stdout as raw text
		raw := strings.TrimSpace(stdout.String())
		return &agents.UsageReport{
			Backend:   agents.BackendAgy,
			RawOutput: raw,
			UpdatedAt: time.Now().UTC(),
		}, nil
	}

	return report, nil
}

// ParseUsageOutput decodes the JSON output of agy into a normalized UsageReport.
func ParseUsageOutput(data []byte) (*agents.UsageReport, error) {
	var out usageOutput
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}

	report := &agents.UsageReport{
		Backend:   agents.BackendAgy,
		RawOutput: strings.TrimSpace(out.Response),
		UpdatedAt: time.Now().UTC(),
	}

	if out.Command != nil && out.Command.Data != nil {
		report.Description = out.Command.Data.Description
		for _, g := range out.Command.Data.Groups {
			group := agents.UsageGroup{
				Name:        g.Name,
				Description: g.Description,
				Buckets:     make([]agents.UsageBucket, 0, len(g.Buckets)),
			}
			for _, b := range g.Buckets {
				group.Buckets = append(group.Buckets, agents.UsageBucket{
					ID:                b.ID,
					Name:              b.Name,
					Description:       b.Description,
					Window:            b.Window,
					RemainingFraction: b.RemainingFraction,
					ResetTime:         b.ResetTime,
				})
			}
			report.Groups = append(report.Groups, group)
		}
	}

	return report, nil
}
