package agentservices

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"devplatform/internal/core/agents"
)

// RegisterRoutes registers the machine-level service routes under r. The caller
// must protect r with session authentication.
func (s *Service) RegisterRoutes(r gin.IRouter) {
	r.GET("/agent-services/:backend", s.handleStatus)
	r.POST("/agent-services/:backend/start", s.handleStart)
	r.POST("/agent-services/:backend/stop", s.handleStop)
}

// serviceStatusResponse is the JSON body of the agent-service routes.
type serviceStatusResponse struct {
	Status agents.ServiceStatus `json:"status"`
}

// backendParam parses the :backend path parameter, writing a 400 when it names
// no known backend.
func backendParam(c *gin.Context) (agents.BackendID, bool) {
	backend := agents.BackendID(c.Param("backend"))
	if !backend.Valid() {
		respondError(c, http.StatusBadRequest, "unknown_backend")
		return "", false
	}
	return backend, true
}

// handleStatus probes and reports one backend's machine-level service.
func (s *Service) handleStatus(c *gin.Context) {
	backend, ok := backendParam(c)
	if !ok {
		return
	}
	status, err := s.Detect(c.Request.Context(), backend)
	if err != nil {
		respondServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, serviceStatusResponse{Status: status})
}

// handleStart starts the service on the operator's request.
func (s *Service) handleStart(c *gin.Context) {
	backend, ok := backendParam(c)
	if !ok {
		return
	}
	status, err := s.Start(c.Request.Context(), backend)
	if err != nil {
		respondServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, serviceStatusResponse{Status: status})
}

// handleStop stops the service on the operator's request.
func (s *Service) handleStop(c *gin.Context) {
	backend, ok := backendParam(c)
	if !ok {
		return
	}
	status, err := s.Stop(c.Request.Context(), backend)
	if err != nil {
		respondServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, serviceStatusResponse{Status: status})
}

// errorResponse is the JSON error envelope for service API failures.
type errorResponse struct {
	Error string `json:"error"`
}

// respondError stops the chain and writes a JSON error envelope.
func respondError(c *gin.Context, status int, code string) {
	c.AbortWithStatusJSON(status, errorResponse{Error: code})
}

// respondServiceError maps a service error to an HTTP status and error code.
func respondServiceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrUnknownBackend):
		respondError(c, http.StatusBadRequest, "unknown_backend")
	case errors.Is(err, agents.ErrServiceNotRunning):
		respondError(c, http.StatusConflict, "service_not_running")
	case errors.Is(err, agents.ErrServiceAlreadyRunning):
		respondError(c, http.StatusConflict, "service_already_running")
	case errors.Is(err, agents.ErrServiceUnsupported):
		respondError(c, http.StatusBadRequest, "service_unsupported")
	default:
		respondError(c, http.StatusInternalServerError, "server_error")
	}
}
