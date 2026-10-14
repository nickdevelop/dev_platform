package sessions

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"devplatform/internal/agents"
	"devplatform/internal/agents/instances"
)

// RegisterRoutes registers the project-scoped session routes under r. The
// caller must protect r with session authentication (auth.RequireSession);
// CSRF enforcement is installed globally by the platform router.
func (s *Service) RegisterRoutes(r gin.IRouter) {
	r.GET("/agent-backends", s.handleBackends)
	r.GET("/agent-backends/diagnostics", s.handleDiagnostics)

	r.GET("/projects/:id/sessions", s.handleList)
	r.POST("/projects/:id/sessions", s.handleCreate)

	// Discovery is project-level: the caller names the backend and the platform
	// picks (or starts) the instance that serves it, so no user-visible
	// instance id is needed.
	r.GET("/projects/:id/sessions/discover", s.handleDiscoverProject)

	// Discovery is instance-scoped: the adapter needs to know which instance
	// (and therefore which working directory) to ask.
	r.GET("/projects/:id/instances/:instanceId/sessions", s.handleDiscover)

	r.POST("/projects/:id/sessions/:sessionId/continue", s.handleContinue)
	r.POST("/projects/:id/sessions/:sessionId/messages", s.handleSend)
	r.POST("/projects/:id/sessions/:sessionId/stop", s.handleStop)
	r.POST("/projects/:id/sessions/:sessionId/interrupt", s.handleInterrupt)
	r.DELETE("/projects/:id/sessions/:sessionId", s.handleDelete)
	r.GET("/projects/:id/sessions/:sessionId/history", s.handleHistory)
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
	case errors.Is(err, instances.ErrBackendDisabled):
		respondError(c, http.StatusConflict, "backend_disabled")
	case errors.Is(err, ErrAgentDisabled):
		respondError(c, http.StatusConflict, "agent_disabled")
	case errors.Is(err, ErrServiceNotRunning):
		// The machine-level service is down. The workbench turns this into a
		// message naming the one place the service can be started.
		respondError(c, http.StatusConflict, "service_not_running")
	case errors.Is(err, instances.ErrStartFailed):
		respondError(c, http.StatusBadGateway, "instance_start_failed")
	case errors.Is(err, ErrUnsupportedCapability):
		respondError(c, http.StatusConflict, "unsupported_capability")
	case errors.Is(err, ErrPermissionExpired):
		respondError(c, http.StatusConflict, "permission_expired")
	case errors.Is(err, ErrFormExpired):
		respondError(c, http.StatusConflict, "form_expired")
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

// backendDiagnosticsResponse is the JSON body of
// GET /api/agent-backends/diagnostics.
type backendDiagnosticsResponse struct {
	Diagnostics []agents.BackendDiagnostics `json:"diagnostics"`
}

// createSessionRequest is the JSON body of POST /api/projects/{id}/sessions.
// The backend selects the instance: the server reuses a running one of that
// backend in the project or starts one. instanceId is no longer accepted.
// Message and model are optional: when present, the session is created on the
// model and the message is admitted as its first turn in the same request.
type createSessionRequest struct {
	Backend agents.BackendID `json:"backend"`
	Title   string           `json:"title,omitempty"`
	Message string           `json:"message,omitempty"`
	Model   string           `json:"model,omitempty"`
}

// createSessionResponse is the JSON body of a successful create. MessageError
// carries the code of a failed optional first message; the session itself is
// always live.
type createSessionResponse struct {
	Session      Session `json:"session"`
	MessageError string  `json:"messageError,omitempty"`
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

// handleDiagnostics returns the per-backend startup diagnostics (executable,
// CLI/API version, API reachability) with redacted, actionable messages so
// the workbench can show why a backend is or is not usable.
func (s *Service) handleDiagnostics(c *gin.Context) {
	report := s.Diagnostics(c.Request.Context())
	if report == nil {
		report = []agents.BackendDiagnostics{}
	}
	c.JSON(http.StatusOK, backendDiagnosticsResponse{Diagnostics: report})
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

// handleCreate starts a new session for the requested backend in the project,
// reusing or starting an instance as needed. An optional first message is
// admitted on the created session in the same request; its failure is reported
// in the response while the session itself is still created.
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
	outcome, err := s.Create(c.Request.Context(), id, req.Backend, req.Title, req.Message, req.Model)
	if err != nil {
		respondServiceError(c, err)
		return
	}
	c.JSON(http.StatusCreated, createSessionResponse{Session: outcome.Session, MessageError: outcome.MessageError})
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

// handleInterrupt ends the session's current turn. The operation is idempotent.
func (s *Service) handleInterrupt(c *gin.Context) {
	id, ok := projectID(c)
	if !ok {
		return
	}
	sessionID := c.Param("sessionId")
	if sessionID == "" {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return
	}
	session, err := s.Interrupt(c.Request.Context(), id, sessionID)
	if err != nil {
		respondServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, session)
}

// handleContinue re-attaches to an indexed native session.
// handleDiscoverProject indexes a project backend's native sessions, starting
// an instance when none is running.
func (s *Service) handleDiscoverProject(c *gin.Context) {
	id, ok := projectID(c)
	if !ok {
		return
	}
	backend := agents.BackendID(c.Query("backend"))
	if backend == "" {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return
	}
	list, err := s.DiscoverProject(c.Request.Context(), id, backend)
	if err != nil {
		respondServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, sessionListResponse{Sessions: list})
}

// handleHistory serves one session's replayed prior turns. History is an
// optional capability: a backend without it answers 409 rather than an empty
// history that would look like a session with no past.
func (s *Service) handleHistory(c *gin.Context) {
	id, ok := projectID(c)
	if !ok {
		return
	}
	sessionID := c.Param("sessionId")
	if sessionID == "" {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return
	}
	history, err := s.History(c.Request.Context(), id, sessionID)
	if err != nil {
		respondServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, history)
}

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

// handleDelete removes a session from the platform's index. The agent-native
// session is untouched, so a later discovery may index it again.
func (s *Service) handleDelete(c *gin.Context) {
	id, ok := projectID(c)
	if !ok {
		return
	}
	sessionID := c.Param("sessionId")
	if sessionID == "" {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return
	}
	if err := s.Delete(id, sessionID); err != nil {
		respondServiceError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
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

	// A browser-native EventSource reconnect carries the last applied frame
	// id in the Last-Event-ID header; the workbench's manual re-dial opens a
	// fresh EventSource, which cannot carry that header, so it passes the
	// same value as a query parameter. Zero means the client is joining.
	_ = s.StreamSession(c.Request.Context(), id, sessionID, resumeSequence(c), c.Writer, func() { c.Writer.Flush() })
}

// resumeSequence extracts the replay position of a resuming SSE client from
// the Last-Event-ID header first and the lastEventId query otherwise. A
// missing or malformed value means the client is joining, not resuming.
func resumeSequence(c *gin.Context) uint64 {
	raw := c.GetHeader("Last-Event-ID")
	if raw == "" {
		raw = c.Query("lastEventId")
	}
	v, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return 0
	}
	return v
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
