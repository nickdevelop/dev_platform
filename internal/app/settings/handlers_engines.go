package settings

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func (s *Service) handleGetEngines(c *gin.Context) {
	setting, err := s.Engines()
	if err != nil {
		respondError(c, http.StatusInternalServerError, "server_error")
		return
	}
	c.JSON(http.StatusOK, setting)
}

func (s *Service) handlePutEngines(c *gin.Context) {
	var input EnginesSetting
	if err := c.ShouldBindJSON(&input); err != nil {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return
	}

	saved, err := s.SetEngines(input)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "server_error")
		return
	}
	c.JSON(http.StatusOK, saved)
}
