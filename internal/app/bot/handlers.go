package bot

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// RegisterRoutes mounts the Bot engine and channel routes under r.
// Caller must wrap r in session auth; CSRF is enforced globally.
func (s *Service) RegisterRoutes(r gin.IRouter) {
	botGroup := r.Group("/bot")
	{
		botGroup.GET("/descriptors", s.handleListDescriptors)
		botGroup.GET("/channels", s.handleListChannels)
		botGroup.POST("/channels", s.handleCreateChannel)
		botGroup.GET("/channels/:id", s.handleGetChannel)
		botGroup.PUT("/channels/:id", s.handleUpdateChannel)
		botGroup.DELETE("/channels/:id", s.handleDeleteChannel)
		botGroup.POST("/channels/:id/start", s.handleStartChannel)
		botGroup.POST("/channels/:id/stop", s.handleStopChannel)
		botGroup.POST("/channels/:id/new", s.handleResetChannelContext)
		botGroup.POST("/channels/:id/login/qr", s.handleStartQRLogin)
		botGroup.POST("/channels/:id/login/cancel", s.handleCancelQRLogin)
		botGroup.GET("/channels/:id/login/status", s.handleQRLoginStatus)
	}
}

type errorResponse struct {
	Error string `json:"error"`
}

func respondError(c *gin.Context, status int, code string) {
	c.AbortWithStatusJSON(status, errorResponse{Error: code})
}

func respondServiceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrChannelNotFound):
		respondError(c, http.StatusNotFound, "channel_not_found")
	case errors.Is(err, ErrInvalidChannel):
		respondError(c, http.StatusBadRequest, "invalid_channel")
	case errors.Is(err, ErrUnknownChannelType):
		respondError(c, http.StatusBadRequest, "unknown_channel_type")
	case errors.Is(err, ErrMissingCredentials):
		respondError(c, http.StatusConflict, "missing_credentials")
	case errors.Is(err, ErrUnsupportedLogin):
		respondError(c, http.StatusBadRequest, "unsupported_login")
	default:
		respondError(c, http.StatusInternalServerError, err.Error())
	}
}

func parseChannelID(c *gin.Context) (uint, bool) {
	val, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || val == 0 {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return 0, false
	}
	return uint(val), true
}

func (s *Service) handleListDescriptors(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"descriptors": s.Descriptors()})
}

func (s *Service) handleResetChannelContext(c *gin.Context) {
	id, ok := parseChannelID(c)
	if !ok {
		return
	}
	ch, err := s.ResetChannelContext(c.Request.Context(), id)
	if err != nil {
		respondServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, ch)
}

func (s *Service) handleListChannels(c *gin.Context) {
	list, err := s.ListChannels()
	if err != nil {
		respondServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"channels": list})
}

func (s *Service) handleGetChannel(c *gin.Context) {
	id, ok := parseChannelID(c)
	if !ok {
		return
	}
	ch, err := s.GetChannel(id)
	if err != nil {
		respondServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, ch)
}

func (s *Service) handleCreateChannel(c *gin.Context) {
	var req CreateChannelInput
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return
	}
	ch, err := s.CreateChannel(c.Request.Context(), req)
	if err != nil {
		respondServiceError(c, err)
		return
	}
	c.JSON(http.StatusCreated, ch)
}

func (s *Service) handleUpdateChannel(c *gin.Context) {
	id, ok := parseChannelID(c)
	if !ok {
		return
	}
	var req UpdateChannelInput
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return
	}
	ch, err := s.UpdateChannel(c.Request.Context(), id, req)
	if err != nil {
		respondServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, ch)
}

func (s *Service) handleDeleteChannel(c *gin.Context) {
	id, ok := parseChannelID(c)
	if !ok {
		return
	}
	if err := s.DeleteChannel(c.Request.Context(), id); err != nil {
		respondServiceError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (s *Service) handleStartChannel(c *gin.Context) {
	id, ok := parseChannelID(c)
	if !ok {
		return
	}
	if err := s.StartChannel(id); err != nil {
		respondServiceError(c, err)
		return
	}
	ch, err := s.GetChannel(id)
	if err != nil {
		respondServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, ch)
}

func (s *Service) handleStopChannel(c *gin.Context) {
	id, ok := parseChannelID(c)
	if !ok {
		return
	}
	if err := s.StopChannel(id); err != nil {
		respondServiceError(c, err)
		return
	}
	ch, err := s.GetChannel(id)
	if err != nil {
		respondServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, ch)
}

func (s *Service) handleStartQRLogin(c *gin.Context) {
	id, ok := parseChannelID(c)
	if !ok {
		return
	}
	state, err := s.StartQRLogin(c.Request.Context(), id)
	if err != nil {
		respondServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, state)
}

func (s *Service) handleCancelQRLogin(c *gin.Context) {
	id, ok := parseChannelID(c)
	if !ok {
		return
	}
	if err := s.CancelQRLogin(id); err != nil {
		respondServiceError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (s *Service) handleQRLoginStatus(c *gin.Context) {
	id, ok := parseChannelID(c)
	if !ok {
		return
	}
	state, err := s.QRLoginStatus(id)
	if err != nil {
		respondServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, state)
}
