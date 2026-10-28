package sessions

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"devplatform/internal/core/agents"
	"devplatform/internal/app/instances"
)

// RegisterRoutes registers the project-scoped session routes under r. The
// caller must protect r with session authentication (auth.RequireSession);
// CSRF enforcement is installed globally by the platform router.
func (s *Service) RegisterRoutes(r gin.IRouter) {
	r.GET("/agent-backends", s.handleBackends)
	r.GET("/agent-backends/diagnostics", s.handleDiagnostics)

	r.GET("/projects/:id/sessions", s.handleList)
	r.POST("/projects/:id/sessions", s.handleCreate)

	r.POST("/projects/:id/sessions/:sessionId/continue", s.handleContinue)
	r.POST("/projects/:id/sessions/:sessionId/messages", s.handleSend)
	r.POST("/projects/:id/sessions/:sessionId/stop", s.handleStop)
	r.POST("/projects/:id/sessions/:sessionId/interrupt", s.handleInterrupt)
	r.DELETE("/projects/:id/sessions/:sessionId", s.handleDelete)
	r.GET("/projects/:id/sessions/:sessionId/history", s.handleHistory)
	r.GET("/projects/:id/sessions/:sessionId/commands", s.handleCommands)
	r.POST("/projects/:id/sessions/:sessionId/command", s.handleExecuteCommand)
	r.POST("/projects/:id/sessions/:sessionId/compact", s.handleCompactSession)
	r.GET("/projects/:id/sessions/:sessionId/events", s.handleEvents)

	// EventBus multiplexed streaming routes.
	r.GET("/events/:uuid", s.handleBusEvents)
	r.POST("/events/:uuid/subscriptions", s.handleBusSubscribe)
	r.DELETE("/events/:uuid/subscriptions/:sessionId", s.handleBusUnsubscribe)
	r.GET("/events/:uuid/subscriptions", s.handleBusListSubscriptions)
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
	Total    int       `json:"total"`
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
	Backend agents.BackendID        `json:"backend"`
	Title   string                  `json:"title,omitempty"`
	Message string                  `json:"message,omitempty"`
	Model   string                  `json:"model,omitempty"`
	Images  []agents.ImageAttachment `json:"images,omitempty"`
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
	Text   string                  `json:"text"`
	Model  string                  `json:"model"`
	Images []agents.ImageAttachment `json:"images,omitempty"`
}

const (
	maxImageSizeBytes   = 5 * 1024 * 1024 // 5MB
	maxImagesPerMessage = 10
)

