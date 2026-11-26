package platformtools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"devplatform/internal/app/agentmodels"
	"devplatform/internal/app/chat"
	"devplatform/internal/app/groups"
	"devplatform/internal/app/projects"
	"devplatform/internal/app/sessions"
	"devplatform/internal/core/agents"
)

// WorktreeLister abstracts listing git worktrees for a project.
type WorktreeLister interface {
	ListWorktrees(ctx context.Context, projectID uint) ([]projects.Worktree, error)
}

// SessionController abstracts creating, inspecting, continuing and interrupting
// agent sessions. *sessions.Service implements it.
type SessionController interface {
	SessionLister
	Backends() []sessions.BackendCapabilities
	CreateInWorktree(ctx context.Context, projectID uint, worktreeID uint, backend agents.BackendID, title, message, model string, images ...[]agents.ImageAttachment) (sessions.CreateOutcome, error)
	Get(projectID uint, sessionID string) (sessions.Session, error)
	History(ctx context.Context, projectID uint, sessionID string) (sessions.History, error)
	Send(ctx context.Context, projectID uint, sessionID, text, model string, images ...[]agents.ImageAttachment) (sessions.Session, error)
	Interrupt(ctx context.Context, projectID uint, sessionID string) (sessions.Session, error)
	ListPermissionRequests(ctx context.Context, projectID uint, sessionID string) ([]sessions.PermissionRequest, error)
	ListForms(ctx context.Context, projectID uint, sessionID string) ([]sessions.FormInfo, error)
}

// ModelCatalogLister abstracts listing persisted enabled models for a backend.
type ModelCatalogLister interface {
	List(backend agents.BackendID) (agentmodels.ModelList, error)
}

// TeamController abstracts listing teams and creating collaborative sessions.
type TeamController interface {
	ListGroups(ctx context.Context) ([]groups.Group, error)
	CreateCollabSession(ctx context.Context, groupID uint, req groups.CreateCollabSessionRequest) (groups.CollabSessionResponse, error)
}

// SessionWatchEvent carries metadata when a tool launches or continues a session.
type SessionWatchEvent struct {
	ProjectID uint
	SessionID string
	Action    string // "created" | "continued"
}

type sessionWatchContextKey struct{}

// WithSessionWatchCallback attaches a callback invoked when a platform tool
// creates or continues a session, enabling Bot channels to register completion watches.
func WithSessionWatchCallback(ctx context.Context, fn func(SessionWatchEvent)) context.Context {
	if fn == nil {
		return ctx
	}
	return context.WithValue(ctx, sessionWatchContextKey{}, fn)
}

func notifySessionWatch(ctx context.Context, ev SessionWatchEvent) {
	if ctx == nil {
		return
	}
	if fn, ok := ctx.Value(sessionWatchContextKey{}).(func(SessionWatchEvent)); ok && fn != nil {
		fn(ev)
	}
}

// ToolMeta describes a configurable platform tool for UI governance.
type ToolMeta struct {
	Name        string `json:"name"`
	Label       string `json:"label"`
	Category    string `json:"category"` // "projects" | "sessions" | "teams" | "prompts"
	Description string `json:"description"`
	Dangerous   bool   `json:"dangerous,omitempty"`
}

// ExecutorOption configures optional collaborators on Executor.
type ExecutorOption func(*Executor)

// WithWorktreeLister attaches a worktree lister.
func WithWorktreeLister(w WorktreeLister) ExecutorOption {
	return func(e *Executor) { e.worktrees = w }
}

// WithSessionController attaches full session orchestration capabilities.
func WithSessionController(sc SessionController) ExecutorOption {
	return func(e *Executor) {
		e.sessionCtrl = sc
		if sc != nil && e.sessions == nil {
			e.sessions = sc
		}
	}
}

// WithModelCatalogLister attaches backend model catalog reading.
func WithModelCatalogLister(mc ModelCatalogLister) ExecutorOption {
	return func(e *Executor) { e.agentModels = mc }
}

// WithTeamController attaches collaborative team operations.
func WithTeamController(tc TeamController) ExecutorOption {
	return func(e *Executor) { e.teams = tc }
}

