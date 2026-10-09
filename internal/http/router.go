// Package http wires the platform's HTTP API.
package http

import (
	"github.com/gin-gonic/gin"

	"devplatform/internal/auth"
	"devplatform/internal/projects"
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
	// Projects manages project records and their directories. When set
	// together with Auth, the project routes are served under /api/projects
	// behind RequireSession; it requires Auth to be set.
	Projects *projects.Service
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

		// Management routes share one session-protected group so no route
		// can accidentally omit authentication (CSRF is installed globally
		// above).
		protected := api.Group("", auth.RequireSession(deps.Auth))
		if deps.Workspaces != nil {
			deps.Workspaces.RegisterRoutes(protected)
		}
		if deps.Projects != nil {
			deps.Projects.RegisterRoutes(protected)
		}
	}

	return router
}
