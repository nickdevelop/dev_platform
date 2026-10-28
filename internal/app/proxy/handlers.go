package proxy

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

func (s *Service) RegisterRoutes(r gin.IRouter) {
	proxies := r.Group("/proxies")
	{
		proxies.GET("", s.handleList)
		proxies.POST("", s.handleCreate)
		proxies.GET("/browse-directories", s.handleBrowseDirectories)
		proxies.GET("/:id", s.handleGet)
		proxies.PUT("/:id", s.handleUpdate)
		proxies.DELETE("/:id", s.handleDelete)
		proxies.POST("/:id/toggle", s.handleToggle)
	}
}

type errorResponse struct {
	Error string `json:"error"`
}

func respondError(c *gin.Context, status int, code string) {
	c.AbortWithStatusJSON(status, errorResponse{Error: code})
}

func parseProxyID(c *gin.Context) (uint, bool) {
	val, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || val == 0 {
		respondError(c, http.StatusBadRequest, "invalid_proxy_id")
		return 0, false
	}
	return uint(val), true
}

func (s *Service) handleList(c *gin.Context) {
	list, err := s.List()
	if err != nil {
		respondError(c, http.StatusInternalServerError, "server_error")
		return
	}
	c.JSON(http.StatusOK, gin.H{"proxies": list})
}

func (s *Service) handleCreate(c *gin.Context) {
	var req CreateProxyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return
	}

	p, err := s.Create(req)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidProxyName):
			respondError(c, http.StatusBadRequest, "invalid_proxy_name")
		case errors.Is(err, ErrInvalidProxyAlias):
			respondError(c, http.StatusBadRequest, "invalid_proxy_alias")
		case errors.Is(err, ErrInvalidProxyType):
			respondError(c, http.StatusBadRequest, "invalid_proxy_type")
		case errors.Is(err, ErrInvalidProxyTarget):
			respondError(c, http.StatusBadRequest, "invalid_proxy_target")
		case errors.Is(err, ErrDuplicateProxyAlias):
			respondError(c, http.StatusConflict, "duplicate_proxy_alias")
		default:
			respondError(c, http.StatusInternalServerError, "server_error")
		}
		return
	}

	c.JSON(http.StatusCreated, p)
}

func (s *Service) handleGet(c *gin.Context) {
	id, ok := parseProxyID(c)
	if !ok {
		return
	}

	p, err := s.Get(id)
	if err != nil {
		if errors.Is(err, ErrProxyNotFound) {
			respondError(c, http.StatusNotFound, "proxy_not_found")
			return
		}
		respondError(c, http.StatusInternalServerError, "server_error")
		return
	}

	c.JSON(http.StatusOK, p)
}

func (s *Service) handleUpdate(c *gin.Context) {
	id, ok := parseProxyID(c)
	if !ok {
		return
	}

	var req UpdateProxyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return
	}

	p, err := s.Update(id, req)
	if err != nil {
		switch {
		case errors.Is(err, ErrProxyNotFound):
			respondError(c, http.StatusNotFound, "proxy_not_found")
		case errors.Is(err, ErrInvalidProxyName):
			respondError(c, http.StatusBadRequest, "invalid_proxy_name")
		case errors.Is(err, ErrInvalidProxyAlias):
			respondError(c, http.StatusBadRequest, "invalid_proxy_alias")
		case errors.Is(err, ErrInvalidProxyType):
			respondError(c, http.StatusBadRequest, "invalid_proxy_type")
		case errors.Is(err, ErrInvalidProxyTarget):
			respondError(c, http.StatusBadRequest, "invalid_proxy_target")
		case errors.Is(err, ErrDuplicateProxyAlias):
			respondError(c, http.StatusConflict, "duplicate_proxy_alias")
		default:
			respondError(c, http.StatusInternalServerError, "server_error")
		}
		return
	}

	c.JSON(http.StatusOK, p)
}

func (s *Service) handleDelete(c *gin.Context) {
	id, ok := parseProxyID(c)
	if !ok {
		return
	}

	if err := s.Delete(id); err != nil {
		if errors.Is(err, ErrProxyNotFound) {
			respondError(c, http.StatusNotFound, "proxy_not_found")
			return
		}
		respondError(c, http.StatusInternalServerError, "server_error")
		return
	}

	c.Status(http.StatusNoContent)
}

func (s *Service) handleToggle(c *gin.Context) {
	id, ok := parseProxyID(c)
	if !ok {
		return
	}

	p, err := s.Toggle(id)
	if err != nil {
		if errors.Is(err, ErrProxyNotFound) {
			respondError(c, http.StatusNotFound, "proxy_not_found")
			return
		}
		respondError(c, http.StatusInternalServerError, "server_error")
		return
	}

	c.JSON(http.StatusOK, p)
}

func (s *Service) handleBrowseDirectories(c *gin.Context) {
	path := c.Query("path")
	res, err := s.BrowseDirectories(path)
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid_target_directory")
		return
	}
	c.JSON(http.StatusOK, res)
}