func validateImages(images []agents.ImageAttachment) error {
	if len(images) > maxImagesPerMessage {
		return errors.New("too many images")
	}
	validTypes := map[string]bool{
		"image/jpeg": true,
		"image/png":  true,
		"image/webp": true,
		"image/gif":  true,
	}
	for _, img := range images {
		if !validTypes[img.MediaType] {
			return errors.New("unsupported image media type")
		}
		if img.Size > maxImageSizeBytes || len(img.Data) > maxImageSizeBytes*4/3+1024 {
			return errors.New("image too large")
		}
	}
	return nil
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

// handleList returns the project's indexed sessions. It optionally supports
// pagination via ?limit=N&offset=M and turn state filtering via ?turn=running.
func (s *Service) handleList(c *gin.Context) {
	id, ok := projectID(c)
	if !ok {
		return
	}
	if turnParam := c.Query("turn"); turnParam != "" {
		list, err := s.ListByTurn(id, TurnState(turnParam))
		if err != nil {
			respondServiceError(c, err)
			return
		}
		c.JSON(http.StatusOK, sessionListResponse{Sessions: list, Total: len(list)})
		return
	}
	limitStr := c.Query("limit")
	offsetStr := c.Query("offset")
	if limitStr != "" {
		limit, _ := strconv.Atoi(limitStr)
		offset, _ := strconv.Atoi(offsetStr)
		list, total, err := s.ListPaged(id, limit, offset)
		if err != nil {
			respondServiceError(c, err)
			return
		}
		c.JSON(http.StatusOK, sessionListResponse{Sessions: list, Total: total})
		return
	}
	list, err := s.List(id)
	if err != nil {
		respondServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, sessionListResponse{Sessions: list, Total: len(list)})
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
	if err := validateImages(req.Images); err != nil {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return
	}
	outcome, err := s.Create(c.Request.Context(), id, req.Backend, req.Title, req.Message, req.Model, req.Images)
	if err != nil {
		respondServiceError(c, err)
		return
	}
	c.JSON(http.StatusCreated, createSessionResponse{Session: outcome.Session, MessageError: outcome.MessageError})
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
	if strings.TrimSpace(req.Text) == "" && len(req.Images) == 0 {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return
	}
	if err := validateImages(req.Images); err != nil {
		respondError(c, http.StatusBadRequest, "invalid_image")
		return
	}
	session, err := s.Send(c.Request.Context(), id, sessionID, req.Text, req.Model, req.Images)
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

// commandsResponse is the JSON body of GET /api/projects/:id/sessions/:sessionId/commands.
type commandsResponse struct {
	Commands []agents.AgentCommand `json:"commands"`
}

// handleCommands lists the slash commands and skills supported by the session's backend.
func (s *Service) handleCommands(c *gin.Context) {
	id, ok := projectID(c)
	if !ok {
		return
	}
	sessionID := c.Param("sessionId")
	if sessionID == "" {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return
	}
	commands, err := s.ListCommands(c.Request.Context(), id, sessionID)
	if err != nil {
		respondServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, commandsResponse{Commands: commands})
}

type executeCommandRequest struct {
	Name      string `json:"name"`
	RawInput  string `json:"rawInput"`
	Arguments string `json:"arguments"`
}

type compactSessionRequest struct {
	Instructions string `json:"instructions"`
}

// handleExecuteCommand executes a slash command on the session's agent backend.
func (s *Service) handleExecuteCommand(c *gin.Context) {
	id, ok := projectID(c)
	if !ok {
		return
	}
	sessionID := c.Param("sessionId")
	if sessionID == "" {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return
	}

	var req executeCommandRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return
	}
	if req.Name == "" {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return
	}

	invocation := agents.CommandInvocation{
		Name:      req.Name,
		RawInput:  req.RawInput,
		Arguments: req.Arguments,
	}

	result, err := s.ExecuteCommand(c.Request.Context(), id, sessionID, invocation)
	if err != nil {
		respondServiceError(c, err)
		return
	}

	c.JSON(http.StatusOK, result)
}

// handleCompactSession requests context compaction on the session's agent backend.
func (s *Service) handleCompactSession(c *gin.Context) {
	id, ok := projectID(c)
	if !ok {
		return
	}
	sessionID := c.Param("sessionId")
	if sessionID == "" {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return
	}

	var req compactSessionRequest
	_ = c.ShouldBindJSON(&req)

	if err := s.CompactSession(c.Request.Context(), id, sessionID, req.Instructions); err != nil {
		respondServiceError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true})
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

// busSubscribeRequest is the JSON payload for subscribing a session to an event bus.
type busSubscribeRequest struct {
	ProjectID   uint   `json:"projectId"`
	SessionID   string `json:"sessionId"`
	LastEventID uint64 `json:"lastEventId,omitempty"`
}

// handleBusEvents establishes the multiplexed SSE connection for a client UUID.
func (s *Service) handleBusEvents(c *gin.Context) {
	uuid := c.Param("uuid")
	if uuid == "" {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	_ = http.NewResponseController(c.Writer).SetWriteDeadline(time.Time{})
	c.Writer.Flush()

	_ = s.eventBus.StreamBus(c.Request.Context(), uuid, c.Writer, func() { c.Writer.Flush() })
}

// handleBusSubscribe registers a session with a client event bus.
func (s *Service) handleBusSubscribe(c *gin.Context) {
	uuid := c.Param("uuid")
	if uuid == "" {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return
	}

	var req busSubscribeRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.ProjectID == 0 || req.SessionID == "" {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return
	}

	snapshot, err := s.eventBus.SubscribeSession(c.Request.Context(), uuid, req.ProjectID, req.SessionID, req.LastEventID)
	if err != nil {
		respondServiceError(c, err)
		return
	}

	c.JSON(http.StatusOK, snapshot)
}

// handleBusUnsubscribe removes a session subscription from an event bus.
func (s *Service) handleBusUnsubscribe(c *gin.Context) {
	uuid := c.Param("uuid")
	sessionID := c.Param("sessionId")
	if uuid == "" || sessionID == "" {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return
	}

	if err := s.eventBus.UnsubscribeSession(uuid, sessionID); err != nil {
		respondServiceError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// handleBusListSubscriptions lists active session subscriptions for an event bus.
func (s *Service) handleBusListSubscriptions(c *gin.Context) {
	uuid := c.Param("uuid")
	if uuid == "" {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return
	}

	subs, err := s.eventBus.ListSubscriptions(uuid)
	if err != nil {
		respondServiceError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"subscriptions": subs})
}

