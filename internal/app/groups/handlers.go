package groups

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// RegisterRoutes mounts all groups REST endpoints.
func (s *Service) RegisterRoutes(r gin.IRouter) {
	groups := r.Group("/groups")
	{
		groups.GET("", s.handleListGroups)
		groups.POST("", s.handleCreateGroup)
		groups.GET("/:id", s.handleGetGroup)
		groups.PUT("/:id", s.handleUpdateGroup)
		groups.DELETE("/:id", s.handleDeleteGroup)

		groups.PUT("/:id/members", s.handleSetGroupMembers)

		groups.GET("/:id/tasks", s.handleListGroupTasks)
		groups.POST("/:id/tasks", s.handleCreateGroupTask)
		groups.GET("/:id/tasks/:taskId", s.handleGetGroupTask)
		groups.PUT("/:id/tasks/:taskId", s.handleUpdateGroupTask)
		groups.DELETE("/:id/tasks/:taskId", s.handleDeleteGroupTask)

		groups.POST("/:id/tasks/:taskId/trigger", s.handleTriggerGroupTask)
		groups.GET("/:id/tasks/:taskId/runs", s.handleListGroupTaskRuns)
	}

	flowRuns := r.Group("/groups/runs")
	{
		flowRuns.GET("/:runId/messages", s.handleListRunMessages)
		flowRuns.POST("/:runId/messages", s.handlePostUserMessage)
		flowRuns.POST("/:runId/cancel", s.handleCancelRun)
	}
}

type errorResponse struct {
	Error string `json:"error"`
}

func respondError(c *gin.Context, status int, code string) {
	c.AbortWithStatusJSON(status, errorResponse{Error: code})
}

func parseGroupID(c *gin.Context) (uint, bool) {
	val, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || val == 0 {
		respondError(c, http.StatusBadRequest, "invalid_group_id")
		return 0, false
	}
	return uint(val), true
}

func parseTaskID(c *gin.Context) (uint, bool) {
	val, err := strconv.ParseUint(c.Param("taskId"), 10, 64)
	if err != nil || val == 0 {
		respondError(c, http.StatusBadRequest, "invalid_task_id")
		return 0, false
	}
	return uint(val), true
}

func parseRunID(c *gin.Context) (uint, bool) {
	val, err := strconv.ParseUint(c.Param("runId"), 10, 64)
	if err != nil || val == 0 {
		respondError(c, http.StatusBadRequest, "invalid_run_id")
		return 0, false
	}
	return uint(val), true
}

func (s *Service) handleListGroups(c *gin.Context) {
	list, err := s.ListGroups(c.Request.Context())
	if err != nil {
		respondError(c, http.StatusInternalServerError, "server_error")
		return
	}
	c.JSON(http.StatusOK, gin.H{"groups": list})
}

func (s *Service) handleCreateGroup(c *gin.Context) {
	var req CreateGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return
	}

	detail, err := s.CreateGroup(c.Request.Context(), req)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidGroupName):
			respondError(c, http.StatusBadRequest, "invalid_group_name")
		case errors.Is(err, ErrInvalidOrchestratorModel):
			respondError(c, http.StatusBadRequest, "invalid_orchestrator_model")
		default:
			respondError(c, http.StatusInternalServerError, "server_error")
		}
		return
	}
	c.JSON(http.StatusCreated, detail)
}

func (s *Service) handleGetGroup(c *gin.Context) {
	id, ok := parseGroupID(c)
	if !ok {
		return
	}

	detail, err := s.GetGroup(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, ErrGroupNotFound) {
			respondError(c, http.StatusNotFound, "group_not_found")
			return
		}
		respondError(c, http.StatusInternalServerError, "server_error")
		return
	}
	c.JSON(http.StatusOK, detail)
}

func (s *Service) handleUpdateGroup(c *gin.Context) {
	id, ok := parseGroupID(c)
	if !ok {
		return
	}

	var req UpdateGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return
	}

	updated, err := s.UpdateGroup(c.Request.Context(), id, req)
	if err != nil {
		switch {
		case errors.Is(err, ErrGroupNotFound):
			respondError(c, http.StatusNotFound, "group_not_found")
		case errors.Is(err, ErrInvalidGroupName):
			respondError(c, http.StatusBadRequest, "invalid_group_name")
		case errors.Is(err, ErrInvalidOrchestratorModel):
			respondError(c, http.StatusBadRequest, "invalid_orchestrator_model")
		default:
			respondError(c, http.StatusInternalServerError, "server_error")
		}
		return
	}
	c.JSON(http.StatusOK, updated)
}

