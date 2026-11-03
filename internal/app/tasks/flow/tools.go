package flow

import (
	"encoding/json"

	"devplatform/internal/app/chat"
)

// Orchestration tool names.
const (
	ToolDispatchRole = "flow_dispatch_role"
	ToolWaitSession  = "flow_wait_session"
	ToolAskUser      = "flow_ask_user"
	ToolCompleteTask = "flow_complete_task"
)

// DispatchRoleArgs defines arguments for flow_dispatch_role.
type DispatchRoleArgs struct {
	RoleName    string `json:"role_name"`
	Instruction string `json:"instruction"`
	SessionID   string `json:"session_id,omitempty"`
}

// WaitSessionArgs defines arguments for flow_wait_session.
type WaitSessionArgs struct {
	SessionID      string `json:"session_id"`
	TimeoutSeconds int    `json:"timeout_seconds,omitempty"`
}

// AskUserArgs defines arguments for flow_ask_user.
type AskUserArgs struct {
	Question string `json:"question"`
}

// CompleteTaskArgs defines arguments for flow_complete_task.
type CompleteTaskArgs struct {
	Summary string `json:"summary"`
}

// FlowTools returns the list of OpenAI-compatible tool definitions available to the flow orchestrator agent.
func FlowTools() []chat.ToolDefinition {
	return []chat.ToolDefinition{
		{
			Type: "function",
			Function: chat.FunctionSchema{
				Name: ToolDispatchRole,
				Description: "向团队中的专业角色（如架构师、开发工程师等）指派具体任务。支持新建会话或在已有会话中继续对话（多轮连续交流）。" +
					"调用后会自动启动并等待该角色执行，返回该角色的执行产出摘要与会话 ID。",
				Parameters: json.RawMessage(`{
					"type": "object",
					"properties": {
						"role_name": {
							"type": "string",
							"description": "被指派角色的确切名称，必须严格匹配可用角色列表中的角色名"
						},
						"instruction": {
							"type": "string",
							"description": "指派给该角色的具体任务目标、需求细节、设计规范或修改要求"
						},
						"session_id": {
							"type": "string",
							"description": "可选：此前该角色已创建的会话 ID。如果传入，将向已有会话追加发送指令继续对话；若未传入或为空，则自动为该角色开启一个新会话"
						}
					},
					"required": ["role_name", "instruction"]
				}`),
			},
		},
		{
			Type: "function",
			Function: chat.FunctionSchema{
				Name: ToolWaitSession,
				Description: "显式等待指定角色会话执行完毕并获取其产出（通常 flow_dispatch_role 已包含自动等待，但当需要再次同步检查或超时重试时可调用）。",
				Parameters: json.RawMessage(`{
					"type": "object",
					"properties": {
						"session_id": {
							"type": "string",
							"description": "需要等待的目标会话 ID"
						},
						"timeout_seconds": {
							"type": "integer",
							"description": "最长等待秒数，默认 300 秒"
						}
					},
					"required": ["session_id"]
				}`),
			},
		},
		{
			Type: "function",
			Function: chat.FunctionSchema{
				Name: ToolAskUser,
				Description: "当执行过程中存在重大疑问、缺少关键需求决策或需要用户人工确认授权时，向群聊中的用户提出问题并挂起流程等待用户输入回复。",
				Parameters: json.RawMessage(`{
					"type": "object",
					"properties": {
						"question": {
							"type": "string",
							"description": "向用户提出的澄清或确认问题内容"
						}
					},
					"required": ["question"]
				}`),
			},
		},
		{
			Type: "function",
			Function: chat.FunctionSchema{
				Name: ToolCompleteTask,
				Description: "当流程中的所有子任务均已完成，方案与代码均已检验合格，达成团队初始目标时调用此工具，提交最终总结报告并宣布任务达成。",
				Parameters: json.RawMessage(`{
					"type": "object",
					"properties": {
						"summary": {
							"type": "string",
							"description": "协同任务的最终完成报告与成果总结"
						}
					},
					"required": ["summary"]
				}`),
			},
		},
	}
}
