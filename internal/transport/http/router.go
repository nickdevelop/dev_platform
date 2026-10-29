// Package http wires the platform's HTTP API.
package http

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"devplatform/internal/app/agentmodels"
	"devplatform/internal/app/agentservices"
	"devplatform/internal/app/auth"
	"devplatform/internal/app/chat"
	"devplatform/internal/app/instances"
	"devplatform/internal/app/modelconfig"
	"devplatform/internal/app/projects"
	"devplatform/internal/app/prompts"
	"devplatform/internal/app/proxy"
	"devplatform/internal/app/sessions"
	"devplatform/internal/app/settings"
	"devplatform/internal/app/tasks"
	"devplatform/internal/app/terminal"
)

// maxRequestBodyBytes bounds every API request body (1 MiB). All platform
// request DTOs are tiny JSON objects; a larger body is malformed or hostile
// and is rejected before it can be buffered.
const maxRequestBodyBytes = 1 << 20

// maxImageRequestBodyBytes bounds the routes whose user turn may carry
// images: pure chat and agent sessions. A message may carry up to four 5 MiB
// images, which base64 expands to roughly 27 MiB; 32 MiB leaves room for JSON
// framing without weakening the limit of every other API route.
const maxImageRequestBodyBytes = 32 << 20

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
	// AgentModels owns the platform-wide, persisted model catalogs of agent
	// backends. When set together with Auth, the model routes are served under
	// /api/agent-backends/{backend}/models behind RequireSession.
	AgentModels *agentmodels.Service
	// Sessions manages the platform session index, native-session discovery
	// and the normalized SSE stream. When set together with Auth, the session
	// routes are served under /api/projects/{id}/sessions behind
	// RequireSession.
	Sessions *sessions.Service
	// Tasks manages tasks, flows, orchestration, and scheduling.
	Tasks *tasks.Service
	// Prompts manages reusable prompt templates.
	Prompts *prompts.Service
	// Terminal manages server connections and interactive terminal sessions.
	Terminal *terminal.Service
	// Chats manages global, non-agent conversations that call an
	// OpenAI-compatible model service directly. When set together with Auth,
	// the chat routes are served under /api/chats behind RequireSession and
	// carry the larger image-message body limit.
	Chats *chat.Service
	// Proxy manages contextpath exposure for local directories and ports.
	Proxy *proxy.Service
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

		api := router.Group("/api")

		// Auth routes keep the standard body limit but are not session
		// protected (login itself must be reachable without a session).
		authRoutes := api.Group("", limitRequestBody(maxRequestBodyBytes))
		deps.Auth.RegisterRoutes(authRoutes)

		// Management routes share one session-protected group so no route
		// can accidentally omit authentication (CSRF is installed globally
		// above).
		authenticated := api.Group("", auth.RequireSession(deps.Auth))
		protected := authenticated.Group("", limitRequestBody(maxRequestBodyBytes))
		if deps.Projects != nil {
			deps.Projects.RegisterRoutes(protected)
			deps.Projects.RegisterPreviewRoutes(api)
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
		if deps.AgentModels != nil {
			deps.AgentModels.RegisterRoutes(protected)
		}
		if deps.Sessions != nil {
			// Session create/send turns may carry the same attached images as
			// pure chat, so this group gets the larger cap too. It is still
			// inside RequireSession and the global CSRF middleware.
			sessions := authenticated.Group("", limitRequestBody(maxImageRequestBodyBytes))
			deps.Sessions.RegisterRoutes(sessions)
			deps.Sessions.RegisterInteractionRoutes(sessions)
		}
		if deps.Tasks != nil {
			deps.Tasks.RegisterRoutes(protected)
		}
		if deps.Prompts != nil {
			deps.Prompts.RegisterRoutes(protected)
		}
		if deps.Terminal != nil {
			deps.Terminal.RegisterRoutes(protected)
		}
		if deps.Proxy != nil {
			deps.Proxy.RegisterRoutes(protected)
		}
		if deps.Chats != nil {
			// Chat messages may carry images; only this group gets the larger
			// cap. It is still inside RequireSession and the global CSRF
			// middleware.
			chats := authenticated.Group("", limitRequestBody(maxImageRequestBodyBytes))
			deps.Chats.RegisterRoutes(chats)
		}
	}

	if deps.Proxy != nil {
		proxy.RegisterGateway(router, deps.Proxy)
	}

	if deps.WebDir != "" {
		serveStatic(router, deps.WebDir)
	}

	return router
}
