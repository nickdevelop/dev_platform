package events

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

type errorResponse struct {
	Error string `json:"error"`
}

func respondError(c *gin.Context, status int, code string) {
	c.AbortWithStatusJSON(status, errorResponse{Error: code})
}

// Service wraps the event Manager and registers unified HTTP gateway routes.
type Service struct {
	Manager *Manager
}

// NewService creates a new events gateway service.
func NewService(manager *Manager) *Service {
	if manager == nil {
		manager = NewManager()
	}
	return &Service{
		Manager: manager,
	}
}

// RegisterRoutes registers the unified event gateway routes on the router.
// Route layout:
//
//	GET    /api/events/bus/:uuid                           - Server-Sent Events stream
//	POST   /api/events/bus/:uuid/subscriptions             - Add subscription (and get optional snapshot)
//	DELETE /api/events/bus/:uuid/subscriptions/:channel/:id - Remove subscription
//	GET    /api/events/bus/:uuid/subscriptions             - List active subscriptions
//	GET    /api/events/diagnostics                         - Per-bus buffer/drop diagnostics
func (s *Service) RegisterRoutes(r gin.IRouter) {
	busGroup := r.Group("/events/bus/:uuid")
	{
		busGroup.GET("", s.handleBusStream)
		busGroup.POST("/subscriptions", s.handleBusSubscribe)
		busGroup.DELETE("/subscriptions/:channel/:targetId", s.handleBusUnsubscribe)
		busGroup.GET("/subscriptions", s.handleBusListSubscriptions)
	}
	r.GET("/events/diagnostics", s.handleDiagnostics)
}

func (s *Service) handleBusStream(c *gin.Context) {
	uuid := c.Param("uuid")
	if uuid == "" {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	responseController := http.NewResponseController(c.Writer)
	_ = responseController.SetWriteDeadline(time.Time{})
	c.Writer.Flush()

	// StreamBus applies a progressive deadline before every frame so a stalled
	// client TCP connection terminates the loop (allowing the client to
	// reconnect and replay) instead of blocking it while the buffer overflows.
	_ = s.Manager.StreamBus(
		c.Request.Context(),
		uuid,
		c.Writer,
		func() { c.Writer.Flush() },
		func(deadline time.Time) error { return responseController.SetWriteDeadline(deadline) },
	)
}

func (s *Service) handleDiagnostics(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"buses": s.Manager.Diagnostics(),
	})
}

func (s *Service) handleBusSubscribe(c *gin.Context) {
	uuid := c.Param("uuid")
	if uuid == "" {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return
	}

	var req SubscribeRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Channel == "" || req.TargetID == "" {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return
	}

	res, err := s.Manager.Subscribe(c.Request.Context(), uuid, req)
	if err != nil {
		if errors.Is(err, ErrInvalidRequest) {
			respondError(c, http.StatusBadRequest, "invalid_request")
			return
		}
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}

	c.JSON(http.StatusOK, res)
}

func (s *Service) handleBusUnsubscribe(c *gin.Context) {
	uuid := c.Param("uuid")
	channel := c.Param("channel")
	targetID := c.Param("targetId")
	if targetID == "" {
		targetID = c.Param("targetID")
	}

	if uuid == "" || channel == "" || targetID == "" {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return
	}

	if err := s.Manager.Unsubscribe(c.Request.Context(), uuid, channel, targetID); err != nil {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (s *Service) handleBusListSubscriptions(c *gin.Context) {
	uuid := c.Param("uuid")
	if uuid == "" {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return
	}

	subs, err := s.Manager.ListSubscriptions(uuid)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "server_error")
		return
	}

	c.JSON(http.StatusOK, gin.H{"subscriptions": subs})
}
