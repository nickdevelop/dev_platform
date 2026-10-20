package deepseek

import (
	"context"

	"devplatform/internal/core/agents"
)

// ListCommands implements agents.CommandDiscoverer by querying dsh's native commands and skills catalogs.
func (a *Adapter) ListCommands(ctx context.Context, ref agents.SessionRef) ([]agents.AgentCommand, error) {
	proc, release, err := a.host.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer release()

	client := proc.Client()

	var out []agents.AgentCommand

	// 1. Fetch commands via commands/list
	var rawCommands []struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := client.Call(ctx, "commands/list", map[string]any{"agentId": ref.SessionID}, &rawCommands); err == nil {
		for _, cmd := range rawCommands {
			out = append(out, agents.AgentCommand{
				Name:        cmd.Name,
				Description: cmd.Description,
				Kind:        agents.CommandKindSlash,
			})
		}
	}

	// 2. Fetch skills via skills/list
	var rawSkills struct {
		Skills []struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		} `json:"skills"`
	}
	if err := client.Call(ctx, "skills/list", map[string]any{
		"request": map[string]any{"sessionId": ref.SessionID},
	}, &rawSkills); err == nil {
		for _, s := range rawSkills.Skills {
			out = append(out, agents.AgentCommand{
				Name:        s.Name,
				Description: s.Description,
				Kind:        agents.CommandKindSkill,
			})
		}
	}

	return out, nil
}
