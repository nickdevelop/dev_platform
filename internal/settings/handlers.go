package settings

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

// RegisterRoutes registers the settings routes under r. The caller must
// protect r with session authentication (auth.RequireSession); CSRF
// enforcement is installed globally by the platform router.
func (s *Service) RegisterRoutes(r gin.IRouter) {
	r.GET("/settings/appearance", s.handleGetAppearance)
	r.PUT("/settings/appearance", s.handlePutAppearance)
	r.GET("/agents", s.handleListAgents)
	r.PUT("/agents/:backend", s.handleSetAgentEnabled)
	r.PUT("/agents/:backend/path", s.handleSetAgentPath)
	r.POST("/agents/:backend/detect", s.handleDetectAgent)
}

// errorResponse is the JSON error envelope for settings API failures.
type errorResponse struct {
	Error string `json:"error"`
}

// respondError stops the chain and writes a JSON error envelope.
func respondError(c *gin.Context, status int, code string) {
	c.AbortWithStatusJSON(status, errorResponse{Error: code})
}

func (s *Service) handleGetAppearance(c *gin.Context) {
	appearance, err := s.Appearance()
	if err != nil {
		respondError(c, http.StatusInternalServerError, "server_error")
		return
	}
	c.JSON(http.StatusOK, appearance)
}

func (s *Service) handlePutAppearance(c *gin.Context) {
	var request Appearance
	if err := c.ShouldBindJSON(&request); err != nil {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return
	}
	saved, err := s.SetAppearance(request)
	if err != nil {
		if errors.Is(err, ErrInvalidAppearance) {
			respondError(c, http.StatusBadRequest, "invalid_appearance")
			return
		}
		respondError(c, http.StatusInternalServerError, "server_error")
		return
	}
	c.JSON(http.StatusOK, saved)
}

// jsonMarshal and jsonUnmarshal are tiny indirections so the service has one
// place to change if the encoding ever does.
func jsonMarshal(value any) (string, error) {
	data, err := json.Marshal(value)
	return string(data), err
}

func jsonUnmarshal(data string, target any) error {
	return json.Unmarshal([]byte(data), target)
}
