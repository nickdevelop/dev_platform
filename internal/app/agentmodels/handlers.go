package agentmodels

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"devplatform/internal/core/agents"
)

// RegisterRoutes registers the platform-wide agent model routes under r. The
// caller must protect r with session authentication; CSRF enforcement is
// installed globally by the platform router.
func (s *Service) RegisterRoutes(r gin.IRouter) {
	r.GET("/agent-backends/:backend/models", s.handleList)
	// Refreshing calls the backend and rewrites the snapshot, so it is a
	// state-changing operation and must never be a plain GET.
	r.POST("/agent-backends/:backend/models/refresh", s.handleRefresh)
}

// errorResponse is the JSON error envelope for model catalog failures.
type errorResponse struct {
	Error string `json:"error"`
}

func respondError(c *gin.Context, status int, code string) {
	c.AbortWithStatusJSON(status, errorResponse{Error: code})
}

// respondServiceError maps a service error to an HTTP status and error code.
func respondServiceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrUnknownBackend):
		respondError(c, http.StatusBadRequest, "unknown_backend")
	case errors.Is(err, ErrUnsupportedCapability):
		respondError(c, http.StatusConflict, "unsupported_capability")
	case errors.Is(err, agents.ErrServiceNotRunning):
		respondError(c, http.StatusConflict, "service_not_running")
	case errors.Is(err, ErrStorage):
		respondError(c, http.StatusInternalServerError, "server_error")
	case errors.Is(err, ErrModelListFailed):
		respondError(c, http.StatusBadGateway, "model_list_failed")
	default:
		respondError(c, http.StatusInternalServerError, "server_error")
	}
}

func backendID(c *gin.Context) (agents.BackendID, bool) {
	backend := agents.BackendID(c.Param("backend"))
	if backend == "" {
		respondError(c, http.StatusBadRequest, "unknown_backend")
		return "", false
	}
	return backend, true
}

// handleList returns the persisted model snapshot for one backend.
func (s *Service) handleList(c *gin.Context) {
	backend, ok := backendID(c)
	if !ok {
		return
	}
	list, err := s.List(backend)
	if err != nil {
		respondServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, list)
}

// handleRefresh reads the backend's current catalog and replaces the persisted
// snapshot with it.
func (s *Service) handleRefresh(c *gin.Context) {
	backend, ok := backendID(c)
	if !ok {
		return
	}
	list, err := s.Refresh(c.Request.Context(), backend)
	if err != nil {
		respondServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, list)
}
