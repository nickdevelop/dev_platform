package prompts

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

func (s *Service) RegisterRoutes(r gin.IRouter) {
	prompts := r.Group("/prompts")
	{
		prompts.GET("", s.handleList)
		prompts.POST("", s.handleCreate)
		prompts.GET("/:id", s.handleGet)
		prompts.PUT("/:id", s.handleUpdate)
		prompts.DELETE("/:id", s.handleDelete)
	}
}

type errorResponse struct {
	Error string `json:"error"`
}

func respondError(c *gin.Context, status int, code string) {
	c.AbortWithStatusJSON(status, errorResponse{Error: code})
}

func parsePromptID(c *gin.Context) (uint, bool) {
	val, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || val == 0 {
		respondError(c, http.StatusBadRequest, "invalid_prompt_id")
		return 0, false
	}
	return uint(val), true
}

func (s *Service) handleList(c *gin.Context) {
	keyword := c.Query("q")
	list, err := s.List(keyword)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "server_error")
		return
	}
	c.JSON(http.StatusOK, gin.H{"prompts": list})
}

func (s *Service) handleCreate(c *gin.Context) {
	var req CreatePromptRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return
	}

	prompt, err := s.Create(req)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidPromptName):
			respondError(c, http.StatusBadRequest, "invalid_prompt_name")
		case errors.Is(err, ErrInvalidPromptContent):
			respondError(c, http.StatusBadRequest, "invalid_prompt_content")
		case errors.Is(err, ErrDuplicatePromptName):
			respondError(c, http.StatusConflict, "duplicate_prompt_name")
		default:
			respondError(c, http.StatusInternalServerError, "server_error")
		}
		return
	}

	c.JSON(http.StatusCreated, prompt)
}

func (s *Service) handleGet(c *gin.Context) {
	id, ok := parsePromptID(c)
	if !ok {
		return
	}

	prompt, err := s.Get(id)
	if err != nil {
		if errors.Is(err, ErrPromptNotFound) {
			respondError(c, http.StatusNotFound, "prompt_not_found")
			return
		}
		respondError(c, http.StatusInternalServerError, "server_error")
		return
	}

	c.JSON(http.StatusOK, prompt)
}

func (s *Service) handleUpdate(c *gin.Context) {
	id, ok := parsePromptID(c)
	if !ok {
		return
	}

	var req UpdatePromptRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return
	}

	prompt, err := s.Update(id, req)
	if err != nil {
		switch {
		case errors.Is(err, ErrPromptNotFound):
			respondError(c, http.StatusNotFound, "prompt_not_found")
		case errors.Is(err, ErrInvalidPromptName):
			respondError(c, http.StatusBadRequest, "invalid_prompt_name")
		case errors.Is(err, ErrInvalidPromptContent):
			respondError(c, http.StatusBadRequest, "invalid_prompt_content")
		case errors.Is(err, ErrDuplicatePromptName):
			respondError(c, http.StatusConflict, "duplicate_prompt_name")
		default:
			respondError(c, http.StatusInternalServerError, "server_error")
		}
		return
	}

	c.JSON(http.StatusOK, prompt)
}

func (s *Service) handleDelete(c *gin.Context) {
	id, ok := parsePromptID(c)
	if !ok {
		return
	}

	if err := s.Delete(id); err != nil {
		if errors.Is(err, ErrPromptNotFound) {
			respondError(c, http.StatusNotFound, "prompt_not_found")
			return
		}
		respondError(c, http.StatusInternalServerError, "server_error")
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true})
}
