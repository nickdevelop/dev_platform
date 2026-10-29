package agentmodels

import (
	"errors"
	"log"
	"net/http"
	"strings"

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
	// Updating allows enabling, disabling and customizing model metadata.
	// Both /models and /models/*model forms are supported to handle model IDs containing slashes.
	r.PATCH("/agent-backends/:backend/models", s.handleUpdateModel)
	r.PATCH("/agent-backends/:backend/models/*model", s.handleUpdateModel)
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
	case errors.Is(err, ErrModelNotFound):
		respondError(c, http.StatusNotFound, "model_not_found")
	case errors.Is(err, ErrInvalidModelPatch):
		respondError(c, http.StatusBadRequest, "invalid_model_patch")
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
		// The wire response stays a stable error code, but the server log keeps
		// the adapter's reason: a backend that cannot even start and a backend
		// that refused the catalog call would otherwise look identical in the
		// access log.
		if !errors.Is(err, ErrUnknownBackend) && !errors.Is(err, ErrUnsupportedCapability) {
			log.Printf("agent models: refresh %s failed: %v", backend, agents.RedactSecrets(err.Error()))
		}
		respondServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, list)
}

type updateModelRequest struct {
	ModelID        string  `json:"modelId"`
	Enabled        *bool   `json:"enabled"`
	Name           *string `json:"name"`
	ContextLimit   *int    `json:"contextLimit"`
	SupportsImages *bool   `json:"supportsImages"`
}

// handleUpdateModel modifies the platform-level configuration (enabled state,
// custom name, context limit, image support) of an agent model.
func (s *Service) handleUpdateModel(c *gin.Context) {
	backend, ok := backendID(c)
	if !ok {
		return
	}

	modelID := strings.TrimPrefix(c.Param("model"), "/")

	var req updateModelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid_payload")
		return
	}

	if modelID == "" {
		modelID = req.ModelID
	} else if req.ModelID != "" && req.ModelID != modelID {
		respondError(c, http.StatusBadRequest, "invalid_model_id")
		return
	}

	if modelID == "" {
		respondError(c, http.StatusBadRequest, "invalid_model_id")
		return
	}

	patch := ModelPatch{
		Enabled:        req.Enabled,
		Name:           req.Name,
		ContextLimit:   req.ContextLimit,
		SupportsImages: req.SupportsImages,
	}

	updated, err := s.UpdateModel(c.Request.Context(), backend, modelID, patch)
	if err != nil {
		respondServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, updated)
}
