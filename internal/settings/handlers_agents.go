package settings

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"devplatform/internal/agents"
)

// agentDTO is the JSON-stable representation of one backend's configuration and
// resolved path.
type agentDTO struct {
	Backend    string `json:"backend"`
	Enabled    bool   `json:"enabled"`
	Path       string `json:"path"`
	PathSource string `json:"pathSource"`
	Available  bool   `json:"available"`
	// Mode is the configured transport for backends that have more than one
	// (pi: "cli" or "rpc"); empty means the backend default. Modes lists the
	// choices so the workbench can render the control.
	Mode      string   `json:"mode,omitempty"`
	Modes     []string `json:"modes,omitempty"`
	Version   string   `json:"version,omitempty"`
	CheckedAt string   `json:"checkedAt,omitempty"`
}

type agentListResponse struct {
	Agents []agentDTO `json:"agents"`
}

// agentDetectionDTO is the outcome of a detect request.
type agentDetectionDTO struct {
	Backend string `json:"backend"`
	Status  string `json:"status"`
	Path    string `json:"path,omitempty"`
	Version string `json:"version,omitempty"`
	Error   string `json:"error,omitempty"`
}

func (s *Service) handleListAgents(c *gin.Context) {
	settings, err := s.Agents()
	if err != nil {
		respondError(c, http.StatusInternalServerError, "server_error")
		return
	}
	descriptors := s.catalog.All()
	out := make([]agentDTO, 0, len(descriptors))
	for _, descriptor := range descriptors {
		out = append(out, s.agentDTO(descriptor.ID, settings[descriptor.ID]))
	}
	c.JSON(http.StatusOK, agentListResponse{Agents: out})
}

func (s *Service) handleSetAgentEnabled(c *gin.Context) {
	backend, ok := s.backendParam(c)
	if !ok {
		return
	}
	var request struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return
	}
	setting, err := s.SetAgentEnabled(backend, request.Enabled)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "server_error")
		return
	}
	c.JSON(http.StatusOK, s.agentDTO(backend, setting))
}

func (s *Service) handleSetAgentPath(c *gin.Context) {
	backend, ok := s.backendParam(c)
	if !ok {
		return
	}
	// A backend auto-detected on PATH is not manually editable.
	if _, source := s.ResolvePath(backend); source == "auto" {
		respondError(c, http.StatusConflict, "auto_detected")
		return
	}
	var request struct {
		Path string `json:"path"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return
	}
	path := strings.TrimSpace(request.Path)
	if path != "" && !isExecutable(path) {
		respondError(c, http.StatusBadRequest, "invalid_path")
		return
	}
	setting, err := s.SetAgentPath(backend, path)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "server_error")
		return
	}
	c.JSON(http.StatusOK, s.agentDTO(backend, setting))
}

// handleSetAgentMode sets a backend's transport mode. The value is validated
// against the modes the platform knows (or empty to restore the default); an
// adapter ignores anything else anyway.
func (s *Service) handleSetAgentMode(c *gin.Context) {
	backend, ok := s.backendParam(c)
	if !ok {
		return
	}
	var request struct {
		Mode string `json:"mode"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return
	}
	mode := strings.TrimSpace(request.Mode)
	if mode != "" {
		descriptor, ok := s.catalog.Get(backend)
		if !ok {
			respondError(c, http.StatusNotFound, "unknown_backend")
			return
		}
		valid := false
		for _, allowed := range descriptor.Modes {
			if mode == allowed {
				valid = true
				break
			}
		}
		if !valid {
			respondError(c, http.StatusBadRequest, "invalid_mode")
			return
		}
	}
	setting, err := s.SetAgentMode(backend, mode)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "server_error")
		return
	}
	c.JSON(http.StatusOK, s.agentDTO(backend, setting))
}

func (s *Service) handleDetectAgent(c *gin.Context) {
	backend, ok := s.backendParam(c)
	if !ok {
		return
	}
	path, source := s.ResolvePath(backend)
	if source == "none" {
		c.JSON(http.StatusOK, agentDetectionDTO{Backend: string(backend), Status: "error", Error: "executable not found"})
		return
	}

	version, err := probeVersion(c.Request.Context(), path)
	if err != nil {
		c.JSON(http.StatusOK, agentDetectionDTO{
			Backend: string(backend), Status: "error", Path: path, Error: agents.RedactSecrets(err.Error()),
		})
		return
	}
	if _, err := s.RecordDetection(backend, version); err != nil {
		respondError(c, http.StatusInternalServerError, "server_error")
		return
	}
	c.JSON(http.StatusOK, agentDetectionDTO{Backend: string(backend), Status: "ok", Path: path, Version: version})
}

// agentDTO resolves the backend's current path and builds its API view.
func (s *Service) agentDTO(backend agents.BackendID, setting AgentSetting) agentDTO {
	path, source := s.ResolvePath(backend)
	descriptor, _ := s.catalog.Get(backend)
	return agentDTO{
		Backend:    string(backend),
		Enabled:    setting.Enabled,
		Path:       path,
		PathSource: source,
		Available:  source != "none" && isExecutable(path),
		Mode:       setting.Mode,
		Modes:      descriptor.Modes,
		Version:    setting.Version,
		CheckedAt:  setting.CheckedAt,
	}
}

// probeVersion runs `<path> --version` and parses a MAJOR.MINOR.PATCH triple,
// falling back to the bounded raw output when it is not parseable.
func probeVersion(ctx context.Context, path string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, "--version").Output()
	if err != nil {
		return "", err
	}
	text := strings.TrimSpace(string(out))
	if version, ok := agents.ParseVersion(text); ok {
		return version.String(), nil
	}
	if text == "" {
		return "", errors.New("no version output")
	}
	if len(text) > 100 {
		text = text[:100]
	}
	return text, nil
}

// isExecutable reports whether path is an existing, executable regular file.
func isExecutable(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	return info.Mode()&0o111 != 0
}

// backendParam parses and validates the :backend path parameter against the
// catalog, so only described backends can be configured.
func (s *Service) backendParam(c *gin.Context) (agents.BackendID, bool) {
	backend := agents.BackendID(c.Param("backend"))
	if _, known := s.catalog.Get(backend); !known {
		respondError(c, http.StatusBadRequest, "invalid_backend")
		return "", false
	}
	return backend, true
}
