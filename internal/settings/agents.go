package settings

import (
	"errors"
	"fmt"
	"os/exec"
	"time"

	"gorm.io/gorm"

	"devplatform/internal/agents"
	"devplatform/internal/database"
)

// agentsKey is the settings-table key holding the agent configuration JSON.
const agentsKey = "agents.config"

// AgentSetting is the platform-level configuration for one agent backend. A
// backend with Enabled true may be used to create new tasks; ManualPath is
// empty when the executable is resolved from PATH, and Version/CheckedAt record
// the last successful detection.
type AgentSetting struct {
	Enabled    bool   `json:"enabled"`
	ManualPath string `json:"manualPath"`
	Version    string `json:"version"`
	CheckedAt  string `json:"checkedAt"`
}

// defaultAgentSettings returns every known backend enabled with no override.
func defaultAgentSettings() map[agents.BackendID]AgentSetting {
	return map[agents.BackendID]AgentSetting{
		agents.BackendOpenCodeV2: {Enabled: true},
		agents.BackendPi:         {Enabled: true},
	}
}

// Agents returns the configuration of every known backend, filling defaults for
// anything never saved.
func (s *Service) Agents() (map[agents.BackendID]AgentSetting, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadAgents()
}

func (s *Service) loadAgents() (map[agents.BackendID]AgentSetting, error) {
	settings := defaultAgentSettings()

	var row database.Setting
	err := s.db.Where("key = ?", agentsKey).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return settings, nil
	}
	if err != nil {
		return nil, fmt.Errorf("settings: load agents: %w", err)
	}

	var stored map[agents.BackendID]AgentSetting
	if err := jsonUnmarshal(row.Value, &stored); err != nil {
		return nil, fmt.Errorf("settings: decode agents: %w", err)
	}
	// Only known backends are honored, so a stale key cannot introduce one.
	for backend, value := range stored {
		if _, ok := settings[backend]; ok {
			settings[backend] = value
		}
	}
	return settings, nil
}

func (s *Service) saveAgents(settings map[agents.BackendID]AgentSetting) error {
	data, err := jsonMarshal(settings)
	if err != nil {
		return fmt.Errorf("settings: encode agents: %w", err)
	}
	if err := s.db.Save(&database.Setting{Key: agentsKey, Value: data}).Error; err != nil {
		return fmt.Errorf("settings: save agents: %w", err)
	}
	return nil
}

// SetAgentEnabled turns a backend on or off platform-wide.
func (s *Service) SetAgentEnabled(backend agents.BackendID, enabled bool) (AgentSetting, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	settings, err := s.loadAgents()
	if err != nil {
		return AgentSetting{}, err
	}
	setting := settings[backend]
	setting.Enabled = enabled
	settings[backend] = setting
	if err := s.saveAgents(settings); err != nil {
		return AgentSetting{}, err
	}
	return setting, nil
}

// SetAgentPath records a manual executable path for a backend. An empty path
// clears the override so the backend resolves from PATH again.
func (s *Service) SetAgentPath(backend agents.BackendID, manualPath string) (AgentSetting, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	settings, err := s.loadAgents()
	if err != nil {
		return AgentSetting{}, err
	}
	setting := settings[backend]
	setting.ManualPath = manualPath
	settings[backend] = setting
	if err := s.saveAgents(settings); err != nil {
		return AgentSetting{}, err
	}
	return setting, nil
}

// RecordDetection stores the version a successful detection reported.
func (s *Service) RecordDetection(backend agents.BackendID, version string) (AgentSetting, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	settings, err := s.loadAgents()
	if err != nil {
		return AgentSetting{}, err
	}
	setting := settings[backend]
	setting.Version = version
	setting.CheckedAt = s.now().Format(time.RFC3339)
	settings[backend] = setting
	if err := s.saveAgents(settings); err != nil {
		return AgentSetting{}, err
	}
	return setting, nil
}

// Enabled reports whether a backend may be used platform-wide. A lookup error
// fails open (enabled) so a settings read failure cannot silently disable every
// agent.
func (s *Service) Enabled(backend agents.BackendID) bool {
	settings, err := s.Agents()
	if err != nil {
		return true
	}
	return settings[backend].Enabled
}

// Executable returns the manual executable path for a backend, or "" when none
// is set. It is the resolver wired into the backend adapters.
func (s *Service) Executable(backend agents.BackendID) string {
	settings, err := s.Agents()
	if err != nil {
		return ""
	}
	return settings[backend].ManualPath
}

// ResolvePath resolves a backend's executable to a path and its source:
// "manual" when an override is set, "auto" when found on PATH, "none"
// otherwise.
func (s *Service) ResolvePath(backend agents.BackendID) (string, string) {
	settings, err := s.Agents()
	if err == nil {
		if manual := settings[backend].ManualPath; manual != "" {
			return manual, "manual"
		}
	}
	if path, lookErr := exec.LookPath(backendBinary(backend)); lookErr == nil {
		return path, "auto"
	}
	return "", "none"
}

// backendBinary is the default executable name of a backend.
func backendBinary(backend agents.BackendID) string {
	switch backend {
	case agents.BackendOpenCodeV2:
		return "opencode"
	default:
		return "pi"
	}
}
