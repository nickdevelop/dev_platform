package projects

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"devplatform/internal/app/paths"
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
	r.POST("/projects/:id/name", s.handleUpdateName)
	r.PUT("/projects/:id/name", s.handleUpdateName)
	r.POST("/projects/:id/unregister", s.handleUnregister)
	r.GET("/projects/:id/git-info", s.handleGitInfo)
	r.GET("/projects/:id/files", s.handleSearchFiles)
	r.GET("/projects/:id/files/tree", s.handleFileTree)
	r.GET("/projects/:id/files/content", s.handleFileContent)
	r.GET("/projects/:id/files/raw", s.handleFileRaw)
	r.GET("/projects/:id/git/status", s.handleGitStatus)
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
	case errors.Is(err, ErrInstanceStop):
		respondError(c, http.StatusConflict, "instance_stop_failed")
	case errors.Is(err, paths.ErrInvalidPath), errors.Is(err, ErrPathChanged):
		respondError(c, http.StatusBadRequest, "invalid_target")
	case errors.Is(err, ErrFileNotFound):
		respondError(c, http.StatusNotFound, "file_not_found")
	case errors.Is(err, ErrPathEscaped):
		respondError(c, http.StatusBadRequest, "path_escaped")
	case errors.Is(err, ErrRollbackFailed):
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

// updateNameRequest is the JSON body of POST /api/projects/{id}/name.
type updateNameRequest struct {
	Name string `json:"name"`
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

// handleUpdateName updates only the project's display name without modifying its filesystem directory.
func (s *Service) handleUpdateName(c *gin.Context) {
	id, ok := projectID(c)
	if !ok {
		return
	}
	var req updateNameRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return
	}

	project, err := s.UpdateName(id, req.Name)
	if err != nil {
		respondServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, project)
}

// handleUnregister removes the project from the platform, stopping its live
// instances first. The directory on disk is never touched.
func (s *Service) handleUnregister(c *gin.Context) {
	id, ok := projectID(c)
	if !ok {
		return
	}
	if err := s.Unregister(c.Request.Context(), id); err != nil {
		respondServiceError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// handleGitInfo reports the git state of the project's directory. A directory
// that is not a git work tree reports isGit false rather than an error.
func (s *Service) handleGitInfo(c *gin.Context) {
	id, ok := projectID(c)
	if !ok {
		return
	}
	info, err := s.GitInfo(c.Request.Context(), id)
	if err != nil {
		respondServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, info)
}

// filesResponse is the JSON body of GET /api/projects/:id/files.
type filesResponse struct {
	Files []FileItem `json:"files"`
}

// handleSearchFiles searches files and directories inside the project workspace.
func (s *Service) handleSearchFiles(c *gin.Context) {
	id, ok := projectID(c)
	if !ok {
		return
	}
	query := c.Query("q")
	limit := 50
	if limitStr := c.Query("limit"); limitStr != "" {
		if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	files, err := s.SearchFiles(c.Request.Context(), id, query, limit)
	if err != nil {
		respondServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, filesResponse{Files: files})
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

// handleFileTree returns the multi-level directory tree of the project workspace.
func (s *Service) handleFileTree(c *gin.Context) {
	id, ok := projectID(c)
	if !ok {
		return
	}
	tree, err := s.GetFileTree(c.Request.Context(), id)
	if err != nil {
		respondServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"tree": tree})
}

// handleFileContent returns the inspected file content and metadata.
func (s *Service) handleFileContent(c *gin.Context) {
	id, ok := projectID(c)
	if !ok {
		return
	}
	filePath := c.Query("path")
	if filePath == "" {
		respondError(c, http.StatusBadRequest, "missing_path")
		return
	}
	resp, err := s.ReadFileContent(c.Request.Context(), id, filePath)
	if err != nil {
		respondServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

// handleFileRaw streams the raw bytes of the requested file (images/media).
func (s *Service) handleFileRaw(c *gin.Context) {
	id, ok := projectID(c)
	if !ok {
		return
	}
	filePath := c.Query("path")
	if filePath == "" {
		respondError(c, http.StatusBadRequest, "missing_path")
		return
	}
	reader, mimeType, size, err := s.ReadFileRaw(c.Request.Context(), id, filePath)
	if err != nil {
		respondServiceError(c, err)
		return
	}
	defer reader.Close()
	c.DataFromReader(http.StatusOK, size, mimeType, reader, nil)
}

// handleGitStatus returns the list of changed files in the git workspace.
func (s *Service) handleGitStatus(c *gin.Context) {
	id, ok := projectID(c)
	if !ok {
		return
	}
	items, err := s.GitStatus(c.Request.Context(), id)
	if err != nil {
		respondServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"changes": items})
}
