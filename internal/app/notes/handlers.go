package notes

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

func (s *Service) RegisterRoutes(r gin.IRouter) {
	notes := r.Group("/notes")
	{
		notes.GET("", s.handleList)
		notes.POST("", s.handleCreate)
		notes.GET("/:id", s.handleGet)
		notes.PUT("/:id", s.handleUpdate)
		notes.DELETE("/:id", s.handleDelete)
	}
}

type errorResponse struct {
	Error string `json:"error"`
}

func respondError(c *gin.Context, status int, code string) {
	c.AbortWithStatusJSON(status, errorResponse{Error: code})
}

func parseNoteID(c *gin.Context) (uint, bool) {
	val, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || val == 0 {
		respondError(c, http.StatusBadRequest, "invalid_note_id")
		return 0, false
	}
	return uint(val), true
}

func (s *Service) handleList(c *gin.Context) {
	keyword := c.Query("q")
	var projectIDPtr *uint
	if pStr := c.Query("projectId"); pStr != "" {
		if pVal, err := strconv.ParseUint(pStr, 10, 64); err == nil {
			uVal := uint(pVal)
			projectIDPtr = &uVal
		}
	}

	list, err := s.List(keyword, projectIDPtr)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "server_error")
		return
	}
	c.JSON(http.StatusOK, gin.H{"notes": list})
}

func (s *Service) handleCreate(c *gin.Context) {
	var req CreateNoteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid_payload")
		return
	}

	note, err := s.Create(req)
	if err != nil {
		if errors.Is(err, ErrInvalidNoteTitle) {
			respondError(c, http.StatusBadRequest, "invalid_title")
			return
		}
		respondError(c, http.StatusInternalServerError, "server_error")
		return
	}
	c.JSON(http.StatusCreated, gin.H{"note": note})
}

func (s *Service) handleGet(c *gin.Context) {
	id, ok := parseNoteID(c)
	if !ok {
		return
	}

	note, err := s.Get(id)
	if err != nil {
		if errors.Is(err, ErrNoteNotFound) {
			respondError(c, http.StatusNotFound, "note_not_found")
			return
		}
		respondError(c, http.StatusInternalServerError, "server_error")
		return
	}
	c.JSON(http.StatusOK, gin.H{"note": note})
}

func (s *Service) handleUpdate(c *gin.Context) {
	id, ok := parseNoteID(c)
	if !ok {
		return
	}

	var req UpdateNoteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid_payload")
		return
	}

	note, err := s.Update(id, req)
	if err != nil {
		if errors.Is(err, ErrNoteNotFound) {
			respondError(c, http.StatusNotFound, "note_not_found")
			return
		}
		if errors.Is(err, ErrInvalidNoteTitle) {
			respondError(c, http.StatusBadRequest, "invalid_title")
			return
		}
		respondError(c, http.StatusInternalServerError, "server_error")
		return
	}
	c.JSON(http.StatusOK, gin.H{"note": note})
}

func (s *Service) handleDelete(c *gin.Context) {
	id, ok := parseNoteID(c)
	if !ok {
		return
	}

	if err := s.Delete(id); err != nil {
		if errors.Is(err, ErrNoteNotFound) {
			respondError(c, http.StatusNotFound, "note_not_found")
			return
		}
		respondError(c, http.StatusInternalServerError, "server_error")
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
