package settings

import (
	"fmt"
	"time"

	"devplatform/internal/core/agents"
)

// agentsKey is the settings-table key holding the agent configuration JSON.
const agentsKey = "agents.config"

// AgentSetting is the platform-level configuration for one agent backend. A
// backend with Enabled true may be used to create new sessions; ManualPath is
// empty when the executable is resolved from PATH, and Version/CheckedAt record
// the last successful detection.
type AgentSetting struct {
	Enabled    bool   `json:"enabled"`
	ManualPath string `json:"manualPath"`
	// Mode selects a backend-specific transport when the backend has more
	// than one (pi: "cli" or "rpc"). Empty means the backend's default, and
	// an unknown value is treated as the default by the adapter. It is the
	// operator's rollback lever when a new transport misbehaves: switch back
	// to the previous mode and restart the platform.
	Mode      string `json:"mode,omitempty"`
	Version   string `json:"version"`
	CheckedAt string `json:"checkedAt"`
	// IdleTimeoutMinutes configures how long an idle backend host process
	// runs before automatic reclamation. -1 means never disconnect, > 0 specifies
	// the minutes. 0 uses the catalog descriptor default.
	IdleTimeoutMinutes int `json:"idleTimeoutMinutes,omitempty"`
	// PermissionMode selects how a backend that supports tool confirmation
	// gates its high-risk calls ("confirm" asks on an approval card, "auto"
	// approves everything). Empty means the backend default (confirm).
	PermissionMode string `json:"permissionMode,omitempty"`
}

// defaultAgentSettings returns every catalog backend at its descriptor's
// default enablement, with no override.
func (s *Service) defaultAgentSettings() map[agents.BackendID]AgentSetting {
	defaults := make(map[agents.BackendID]AgentSetting)
	for _, descriptor := range s.catalog.All() {
		defaults[descriptor.ID] = AgentSetting{
			Enabled:            descriptor.DefaultEnabled,
			IdleTimeoutMinutes: descriptor.DefaultIdleTimeoutMinutes,
		}
	}
	return defaults
}

// Agents returns the configuration of every known backend, filling defaults for
// anything never saved.
func (s *Service) Agents() (map[agents.BackendID]AgentSetting, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadAgents()
}

func (s *Service) loadAgents() (map[agents.BackendID]AgentSetting, error) {
	settings := s.defaultAgentSettings()

	val, found, err := s.repo.Get(agentsKey)
	if err != nil {
		return nil, err
	}
	if !found {
		return settings, nil
	}

	var stored map[agents.BackendID]AgentSetting
	if err := jsonUnmarshal(val, &stored); err != nil {
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
	if err := s.repo.Set(agentsKey, data); err != nil {
		return err
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

// SetAgentMode records a backend transport mode. An empty mode restores the
// backend's default. The value is stored verbatim; the adapter ignores values
// it does not recognize.
func (s *Service) SetAgentMode(backend agents.BackendID, mode string) (AgentSetting, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	settings, err := s.loadAgents()
	if err != nil {
		return AgentSetting{}, err
	}
	setting := settings[backend]
	setting.Mode = mode
	settings[backend] = setting
	if err := s.saveAgents(settings); err != nil {
		return AgentSetting{}, err
	}
	return setting, nil
}

// SetAgentIdleTimeout records the idle disconnection timeout in minutes for a backend.
// -1 means never disconnect, > 0 specifies the minutes.
func (s *Service) SetAgentIdleTimeout(backend agents.BackendID, minutes int) (AgentSetting, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	settings, err := s.loadAgents()
	if err != nil {
		return AgentSetting{}, err
	}
	setting := settings[backend]
	setting.IdleTimeoutMinutes = minutes
	settings[backend] = setting
	if err := s.saveAgents(settings); err != nil {
		return AgentSetting{}, err
	}
	return setting, nil
}

// AgentIdleTimeout returns the idle duration for a backend before reclamation.
// -1 returns 0 (indicating no idle timeout). Values > 0 are converted to minutes.
// If unset (0), defaults to the descriptor's configured timeout or 30 minutes.
func (s *Service) AgentIdleTimeout(backend agents.BackendID) time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	settings, err := s.loadAgents()
	if err != nil {
		if descriptor, ok := s.catalog.Get(backend); ok && descriptor.DefaultIdleTimeoutMinutes > 0 {
			return time.Duration(descriptor.DefaultIdleTimeoutMinutes) * time.Minute
		}
		return 30 * time.Minute
	}
	minutes := settings[backend].IdleTimeoutMinutes
	if minutes < 0 {
		return 0 // never disconnect
	}
	if minutes == 0 {
		if descriptor, ok := s.catalog.Get(backend); ok && descriptor.DefaultIdleTimeoutMinutes > 0 {
			return time.Duration(descriptor.DefaultIdleTimeoutMinutes) * time.Minute
		}
		return 30 * time.Minute
	}
	return time.Duration(minutes) * time.Minute
}

// AgentMode returns the configured transport mode for a backend (empty when
// unset), used by the composition root when constructing the adapter.
func (s *Service) AgentMode(backend agents.BackendID) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	settings, err := s.loadAgents()
	if err != nil {
		return ""
	}
	return settings[backend].Mode
}

// AgentPermissionMode returns the configured permission mode of a backend
// (empty means the backend default). It is consulted by the embedding
// adapter before every turn so operator changes apply at once.
func (s *Service) AgentPermissionMode(backend agents.BackendID) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	settings, err := s.loadAgents()
	if err != nil {
		return ""
	}
	return settings[backend].PermissionMode
}

// SetAgentPermissionMode stores a backend's tool permission mode. The value
// must be one the descriptor offers, or empty to restore the default.
func (s *Service) SetAgentPermissionMode(backend agents.BackendID, mode string) (AgentSetting, error) {
	if mode != "" {
		s.mu.Lock()
		descriptor, known := s.catalog.Get(backend)
		s.mu.Unlock()
		if !known {
			return AgentSetting{}, fmt.Errorf("settings: unknown backend %q", backend)
		}
		valid := false
		for _, allowed := range descriptor.PermissionModes {
			if mode == allowed {
				valid = true
				break
			}
		}
		if !valid {
			return AgentSetting{}, fmt.Errorf("settings: backend %q does not support permission mode %q", backend, mode)
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	settings, err := s.loadAgents()
	if err != nil {
		return AgentSetting{}, err
	}
	current, ok := settings[backend]
	if !ok {
		return AgentSetting{}, fmt.Errorf("settings: unknown backend %q", backend)
	}
	current.PermissionMode = mode
	settings[backend] = current
	if err := s.saveAgents(settings); err != nil {
		return AgentSetting{}, err
	}
	return current, nil
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
	descriptor, known := s.catalog.Get(backend)
	if !known {
		return "", "none"
	}
	// Backends with a platform-specific layout (Windows npm shims) resolve
	// through the descriptor; the port default covers everyone else.
	resolve := descriptor.ResolveExecutable
	if resolve == nil {
		executables := s.platform.Executables()
		resolve = executables.Resolve
	}
	if path, lookErr := resolve(descriptor.Binary); lookErr == nil {
		return path, "auto"
	}
	return "", "none"
}