// ToolCatalog returns metadata for every platform tool (Steward + Bot).
func (e *Executor) ToolCatalog() []ToolMeta {
	return []ToolMeta{
		{Name: "platform_list_projects", Label: "查询项目列表", Category: "projects", Description: "列出平台已登记的项目及路径"},
		{Name: "platform_list_worktrees", Label: "查询项目工作树", Category: "projects", Description: "列出指定项目的 Git Worktree 分支与目录"},
		{Name: "platform_list_agent_backends", Label: "查询 Agent 后端与模型", Category: "sessions", Description: "列出平台已启用的编码 Agent 后端与可用模型"},
		{Name: "platform_create_session", Label: "创建 Agent 会话", Category: "sessions", Description: "在指定项目/工作树创建会话并派发执行任务", Dangerous: true},
		{Name: "platform_list_sessions", Label: "查询会话列表", Category: "sessions", Description: "查看指定项目中的 Agent 会话与实时状态"},
		{Name: "platform_get_session_status", Label: "查询会话详情与结果", Category: "sessions", Description: "查看会话当前运行状态、待确认项与最新回复摘要"},
		{Name: "platform_send_session_message", Label: "向会话发送指令", Category: "sessions", Description: "向已有 Agent 会话追加消息继续执行", Dangerous: true},
		{Name: "platform_interrupt_session", Label: "中断运行中会话", Category: "sessions", Description: "停止正在执行轮次的 Agent 会话", Dangerous: true},
		{Name: "platform_list_teams", Label: "查询 AI 团队列表", Category: "teams", Description: "查看平台已配置的协同团队与中控模型"},
		{Name: "platform_create_collab_session", Label: "发起团队协同会话", Category: "teams", Description: "基于指定团队在项目中启动多 Agent 协同任务", Dangerous: true},
		{Name: "platform_list_prompts", Label: "查询提示词列表", Category: "prompts", Description: "检索平台提示词库"},
		{Name: "platform_get_prompt", Label: "读取提示词详情", Category: "prompts", Description: "读取指定提示词正文"},
		{Name: "platform_create_prompt", Label: "创建提示词", Category: "prompts", Description: "新建提示词模板", Dangerous: true},
		{Name: "platform_update_prompt", Label: "更新提示词", Category: "prompts", Description: "修改提示词模板", Dangerous: true},
		{Name: "platform_delete_prompt", Label: "删除提示词", Category: "prompts", Description: "删除提示词模板", Dangerous: true},
	}
}

