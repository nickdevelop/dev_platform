package sessions

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// RegisterOpencodeRoutes registers the opencode-only interaction routes:
// tool approval, question forms and the raw event stream. Sending messages and
// interrupting a turn are generic and live in RegisterRoutes.
func (s *Service) RegisterOpencodeRoutes(r gin.IRouter) {
	r.GET("/projects/:id/opencode/sessions/:sessionId/permission", s.handleOpencodePermissionList)
	r.POST("/projects/:id/opencode/sessions/:sessionId/permission/:requestId/reply", s.handleOpencodePermissionReply)
	r.GET("/projects/:id/opencode/sessions/:sessionId/form", s.handleOpencodeFormList)
	r.POST("/projects/:id/opencode/sessions/:sessionId/form/:formId/reply", s.handleOpencodeFormReply)
	r.GET("/projects/:id/opencode/sessions/:sessionId/raw-events", s.handleOpencodeRawEvents)
}

type permissionListResponse struct {
	Permissions []PermissionRequest `json:"permissions"`
}

type permissionReplyRequest struct {
	Decision string `json:"decision"`
	Message  string `json:"message"`
}

type formListResponse struct {
	Forms []FormInfo `json:"forms"`
}

type formReplyRequest struct {
	Answer map[string]any `json:"answer"`
}

func (s *Service) handleOpencodePermissionList(c *gin.Context) {
	id, ok := projectID(c)
	if !ok {
		return
	}
	sessionID := c.Param("sessionId")
	if sessionID == "" {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return
	}
	list, err := s.ListOpencodePermissions(c.Request.Context(), id, sessionID)
	if err != nil {
		respondServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, permissionListResponse{Permissions: list})
}

func (s *Service) handleOpencodePermissionReply(c *gin.Context) {
	id, ok := projectID(c)
	if !ok {
		return
	}
	sessionID := c.Param("sessionId")
	requestID := c.Param("requestId")
	if sessionID == "" || requestID == "" {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return
	}
	var req permissionReplyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return
	}
	if err := s.ReplyOpencodePermission(c.Request.Context(), id, sessionID, requestID, req.Decision, req.Message); err != nil {
		respondServiceError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (s *Service) handleOpencodeFormList(c *gin.Context) {
	id, ok := projectID(c)
	if !ok {
		return
	}
	sessionID := c.Param("sessionId")
	if sessionID == "" {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return
	}
	list, err := s.ListOpencodeForms(c.Request.Context(), id, sessionID)
	if err != nil {
		respondServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, formListResponse{Forms: list})
}

func (s *Service) handleOpencodeFormReply(c *gin.Context) {
	id, ok := projectID(c)
	if !ok {
		return
	}
	sessionID := c.Param("sessionId")
	formID := c.Param("formId")
	if sessionID == "" || formID == "" {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return
	}
	var req formReplyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return
	}
	if err := s.ReplyOpencodeForm(c.Request.Context(), id, sessionID, formID, req.Answer); err != nil {
		respondServiceError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// handleOpencodeRawEvents proxies the shared server's untouched SSE bytes.
// Authorization reuses the project-scoped session check; no normalization or
// session filtering happens here.
func (s *Service) handleOpencodeRawEvents(c *gin.Context) {
	id, ok := projectID(c)
	if !ok {
		return
	}
	sessionID := c.Param("sessionId")
	if sessionID == "" {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return
	}

	session, err := s.Get(id, sessionID)
	if err != nil {
		respondServiceError(c, err)
		return
	}
	if session.Status != SessionActive {
		respondError(c, http.StatusConflict, "instance_not_running")
		return
	}
	if !session.Capabilities.Streaming {
		respondError(c, http.StatusConflict, "unsupported_capability")
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	_ = http.NewResponseController(c.Writer).SetWriteDeadline(time.Time{})
	c.Writer.Flush()

	_ = s.StreamOpencodeRaw(c.Request.Context(), id, sessionID, c.Writer, func() { c.Writer.Flush() })
}
