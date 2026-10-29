package instances

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"devplatform/internal/core/agents"
	"devplatform/internal/app/paths"
	"devplatform/internal/app/projects"
)

// RegisterRoutes registers the project-scoped agent instance routes under r.
// The caller must protect r with session authentication (auth.RequireSession);
// CSRF enforcement is installed globally by the platform router.
func (s *Service) RegisterRoutes(r gin.IRouter) {
	r.GET("/projects/:id/instances", s.handleList)
	r.POST("/projects/:id/instances", s.handleStart)
	r.POST("/projects/:id/instances/:instanceId/stop", s.handleStop)
}

// errorResponse is the JSON error envelope for instance API failures.
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
	case errors.Is(err, ErrProjectNotFound):
		respondError(c, http.StatusNotFound, "project_not_found")
	case errors.Is(err, ErrInstanceNotFound):
		respondError(c, http.StatusNotFound, "instance_not_found")
	case errors.Is(err, ErrUnknownBackend):
		respondError(c, http.StatusBadRequest, "unknown_backend")
	case errors.Is(err, ErrInstanceNotRunning):
		respondError(c, http.StatusConflict, "instance_not_running")
	case errors.Is(err, ErrAgentDisabled):
		respondError(c, http.StatusConflict, "agent_disabled")
	case errors.Is(err, ErrStartFailed):
		// The failed attempt is recorded and discoverable through the list
		// route; the start request itself did not succeed.
		respondError(c, http.StatusBadGateway, "instance_start_failed")
	case errors.Is(err, ErrDirectoryUnavailable), errors.Is(err, projects.ErrDirectoryMissing), errors.Is(err, projects.ErrNotDirectory):
		respondError(c, http.StatusConflict, "project_directory_unavailable")
	case errors.Is(err, ErrProjectNotAuthorized), errors.Is(err, paths.ErrInvalidPath), errors.Is(err, projects.ErrPathChanged):
		respondError(c, http.StatusBadRequest, "project_not_authorized")
	default:
		respondError(c, http.StatusInternalServerError, "server_error")
	}
}

// startRequest is the JSON body of POST /api/projects/{id}/instances.
type startRequest struct {
	Backend string `json:"backend"`
}

// instanceListResponse is the JSON body of GET /api/projects/{id}/instances.
type instanceListResponse struct {
	Instances []Instance `json:"instances"`
}

// handleList returns the project's agent instances, optionally filtered by status (e.g. ?status=live).
func (s *Service) handleList(c *gin.Context) {
	id, ok := projectID(c)
	if !ok {
		return
	}
	status := c.Query("status")
	var list []Instance
	var err error
	if status != "" {
		list, err = s.ListFiltered(id, status)
	} else {
		list, err = s.List(id)
	}
	if err != nil {
		respondServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, instanceListResponse{Instances: list})
}

// handleStart starts a new instance of the requested backend in the project.
func (s *Service) handleStart(c *gin.Context) {
	id, ok := projectID(c)
	if !ok {
		return
	}
	var req startRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return
	}
	if req.Backend == "" {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return
	}

	instance, err := s.Start(c.Request.Context(), id, agents.BackendID(req.Backend))
	if err != nil {
		respondServiceError(c, err)
		return
	}
	c.JSON(http.StatusCreated, instance)
}

// handleStop stops the named instance. The operation is idempotent.
func (s *Service) handleStop(c *gin.Context) {
	id, ok := projectID(c)
	if !ok {
		return
	}
	instanceID := c.Param("instanceId")
	if instanceID == "" {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return
	}

	// Resolve the instance first so a stop can never reach an instance that
	// belongs to a different project.
	existing, err := s.Get(instanceID)
	if err != nil {
		respondServiceError(c, err)
		return
	}
	if existing.ProjectID != id {
		respondError(c, http.StatusNotFound, "instance_not_found")
		return
	}

	instance, err := s.Stop(c.Request.Context(), instanceID)
	if err != nil {
		respondServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, instance)
}

// projectID parses the :id path parameter, writing a 400 on malformed input.
func projectID(c *gin.Context) (uint, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return 0, false
	}
	return uint(id), true
}
