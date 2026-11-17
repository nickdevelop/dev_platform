package agentusage

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"devplatform/internal/core/agents"
)

// RegisterRoutes registers the agent usage routes under r.
// The caller must protect r with session authentication.
func (s *Service) RegisterRoutes(r gin.IRouter) {
	r.GET("/agent-backends/:backend/usage", s.handleGetUsage)
}

func backendParam(c *gin.Context) (agents.BackendID, bool) {
	backend := agents.BackendID(c.Param("backend"))
	if !backend.Valid() {
		respondError(c, http.StatusBadRequest, "unknown_backend")
		return "", false
	}
	return backend, true
}

func (s *Service) handleGetUsage(c *gin.Context) {
	backend, ok := backendParam(c)
	if !ok {
		return
	}

	report, err := s.GetUsage(c.Request.Context(), backend)
	if err != nil {
		respondUsageError(c, err)
		return
	}

	c.JSON(http.StatusOK, report)
}

type errorResponse struct {
	Error string `json:"error"`
}

func respondError(c *gin.Context, status int, code string) {
	c.AbortWithStatusJSON(status, errorResponse{Error: code})
}

func respondUsageError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrUnknownBackend):
		respondError(c, http.StatusBadRequest, "unknown_backend")
	case errors.Is(err, ErrUnsupportedCapability):
		respondError(c, http.StatusBadRequest, "unsupported_capability")
	default:
		respondError(c, http.StatusInternalServerError, "usage_failed")
	}
}
