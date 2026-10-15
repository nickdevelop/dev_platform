package modelconfig

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"devplatform/internal/app/modelprobe"
)

// RegisterRoutes registers the model management routes under r. The caller must
// protect r with session authentication (auth.RequireSession); CSRF enforcement
// is installed globally by the platform router.
func (s *Service) RegisterRoutes(r gin.IRouter) {
	r.GET("/model-services", s.handleList)
	r.POST("/model-services", s.handleCreate)
	r.POST("/model-services/fetch-models", s.handleFetchModels)
	r.PUT("/model-services/:id", s.handleUpdate)
	r.DELETE("/model-services/:id", s.handleDelete)
	r.POST("/model-services/:id/models", s.handleAddModel)
	r.PUT("/models/:id", s.handleUpdateModel)
	r.DELETE("/models/:id", s.handleDeleteModel)
	r.POST("/models/test", s.handleTestModel)
}

// errorResponse is the JSON error envelope for model API failures.
type errorResponse struct {
	Error string `json:"error"`
}

func respondError(c *gin.Context, status int, code string) {
	c.AbortWithStatusJSON(status, errorResponse{Error: code})
}

// respondServiceError maps a service/probe error to an HTTP status and code.
func respondServiceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrServiceNotFound):
		respondError(c, http.StatusNotFound, "service_not_found")
	case errors.Is(err, ErrModelNotFound):
		respondError(c, http.StatusNotFound, "model_not_found")
	case errors.Is(err, ErrModelConflict):
		respondError(c, http.StatusConflict, "model_conflict")
	case errors.Is(err, ErrInvalidService), errors.Is(err, modelprobe.ErrInvalidBaseURL):
		respondError(c, http.StatusBadRequest, "invalid_service")
	case errors.Is(err, ErrInvalidModel):
		respondError(c, http.StatusBadRequest, "invalid_model")
	case errors.Is(err, modelprobe.ErrUpstreamUnreachable):
		respondError(c, http.StatusBadGateway, "upstream_unreachable")
	case errors.Is(err, modelprobe.ErrUpstreamError):
		respondError(c, http.StatusBadGateway, "upstream_error")
	default:
		respondError(c, http.StatusInternalServerError, "server_error")
	}
}

type serviceRequest struct {
	Name    string `json:"name"`
	BaseURL string `json:"baseUrl"`
	APIKey  string `json:"apiKey"`
	Enabled bool   `json:"enabled"`
}

type modelRequest struct {
	ModelID       string `json:"modelId"`
	Name          string `json:"name"`
	SupportsText  bool   `json:"supportsText"`
	SupportsImage bool   `json:"supportsImage"`
	SupportsTools bool   `json:"supportsTools"`
	ContextTokens int    `json:"contextTokens"`
	OutputTokens  int    `json:"outputTokens"`
	Enabled       bool   `json:"enabled"`
}

type fetchModelsRequest struct {
	BaseURL   string `json:"baseUrl"`
	APIKey    string `json:"apiKey"`
	SecretRef string `json:"secretRef"`
}

type testModelRequest struct {
	BaseURL   string `json:"baseUrl"`
	APIKey    string `json:"apiKey"`
	SecretRef string `json:"secretRef"`
	ModelID   string `json:"modelId"`
}

type serviceListResponse struct {
	Services []ServiceDTO `json:"services"`
}

func (s *Service) handleList(c *gin.Context) {
	services, err := s.List()
	if err != nil {
		respondServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, serviceListResponse{Services: services})
}

func (s *Service) handleCreate(c *gin.Context) {
	var request serviceRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return
	}
	created, err := s.Create(ServiceInput{
		Name: request.Name, BaseURL: request.BaseURL, APIKey: request.APIKey, Enabled: request.Enabled,
	})
	if err != nil {
		respondServiceError(c, err)
		return
	}
	c.JSON(http.StatusCreated, created)
}

func (s *Service) handleUpdate(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	var request serviceRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return
	}
	updated, err := s.Update(id, ServiceInput{
		Name: request.Name, BaseURL: request.BaseURL, APIKey: request.APIKey, Enabled: request.Enabled,
	})
	if err != nil {
		respondServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, updated)
}

func (s *Service) handleDelete(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	if err := s.Delete(id); err != nil {
		respondServiceError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (s *Service) handleAddModel(c *gin.Context) {
	serviceID, ok := pathID(c, "id")
	if !ok {
		return
	}
	var request modelRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return
	}
	model, err := s.AddModel(serviceID, modelInput(request))
	if err != nil {
		respondServiceError(c, err)
		return
	}
	c.JSON(http.StatusCreated, model)
}

func (s *Service) handleUpdateModel(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	var request modelRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return
	}
	model, err := s.UpdateModel(id, modelInput(request))
	if err != nil {
		respondServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, model)
}

func (s *Service) handleDeleteModel(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	if err := s.DeleteModel(id); err != nil {
		respondServiceError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (s *Service) handleFetchModels(c *gin.Context) {
	var request fetchModelsRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return
	}
	key, ok := s.resolveRequestKey(c, request.APIKey, request.SecretRef)
	if !ok {
		return
	}
	models, err := modelprobe.FetchModels(c.Request.Context(), request.BaseURL, key)
	if err != nil {
		respondServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"models": models})
}

func (s *Service) handleTestModel(c *gin.Context) {
	var request testModelRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return
	}
	key, ok := s.resolveRequestKey(c, request.APIKey, request.SecretRef)
	if !ok {
		return
	}
	result, err := modelprobe.TestModel(c.Request.Context(), request.BaseURL, key, request.ModelID)
	if err != nil {
		respondServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

// resolveRequestKey picks the plaintext key for a probe request: an explicit
// apiKey wins, otherwise the referenced stored secret is read. It writes a 400
// and reports false when the reference is unknown.
func (s *Service) resolveRequestKey(c *gin.Context, apiKey, secretRef string) (string, bool) {
	if apiKey != "" {
		return apiKey, true
	}
	if secretRef == "" {
		return "", true
	}
	value, err := s.store.Get(context.Background(), secretRef)
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return "", false
	}
	return value, true
}

// pathID parses a positive uint path parameter.
func pathID(c *gin.Context, name string) (uint, bool) {
	value, err := strconv.ParseUint(c.Param(name), 10, 64)
	if err != nil || value == 0 {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return 0, false
	}
	return uint(value), true
}

func modelInput(request modelRequest) ModelInput {
	return ModelInput{
		ModelID:       request.ModelID,
		Name:          request.Name,
		SupportsText:  request.SupportsText,
		SupportsImage: request.SupportsImage,
		SupportsTools: request.SupportsTools,
		ContextTokens: request.ContextTokens,
		OutputTokens:  request.OutputTokens,
		Enabled:       request.Enabled,
	}
}
