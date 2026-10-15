package secrets

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

// Handler exposes the secret value store over the management API. It never
// returns a stored value: create and rotate accept a value, and every read
// reports only the opaque reference and whether a value is present.
type Handler struct {
	store *Store
}

// NewHandler builds the secret API handler over a store.
func NewHandler(store *Store) *Handler {
	return &Handler{store: store}
}

// RegisterRoutes registers the secret routes under r. The caller must protect
// r with session authentication (auth.RequireSession); CSRF enforcement is
// installed globally by the platform router.
func (h *Handler) RegisterRoutes(r gin.IRouter) {
	r.POST("/secrets", h.handleCreate)
	r.GET("/secrets/:id", h.handleGet)
	r.PUT("/secrets/:id", h.handleRotate)
	r.DELETE("/secrets/:id", h.handleDelete)
}

// errorResponse is the JSON error envelope for secret API failures.
type errorResponse struct {
	Error string `json:"error"`
}

func respondError(c *gin.Context, status int, code string) {
	c.AbortWithStatusJSON(status, errorResponse{Error: code})
}

// respondServiceError maps a store error to an HTTP status and error code.
func respondServiceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrInvalidSecretID):
		respondError(c, http.StatusBadRequest, "invalid_secret_id")
	case errors.Is(err, ErrEmptySecretValue):
		respondError(c, http.StatusBadRequest, "invalid_secret_value")
	case errors.Is(err, ErrSecretNotFound):
		respondError(c, http.StatusNotFound, "secret_not_found")
	default:
		respondError(c, http.StatusInternalServerError, "server_error")
	}
}

// secretWriteRequest is the JSON body that carries a value to store. The
// value is write-only and never echoed.
type secretWriteRequest struct {
	Value string `json:"value"`
}

// secretMetadata is the JSON body describing a reference without its value.
type secretMetadata struct {
	ID      string `json:"id"`
	Present bool   `json:"present"`
}

func (h *Handler) handleCreate(c *gin.Context) {
	var req secretWriteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return
	}
	id, err := NewSecretID()
	if err != nil {
		respondError(c, http.StatusInternalServerError, "server_error")
		return
	}
	if err := h.store.Set(context.Background(), id, req.Value); err != nil {
		respondServiceError(c, err)
		return
	}
	c.JSON(http.StatusCreated, secretMetadata{ID: id, Present: true})
}

func (h *Handler) handleGet(c *gin.Context) {
	id := c.Param("id")
	exists, err := h.store.Exists(context.Background(), id)
	if err != nil {
		respondServiceError(c, err)
		return
	}
	if !exists {
		respondError(c, http.StatusNotFound, "secret_not_found")
		return
	}
	c.JSON(http.StatusOK, secretMetadata{ID: id, Present: true})
}

func (h *Handler) handleRotate(c *gin.Context) {
	id := c.Param("id")
	var req secretWriteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return
	}
	exists, err := h.store.Exists(context.Background(), id)
	if err != nil {
		respondServiceError(c, err)
		return
	}
	if !exists {
		respondError(c, http.StatusNotFound, "secret_not_found")
		return
	}
	if err := h.store.Set(context.Background(), id, req.Value); err != nil {
		respondServiceError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) handleDelete(c *gin.Context) {
	if err := h.store.Delete(context.Background(), c.Param("id")); err != nil {
		respondServiceError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
