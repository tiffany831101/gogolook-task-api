package handler_test

import (
	"context"
	"fmt"
	"math/rand/v2"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/tiffany831101/gogolook-task-api/internal/handler"
	"github.com/tiffany831101/gogolook-task-api/internal/model"
	"github.com/tiffany831101/gogolook-task-api/internal/repository"
	"github.com/tiffany831101/gogolook-task-api/internal/router"
	"github.com/tiffany831101/gogolook-task-api/internal/service"
)

const benchStoreSize = 1_000

func newBenchServer(b *testing.B) *gin.Engine {
	b.Helper()
	repo := repository.NewMemoryTaskRepository()
	for i := 0; i < benchStoreSize; i++ {
		if _, err := repo.Create(context.Background(), model.Task{Name: "task"}); err != nil {
			b.Fatal(err)
		}
	}
	return router.New(handler.NewTaskHandler(service.NewTaskService(repo)))
}

// BenchmarkAPI measures each endpoint through the full gin stack (routing,
// JSON binding, validation, storage, JSON encoding) with concurrent clients.
// It excludes real network I/O, so it shows the server's own overhead.
func BenchmarkAPI(b *testing.B) {
	type request struct {
		method, path, body string
		wantStatus         int
	}

	endpoints := []struct {
		name string
		next func() request
	}{
		{
			name: "GET /tasks",
			next: func() request {
				return request{http.MethodGet, "/tasks?limit=100", "", http.StatusOK}
			},
		},
		{
			name: "POST /tasks",
			next: func() request {
				return request{http.MethodPost, "/tasks", `{"name":"bench","status":0}`, http.StatusCreated}
			},
		},
		{
			name: "PUT /tasks/:id",
			next: func() request {
				path := fmt.Sprintf("/tasks/%d", rand.IntN(benchStoreSize)+1)
				return request{http.MethodPut, path, `{"name":"bench","status":1}`, http.StatusOK}
			},
		},
		{
			name: "mixed 90% read",
			next: func() request {
				if rand.IntN(100) < 90 {
					return request{http.MethodGet, "/tasks?limit=100", "", http.StatusOK}
				}
				path := fmt.Sprintf("/tasks/%d", rand.IntN(benchStoreSize)+1)
				return request{http.MethodPut, path, `{"name":"bench","status":1}`, http.StatusOK}
			},
		},
	}

	for _, ep := range endpoints {
		b.Run(ep.name, func(b *testing.B) {
			r := newBenchServer(b)

			b.ReportAllocs()
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					req := ep.next()
					if w := doRequest(r, req.method, req.path, req.body); w.Code != req.wantStatus {
						b.Errorf("%s %s: got %d, want %d", req.method, req.path, w.Code, req.wantStatus)
						return
					}
				}
			})
		})
	}
}
