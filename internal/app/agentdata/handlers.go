package agentdata

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// Routes the transcript data management API. All routes operate strictly on
// the transcript tables of the backend bound at the composition root; no
// backend identifier appears in this file.
const (
	pageSizeUpperBound = 100
	pageSizeDefault    = 20
)

// errorResponse is the JSON error envelope for the management API.
type errorResponse struct {
	Error string `json:"error"`
}

func respondError(c *gin.Context, status int, code string) {
	c.AbortWithStatusJSON(status, errorResponse{Error: code})
}

// sessionSummaryDTO is one transcript session as the API lists it.
type sessionSummaryDTO struct {
	SessionID    string    `json:"sessionId"`
	Title        string    `json:"title"`
	Directory    string    `json:"directory"`
	Model        string    `json:"model,omitempty"`
	MessageCount int64     `json:"messageCount"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

type sessionListResponse struct {
	Sessions []sessionSummaryDTO `json:"sessions"`
	Total    int64               `json:"total"`
}

type clearResponse struct {
	Deleted int64 `json:"deleted"`
}

type deleteResponse struct {
	Deleted bool `json:"deleted"`
}

// RegisterRoutes mounts the management endpoints under the protected group.
func (s *Service) RegisterRoutes(r gin.IRouter) {
	r.GET("/settings/agent-data/sessions", s.handleListSessions)
	r.DELETE("/settings/agent-data/sessions/:sessionId", s.handleDeleteSession)
	r.DELETE("/settings/agent-data/sessions", s.handleClearSessions)
}

func (s *Service) handleListSessions(c *gin.Context) {
	limit := pageSizeDefault
	if raw := c.Query("pageSize"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 || parsed > pageSizeUpperBound {
			respondError(c, http.StatusBadRequest, "invalid_request")
			return
		}
		limit = parsed
	}
	offset := 0
	if raw := c.Query("page"); raw != "" && raw != "1" {
		page, err := strconv.Atoi(raw)
		if err != nil || page < 1 {
			respondError(c, http.StatusBadRequest, "invalid_request")
			return
		}
		offset = (page - 1) * limit
	}

	sessions, total, err := s.ListSessions(c.Request.Context(), limit, offset)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "server_error")
		return
	}
	out := make([]sessionSummaryDTO, 0, len(sessions))
	for _, session := range sessions {
		out = append(out, sessionSummaryDTO{
			SessionID:    session.SessionID,
			Title:        session.Title,
			Directory:    session.Directory,
			Model:        session.Model,
			MessageCount: session.MessageCount,
			CreatedAt:    session.CreatedAt,
			UpdatedAt:    session.UpdatedAt,
		})
	}
	c.JSON(http.StatusOK, sessionListResponse{Sessions: out, Total: total})
}

func (s *Service) handleDeleteSession(c *gin.Context) {
	sessionID := c.Param("sessionId")
	if sessionID == "" {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return
	}
	if err := s.DeleteSession(c.Request.Context(), sessionID); err != nil {
		switch {
		case errors.Is(err, ErrSessionBusy):
			respondError(c, http.StatusConflict, "session_busy")
		case errors.Is(err, ErrSessionNotFound):
			respondError(c, http.StatusNotFound, "session_not_found")
		default:
			respondError(c, http.StatusInternalServerError, "server_error")
		}
		return
	}
	c.JSON(http.StatusOK, deleteResponse{Deleted: true})
}

func (s *Service) handleClearSessions(c *gin.Context) {
	deleted, err := s.ClearSessions(c.Request.Context())
	if err != nil {
		respondError(c, http.StatusInternalServerError, "server_error")
		return
	}
	c.JSON(http.StatusOK, clearResponse{Deleted: deleted})
}
