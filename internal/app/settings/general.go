package settings

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

// AgentGeneralSetting represents the platform-wide configuration for all agents.
// SystemPrompt is the authoritative system prompt / AGENTS.md content.
type AgentGeneralSetting struct {
	SystemPrompt string `json:"systemPrompt"`
	Path         string `json:"path,omitempty"`
	UpdatedAt    string `json:"updatedAt,omitempty"`
}

// AgentGeneral returns the current global agent settings.
// It prioritizes dataDir/AGENTS.md if available, then falls back to the database
// and finally checks the local opencode global instructions file for migration compatibility.
func (s *Service) AgentGeneral() (AgentGeneralSetting, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var prompt string
	var path string
	var updatedAt string

	// 1. Check dataDir/AGENTS.md
	if s.dataDir != "" {
		primaryFile := filepath.Join(s.dataDir, "AGENTS.md")
		if data, err := os.ReadFile(primaryFile); err == nil {
			prompt = string(data)
			path = primaryFile
			if info, err := os.Stat(primaryFile); err == nil {
				updatedAt = info.ModTime().UTC().Format(time.RFC3339)
			}
		}
	}

	// 2. If prompt is still empty, check the database
	if prompt == "" {
		if val, found, err := s.repo.Get(agentGeneralKey); err == nil && found && val != "" {
			prompt = val
		}
	}

	// 3. Fallback: check ~/.config/opencode/AGENTS.md if still empty and dataDir is configured
	if prompt == "" && s.dataDir != "" {
		if configDir, err := s.platform.Dirs().ConfigDir(); err == nil && configDir != "" {
			opencodeFile := filepath.Join(configDir, "opencode", "AGENTS.md")
			if data, err := os.ReadFile(opencodeFile); err == nil {
				prompt = string(data)
				if path == "" {
					path = opencodeFile
				}
				if updatedAt == "" {
					if info, err := os.Stat(opencodeFile); err == nil {
						updatedAt = info.ModTime().UTC().Format(time.RFC3339)
					}
				}
			}
		}
	}

	if path == "" && s.dataDir != "" {
		path = filepath.Join(s.dataDir, "AGENTS.md")
	}

	return AgentGeneralSetting{
		SystemPrompt: prompt,
		Path:         path,
		UpdatedAt:    updatedAt,
	}, nil
}

// SetAgentGeneral saves the global agent settings.
// It writes to dataDir/AGENTS.md, synchronizes to ~/.config/opencode/AGENTS.md using safe
// cross-platform file write, and records the prompt in the database.
func (s *Service) SetAgentGeneral(setting AgentGeneralSetting) (AgentGeneralSetting, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cleanPrompt := strings.TrimRight(setting.SystemPrompt, " \t\r\n")
	if cleanPrompt != "" {
		cleanPrompt += "\n"
	}

	// 1. Write authoritative file in dataDir/AGENTS.md
	var primaryPath string
	if s.dataDir != "" {
		if err := os.MkdirAll(s.dataDir, 0755); err != nil {
			return AgentGeneralSetting{}, err
		}
		primaryPath = filepath.Join(s.dataDir, "AGENTS.md")
		if err := os.WriteFile(primaryPath, []byte(cleanPrompt), 0644); err != nil {
			return AgentGeneralSetting{}, err
		}

		// 2. Cross-platform sync to OpenCode global config directory (~/.config/opencode/AGENTS.md)
		// Only synchronize when dataDir is configured (prevents unit tests without dataDir touching host environment)
		if configDir, err := s.platform.Dirs().ConfigDir(); err == nil && configDir != "" {
			opencodeDir := filepath.Join(configDir, "opencode")
			_ = os.MkdirAll(opencodeDir, 0755)
			opencodeFile := filepath.Join(opencodeDir, "AGENTS.md")
			_ = os.WriteFile(opencodeFile, []byte(cleanPrompt), 0644)
		}
	}

	// 3. Persist to database
	if err := s.repo.Set(agentGeneralKey, cleanPrompt); err != nil {
		return AgentGeneralSetting{}, err
	}

	nowStr := s.now().Format(time.RFC3339)
	return AgentGeneralSetting{
		SystemPrompt: cleanPrompt,
		Path:         primaryPath,
		UpdatedAt:    nowStr,
	}, nil
}
