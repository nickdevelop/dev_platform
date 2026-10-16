package opencode

import (
	"context"

	"devplatform/internal/core/agents"
)

var _ agents.CommandDiscoverer = (*Adapter)(nil)

// ListCommands returns the slash commands and skills supported by the opencode backend.
func (a *Adapter) ListCommands(ctx context.Context, ref agents.SessionRef) ([]agents.AgentCommand, error) {
	commands := []agents.AgentCommand{
		{
			Name:         "review",
			Description:  "审查当前工作区的代码变更或指定文件",
			Kind:         agents.CommandKindSlash,
			ArgumentHint: "[path]",
		},
		{
			Name:         "commit",
			Description:  "根据当前变更自动生成规范的 Git 提交信息并提交",
			Kind:         agents.CommandKindSlash,
			ArgumentHint: "[message]",
		},
		{
			Name:         "test",
			Description:  "执行项目测试套件并分析修复失败项",
			Kind:         agents.CommandKindSlash,
		},
		{
			Name:         "opencode",
			Description:  "加载 OpenCode 专家系统技能与知识库",
			Kind:         agents.CommandKindSkill,
		},
	}

	return commands, nil
}
