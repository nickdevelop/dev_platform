package resourcesync

import (
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"devplatform/internal/app/projects"
	"devplatform/internal/app/resources"
)

// RegisterRoutes registers the project resource-sync routes under r. The
// caller must protect r with session authentication (auth.RequireSession);
// CSRF enforcement is installed globally by the platform router.
func (c *Coordinator) RegisterRoutes(r gin.IRouter) {
	r.GET("/projects/:id/resources/sync", c.handlePreview)
	r.POST("/projects/:id/resources/sync", c.handleApply)
	r.POST("/projects/:id/resources/sync/resolve", c.handleResolve)
}

// errorResponse is the JSON error envelope for sync API failures.
type errorResponse struct {
	Error string `json:"error"`
}

func respondError(ctx *gin.Context, status int, code string) {
	ctx.AbortWithStatusJSON(status, errorResponse{Error: code})
}

func respondServiceError(ctx *gin.Context, err error) {
	switch {
	case errors.Is(err, projects.ErrProjectNotFound):
		respondError(ctx, http.StatusNotFound, "project_not_found")
	case errors.Is(err, resources.ErrResourceNotFound):
		respondError(ctx, http.StatusNotFound, "resource_not_found")
	case errors.Is(err, ErrConflict), errors.Is(err, ErrInvalidTarget), errors.Is(err, ErrInvalidPreview):
		respondError(ctx, http.StatusConflict, "sync_conflict")
	default:
		respondError(ctx, http.StatusInternalServerError, "server_error")
	}
}

// previewListResponse is the JSON body of a preview or apply.
type previewListResponse struct {
	Previews []SyncPreview `json:"previews"`
}

// applySyncRequest is the JSON body of an apply. Force is the explicit user
// resolution that allows overwriting an unmanaged target or external drift.
type applySyncRequest struct {
	Force bool `json:"force"`
}

// resolveSyncRequest is the JSON body of a conflict resolution.
type resolveSyncRequest struct {
	ResourceID uint `json:"resourceId"`
}

func (c *Coordinator) handlePreview(ctx *gin.Context) {
	projectID, ok := syncProjectID(ctx)
	if !ok {
		return
	}
	previews, err := c.Preview(ctx.Request.Context(), projectID)
	if err != nil {
		respondServiceError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, previewListResponse{Previews: previews})
}

func (c *Coordinator) handleApply(ctx *gin.Context) {
	projectID, ok := syncProjectID(ctx)
	if !ok {
		return
	}
	var req applySyncRequest
	if err := ctx.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		respondError(ctx, http.StatusBadRequest, "invalid_request")
		return
	}
	previews, err := c.Apply(ctx.Request.Context(), projectID, req.Force)
	if err != nil {
		respondServiceError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, previewListResponse{Previews: previews})
}

func (c *Coordinator) handleResolve(ctx *gin.Context) {
	projectID, ok := syncProjectID(ctx)
	if !ok {
		return
	}
	var req resolveSyncRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		respondError(ctx, http.StatusBadRequest, "invalid_request")
		return
	}
	previews, err := c.ResolveConflict(ctx.Request.Context(), projectID, req.ResourceID)
	if err != nil {
		respondServiceError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, previewListResponse{Previews: previews})
}

// syncProjectID parses the :id path parameter.
func syncProjectID(ctx *gin.Context) (uint, bool) {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 64)
	if err != nil {
		respondError(ctx, http.StatusBadRequest, "invalid_request")
		return 0, false
	}
	return uint(id), true
}
