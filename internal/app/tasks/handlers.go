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
		tasks.GET("/session-ids", s.handleGetSessionIDs)
		tasks.GET("/:id", s.handleGet)
		tasks.PUT("/:id", s.handleUpdate)
		tasks.DELETE("/:id", s.handleDelete)
		tasks.GET("/:id/runs", s.handleListRuns)
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

func (s *Service) handleList(c *gin.Context) {
	var filter ListFilter
	if pStr := c.Query("projectId"); pStr != "" {
		if pID, err := strconv.ParseUint(pStr, 10, 64); err == nil && pID > 0 {
			val := uint(pID)
			filter.ProjectID = &val
		}
	}
	if tStr := c.Query("taskType"); tStr != "" {
		tt := TaskType(tStr)
		filter.TaskType = &tt
	}
	if sStr := c.Query("status"); sStr != "" {
		st := TaskStatus(sStr)
		filter.Status = &st
	}
	if kw := c.Query("keyword"); kw != "" {
		filter.Keyword = kw
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
		if errors.Is(err, ErrFlowTasksNotImplementedYet) {
			respondError(c, http.StatusBadRequest, "flow_tasks_not_implemented_yet")
			return
		}
		if errors.Is(err, ErrInvalidTaskType) {
			respondError(c, http.StatusBadRequest, "invalid_task_type")
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

	err := s.Delete(id)
	if err != nil {
		if errors.Is(err, ErrTaskNotFound) {
			respondError(c, http.StatusNotFound, "task_not_found")
			return
		}
		respondError(c, http.StatusInternalServerError, "server_error")
		return
	}

	c.Status(http.StatusNoContent)
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

type triggerRequest struct {
	TriggerType string `json:"triggerType"`
}

func (s *Service) handleTrigger(c *gin.Context) {
	id, ok := parseTaskID(c)
	if !ok {
		return
	}

	var req triggerRequest
	_ = c.ShouldBindJSON(&req)

	trigType := "manual"
	if req.TriggerType != "" {
		trigType = req.TriggerType
	}

	run, err := s.TriggerExecution(c.Request.Context(), id, trigType)
	if err != nil {
		if errors.Is(err, ErrTaskNotFound) {
			respondError(c, http.StatusNotFound, "task_not_found")
			return
		}
		if errors.Is(err, ErrFlowTasksNotImplementedYet) {
			respondError(c, http.StatusBadRequest, "flow_tasks_not_implemented_yet")
			return
		}
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{"run": run})
}

func (s *Service) handleGetSessionIDs(c *gin.Context) {
	var projectID uint
	if pStr := c.Query("projectId"); pStr != "" {
		if val, err := strconv.ParseUint(pStr, 10, 64); err == nil {
			projectID = uint(val)
		}
	}

	ids, err := s.GetTaskSessionIDs(projectID)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "server_error")
		return
	}

	c.JSON(http.StatusOK, gin.H{"sessionIds": ids})
}
