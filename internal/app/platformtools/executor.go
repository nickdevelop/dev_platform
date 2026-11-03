package platformtools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"devplatform/internal/app/chat"
	"devplatform/internal/app/projects"
	"devplatform/internal/app/prompts"
	"devplatform/internal/app/sessions"
)

// SessionLister abstracts querying sessions for platform tools.
type SessionLister interface {
	List(projectID uint) ([]sessions.Session, error)
}

// Executor implements chat.ToolExecutor to provide platform management capabilities.
type Executor struct {
	projects *projects.Service
	prompts  *prompts.Service
	sessions SessionLister
}

// NewExecutor creates a new platform tool executor.
func NewExecutor(projects *projects.Service, prompts *prompts.Service, sessionLister ...SessionLister) *Executor {
	var sl SessionLister
	if len(sessionLister) > 0 {
		sl = sessionLister[0]
	}
	return &Executor{
		projects: projects,
		prompts:  prompts,
		sessions: sl,
	}
}

var _ chat.ToolExecutor = (*Executor)(nil)

// Definitions returns the JSON Schema tool definitions exposed to LLMs.
func (e *Executor) Definitions() []chat.ToolDefinition {
	return []chat.ToolDefinition{
		{
			Type: "function",
			Function: chat.FunctionSchema{
				Name:        "platform_list_projects",
				Description: "查询当前平台中所有已登记注册的项目列表，支持按关键词检索项目名称与路径",
				Parameters: json.RawMessage(`{
					"type": "object",
					"properties": {
						"keyword": {
							"type": "string",
							"description": "可选的项目搜索关键词（名称或路径包含匹配）"
						}
					}
				}`),
			},
		},
		{
			Type: "function",
			Function: chat.FunctionSchema{
				Name:        "platform_list_prompts",
				Description: "查询当前平台中已保存的提示词模板列表，支持按关键词检索",
				Parameters: json.RawMessage(`{
					"type": "object",
					"properties": {
						"keyword": {
							"type": "string",
							"description": "可选的提示词搜索关键词（名称或描述匹配）"
						}
					}
				}`),
			},
		},
		{
			Type: "function",
			Function: chat.FunctionSchema{
				Name:        "platform_get_prompt",
				Description: "获取指定 ID 的提示词模板完整详情，包含提示词正文内容",
				Parameters: json.RawMessage(`{
					"type": "object",
					"properties": {
						"id": {
							"type": "integer",
							"description": "提示词的唯一数字 ID"
						}
					},
					"required": ["id"]
				}`),
			},
		},
		{
			Type: "function",
			Function: chat.FunctionSchema{
				Name:        "platform_create_prompt",
				Description: "在平台中新建一个提示词模板",
				Parameters: json.RawMessage(`{
					"type": "object",
					"properties": {
						"name": {
							"type": "string",
							"description": "提示词名称（必须唯一且非空）"
						},
						"description": {
							"type": "string",
							"description": "简要说明此提示词的用途"
						},
						"content": {
							"type": "string",
							"description": "提示词的完整正文内容（非空）"
						}
					},
					"required": ["name", "content"]
				}`),
			},
		},
		{
			Type: "function",
			Function: chat.FunctionSchema{
				Name:        "platform_update_prompt",
				Description: "修改平台中已有的提示词模板，支持更新名称、说明或正文内容",
				Parameters: json.RawMessage(`{
					"type": "object",
					"properties": {
						"id": {
							"type": "integer",
							"description": "要更新的提示词唯一数字 ID"
						},
						"name": {
							"type": "string",
							"description": "可选：新的提示词名称"
						},
						"description": {
							"type": "string",
							"description": "可选：新的说明"
						},
						"content": {
							"type": "string",
							"description": "可选：新的正文内容"
						}
					},
					"required": ["id"]
				}`),
			},
		},
		{
			Type: "function",
			Function: chat.FunctionSchema{
				Name:        "platform_delete_prompt",
				Description: "删除平台中指定的提示词模板",
				Parameters: json.RawMessage(`{
					"type": "object",
					"properties": {
						"id": {
							"type": "integer",
							"description": "要删除的提示词唯一数字 ID"
						}
					},
					"required": ["id"]
				}`),
			},
		},
		{
			Type: "function",
			Function: chat.FunctionSchema{
				Name:        "platform_list_sessions",
				Description: "查询指定项目中的所有 AI Agent 会话列表，支持查看会话 ID、标题、模型、运行状态等信息",
				Parameters: json.RawMessage(`{
					"type": "object",
					"properties": {
						"project_id": {
							"type": "integer",
							"description": "可选：项目唯一数字 ID。如果未提供，将自动使用系统中的首个有效项目"
						}
					}
				}`),
			},
		},
	}
}

// Execute routes and executes the requested tool.
func (e *Executor) Execute(ctx context.Context, name string, argumentsJSON string) (string, error) {
	switch name {
	case "platform_list_projects":
		return e.listProjects(argumentsJSON)
	case "platform_list_prompts":
		return e.listPrompts(argumentsJSON)
	case "platform_list_sessions":
		return e.listSessions(argumentsJSON)
	case "platform_get_prompt":
		return e.getPrompt(argumentsJSON)
	case "platform_create_prompt":
		return e.createPrompt(argumentsJSON)
	case "platform_update_prompt":
		return e.updatePrompt(argumentsJSON)
	case "platform_delete_prompt":
		return e.deletePrompt(argumentsJSON)
	default:
		return "", fmt.Errorf("unknown platform tool: %s", name)
	}
}

type listProjectsArgs struct {
	Keyword string `json:"keyword"`
}

type projectSummary struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"`
}

