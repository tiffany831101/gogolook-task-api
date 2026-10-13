package router

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/tiffany831101/gogolook-task-api/internal/handler"
)

// MaxBodyBytes caps request bodies. Task payloads are tiny, so 1 MiB is
// generous while stopping clients from making the server buffer huge bodies.
const MaxBodyBytes = 1 << 20

// New wires the task routes onto a fresh gin engine.
func New(h *handler.TaskHandler) *gin.Engine {
	r := gin.New()
	r.HandleMethodNotAllowed = true
	r.Use(gin.Logger(), gin.Recovery(), limitBody(MaxBodyBytes))

	r.NoRoute(func(c *gin.Context) {
		c.JSON(http.StatusNotFound, handler.ErrorResponse{Error: "route not found"})
	})
	r.NoMethod(func(c *gin.Context) {
		c.JSON(http.StatusMethodNotAllowed, handler.ErrorResponse{Error: "method not allowed"})
	})

	tasks := r.Group("/tasks")
	{
		tasks.GET("", h.List)
		tasks.POST("", h.Create)
		tasks.PUT("/:id", h.Update)
		tasks.DELETE("/:id", h.Delete)
	}
	return r
}

// limitBody makes reads past n bytes fail with *http.MaxBytesError, which
// the handlers turn into 413 Request Entity Too Large.
func limitBody(n int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, n)
		c.Next()
	}
}
