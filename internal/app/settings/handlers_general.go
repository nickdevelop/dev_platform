package settings

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func (s *Service) handleGetAgentGeneral(c *gin.Context) {
	general, err := s.AgentGeneral()
	if err != nil {
		respondError(c, http.StatusInternalServerError, "server_error")
		return
	}
	c.JSON(http.StatusOK, general)
}

func (s *Service) handlePutAgentGeneral(c *gin.Context) {
	var request struct {
		SystemPrompt string `json:"systemPrompt"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return
	}
	saved, err := s.SetAgentGeneral(AgentGeneralSetting{SystemPrompt: request.SystemPrompt})
	if err != nil {
		respondError(c, http.StatusInternalServerError, "server_error")
		return
	}
	c.JSON(http.StatusOK, saved)
}
