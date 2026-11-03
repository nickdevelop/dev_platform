package roles

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

func (s *Service) RegisterRoutes(r gin.IRouter) {
	roles := r.Group("/roles")
	{
		roles.GET("", s.handleList)
		roles.POST("", s.handleCreate)
		roles.GET("/:id", s.handleGet)
		roles.PUT("/:id", s.handleUpdate)
		roles.DELETE("/:id", s.handleDelete)
	}
}

type errorResponse struct {
	Error string `json:"error"`
}

func respondError(c *gin.Context, status int, code string) {
	c.AbortWithStatusJSON(status, errorResponse{Error: code})
}

func parseRoleID(c *gin.Context) (uint, bool) {
	val, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || val == 0 {
		respondError(c, http.StatusBadRequest, "invalid_role_id")
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
	c.JSON(http.StatusOK, gin.H{"roles": list})
}

func (s *Service) handleCreate(c *gin.Context) {
	var req CreateRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return
	}

	role, err := s.Create(req)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidRoleName):
			respondError(c, http.StatusBadRequest, "invalid_role_name")
		case errors.Is(err, ErrInvalidRoleModel):
			respondError(c, http.StatusBadRequest, "invalid_role_model")
		default:
			respondError(c, http.StatusInternalServerError, "server_error")
		}
		return
	}

	c.JSON(http.StatusCreated, role)
}

func (s *Service) handleGet(c *gin.Context) {
	id, ok := parseRoleID(c)
	if !ok {
		return
	}

	role, err := s.Get(id)
	if err != nil {
		if errors.Is(err, ErrRoleNotFound) {
			respondError(c, http.StatusNotFound, "role_not_found")
			return
		}
		respondError(c, http.StatusInternalServerError, "server_error")
		return
	}

	c.JSON(http.StatusOK, role)
}

func (s *Service) handleUpdate(c *gin.Context) {
	id, ok := parseRoleID(c)
	if !ok {
		return
	}

	var req UpdateRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return
	}

	role, err := s.Update(id, req)
	if err != nil {
		switch {
		case errors.Is(err, ErrRoleNotFound):
			respondError(c, http.StatusNotFound, "role_not_found")
		case errors.Is(err, ErrInvalidRoleName):
			respondError(c, http.StatusBadRequest, "invalid_role_name")
		case errors.Is(err, ErrInvalidRoleModel):
			respondError(c, http.StatusBadRequest, "invalid_role_model")
		default:
			respondError(c, http.StatusInternalServerError, "server_error")
		}
		return
	}

	c.JSON(http.StatusOK, role)
}

func (s *Service) handleDelete(c *gin.Context) {
	id, ok := parseRoleID(c)
	if !ok {
		return
	}

	if err := s.Delete(id); err != nil {
		if errors.Is(err, ErrRoleNotFound) {
			respondError(c, http.StatusNotFound, "role_not_found")
			return
		}
		respondError(c, http.StatusInternalServerError, "server_error")
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true})
}