func (e *Executor) extendedDefinitions() []chat.ToolDefinition {
	return []chat.ToolDefinition{
		{
			Type: "function",
			Function: chat.FunctionSchema{
				Name:        "platform_list_worktrees",
				Description: "查询指定项目的 Git 工作树（Worktree）列表，包含工作树 ID、分支名称与目录路径",
				Parameters: json.RawMessage(`{
					"type": "object",
					"properties": {
						"project_id": {
							"type": "integer",
							"description": "项目唯一数字 ID"
						}
					},
					"required": ["project_id"]
				}`),
			},
		},
		{
			Type: "function",
			Function: chat.FunctionSchema{
				Name:        "platform_list_agent_backends",
				Description: "查询平台所有已启用的编码 Agent 后端列表及其已启用模型",
				Parameters: json.RawMessage(`{
					"type": "object",
					"properties": {}
				}`),
			},
		},
		{
			Type: "function",
			Function: chat.FunctionSchema{
				Name:        "platform_create_session",
				Description: "在指定项目（及可选 Git 工作树）中创建一个新的 AI Agent 编码会话并立即发送首条任务指令。创建后立即返回会话 ID，Agent 在后台异步执行。",
				Parameters: json.RawMessage(`{
					"type": "object",
					"properties": {
						"project_id": {
							"type": "integer",
							"description": "目标项目数字 ID（可先通过 platform_list_projects 查询）"
						},
						"worktree_id": {
							"type": "integer",
							"description": "可选：目标 Git 工作树 ID，0 或省略表示主工作区"
						},
						"backend": {
							"type": "string",
							"description": "Agent 后端 ID（可先通过 platform_list_agent_backends 查询已启用的后端）"
						},
						"model": {
							"type": "string",
							"description": "可选：指定模型 ID；省略则使用后端默认模型"
						},
						"title": {
							"type": "string",
							"description": "可选：会话标题；省略则自动由任务指令推导"
						},
						"message": {
							"type": "string",
							"description": "要派发给该 Agent 会话的首条任务指令"
						}
					},
					"required": ["project_id", "backend", "message"]
				}`),
			},
		},
		{
			Type: "function",
			Function: chat.FunctionSchema{
				Name:        "platform_get_session_status",
				Description: "查询指定 Agent 会话的当前运行状态（active/stopped/disconnected）、轮次状态（idle/running）、待处理的审批或提问数量，以及最新一条助手执行结果摘要",
				Parameters: json.RawMessage(`{
					"type": "object",
					"properties": {
						"project_id": {
							"type": "integer",
							"description": "项目数字 ID"
						},
						"session_id": {
							"type": "string",
							"description": "会话 ID"
						}
					},
					"required": ["project_id", "session_id"]
				}`),
			},
		},
		{
			Type: "function",
			Function: chat.FunctionSchema{
				Name:        "platform_send_session_message",
				Description: "向一个已有的 Agent 会话发送后续指令或追问消息，驱动其继续执行",
				Parameters: json.RawMessage(`{
					"type": "object",
					"properties": {
						"project_id": {
							"type": "integer",
							"description": "项目数字 ID"
						},
						"session_id": {
							"type": "string",
							"description": "目标会话 ID"
						},
						"message": {
							"type": "string",
							"description": "发送给会话的新指令内容"
						},
						"model": {
							"type": "string",
							"description": "可选：切换使用的模型 ID"
						}
					},
					"required": ["project_id", "session_id", "message"]
				}`),
			},
		},
		{
			Type: "function",
			Function: chat.FunctionSchema{
				Name:        "platform_interrupt_session",
				Description: "中断指定 Agent 会话当前正在进行的执行轮次",
				Parameters: json.RawMessage(`{
					"type": "object",
					"properties": {
						"project_id": {
							"type": "integer",
							"description": "项目数字 ID"
						},
						"session_id": {
							"type": "string",
							"description": "需要中断的会话 ID"
						}
					},
					"required": ["project_id", "session_id"]
				}`),
			},
		},
		{
			Type: "function",
			Function: chat.FunctionSchema{
				Name:        "platform_list_teams",
				Description: "查询平台中已配置的 AI 团队列表（含团队 ID、名称、职责描述与中控模型）",
				Parameters: json.RawMessage(`{
					"type": "object",
					"properties": {}
				}`),
			},
		},
		{
			Type: "function",
			Function: chat.FunctionSchema{
				Name:        "platform_create_collab_session",
				Description: "在指定项目中基于某个 AI 团队发起多 Agent 协同会话并自动触发中控调度执行",
				Parameters: json.RawMessage(`{
					"type": "object",
					"properties": {
						"group_id": {
							"type": "integer",
							"description": "AI 团队 ID"
						},
						"project_id": {
							"type": "integer",
							"description": "目标项目 ID"
						},
						"worktree_id": {
							"type": "integer",
							"description": "可选：目标 Git 工作树 ID"
						},
						"title": {
							"type": "string",
							"description": "可选：协同任务标题"
						},
						"prompt": {
							"type": "string",
							"description": "协同任务的总体目标描述"
						}
					},
					"required": ["group_id", "project_id", "prompt"]
				}`),
			},
		},
	}
}

// AllDefinitions returns both the core Steward tools and the extended Bot tools.
func (e *Executor) AllDefinitions() []chat.ToolDefinition {
	base := e.Definitions()
	ext := e.extendedDefinitions()
	out := make([]chat.ToolDefinition, 0, len(base)+len(ext))
	out = append(out, base...)
	out = append(out, ext...)
	return out
}

