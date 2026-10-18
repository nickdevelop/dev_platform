package tasks

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// RegisterRoutes mounts task API endpoints under r.
func (s *Service) RegisterRoutes(r gin.IRouter) {
	tasks := r.Group("/tasks")
	{
		tasks.GET("", s.handleList)
		tasks.POST("", s.handleCreate)
		tasks.GET("/flows", s.handleFlowDefs)
		tasks.GET("/session-ids", s.handleGetSessionIDs)
		tasks.GET("/:id", s.handleGet)
		tasks.PUT("/:id", s.handleUpdate)
		tasks.DELETE("/:id", s.handleDelete)
		tasks.POST("/:id/advance", s.handleAdvance)
		tasks.PUT("/:id/execution-prompt", s.handleSaveExecutionPrompt)
		tasks.GET("/:id/runs", s.handleListRuns)
		tasks.PUT("/:id/schedule", s.handleUpdateSchedule)
		tasks.POST("/:id/session", s.handleCreateSession)
		tasks.POST("/:id/trigger", s.handleTrigger)
	}
}

type errorResponse struct {
	Error string `json:"error"`
}

func respondError(c *gin.Context, status int, code string) {
	c.AbortWithStatusJSON(status, errorResponse{Error: code})
}

func parseTaskID(c *gin.Context) (uint, bool) {
	val, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || val == 0 {
		respondError(c, http.StatusBadRequest, "invalid_task_id")
		return 0, false
	}
	return uint(val), true
}

func (s *Service) handleFlowDefs(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"flows": []FlowDef{FlowTransactionalDef, FlowDevelopmentDef},
	})
}

func (s *Service) handleList(c *gin.Context) {
	var filter ListFilter
	if pStr := c.Query("projectId"); pStr != "" {
		if pID, err := strconv.ParseUint(pStr, 10, 64); err == nil && pID > 0 {
			val := uint(pID)
			filter.ProjectID = &val
		}
	}
	if fStr := c.Query("flowType"); fStr != "" {
		f := FlowType(fStr)
		filter.FlowType = &f
	}
	if sStr := c.Query("status"); sStr != "" {
		st := TaskStatus(sStr)
		filter.Status = &st
	}

	list, err := s.List(filter)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "server_error")
		return
	}
	c.JSON(http.StatusOK, gin.H{"tasks": list})
}

func (s *Service) handleCreate(c *gin.Context) {
	var req CreateTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return
	}

	task, err := s.Create(c.Request.Context(), req)
	if err != nil {
		if errors.Is(err, ErrInvalidFlowType) {
			respondError(c, http.StatusBadRequest, "invalid_flow_type")
			return
		}
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}

	c.JSON(http.StatusCreated, task)
}

func (s *Service) handleGet(c *gin.Context) {
	id, ok := parseTaskID(c)
	if !ok {
		return
	}

	task, err := s.Get(id)
	if err != nil {
		if errors.Is(err, ErrTaskNotFound) {
			respondError(c, http.StatusNotFound, "task_not_found")
			return
		}
		respondError(c, http.StatusInternalServerError, "server_error")
		return
	}

	c.JSON(http.StatusOK, task)
}

func (s *Service) handleUpdate(c *gin.Context) {
	id, ok := parseTaskID(c)
	if !ok {
		return
	}

	var req UpdateTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return
	}

	task, err := s.Update(c.Request.Context(), id, req)
	if err != nil {
		if errors.Is(err, ErrTaskNotFound) {
			respondError(c, http.StatusNotFound, "task_not_found")
			return
		}
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}

	c.JSON(http.StatusOK, task)
}

func (s *Service) handleDelete(c *gin.Context) {
	id, ok := parseTaskID(c)
	if !ok {
		return
	}

	if err := s.Delete(id); err != nil {
		if errors.Is(err, ErrTaskNotFound) {
			respondError(c, http.StatusNotFound, "task_not_found")
			return
		}
		respondError(c, http.StatusInternalServerError, "server_error")
		return
	}

	c.Status(http.StatusNoContent)
}

