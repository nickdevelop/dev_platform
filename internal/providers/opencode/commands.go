package opencode

import (
	"context"

	"devplatform/internal/core/agents"
)

var _ agents.CommandDiscoverer = (*Adapter)(nil)

// ListCommands returns the built-in slash commands supported across the platform.
func (a *Adapter) ListCommands(ctx context.Context, ref agents.SessionRef) ([]agents.AgentCommand, error) {
	commands := []agents.AgentCommand{
		{
			Name:        "new",
			Description: "新建并切换到全新会话",
			Kind:        agents.CommandKindSlash,
		},
		{
			Name:        "stop",
			Description: "停止当前正在运行的 Agent 任务",
			Kind:        agents.CommandKindSlash,
		},
		{
			Name:         "model",
			Description:  "切换当前会话所使用的模型",
			Kind:         agents.CommandKindSlash,
			ArgumentHint: "[model-name]",
		},
		{
			Name:        "help",
			Description: "查看可用命令与快捷操作提示",
			Kind:        agents.CommandKindSlash,
		},
	}

	return commands, nil
}
