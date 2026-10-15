package resources

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"devplatform/internal/core/agents"
)

// RegisterRoutes registers the resource management routes under r. The caller
// must protect r with session authentication (auth.RequireSession); CSRF
// enforcement is installed globally by the platform router.
func (s *Service) RegisterRoutes(r gin.IRouter) {
	r.GET("/resources", s.handleList)
	r.POST("/resources", s.handleCreate)
	r.GET("/resources/:id", s.handleGet)
	r.POST("/resources/:id/update", s.handleUpdate)
	r.POST("/resources/:id/delete", s.handleDelete)

	r.GET("/projects/:id/resources", s.handleListProjectResources)
	r.POST("/projects/:id/resources", s.handleAssociate)
	r.POST("/projects/:id/resources/remove", s.handleDisassociate)

	r.GET("/projects/:id/sessions/:sessionId/prompts", s.handleListSessionPrompts)
	r.POST("/projects/:id/sessions/:sessionId/prompts", s.handleSelectSessionPrompt)
	r.POST("/projects/:id/sessions/:sessionId/prompts/remove", s.handleRemoveSessionPrompt)
}

// errorResponse is the JSON error envelope for resource API failures.
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
	case errors.Is(err, ErrResourceNotFound):
		respondError(c, http.StatusNotFound, "resource_not_found")
	case errors.Is(err, ErrProjectNotFound):
		respondError(c, http.StatusNotFound, "project_not_found")
	case errors.Is(err, ErrSessionNotFound):
		respondError(c, http.StatusNotFound, "session_not_found")
	case errors.Is(err, ErrNotPrompt):
		respondError(c, http.StatusBadRequest, "not_a_prompt")
	case errors.Is(err, ErrResourceConflict):
		respondError(c, http.StatusConflict, "resource_conflict")
	case errors.Is(err, ErrInvalidResource):
		respondError(c, http.StatusBadRequest, "invalid_resource")
	default:
		respondError(c, http.StatusInternalServerError, "server_error")
	}
}

// resourceRequest is the JSON body of a create or update. Backends are plain
// strings on the wire; the handler converts them to backend ids.
type resourceRequest struct {
	Kind        string   `json:"kind"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Content     string   `json:"content"`
	Enabled     bool     `json:"enabled"`
	PromptScope string   `json:"promptScope"`
	SecretRef   string   `json:"secretRef"`
	SecretValue string   `json:"secretValue"`
	Backends    []string `json:"backends"`
}

func (req resourceRequest) input() CreateInput {
	backends := make([]agents.BackendID, 0, len(req.Backends))
	for _, backend := range req.Backends {
		backends = append(backends, agents.BackendID(backend))
	}
	return CreateInput{
		Kind:        ResourceKind(req.Kind),
		Name:        req.Name,
		Description: req.Description,
		Content:     req.Content,
		Enabled:     req.Enabled,
		PromptScope: PromptScope(req.PromptScope),
		SecretRef:   req.SecretRef,
		SecretValue: req.SecretValue,
		Backends:    backends,
	}
}

// resourceReference is the JSON body that names an existing resource.
type resourceReference struct {
	ResourceID uint `json:"resourceId"`
}

// resourceListResponse is the JSON body that wraps a list of resources.
type resourceListResponse struct {
	Resources []ResourceDefinition `json:"resources"`
}

// promptListResponse is the JSON body that wraps a list of selected prompts.
type promptListResponse struct {
	Prompts []ResourceDefinition `json:"prompts"`
}

func (s *Service) handleList(c *gin.Context) {
	kind := ResourceKind(c.Query("kind"))
	definitions, err := s.List(kind)
	if err != nil {
		respondServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, resourceListResponse{Resources: definitions})
}

func (s *Service) handleCreate(c *gin.Context) {
	var req resourceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return
	}
	definition, err := s.Create(req.input())
	if err != nil {
		respondServiceError(c, err)
		return
	}
	c.JSON(http.StatusCreated, definition)
}

func (s *Service) handleGet(c *gin.Context) {
	id, ok := resourcePathID(c)
	if !ok {
		return
	}
	definition, err := s.Get(id)
	if err != nil {
		respondServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, definition)
}

func (s *Service) handleUpdate(c *gin.Context) {
	id, ok := resourcePathID(c)
	if !ok {
		return
	}
	var req resourceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return
	}
	definition, err := s.Update(id, req.input())
	if err != nil {
		respondServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, definition)
}

func (s *Service) handleDelete(c *gin.Context) {
	id, ok := resourcePathID(c)
	if !ok {
		return
	}
	if err := s.Delete(id); err != nil {
		respondServiceError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (s *Service) handleListProjectResources(c *gin.Context) {
	projectID, ok := resourcePathID(c)
	if !ok {
		return
	}
	definitions, err := s.ListProjectResources(projectID)
	if err != nil {
		respondServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, resourceListResponse{Resources: definitions})
}

func (s *Service) handleAssociate(c *gin.Context) {
	projectID, ok := resourcePathID(c)
	if !ok {
		return
	}
	var req resourceReference
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return
	}
	if err := s.AssociateProject(projectID, req.ResourceID); err != nil {
		respondServiceError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (s *Service) handleDisassociate(c *gin.Context) {
	projectID, ok := resourcePathID(c)
	if !ok {
		return
	}
	var req resourceReference
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return
	}
	if err := s.DisassociateProject(projectID, req.ResourceID); err != nil {
		respondServiceError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (s *Service) handleListSessionPrompts(c *gin.Context) {
	projectID, sessionID, ok := sessionPath(c)
	if !ok {
		return
	}
	definitions, err := s.ListSessionPrompts(projectID, sessionID)
	if err != nil {
		respondServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, promptListResponse{Prompts: definitions})
}

func (s *Service) handleSelectSessionPrompt(c *gin.Context) {
	projectID, sessionID, ok := sessionPath(c)
	if !ok {
		return
	}
	var req resourceReference
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return
	}
	if err := s.SelectSessionPrompt(projectID, sessionID, req.ResourceID); err != nil {
		respondServiceError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (s *Service) handleRemoveSessionPrompt(c *gin.Context) {
	projectID, sessionID, ok := sessionPath(c)
	if !ok {
		return
	}
	var req resourceReference
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return
	}
	if err := s.RemoveSessionPrompt(projectID, sessionID, req.ResourceID); err != nil {
		respondServiceError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// resourcePathID parses the :id path parameter, writing a 400 on malformed
// input.
func resourcePathID(c *gin.Context) (uint, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return 0, false
	}
	return uint(id), true
}

// sessionPath parses the :id and :sessionId path parameters, writing a 400 on
// malformed input.
func sessionPath(c *gin.Context) (uint, string, bool) {
	projectID, ok := resourcePathID(c)
	if !ok {
		return 0, "", false
	}
	sessionID := c.Param("sessionId")
	if sessionID == "" {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return 0, "", false
	}
	return projectID, sessionID, true
}