func (s *Service) handleAdvance(c *gin.Context) {
	id, ok := parseTaskID(c)
	if !ok {
		return
	}

	var req AdvanceStepRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return
	}

	task, err := s.AdvanceStep(c.Request.Context(), id, req)
	if err != nil {
		if errors.Is(err, ErrTaskNotFound) {
			respondError(c, http.StatusNotFound, "task_not_found")
			return
		}
		if errors.Is(err, ErrStepNotConfirmed) {
			respondError(c, http.StatusBadRequest, "step_not_confirmed")
			return
		}
		if errors.Is(err, ErrInvalidStep) {
			respondError(c, http.StatusBadRequest, "invalid_step")
			return
		}
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}

	c.JSON(http.StatusOK, task)
}

func (s *Service) handleUpdateSchedule(c *gin.Context) {
	id, ok := parseTaskID(c)
	if !ok {
		return
	}

	var req ScheduleConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return
	}

	task, err := s.UpdateSchedule(c.Request.Context(), id, req)
	if err != nil {
		if errors.Is(err, ErrTaskNotFound) {
			respondError(c, http.StatusNotFound, "task_not_found")
			return
		}
		if errors.Is(err, ErrScheduleNotAllowed) {
			respondError(c, http.StatusConflict, "scheduling_not_allowed")
			return
		}
		if errors.Is(err, ErrCronNotSupported) {
			respondError(c, http.StatusBadRequest, "cron_not_supported_for_flow")
			return
		}
		if errors.Is(err, ErrInvalidScheduleTime) {
			respondError(c, http.StatusBadRequest, "invalid_schedule_time")
			return
		}
		if errors.Is(err, ErrInvalidCronExpr) {
			respondError(c, http.StatusBadRequest, "invalid_cron_expression")
			return
		}
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}

	c.JSON(http.StatusOK, task)
}

func (s *Service) handleCreateSession(c *gin.Context) {
	id, ok := parseTaskID(c)
	if !ok {
		return
	}

	task, sess, err := s.CreateSessionForTask(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, ErrTaskNotFound) {
			respondError(c, http.StatusNotFound, "task_not_found")
			return
		}
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"task":    task,
		"session": sess,
	})
}

type saveExecutionPromptRequest struct {
	ExecutionPrompt string `json:"executionPrompt"`
}

func (s *Service) handleSaveExecutionPrompt(c *gin.Context) {
	id, ok := parseTaskID(c)
	if !ok {
		return
	}

	var req saveExecutionPromptRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid_request")
		return
	}

	task, err := s.SaveExecutionPrompt(c.Request.Context(), id, req.ExecutionPrompt)
	if err != nil {
		if errors.Is(err, ErrTaskNotFound) {
			respondError(c, http.StatusNotFound, "task_not_found")
			return
		}
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}

	c.JSON(http.StatusOK, task)
}

func (s *Service) handleListRuns(c *gin.Context) {
	id, ok := parseTaskID(c)
	if !ok {
		return
	}

	runs, err := s.ListRuns(c.Request.Context(), id)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "server_error")
		return
	}

	c.JSON(http.StatusOK, gin.H{"runs": runs})
}

func (s *Service) handleTrigger(c *gin.Context) {
	id, ok := parseTaskID(c)
	if !ok {
		return
	}

	triggerType := c.DefaultQuery("triggerType", "manual")

	run, err := s.TriggerExecution(c.Request.Context(), id, triggerType)
	if err != nil {
		if errors.Is(err, ErrTaskNotFound) {
			respondError(c, http.StatusNotFound, "task_not_found")
			return
		}
		if errors.Is(err, ErrTaskAlreadyCompleted) {
			respondError(c, http.StatusConflict, "task_already_completed")
			return
		}
		if errors.Is(err, ErrScheduleNotAllowed) {
			respondError(c, http.StatusConflict, "execution_not_allowed_not_ready")
			return
		}
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}

	task, _ := s.Get(id)
	c.JSON(http.StatusOK, gin.H{
		"message": "execution_triggered",
		"task":    task,
		"run":     run,
	})
}

func (s *Service) handleGetSessionIDs(c *gin.Context) {
	var projectID uint
	if pidStr := c.Query("projectId"); pidStr != "" {
		if val, err := strconv.ParseUint(pidStr, 10, 64); err == nil {
			projectID = uint(val)
		}
	}

	ids, err := s.GetTaskSessionIDs(projectID)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	if ids == nil {
		ids = []string{}
	}
	c.JSON(http.StatusOK, gin.H{"sessionIds": ids})
}