func (e *Executor) listProjects(argsJSON string) (string, error) {
	var args listProjectsArgs
	if strings.TrimSpace(argsJSON) != "" {
		_ = json.Unmarshal([]byte(argsJSON), &args)
	}
	allProjects, err := e.projects.List()
	if err != nil {
		return "", fmt.Errorf("list projects: %w", err)
	}
	kw := strings.ToLower(strings.TrimSpace(args.Keyword))
	out := make([]projectSummary, 0, len(allProjects))
	for _, p := range allProjects {
		if kw != "" {
			nameMatch := strings.Contains(strings.ToLower(p.Name), kw)
			pathMatch := strings.Contains(strings.ToLower(p.Path), kw)
			if !nameMatch && !pathMatch {
				continue
			}
		}
		out = append(out, projectSummary{
			ID:   p.ID,
			Name: p.Name,
			Path: p.Path,
		})
	}
	bytes, err := json.Marshal(out)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

type listPromptsArgs struct {
	Keyword string `json:"keyword"`
}

func (e *Executor) listPrompts(argsJSON string) (string, error) {
	var args listPromptsArgs
	if strings.TrimSpace(argsJSON) != "" {
		_ = json.Unmarshal([]byte(argsJSON), &args)
	}
	allPrompts, err := e.prompts.List(strings.TrimSpace(args.Keyword), "")
	if err != nil {
		return "", fmt.Errorf("list prompts: %w", err)
	}
	type promptSummary struct {
		ID          uint   `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description"`
		UpdatedAt   string `json:"updatedAt"`
	}
	out := make([]promptSummary, 0, len(allPrompts))
	for _, p := range allPrompts {
		out = append(out, promptSummary{
			ID:          p.ID,
			Name:        p.Name,
			Description: p.Description,
			UpdatedAt:   p.UpdatedAt.Format("2006-01-02 15:04:05"),
		})
	}
	bytes, err := json.Marshal(out)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

type idArgs struct {
	ID uint `json:"id"`
}

func (e *Executor) getPrompt(argsJSON string) (string, error) {
	var args idArgs
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("parse arguments: %w", err)
	}
	if args.ID == 0 {
		return "", fmt.Errorf("invalid prompt id: must be > 0")
	}
	p, err := e.prompts.Get(args.ID)
	if err != nil {
		return "", fmt.Errorf("get prompt %d: %w", args.ID, err)
	}
	bytes, err := json.Marshal(p)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

type createPromptArgs struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Content     string `json:"content"`
}

func (e *Executor) createPrompt(argsJSON string) (string, error) {
	var args createPromptArgs
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("parse arguments: %w", err)
	}
	created, err := e.prompts.Create(prompts.CreatePromptRequest{
		Name:        args.Name,
		Description: args.Description,
		Content:     args.Content,
	})
	if err != nil {
		return "", fmt.Errorf("create prompt: %w", err)
	}
	bytes, err := json.Marshal(created)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

type updatePromptArgs struct {
	ID          uint    `json:"id"`
	Name        *string `json:"name"`
	Description *string `json:"description"`
	Content     *string `json:"content"`
}

func (e *Executor) updatePrompt(argsJSON string) (string, error) {
	var args updatePromptArgs
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("parse arguments: %w", err)
	}
	if args.ID == 0 {
		return "", fmt.Errorf("invalid prompt id: must be > 0")
	}
	updated, err := e.prompts.Update(args.ID, prompts.UpdatePromptRequest{
		Name:        args.Name,
		Description: args.Description,
		Content:     args.Content,
	})
	if err != nil {
		return "", fmt.Errorf("update prompt %d: %w", args.ID, err)
	}
	bytes, err := json.Marshal(updated)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func (e *Executor) deletePrompt(argsJSON string) (string, error) {
	var args idArgs
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("parse arguments: %w", err)
	}
	if args.ID == 0 {
		return "", fmt.Errorf("invalid prompt id: must be > 0")
	}
	if err := e.prompts.Delete(args.ID); err != nil {
		return "", fmt.Errorf("delete prompt %d: %w", args.ID, err)
	}
	return fmt.Sprintf(`{"deleted":true,"id":%d}`, args.ID), nil
}

type listSessionsArgs struct {
	ProjectID uint `json:"project_id"`
}

type sessionOverview struct {
	ID        string `json:"id"`
	ProjectID uint   `json:"projectId"`
	Backend   string `json:"backend"`
	Title     string `json:"title,omitempty"`
	Model     string `json:"model,omitempty"`
	Status    string `json:"status"`
	Turn      string `json:"turn"`
}

func (e *Executor) listSessions(argsJSON string) (string, error) {
	if e.sessions == nil {
		return `{"sessions":[],"message":"会话服务未挂载"}`, nil
	}
	var args listSessionsArgs
	if strings.TrimSpace(argsJSON) != "" {
		_ = json.Unmarshal([]byte(argsJSON), &args)
	}
	projectID := args.ProjectID
	if projectID == 0 {
		allProjects, err := e.projects.List()
		if err != nil || len(allProjects) == 0 {
			return `{"sessions":[],"message":"系统中无有效项目"}`, nil
		}
		projectID = allProjects[0].ID
	}
	list, err := e.sessions.List(projectID)
	if err != nil {
		return "", fmt.Errorf("list sessions for project %d: %w", projectID, err)
	}
	out := make([]sessionOverview, 0, len(list))
	for _, s := range list {
		out = append(out, sessionOverview{
			ID:        s.ID,
			ProjectID: s.ProjectID,
			Backend:   string(s.Backend),
			Title:     s.Title,
			Model:     s.Model,
			Status:    string(s.Status),
			Turn:      string(s.Turn),
		})
	}
	bytes, err := json.Marshal(out)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

