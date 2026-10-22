package chat

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"devplatform/internal/app/modelconfig"
)

// RegisterRoutes mounts the pure-chat routes under r. The caller must protect
// r with session authentication; CSRF enforcement is installed globally by the
// platform router. The chat group also carries the larger body limit needed by
// image messages (see transport/http/router.go).
func (s *Service) RegisterRoutes(r gin.IRouter) {
	chats := r.Group("/chats")
	{
		chats.GET("", s.handleList)
		chats.POST("", s.handleCreate)
		chats.GET("/:id", s.handleGet)
		chats.PUT("/:id", s.handleUpdate)
		chats.DELETE("/:id", s.handleDelete)
		chats.POST("/:id/messages", s.handleSend)
		chats.POST("/:id/stop", s.handleStop)
		chats.POST("/:id/regenerate", s.handleRegenerate)
		chats.POST("/:id/messages/:messageId/edit", s.handleEdit)
		chats.GET("/:id/events", s.handleEvents)
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
	case errors.Is(err, ErrChatNotFound):
		respondError(c, http.StatusNotFound, "chat_not_found")
	case errors.Is(err, ErrMessageNotFound):
		respondError(c, http.StatusNotFound, "message_not_found")
	case errors.Is(err, ErrMessageNotEditable):
		respondError(c, http.StatusBadRequest, "message_not_editable")
	case errors.Is(err, ErrChatBusy):
		respondError(c, http.StatusConflict, "chat_busy")
	case errors.Is(err, ErrInvalidMessage):
		respondError(c, http.StatusBadRequest, "invalid_message")
	case errors.Is(err, ErrInvalidTitle):
		respondError(c, http.StatusBadRequest, "invalid_title")
	case errors.Is(err, ErrInvalidImage):
		respondError(c, http.StatusBadRequest, "invalid_image")
	case errors.Is(err, ErrUnsupportedImage):
		respondError(c, http.StatusBadRequest, "unsupported_image")
	case errors.Is(err, ErrModelUnavailable),
		errors.Is(err, modelconfig.ErrModelNotFound),
		errors.Is(err, modelconfig.ErrModelUnavailable):
		respondError(c, http.StatusBadRequest, "model_unavailable")
	default:
		respondError(c, http.StatusInternalServerError, "server_error")
	}
}

type imageRequest struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	MediaType string `json:"mediaType"`
	Data      string `json:"data"`
	Size      int    `json:"size"`
}

type createRequest struct {
	ModelID uint           `json:"modelId"`
	Text    string         `json:"text"`
	Images  []imageRequest `json:"images"`
}

type sendRequest struct {
	ModelID uint           `json:"modelId"`
	Text    string         `json:"text"`
	Images  []imageRequest `json:"images"`
}

type editRequest struct {
	Text   string         `json:"text"`
	Images []imageRequest `json:"images"`
}

type updateRequest struct {
	Title   *string `json:"title"`
	ModelID *uint   `json:"modelId"`
}

type chatListResponse struct {
	Chats []Chat `json:"chats"`
	Total int    `json:"total"`
}

func (s *Service) handleList(c *gin.Context) {
	limit, _ := strconv.Atoi(c.Query("limit"))
	offset, _ := strconv.Atoi(c.Query("offset"))
	chats, total, err := s.List(limit, offset)
	if err != nil {
		respondServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, chatListResponse{Chats: chats, Total: total})
}

func (s *Service) handleCreate(c *gin.Context) {
	var req createRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return
	}
	chat, err := s.Create(c.Request.Context(), CreateInput{
		ModelID: req.ModelID,
		Text:    req.Text,
		Images:  imagesFromRequest(req.Images),
	})
	if err != nil {
		respondServiceError(c, err)
		return
	}
	c.JSON(http.StatusCreated, chat)
}

func (s *Service) handleGet(c *gin.Context) {
	id, ok := chatID(c)
	if !ok {
		return
	}
	chat, err := s.Get(id)
	if err != nil {
		respondServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, chat)
}

func (s *Service) handleUpdate(c *gin.Context) {
	id, ok := chatID(c)
	if !ok {
		return
	}
	var req updateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return
	}
	chat, err := s.Update(id, UpdateInput{Title: req.Title, ModelID: req.ModelID})
	if err != nil {
		respondServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, chat)
}

func (s *Service) handleDelete(c *gin.Context) {
	id, ok := chatID(c)
	if !ok {
		return
	}
	if err := s.Delete(id); err != nil {
		respondServiceError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (s *Service) handleSend(c *gin.Context) {
	id, ok := chatID(c)
	if !ok {
		return
	}
	var req sendRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return
	}
	chat, err := s.Send(c.Request.Context(), id, SendInput{
		ModelID: req.ModelID,
		Text:    req.Text,
		Images:  imagesFromRequest(req.Images),
	})
	if err != nil {
		respondServiceError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, chat)
}

func (s *Service) handleStop(c *gin.Context) {
	id, ok := chatID(c)
	if !ok {
		return
	}
	if err := s.Stop(id); err != nil {
		respondServiceError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (s *Service) handleRegenerate(c *gin.Context) {
	id, ok := chatID(c)
	if !ok {
		return
	}
	chat, err := s.Regenerate(c.Request.Context(), id)
	if err != nil {
		respondServiceError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, chat)
}

func (s *Service) handleEdit(c *gin.Context) {
	id, ok := chatID(c)
	if !ok {
		return
	}
	messageID, ok := messageID(c)
	if !ok {
		return
	}
	var req editRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return
	}
	chat, err := s.EditAndResend(c.Request.Context(), id, messageID, EditInput{
		Text:   req.Text,
		Images: imagesFromRequest(req.Images),
	})
	if err != nil {
		respondServiceError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, chat)
}

// handleEvents streams the conversation as Server-Sent Events. The route is a
// GET so EventSource can reconnect with the session cookie; CSRF is not
// involved on a safe method.
func (s *Service) handleEvents(c *gin.Context) {
	id, ok := chatID(c)
	if !ok {
		return
	}
	if _, err := s.Get(id); err != nil {
		respondServiceError(c, err)
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	_ = http.NewResponseController(c.Writer).SetWriteDeadline(time.Time{})
	c.Writer.Flush()

	_ = s.StreamChat(c.Request.Context(), id, c.Writer, func() { c.Writer.Flush() })
}

func chatID(c *gin.Context) (uint, bool) {
	value, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || value == 0 {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return 0, false
	}
	return uint(value), true
}

func messageID(c *gin.Context) (uint, bool) {
	value, err := strconv.ParseUint(c.Param("messageId"), 10, 64)
	if err != nil || value == 0 {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return 0, false
	}
	return uint(value), true
}

func imagesFromRequest(images []imageRequest) []ImageAttachment {
	out := make([]ImageAttachment, 0, len(images))
	for _, image := range images {
		out = append(out, ImageAttachment{
			ID:        image.ID,
			Name:      image.Name,
			MediaType: image.MediaType,
			Data:      image.Data,
			Size:      image.Size,
		})
	}
	return out
}
