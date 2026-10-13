package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/tiffany831101/gogolook-task-api/internal/model"
	"github.com/tiffany831101/gogolook-task-api/internal/service"
)

// createTaskRequest is the body of POST /tasks.
// Pointers distinguish "field missing" from the zero value; status is
// optional on create and defaults to 0 (incomplete).
type createTaskRequest struct {
	Name   *string           `json:"name" binding:"required"`
	Status *model.TaskStatus `json:"status"`
}

// updateTaskRequest is the body of PUT /tasks/{id}.
// PUT replaces the resource, so every field is required.
type updateTaskRequest struct {
	Name   *string           `json:"name" binding:"required"`
	Status *model.TaskStatus `json:"status" binding:"required"`
}

// ErrorResponse is the body returned for every non-2xx response.
// Field names the offending request field when the error is about one.
type ErrorResponse struct {
	Error string `json:"error"`
	Field string `json:"field,omitempty"`
}

// TaskHandler exposes TaskService over HTTP.
type TaskHandler struct {
	svc *service.TaskService
}

// NewTaskHandler builds a TaskHandler.
func NewTaskHandler(svc *service.TaskService) *TaskHandler {
	return &TaskHandler{svc: svc}
}

// List handles GET /tasks?offset=&limit=.
// The total number of tasks is returned in the X-Total-Count header.
func (h *TaskHandler) List(c *gin.Context) {
	offset, ok := queryInt(c, "offset", 0, service.ErrInvalidOffset)
	if !ok {
		return
	}
	limit, ok := queryInt(c, "limit", service.DefaultListLimit, service.ErrInvalidLimit)
	if !ok {
		return
	}

	tasks, total, err := h.svc.List(c.Request.Context(), offset, limit)
	if err != nil {
		writeError(c, err)
		return
	}
	c.Header("X-Total-Count", strconv.Itoa(total))
	c.JSON(http.StatusOK, tasks)
}

// Create handles POST /tasks.
func (h *TaskHandler) Create(c *gin.Context) {
	var req createTaskRequest
	if err := bindJSON(c, &req); err != nil {
		c.JSON(bindErrorResponse(err))
		return
	}

	status := model.StatusIncomplete
	if req.Status != nil {
		status = *req.Status
	}

	task, err := h.svc.Create(c.Request.Context(), *req.Name, status)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, task)
}

// Update handles PUT /tasks/:id.
func (h *TaskHandler) Update(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}

	var req updateTaskRequest
	if err := bindJSON(c, &req); err != nil {
		c.JSON(bindErrorResponse(err))
		return
	}

	task, err := h.svc.Update(c.Request.Context(), id, *req.Name, *req.Status)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, task)
}

// Delete handles DELETE /tasks/:id.
func (h *TaskHandler) Delete(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}

	if err := h.svc.Delete(c.Request.Context(), id); err != nil {
		writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func parseID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "id must be a positive integer"})
		return 0, false
	}
	return id, true
}

// queryInt reads an optional integer query parameter, falling back to def
// when it is absent. A present but non-integer value is rejected with invalid.
func queryInt(c *gin.Context, key string, def int, invalid error) (int, bool) {
	raw, ok := c.GetQuery(key)
	if !ok {
		return def, true
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: invalid.Error(), Field: key})
		return 0, false
	}
	return v, true
}

// validationErrFields maps service validation errors to the request field
// they concern.
var validationErrFields = []struct {
	err   error
	field string
}{
	{service.ErrInvalidName, "name"},
	{service.ErrInvalidStatus, "status"},
	{service.ErrInvalidLimit, "limit"},
	{service.ErrInvalidOffset, "offset"},
}

func writeError(c *gin.Context, err error) {
	for _, v := range validationErrFields {
		if errors.Is(err, v.err) {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error(), Field: v.field})
			return
		}
	}

	if errors.Is(err, service.ErrNotFound) {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
		return
	}
	_ = c.Error(err)
	c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "internal server error"})
}