func (s *Service) handleDeleteGroup(c *gin.Context) {
	id, ok := parseGroupID(c)
	if !ok {
		return
	}

	if err := s.DeleteGroup(c.Request.Context(), id); err != nil {
		if errors.Is(err, ErrGroupNotFound) {
			respondError(c, http.StatusNotFound, "group_not_found")
			return
		}
		respondError(c, http.StatusInternalServerError, "server_error")
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *Service) handleSetGroupMembers(c *gin.Context) {
	id, ok := parseGroupID(c)
	if !ok {
		return
	}

	var body struct {
		RoleIDs []uint `json:"roleIds"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return
	}

	members, err := s.SetGroupMembers(c.Request.Context(), id, body.RoleIDs)
	if err != nil {
		if errors.Is(err, ErrGroupNotFound) {
			respondError(c, http.StatusNotFound, "group_not_found")
			return
		}
		respondError(c, http.StatusInternalServerError, "server_error")
		return
	}
	c.JSON(http.StatusOK, gin.H{"members": members})
}

func (s *Service) handleListGroupTasks(c *gin.Context) {
	id, ok := parseGroupID(c)
	if !ok {
		return
	}

	list, err := s.ListGroupTasks(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, ErrGroupNotFound) {
			respondError(c, http.StatusNotFound, "group_not_found")
			return
		}
		respondError(c, http.StatusInternalServerError, "server_error")
		return
	}
	c.JSON(http.StatusOK, gin.H{"tasks": list})
}

func (s *Service) handleCreateGroupTask(c *gin.Context) {
	id, ok := parseGroupID(c)
	if !ok {
		return
	}

	var req CreateGroupTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return
	}

	task, err := s.CreateGroupTask(c.Request.Context(), id, req)
	if err != nil {
		switch {
		case errors.Is(err, ErrGroupNotFound):
			respondError(c, http.StatusNotFound, "group_not_found")
		case errors.Is(err, ErrInvalidTaskTitle):
			respondError(c, http.StatusBadRequest, "invalid_task_title")
		case errors.Is(err, ErrInvalidProject):
			respondError(c, http.StatusBadRequest, "invalid_project")
		case errors.Is(err, ErrInvalidPrompt):
			respondError(c, http.StatusBadRequest, "invalid_prompt")
		default:
			respondError(c, http.StatusInternalServerError, "server_error")
		}
		return
	}
	c.JSON(http.StatusCreated, task)
}

func (s *Service) handleGetGroupTask(c *gin.Context) {
	taskID, ok := parseTaskID(c)
	if !ok {
		return
	}

	task, err := s.GetGroupTask(c.Request.Context(), taskID)
	if err != nil {
		if errors.Is(err, ErrGroupTaskNotFound) {
			respondError(c, http.StatusNotFound, "task_not_found")
			return
		}
		respondError(c, http.StatusInternalServerError, "server_error")
		return
	}
	c.JSON(http.StatusOK, task)
}

func (s *Service) handleUpdateGroupTask(c *gin.Context) {
	taskID, ok := parseTaskID(c)
	if !ok {
		return
	}

	var req UpdateGroupTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return
	}

	task, err := s.UpdateGroupTask(c.Request.Context(), taskID, req)
	if err != nil {
		switch {
		case errors.Is(err, ErrGroupTaskNotFound):
			respondError(c, http.StatusNotFound, "task_not_found")
		case errors.Is(err, ErrInvalidTaskTitle):
			respondError(c, http.StatusBadRequest, "invalid_task_title")
		case errors.Is(err, ErrInvalidProject):
			respondError(c, http.StatusBadRequest, "invalid_project")
		case errors.Is(err, ErrInvalidPrompt):
			respondError(c, http.StatusBadRequest, "invalid_prompt")
		default:
			respondError(c, http.StatusInternalServerError, "server_error")
		}
		return
	}
	c.JSON(http.StatusOK, task)
}

func (s *Service) handleDeleteGroupTask(c *gin.Context) {
	taskID, ok := parseTaskID(c)
	if !ok {
		return
	}

	if err := s.DeleteGroupTask(c.Request.Context(), taskID); err != nil {
		if errors.Is(err, ErrGroupTaskNotFound) {
			respondError(c, http.StatusNotFound, "task_not_found")
			return
		}
		respondError(c, http.StatusInternalServerError, "server_error")
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *Service) handleListGroupTaskRuns(c *gin.Context) {
	taskID, ok := parseTaskID(c)
	if !ok {
		return
	}

	runs, err := s.ListGroupTaskRuns(c.Request.Context(), taskID)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "server_error")
		return
	}
	c.JSON(http.StatusOK, gin.H{"runs": runs})
}

func (s *Service) handleTriggerGroupTask(c *gin.Context) {
	groupID, ok := parseGroupID(c)
	if !ok {
		return
	}
	taskID, ok := parseTaskID(c)
	if !ok {
		return
	}

	var body struct {
		Prompt string `json:"prompt"`
	}
	_ = c.ShouldBindJSON(&body)

	run, err := s.TriggerGroupTask(c.Request.Context(), groupID, taskID, body.Prompt)
	if err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	c.JSON(http.StatusOK, run)
}

func (s *Service) handleListRunMessages(c *gin.Context) {
	runID, ok := parseRunID(c)
	if !ok {
		return
	}

	msgs, err := s.ListFlowMessages(c.Request.Context(), runID)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"messages": msgs})
}

func (s *Service) handlePostUserMessage(c *gin.Context) {
	runID, ok := parseRunID(c)
	if !ok {
		return
	}

	var body struct {
		Content string `json:"content"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.Content == "" {
		respondError(c, http.StatusBadRequest, "content_required")
		return
	}

	msg, err := s.PostUserMessage(c.Request.Context(), runID, body.Content)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, msg)
}

func (s *Service) handleCancelRun(c *gin.Context) {
	runID, ok := parseRunID(c)
	if !ok {
		return
	}

	if err := s.CancelRun(c.Request.Context(), runID); err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
