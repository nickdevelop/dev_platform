// Package http wires the platform's HTTP API.
package http

import "github.com/gin-gonic/gin"

// Dependencies carries the collaborators the API routes need. Task 1 has
// none; later tasks extend this struct (database, auth, workspace services).
type Dependencies struct{}

// NewRouter builds the Gin engine with all API routes registered.
func NewRouter(deps Dependencies) *gin.Engine {
	_ = deps // reserved for future tasks; routes added as dependencies arrive.

	router := gin.New()
	router.Use(gin.Recovery())

	router.GET("/api/health", handleHealth)

	return router
}