// FilteredExecutor returns a chat.ToolExecutor restricted strictly to the
// provided tool names. An empty slice yields an executor with zero tools.
func (e *Executor) FilteredExecutor(allowed []string) chat.ToolExecutor {
	allowSet := make(map[string]bool, len(allowed))
	for _, name := range allowed {
		if trimmed := strings.TrimSpace(name); trimmed != "" {
			allowSet[trimmed] = true
		}
	}
	return &filteredExecutor{
		inner:    e,
		allowSet: allowSet,
	}
}

type filteredExecutor struct {
	inner    *Executor
	allowSet map[string]bool
}

func (f *filteredExecutor) Definitions() []chat.ToolDefinition {
	if len(f.allowSet) == 0 {
		return nil
	}
	all := f.inner.AllDefinitions()
	out := make([]chat.ToolDefinition, 0, len(f.allowSet))
	for _, def := range all {
		if f.allowSet[def.Function.Name] {
			out = append(out, def)
		}
	}
	return out
}

func (f *filteredExecutor) Execute(ctx context.Context, name string, argumentsJSON string) (string, error) {
	if !f.allowSet[name] {
		return "", fmt.Errorf("tool %q is not enabled by bot engine configuration", name)
	}
	return f.inner.Execute(ctx, name, argumentsJSON)
}

func (e *Executor) executeExtended(ctx context.Context, name, argsJSON string) (string, bool, error) {
	switch name {
	case "platform_list_worktrees":
		res, err := e.listWorktrees(ctx, argsJSON)
		return res, true, err
	case "platform_list_agent_backends":
		res, err := e.listAgentBackends()
		return res, true, err
	case "platform_create_session":
		res, err := e.createSession(ctx, argsJSON)
		return res, true, err
	case "platform_get_session_status":
		res, err := e.getSessionStatus(ctx, argsJSON)
		return res, true, err
	case "platform_send_session_message":
		res, err := e.sendSessionMessage(ctx, argsJSON)
		return res, true, err
	case "platform_interrupt_session":
		res, err := e.interruptSession(ctx, argsJSON)
		return res, true, err
	case "platform_list_teams":
		res, err := e.listTeams(ctx)
		return res, true, err
	case "platform_create_collab_session":
		res, err := e.createCollabSession(ctx, argsJSON)
		return res, true, err
	default:
		return "", false, nil
	}
}

