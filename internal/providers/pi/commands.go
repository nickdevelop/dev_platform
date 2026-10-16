package pi

import (
	"context"

	"devplatform/internal/core/agents"
)

var _ agents.CommandDiscoverer = (*Adapter)(nil)

// ListCommands returns the slash commands and skills supported by the pi backend.
func (a *Adapter) ListCommands(ctx context.Context, ref agents.SessionRef) ([]agents.AgentCommand, error) {
	commands := []agents.AgentCommand{
		{
			Name:        "compact",
			Description: "清理并压缩当前会话上下文，释放模型上下文窗口",
			Kind:        agents.CommandKindSlash,
		},
		{
			Name:        "export",
			Description: "将当前会话历史导出为 Markdown 或 JSON 文档",
			Kind:        agents.CommandKindSlash,
		},
		{
			Name:        "undo",
			Description: "撤销上一步的交互或代码操作",
			Kind:        agents.CommandKindSlash,
		},
		{
			Name:        "settings",
			Description: "查看或调整 Pi Agent 运行设置与参数",
			Kind:        agents.CommandKindSlash,
		},
		{
			Name:        "skill:interactive-testing",
			Description: "加载 Pi 交互式测试验证技能",
			Kind:        agents.CommandKindSkill,
		},
	}

	return commands, nil
}
