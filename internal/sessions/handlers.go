package sessions

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"devplatform/internal/agents/instances"
)

// RegisterRoutes registers the project-scoped session routes under r. The
// caller must protect r with session authentication (auth.RequireSession);
// CSRF enforcement is installed globally by the platform router.
func (s *Service) RegisterRoutes(r gin.IRouter) {
	r.GET("/agent-backends", s.handleBackends)

	r.GET("/projects/:id/sessions", s.handleList)
	r.POST("/projects/:id/sessions", s.handleCreate)

	// Discovery is instance-scoped: the adapter needs to know which instance
	// (and therefore which working directory) to ask.
	r.GET("/projects/:id/instances/:instanceId/sessions", s.handleDiscover)

	r.POST("/projects/:id/sessions/:sessionId/continue", s.handleContinue)
	r.POST("/projects/:id/sessions/:sessionId/messages", s.handleSend)
	r.POST("/projects/:id/sessions/:sessionId/stop", s.handleStop)
	r.GET("/projects/:id/sessions/:sessionId/events", s.handleEvents)
}

// errorResponse is the JSON error envelope for session API failures.
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
	case errors.Is(err, ErrSessionNotFound):
		respondError(c, http.StatusNotFound, "session_not_found")
	case errors.Is(err, instances.ErrProjectNotFound):
		respondError(c, http.StatusNotFound, "project_not_found")
	case errors.Is(err, instances.ErrInstanceNotFound):
		respondError(c, http.StatusNotFound, "instance_not_found")
	case errors.Is(err, instances.ErrInstanceNotRunning):
		respondError(c, http.StatusConflict, "instance_not_running")
	case errors.Is(err, ErrUnsupportedCapability):
		respondError(c, http.StatusConflict, "unsupported_capability")
	case errors.Is(err, ErrBackendFailed), errors.Is(err, ErrInvalidSession):
		respondError(c, http.StatusBadGateway, "agent_operation_failed")
	case errors.Is(err, ErrInvalidRequest):
		respondError(c, http.StatusBadRequest, "invalid_request")
	case errors.Is(err, instances.ErrUnknownBackend):
		respondError(c, http.StatusBadRequest, "unknown_backend")
	default:
		respondError(c, http.StatusInternalServerError, "server_error")
	}
}

// sessionListResponse is the JSON body of GET /api/projects/{id}/sessions.
type sessionListResponse struct {
	Sessions []Session `json:"sessions"`
}

// backendListResponse is the JSON body of GET /api/agent-backends.
type backendListResponse struct {
	Backends []BackendCapabilities `json:"backends"`
}

// createSessionRequest is the JSON body of POST /api/projects/{id}/sessions.
type createSessionRequest struct {
	InstanceID string `json:"instanceId"`
	Title      string `json:"title"`
	Model      string `json:"model"`
}

// sendMessageRequest is the JSON body of a session message.
type sendMessageRequest struct {
	Text  string `json:"text"`
	Model string `json:"model"`
}

// handleBackends returns the registered backends with their declared
// capabilities so the workbench can hide unsupported controls.
func (s *Service) handleBackends(c *gin.Context) {
	c.JSON(http.StatusOK, backendListResponse{Backends: s.Backends()})
}

// handleList returns the project's indexed sessions.
func (s *Service) handleList(c *gin.Context) {
	id, ok := projectID(c)
	if !ok {
		return
	}
	list, err := s.List(id)
	if err != nil {
		respondServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, sessionListResponse{Sessions: list})
}

// handleCreate starts a new session on an instance in the project.
func (s *Service) handleCreate(c *gin.Context) {
	id, ok := projectID(c)
	if !ok {
		return
	}
	var req createSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return
	}
	if req.InstanceID == "" {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return
	}
	session, err := s.Create(c.Request.Context(), id, req.InstanceID, req.Title, req.Model)
	if err != nil {
		respondServiceError(c, err)
		return
	}
	c.JSON(http.StatusCreated, session)
}

// handleDiscover lists and indexes an instance's native sessions.
func (s *Service) handleDiscover(c *gin.Context) {
	id, ok := projectID(c)
	if !ok {
		return
	}
	instanceID := c.Param("instanceId")
	if instanceID == "" {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return
	}
	list, err := s.Discover(c.Request.Context(), id, instanceID)
	if err != nil {
		respondServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, sessionListResponse{Sessions: list})
}

// handleContinue re-attaches to an indexed native session.
func (s *Service) handleContinue(c *gin.Context) {
	id, ok := projectID(c)
	if !ok {
		return
	}
	sessionID := c.Param("sessionId")
	if sessionID == "" {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return
	}
	session, err := s.Continue(c.Request.Context(), id, sessionID)
	if err != nil {
		respondServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, session)
}

// handleSend delivers one user turn to a session.
func (s *Service) handleSend(c *gin.Context) {
	id, ok := projectID(c)
	if !ok {
		return
	}
	sessionID := c.Param("sessionId")
	if sessionID == "" {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return
	}
	var req sendMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return
	}
	session, err := s.Send(c.Request.Context(), id, sessionID, req.Text, req.Model)
	if err != nil {
		respondServiceError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, session)
}

// handleStop stops the session's run. The operation is idempotent.
func (s *Service) handleStop(c *gin.Context) {
	id, ok := projectID(c)
	if !ok {
		return
	}
	sessionID := c.Param("sessionId")
	if sessionID == "" {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return
	}
	session, err := s.Stop(c.Request.Context(), id, sessionID)
	if err != nil {
		respondServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, session)
}

// handleEvents streams the session's normalized events as Server-Sent Events.
func (s *Service) handleEvents(c *gin.Context) {
	id, ok := projectID(c)
	if !ok {
		return
	}
	sessionID := c.Param("sessionId")
	if sessionID == "" {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return
	}

	// Validate project scope and instance state before committing to SSE
	// headers, so failures are still ordinary JSON error responses.
	session, err := s.Get(id, sessionID)
	if err != nil {
		respondServiceError(c, err)
		return
	}
	if session.Status != SessionActive {
		respondError(c, http.StatusConflict, "instance_not_running")
		return
	}
	// Refuse a stream for a backend that does not declare streaming support,
	// consistent with the other capability gates.
	if !session.Capabilities.Streaming {
		respondError(c, http.StatusConflict, "unsupported_capability")
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	// Disable intermediary (for example nginx) response buffering so events
	// reach the browser as they are produced.
	c.Header("X-Accel-Buffering", "no")
	// An SSE response is long-lived and must outlive the server's absolute
	// WriteTimeout; clear the connection's write deadline (the controller
	// unwraps gin's ResponseWriter to the net/http response). Ignore an
	// unsupported result, which only happens for writers with no deadline.
	_ = http.NewResponseController(c.Writer).SetWriteDeadline(time.Time{})
	c.Writer.Flush()

	_ = s.StreamSession(c.Request.Context(), id, sessionID, c.Writer, func() { c.Writer.Flush() })
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
