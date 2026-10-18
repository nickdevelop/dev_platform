// Package http wires the platform's HTTP API.
package http

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"devplatform/internal/app/agentservices"
	"devplatform/internal/app/instances"
	"devplatform/internal/app/auth"
	"devplatform/internal/app/modelconfig"
	"devplatform/internal/app/projects"
	"devplatform/internal/app/prompts"
	"devplatform/internal/app/sessions"
	"devplatform/internal/app/settings"
	"devplatform/internal/app/tasks"
)

// maxRequestBodyBytes bounds every API request body (1 MiB). All platform
// request DTOs are tiny JSON objects; a larger body is malformed or hostile
// and is rejected before it can be buffered.
const maxRequestBodyBytes = 1 << 20

// limitRequestBody caps the request body a handler may read from. Reads past
// the cap fail, so gin's binder reports the malformed request instead of the
// server buffering an unbounded payload.
func limitRequestBody(max int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Body != nil {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, max)
		}
		c.Next()
	}
}

// Dependencies carries the collaborators the API routes need. Later tasks
// extend this struct (database, workspace services); services are injected
// explicitly so the router owns no global state.
type Dependencies struct {
	// Auth provides password login, sessions and CSRF. When nil, only the
	// public health route is served (used by tests that do not need auth).
	Auth *auth.Service
	// Projects manages project records and their directories. When set
	// together with Auth, the project routes are served under /api/projects
	// behind RequireSession; it requires Auth to be set.
	Projects *projects.Service
	// Settings manages platform-wide appearance and agent configuration. When
	// set together with Auth, the settings routes are served under
	// /api/settings behind RequireSession.
	Settings *settings.Service
	// Models manages OpenAI-compatible model services and their models. When
	// set together with Auth, the model routes are served under
	// /api/model-services and /api/models behind RequireSession.
	Models *modelconfig.Service
	// Instances manages per-project agent instances and their event fan-out.
	// When set together with Auth, the instance routes are served under
	// /api/projects/{id}/instances behind RequireSession.
	Instances *instances.Service
	// AgentServices controls the machine-level services of agent backends. When
	// set together with Auth, the service routes are served under
	// /api/agent-services behind RequireSession.
	AgentServices *agentservices.Service
	// Sessions manages the platform session index, native-session discovery
	// and the normalized SSE stream. When set together with Auth, the session
	// routes are served under /api/projects/{id}/sessions behind
	// RequireSession.
	Sessions *sessions.Service
	// Tasks manages tasks, flows, orchestration, and scheduling.
	Tasks *tasks.Service
	// Prompts manages reusable prompt templates.
	Prompts *prompts.Service
	// WebDir is the directory holding the built web UI assets (the Vite
	// build output, typically web/dist). When non-empty, the engine serves
	// the single-page app from it so the UI and the API share one origin in
	// production: /api routes always take precedence, and unknown non-API
	// paths fall back to the SPA entry. Empty disables static hosting.
	WebDir string
}

// NewRouter builds the Gin engine with all API routes registered.
func NewRouter(deps Dependencies) *gin.Engine {
	router := gin.New()
	// AccessLog runs before Recovery so even a panicking handler still
	// produces one access line (with its 500 status) carrying the response
	// time needed to spot stuck operations.
	router.Use(AccessLog(), gin.Recovery())

	// The platform is exposed directly on the LAN with no reverse proxy in
	// front of it, so no proxy headers (X-Forwarded-*) are trusted when the
	// engine derives a client address; a client cannot forge them.
	_ = router.SetTrustedProxies(nil)

	router.GET("/api/health", handleHealth)

	if deps.Auth != nil {
		// Enforce CSRF on every mutating API request centrally so no later
		// management route can accidentally omit it.
		router.Use(auth.RequireCSRF())

		api := router.Group("/api", limitRequestBody(maxRequestBodyBytes))
		deps.Auth.RegisterRoutes(api)

		// Management routes share one session-protected group so no route
		// can accidentally omit authentication (CSRF is installed globally
		// above).
		protected := api.Group("", auth.RequireSession(deps.Auth))
		if deps.Projects != nil {
			deps.Projects.RegisterRoutes(protected)
		}
		if deps.Settings != nil {
			deps.Settings.RegisterRoutes(protected)
		}
		if deps.Models != nil {
			deps.Models.RegisterRoutes(protected)
		}
		if deps.Instances != nil {
			deps.Instances.RegisterRoutes(protected)
		}
		if deps.AgentServices != nil {
			deps.AgentServices.RegisterRoutes(protected)
		}
		if deps.Sessions != nil {
			deps.Sessions.RegisterRoutes(protected)
			deps.Sessions.RegisterInteractionRoutes(protected)
		}
		if deps.Tasks != nil {
			deps.Tasks.RegisterRoutes(protected)
		}
		if deps.Prompts != nil {
			deps.Prompts.RegisterRoutes(protected)
		}
	}

	if deps.WebDir != "" {
		serveStatic(router, deps.WebDir)
	}

	return router
}
