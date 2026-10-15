package http

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// healthResponse is the JSON body returned by GET /api/health.
type healthResponse struct {
	Status string `json:"status"`
}

// handleHealth reports service liveness.
func handleHealth(c *gin.Context) {
	c.JSON(http.StatusOK, healthResponse{Status: "ok"})
}
