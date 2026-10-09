// Package http wires the platform's HTTP API.
package http

import (
	"github.com/gin-gonic/gin"

	"devplatform/internal/auth"
	"devplatform/internal/workspaces"
)

// Dependencies carries the collaborators the API routes need. Later tasks
// extend this struct (database, workspace services); services are injected
// explicitly so the router owns no global state.
type Dependencies struct {
	// Auth provides password login, sessions and CSRF. When nil, only the
	// public health route is served (used by tests that do not need auth).
	Auth *auth.Service
	// Workspaces manages the authorized workspace roots. When set together
	// with Auth, the workspace-root management routes are served under
	// /api/workspaces behind RequireSession; it requires Auth to be set.
	Workspaces *workspaces.Service
}

// NewRouter builds the Gin engine with all API routes registered.
func NewRouter(deps Dependencies) *gin.Engine {
	router := gin.New()
	router.Use(gin.Recovery())

	router.GET("/api/health", handleHealth)

	if deps.Auth != nil {
		// Enforce CSRF on every mutating API request centrally so no later
		// management route can accidentally omit it.
		router.Use(auth.RequireCSRF())

		api := router.Group("/api")
		deps.Auth.RegisterRoutes(api)

		if deps.Workspaces != nil {
			protected := api.Group("", auth.RequireSession(deps.Auth))
			deps.Workspaces.RegisterRoutes(protected)
		}
	}

	return router
}
