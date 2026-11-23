package settings

import (
	"encoding/json"
	"time"
)

const enginesConfigKey = "platform:engines:config"

// StewardConfig models configuration for the platform steward assistant.
type StewardConfig struct {
	Model               string   `json:"model"`
	MaxSteps            int      `json:"maxSteps"`
	EnableThinking      bool     `json:"enableThinking"`
	SystemPrompt        string   `json:"systemPrompt"`
	PromptID            *uint    `json:"promptId,omitempty"`
	EnablePlatformTools bool     `json:"enablePlatformTools"`
	EnabledTools        []string `json:"enabledTools"`
}

// BuiltinCoderConfig models configuration for the builtin coding agent.
type BuiltinCoderConfig struct {
	Model             string   `json:"model"`
	MaxSteps          int      `json:"maxSteps"`
	PermissionMode    string   `json:"permissionMode"` // "strict" or "trust"
	SystemPrompt      string   `json:"systemPrompt"`
	InjectPlatformDoc bool     `json:"injectPlatformDoc"`
	InjectUserDoc     bool     `json:"injectUserDoc"`
	InjectProjectDoc  bool     `json:"injectProjectDoc"`
	EnableSkills      bool     `json:"enableSkills"`
	EnabledTools      []string `json:"enabledTools"`
}

// OrchestratorConfig models configuration for collaborative multi-agent brain.
type OrchestratorConfig struct {
	Model           string   `json:"model"`
	MaxSteps        int      `json:"maxSteps"`
	TimeoutSeconds  int      `json:"timeoutSeconds"`
	SystemPrompt    string   `json:"systemPrompt"`
	AutoInjectRoles bool     `json:"autoInjectRoles"`
	EnabledTools    []string `json:"enabledTools"`
}

// CoreEngineConfig models general ReAct engine runtime parameters.
type CoreEngineConfig struct {
	FallbackModel   string `json:"fallbackModel"`
	EnableDebugLog  bool   `json:"enableDebugLog"`
	LogRetentionDays int   `json:"logRetentionDays"`
	SearchEngine    string `json:"searchEngine"` // "none", "tavily", "searxng"
}

// EnginesSetting consolidates configuration for all three AI agent personas and the core.
type EnginesSetting struct {
	Steward      StewardConfig      `json:"steward"`
	Coder        BuiltinCoderConfig `json:"coder"`
	Orchestrator OrchestratorConfig `json:"orchestrator"`
	Core         CoreEngineConfig   `json:"core"`
	UpdatedAt    string             `json:"updatedAt,omitempty"`
}

// DefaultEnginesSetting returns platform defaults for all engines.
func DefaultEnginesSetting() EnginesSetting {
	return EnginesSetting{
		Steward: StewardConfig{
			Model:               "",
			MaxSteps:            5,
			EnableThinking:      true,
			SystemPrompt:        "你是 dev_platform 平台的专职管家助手，熟悉本平台所有项目拓扑、提示词库和专家角色配置。请优先通过内置平台工具调阅系统元数据并以结构化、简练的形式回答用户。",
			EnablePlatformTools: true,
			EnabledTools: []string{
				"platform_list_projects",
				"platform_list_prompts",
				"platform_get_prompt",
				"platform_create_prompt",
			},
		},
		Coder: BuiltinCoderConfig{
			Model:             "",
			MaxSteps:          20,
			PermissionMode:    "strict",
			SystemPrompt:      "你是 dev_platform 平台的内置智能编码 Agent，直接操作用户的项目工作区。",
			InjectPlatformDoc: true,
			InjectUserDoc:     true,
			InjectProjectDoc:  true,
			EnableSkills:      true,
			EnabledTools: []string{
				"read_file",
				"write_file",
				"edit_file",
				"bash",
				"git_status",
				"git_diff",
			},
		},
		Orchestrator: OrchestratorConfig{
			Model:           "",
			MaxSteps:        20,
			TimeoutSeconds:  300,
			SystemPrompt:    "你是软件开发团队的【中枢调度大脑】(Orchestrator)。统揽全局，分析用户总体目标，合理拆解执行计划，通过流程工具调度团队专家角色分工执行，并把关验收。",
			AutoInjectRoles: true,
			EnabledTools: []string{
				"flow_dispatch_role",
				"flow_wait_session",
				"flow_ask_user",
				"flow_complete_task",
			},
		},
		Core: CoreEngineConfig{
			FallbackModel:    "",
			EnableDebugLog:   true,
			LogRetentionDays: 7,
			SearchEngine:     "none",
		},
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
	}
}

// Engines returns the current platform engines setting.
func (s *Service) Engines() (EnginesSetting, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	raw, found, err := s.repo.Get(enginesConfigKey)
	if err != nil {
		return DefaultEnginesSetting(), err
	}
	if !found || raw == "" {
		return DefaultEnginesSetting(), nil
	}

	setting := DefaultEnginesSetting()
	if err := json.Unmarshal([]byte(raw), &setting); err != nil {
		return DefaultEnginesSetting(), nil
	}
	return setting, nil
}

// SetEngines updates and persists the engines setting.
func (s *Service) SetEngines(setting EnginesSetting) (EnginesSetting, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	setting.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	bytes, err := json.Marshal(setting)
	if err != nil {
		return EnginesSetting{}, err
	}

	if err := s.repo.Set(enginesConfigKey, string(bytes)); err != nil {
		return EnginesSetting{}, err
	}
	return setting, nil
}