func (e *Executor) listWorktrees(ctx context.Context, argsJSON string) (string, error) {
	if e.worktrees == nil {
		return `{"worktrees":[],"message":"工作树服务未挂载"}`, nil
	}
	var args struct {
		ProjectID uint `json:"project_id"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("parse arguments: %w", err)
	}
	if args.ProjectID == 0 {
		return "", fmt.Errorf("project_id is required")
	}
	list, err := e.worktrees.ListWorktrees(ctx, args.ProjectID)
	if err != nil {
		return "", fmt.Errorf("list worktrees: %w", err)
	}
	bytes, err := json.Marshal(list)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func (e *Executor) listAgentBackends() (string, error) {
	if e.sessionCtrl == nil {
		return `{"backends":[],"message":"会话服务未挂载"}`, nil
	}
	caps := e.sessionCtrl.Backends()
	type backendSummary struct {
		Backend string   `json:"backend"`
		Label   string   `json:"label"`
		Enabled bool     `json:"enabled"`
		Models  []string `json:"models,omitempty"`
	}
	out := make([]backendSummary, 0, len(caps))
	for _, b := range caps {
		if !b.Enabled {
			continue
		}
		item := backendSummary{
			Backend: string(b.Backend),
			Label:   b.Label,
			Enabled: b.Enabled,
		}
		if e.agentModels != nil {
			if list, err := e.agentModels.List(b.Backend); err == nil {
				for _, m := range list.Models {
					if m.Enabled {
						item.Models = append(item.Models, m.ID)
					}
				}
			}
		}
		out = append(out, item)
	}
	bytes, err := json.Marshal(out)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func (e *Executor) createSession(ctx context.Context, argsJSON string) (string, error) {
	if e.sessionCtrl == nil {
		return "", fmt.Errorf("session controller not mounted")
	}
	var args struct {
		ProjectID  uint   `json:"project_id"`
		WorktreeID uint   `json:"worktree_id"`
		Backend    string `json:"backend"`
		Model      string `json:"model"`
		Title      string `json:"title"`
		Message    string `json:"message"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("parse arguments: %w", err)
	}
	if args.ProjectID == 0 || strings.TrimSpace(args.Backend) == "" || strings.TrimSpace(args.Message) == "" {
		return "", fmt.Errorf("project_id, backend and message are required")
	}
	outcome, err := e.sessionCtrl.CreateInWorktree(
		ctx,
		args.ProjectID,
		args.WorktreeID,
		agents.BackendID(strings.TrimSpace(args.Backend)),
		strings.TrimSpace(args.Title),
		strings.TrimSpace(args.Message),
		strings.TrimSpace(args.Model),
	)
	if err != nil {
		return "", fmt.Errorf("create session: %w", err)
	}
	notifySessionWatch(ctx, SessionWatchEvent{
		ProjectID: outcome.Session.ProjectID,
		SessionID: outcome.Session.ID,
		Action:    "created",
	})
	resp := map[string]any{
		"sessionId":    outcome.Session.ID,
		"projectId":    outcome.Session.ProjectID,
		"worktreeId":   outcome.Session.WorktreeID,
		"backend":      outcome.Session.Backend,
		"model":        outcome.Session.Model,
		"title":        outcome.Session.Title,
		"status":       outcome.Session.Status,
		"turn":         outcome.Session.Turn,
		"messageError": outcome.MessageError,
	}
	bytes, err := json.Marshal(resp)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func (e *Executor) getSessionStatus(ctx context.Context, argsJSON string) (string, error) {
	if e.sessionCtrl == nil {
		return "", fmt.Errorf("session controller not mounted")
	}
	var args struct {
		ProjectID uint   `json:"project_id"`
		SessionID string `json:"session_id"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("parse arguments: %w", err)
	}
	if args.ProjectID == 0 || strings.TrimSpace(args.SessionID) == "" {
		return "", fmt.Errorf("project_id and session_id are required")
	}
	sess, err := e.sessionCtrl.Get(args.ProjectID, strings.TrimSpace(args.SessionID))
	if err != nil {
		return "", fmt.Errorf("get session: %w", err)
	}
	summary := sessions.ExtractLatestSummary(ctx, e.sessionCtrl, args.ProjectID, sess.ID)
	if len([]rune(summary)) > 1500 {
		runes := []rune(summary)
		summary = string(runes[:1500]) + "\n...(内容过长已截断)"
	}
	pendingPermissions := 0
	if perms, err := e.sessionCtrl.ListPermissionRequests(ctx, args.ProjectID, sess.ID); err == nil {
		pendingPermissions = len(perms)
	}
	pendingForms := 0
	if forms, err := e.sessionCtrl.ListForms(ctx, args.ProjectID, sess.ID); err == nil {
		pendingForms = len(forms)
	}
	resp := map[string]any{
		"sessionId":          sess.ID,
		"projectId":          sess.ProjectID,
		"worktreeId":         sess.WorktreeID,
		"backend":            sess.Backend,
		"model":              sess.Model,
		"title":              sess.Title,
		"status":             sess.Status,
		"turn":               sess.Turn,
		"pendingPermissions": pendingPermissions,
		"pendingForms":       pendingForms,
		"latestSummary":      summary,
		"updatedAt":          sess.UpdatedAt.Format("2006-01-02 15:04:05"),
	}
	bytes, err := json.Marshal(resp)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func (e *Executor) sendSessionMessage(ctx context.Context, argsJSON string) (string, error) {
	if e.sessionCtrl == nil {
		return "", fmt.Errorf("session controller not mounted")
	}
	var args struct {
		ProjectID uint   `json:"project_id"`
		SessionID string `json:"session_id"`
		Message   string `json:"message"`
		Model     string `json:"model"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("parse arguments: %w", err)
	}
	if args.ProjectID == 0 || strings.TrimSpace(args.SessionID) == "" || strings.TrimSpace(args.Message) == "" {
		return "", fmt.Errorf("project_id, session_id and message are required")
	}
	sess, err := e.sessionCtrl.Send(ctx, args.ProjectID, strings.TrimSpace(args.SessionID), strings.TrimSpace(args.Message), strings.TrimSpace(args.Model))
	if err != nil {
		return "", fmt.Errorf("send session message: %w", err)
	}
	notifySessionWatch(ctx, SessionWatchEvent{
		ProjectID: sess.ProjectID,
		SessionID: sess.ID,
		Action:    "continued",
	})
	resp := map[string]any{
		"sessionId": sess.ID,
		"projectId": sess.ProjectID,
		"status":    sess.Status,
		"turn":      sess.Turn,
		"sent":      true,
	}
	bytes, err := json.Marshal(resp)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func (e *Executor) interruptSession(ctx context.Context, argsJSON string) (string, error) {
	if e.sessionCtrl == nil {
		return "", fmt.Errorf("session controller not mounted")
	}
	var args struct {
		ProjectID uint   `json:"project_id"`
		SessionID string `json:"session_id"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("parse arguments: %w", err)
	}
	if args.ProjectID == 0 || strings.TrimSpace(args.SessionID) == "" {
		return "", fmt.Errorf("project_id and session_id are required")
	}
	sess, err := e.sessionCtrl.Interrupt(ctx, args.ProjectID, strings.TrimSpace(args.SessionID))
	if err != nil {
		return "", fmt.Errorf("interrupt session: %w", err)
	}
	resp := map[string]any{
		"sessionId":   sess.ID,
		"status":      sess.Status,
		"turn":        sess.Turn,
		"interrupted": true,
	}
	bytes, err := json.Marshal(resp)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func (e *Executor) listTeams(ctx context.Context) (string, error) {
	if e.teams == nil {
		return `{"teams":[],"message":"团队服务未挂载"}`, nil
	}
	list, err := e.teams.ListGroups(ctx)
	if err != nil {
		return "", fmt.Errorf("list teams: %w", err)
	}
	type teamSummary struct {
		ID                uint   `json:"id"`
		Name              string `json:"name"`
		Description       string `json:"description"`
		OrchestratorModel string `json:"orchestratorModel"`
	}
	out := make([]teamSummary, 0, len(list))
	for _, g := range list {
		out = append(out, teamSummary{
			ID:                g.ID,
			Name:              g.Name,
			Description:       g.Description,
			OrchestratorModel: g.OrchestratorModel,
		})
	}
	bytes, err := json.Marshal(out)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func (e *Executor) createCollabSession(ctx context.Context, argsJSON string) (string, error) {
	if e.teams == nil {
		return "", fmt.Errorf("team controller not mounted")
	}
	var args struct {
		GroupID    uint   `json:"group_id"`
		ProjectID  uint   `json:"project_id"`
		WorktreeID uint   `json:"worktree_id"`
		Title      string `json:"title"`
		Prompt     string `json:"prompt"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("parse arguments: %w", err)
	}
	if args.GroupID == 0 || args.ProjectID == 0 || strings.TrimSpace(args.Prompt) == "" {
		return "", fmt.Errorf("group_id, project_id and prompt are required")
	}
	res, err := e.teams.CreateCollabSession(ctx, args.GroupID, groups.CreateCollabSessionRequest{
		ProjectID:  args.ProjectID,
		WorktreeID: args.WorktreeID,
		Title:      strings.TrimSpace(args.Title),
		Prompt:     strings.TrimSpace(args.Prompt),
	})
	if err != nil {
		return "", fmt.Errorf("create collab session: %w", err)
	}
	resp := map[string]any{
		"sessionId": res.Session.ID,
		"taskId":    res.Task.ID,
		"projectId": res.Session.ProjectID,
		"title":     res.Session.Title,
		"status":    res.Session.Status,
	}
	bytes, err := json.Marshal(resp)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}
