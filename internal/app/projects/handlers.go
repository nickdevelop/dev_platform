package projects

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"devplatform/internal/app/paths"
	"devplatform/internal/core/platform"
)

// RegisterRoutes registers the project management routes under r. The caller
// must protect r with session authentication (auth.RequireSession); CSRF
// enforcement is installed globally by the platform router.
func (s *Service) RegisterRoutes(r gin.IRouter) {
	r.GET("/projects", s.handleList)
	r.POST("/projects", s.handleRegister)
	r.POST("/projects/create", s.handleCreate)
	r.POST("/projects/:id/move", s.handleMove)
	r.POST("/projects/:id/rename", s.handleRename)
	r.POST("/projects/:id/unregister", s.handleUnregister)
	r.POST("/projects/:id/delete", s.handleDelete)
}

// errorResponse is the JSON error envelope for project API failures.
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
	case errors.Is(err, ErrDirectoryMissing):
		respondError(c, http.StatusConflict, "directory_missing")
	case errors.Is(err, ErrNotDirectory):
		respondError(c, http.StatusBadRequest, "not_a_directory")
	case errors.Is(err, ErrTargetExists), errors.Is(err, ErrProjectExists):
		respondError(c, http.StatusConflict, "path_conflict")
	case errors.Is(err, ErrInvalidTarget):
		respondError(c, http.StatusBadRequest, "invalid_target")
	case errors.Is(err, ErrInvalidName):
		respondError(c, http.StatusBadRequest, "invalid_name")
	case errors.Is(err, ErrConfirmationMismatch):
		respondError(c, http.StatusBadRequest, "confirmation_mismatch")
	case errors.Is(err, platform.ErrTrashUnavailable):
		respondError(c, http.StatusConflict, "trash_unavailable")
	case errors.Is(err, paths.ErrInvalidPath), errors.Is(err, ErrPathChanged):
		respondError(c, http.StatusBadRequest, "invalid_target")
	case errors.Is(err, ErrRestoreFailed), errors.Is(err, ErrRollbackFailed):
		respondError(c, http.StatusInternalServerError, "inconsistent_state")
	default:
		respondError(c, http.StatusInternalServerError, "server_error")
	}
}

// projectListResponse is the JSON body of GET /api/projects.
type projectListResponse struct {
	Projects []Project `json:"projects"`
}

// registerRequest is the JSON body of POST /api/projects.
type registerRequest struct {
	Path string `json:"path"`
}

// createRequest is the JSON body of POST /api/projects/create.
type createRequest struct {
	Parent string `json:"parent"`
	Name   string `json:"name"`
}

// moveRequest is the JSON body of POST /api/projects/{id}/move.
type moveRequest struct {
	Path        string `json:"path"`
	ConfirmName string `json:"confirmName"`
}

// renameRequest is the JSON body of POST /api/projects/{id}/rename.
type renameRequest struct {
	Name        string `json:"name"`
	ConfirmName string `json:"confirmName"`
}

// deleteRequest is the JSON body of POST /api/projects/{id}/delete. The typed
// confirmation must match the project name; Permanent is the explicit second
// confirmation required when the OS trash is unavailable.
type deleteRequest struct {
	ConfirmName string `json:"confirmName"`
	Permanent   bool   `json:"permanent"`
}

// handleList returns all registered projects.
func (s *Service) handleList(c *gin.Context) {
	projects, err := s.List()
	if err != nil {
		respondServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, projectListResponse{Projects: projects})
}

// handleRegister registers an existing directory under an authorized root.
func (s *Service) handleRegister(c *gin.Context) {
	var req registerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return
	}

	project, err := s.Register(req.Path)
	if err != nil {
		respondServiceError(c, err)
		return
	}
	c.JSON(http.StatusCreated, project)
}

// handleCreate creates a new project directory under an authorized parent.
func (s *Service) handleCreate(c *gin.Context) {
	var req createRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return
	}

	project, err := s.Create(req.Parent, req.Name)
	if err != nil {
		respondServiceError(c, err)
		return
	}
	c.JSON(http.StatusCreated, project)
}

// handleMove moves a project directory to an authorized destination.
func (s *Service) handleMove(c *gin.Context) {
	id, ok := projectID(c)
	if !ok {
		return
	}
	var req moveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return
	}

	project, err := s.Move(id, req.Path, req.ConfirmName)
	if err != nil {
		respondServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, project)
}

// handleRename renames a project directory within its parent.
func (s *Service) handleRename(c *gin.Context) {
	id, ok := projectID(c)
	if !ok {
		return
	}
	var req renameRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return
	}

	project, err := s.Rename(id, req.Name, req.ConfirmName)
	if err != nil {
		respondServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, project)
}

// handleUnregister removes the platform record without touching the directory.
func (s *Service) handleUnregister(c *gin.Context) {
	id, ok := projectID(c)
	if !ok {
		return
	}
	if err := s.Unregister(id); err != nil {
		respondServiceError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// handleDelete deletes the project directory (trash first, permanent only with
// explicit confirmation) and its record.
func (s *Service) handleDelete(c *gin.Context) {
	id, ok := projectID(c)
	if !ok {
		return
	}
	var req deleteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return
	}

	if err := s.Delete(id, req.ConfirmName, req.Permanent); err != nil {
		respondServiceError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
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
