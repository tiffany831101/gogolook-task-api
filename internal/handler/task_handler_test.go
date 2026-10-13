package handler_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tiffany831101/gogolook-task-api/internal/handler"
	"github.com/tiffany831101/gogolook-task-api/internal/model"
	"github.com/tiffany831101/gogolook-task-api/internal/repository"
	"github.com/tiffany831101/gogolook-task-api/internal/router"
	"github.com/tiffany831101/gogolook-task-api/internal/service"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	gin.DefaultWriter = io.Discard
	os.Exit(m.Run())
}

// newTestServer returns a router backed by repo, pre-seeded with seed tasks.
func newTestServer(t *testing.T, repo repository.TaskRepository, seed ...model.Task) *gin.Engine {
	t.Helper()
	for _, task := range seed {
		_, err := repo.Create(context.Background(), task)
		require.NoError(t, err)
	}
	return router.New(handler.NewTaskHandler(service.NewTaskService(repo)))
}

func doRequest(r http.Handler, method, path, body string) *httptest.ResponseRecorder {
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func decode[T any](t *testing.T, w *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &v), "body: %s", w.Body.String())
	return v
}

// assertNoInternalsLeaked guards against Go type names or validator output
// reaching API clients.
func assertNoInternalsLeaked(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	for _, leak := range []string{"createTaskRequest", "updateTaskRequest", "model.", "Go struct", "Key: '"} {
		assert.NotContains(t, w.Body.String(), leak)
	}
}

// failingRepo simulates a broken storage backend to exercise the 500 path.
type failingRepo struct{}

var errBoom = errors.New("storage unavailable")

func (failingRepo) List(context.Context, repository.ListParams) ([]model.Task, int, error) {
	return nil, 0, errBoom
}
func (failingRepo) Create(context.Context, model.Task) (model.Task, error) {
	return model.Task{}, errBoom
}
func (failingRepo) Update(context.Context, model.Task) (model.Task, error) {
	return model.Task{}, errBoom
}
func (failingRepo) Delete(context.Context, int64) error { return errBoom }

var seedTasks = []model.Task{
	{Name: "first", Status: model.StatusIncomplete},
	{Name: "second", Status: model.StatusCompleted},
}

func TestListTasks(t *testing.T) {
	t.Run("happy: empty list returns [] not null", func(t *testing.T) {
		r := newTestServer(t, repository.NewMemoryTaskRepository())

		w := doRequest(r, http.MethodGet, "/tasks", "")

		assert.Equal(t, http.StatusOK, w.Code)
		assert.JSONEq(t, `[]`, w.Body.String())
	})

	t.Run("happy: returns all tasks ordered by id", func(t *testing.T) {
		r := newTestServer(t, repository.NewMemoryTaskRepository(), seedTasks...)

		w := doRequest(r, http.MethodGet, "/tasks", "")

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "2", w.Header().Get("X-Total-Count"))
		assert.JSONEq(t, `[
			{"id":1,"name":"first","status":0},
			{"id":2,"name":"second","status":1}
		]`, w.Body.String())
	})

	t.Run("happy: default page size is applied", func(t *testing.T) {
		repo := repository.NewMemoryTaskRepository()
		seed := make([]model.Task, service.DefaultListLimit+5)
		for i := range seed {
			seed[i] = model.Task{Name: fmt.Sprintf("t%d", i)}
		}
		r := newTestServer(t, repo, seed...)

		w := doRequest(r, http.MethodGet, "/tasks", "")

		require.Equal(t, http.StatusOK, w.Code)
		assert.Len(t, decode[[]model.Task](t, w), service.DefaultListLimit)
		assert.Equal(t, strconv.Itoa(service.DefaultListLimit+5), w.Header().Get("X-Total-Count"))
	})

	pageTests := []struct {
		name     string
		query    string
		wantBody string
	}{
		{name: "happy: limit", query: "?limit=1", wantBody: `[{"id":1,"name":"first","status":0}]`},
		{name: "happy: offset and limit", query: "?offset=1&limit=1", wantBody: `[{"id":2,"name":"second","status":1}]`},
		{name: "happy: offset past the end", query: "?offset=5", wantBody: `[]`},
	}
	for _, tc := range pageTests {
		t.Run(tc.name, func(t *testing.T) {
			r := newTestServer(t, repository.NewMemoryTaskRepository(), seedTasks...)

			w := doRequest(r, http.MethodGet, "/tasks"+tc.query, "")

			require.Equal(t, http.StatusOK, w.Code)
			assert.Equal(t, "2", w.Header().Get("X-Total-Count"))
			assert.JSONEq(t, tc.wantBody, w.Body.String())
		})
	}

	limitErr := handler.ErrorResponse{Error: service.ErrInvalidLimit.Error(), Field: "limit"}
	offsetErr := handler.ErrorResponse{Error: service.ErrInvalidOffset.Error(), Field: "offset"}
	badQueryTests := []struct {
		name    string
		query   string
		wantErr handler.ErrorResponse
	}{
		{name: "unhappy: non-numeric limit", query: "?limit=abc", wantErr: limitErr},
		{name: "unhappy: empty limit", query: "?limit=", wantErr: limitErr},
		{name: "unhappy: zero limit", query: "?limit=0", wantErr: limitErr},
		{name: "unhappy: negative limit", query: "?limit=-1", wantErr: limitErr},
		{name: "unhappy: limit above max", query: fmt.Sprintf("?limit=%d", service.MaxListLimit+1), wantErr: limitErr},
		{name: "unhappy: non-numeric offset", query: "?offset=abc", wantErr: offsetErr},
		{name: "unhappy: negative offset", query: "?offset=-1", wantErr: offsetErr},
	}
	for _, tc := range badQueryTests {
		t.Run(tc.name, func(t *testing.T) {
			r := newTestServer(t, repository.NewMemoryTaskRepository(), seedTasks...)

			w := doRequest(r, http.MethodGet, "/tasks"+tc.query, "")

			require.Equal(t, http.StatusBadRequest, w.Code)
			assert.Equal(t, tc.wantErr, decode[handler.ErrorResponse](t, w))
		})
	}

	t.Run("unhappy: storage failure returns 500", func(t *testing.T) {
		r := newTestServer(t, failingRepo{})

		w := doRequest(r, http.MethodGet, "/tasks", "")

		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.Equal(t, "internal server error", decode[handler.ErrorResponse](t, w).Error)
	})
}

func TestCreateTask(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantBody   string                // exact JSON for success cases
		wantErr    handler.ErrorResponse // exact error body for failure cases
	}{
		{
			name:       "happy: incomplete task",
			body:       `{"name":"buy milk","status":0}`,
			wantStatus: http.StatusCreated,
			wantBody:   `{"id":1,"name":"buy milk","status":0}`,
		},
		{
			name:       "happy: completed task",
			body:       `{"name":"buy milk","status":1}`,
			wantStatus: http.StatusCreated,
			wantBody:   `{"id":1,"name":"buy milk","status":1}`,
		},
		{
			name:       "happy: status omitted defaults to 0",
			body:       `{"name":"buy milk"}`,
			wantStatus: http.StatusCreated,
			wantBody:   `{"id":1,"name":"buy milk","status":0}`,
		},
		{
			name:       "happy: trailing whitespace is allowed",
			body:       "{\"name\":\"buy milk\",\"status\":1}\n  ",
			wantStatus: http.StatusCreated,
			wantBody:   `{"id":1,"name":"buy milk","status":1}`,
		},
		{
			name:       "happy: unknown fields and client id are ignored",
			body:       `{"id":99,"name":"buy milk","status":1,"extra":true}`,
			wantStatus: http.StatusCreated,
			wantBody:   `{"id":1,"name":"buy milk","status":1}`,
		},
		{name: "unhappy: empty body", body: ``, wantStatus: http.StatusBadRequest, wantErr: handler.ErrorResponse{Error: "request body is required"}},
		{name: "unhappy: malformed json", body: `{"name":`, wantStatus: http.StatusBadRequest, wantErr: handler.ErrorResponse{Error: "request body must be valid JSON"}},
		{name: "unhappy: json array", body: `[]`, wantStatus: http.StatusBadRequest, wantErr: handler.ErrorResponse{Error: "request body must be a JSON object"}},
		{name: "unhappy: missing name", body: `{"status":0}`, wantStatus: http.StatusBadRequest, wantErr: handler.ErrorResponse{Error: "name is required", Field: "name"}},
		{name: "unhappy: null name", body: `{"name":null,"status":0}`, wantStatus: http.StatusBadRequest, wantErr: handler.ErrorResponse{Error: "name is required", Field: "name"}},
		{name: "unhappy: empty name", body: `{"name":"","status":0}`, wantStatus: http.StatusBadRequest, wantErr: handler.ErrorResponse{Error: "name must be a non-empty string of at most 255 characters", Field: "name"}},
		{name: "unhappy: whitespace name", body: `{"name":"   ","status":0}`, wantStatus: http.StatusBadRequest, wantErr: handler.ErrorResponse{Error: "name must be a non-empty string of at most 255 characters", Field: "name"}},
		{
			name:       "unhappy: name too long",
			body:       fmt.Sprintf(`{"name":%q,"status":0}`, strings.Repeat("a", service.MaxNameLength+1)),
			wantStatus: http.StatusBadRequest,
			wantErr:    handler.ErrorResponse{Error: "name must be a non-empty string of at most 255 characters", Field: "name"},
		},
		{name: "unhappy: name wrong type", body: `{"name":123,"status":0}`, wantStatus: http.StatusBadRequest, wantErr: handler.ErrorResponse{Error: "name must be a string", Field: "name"}},
		{name: "unhappy: status out of enum", body: `{"name":"x","status":2}`, wantStatus: http.StatusBadRequest, wantErr: handler.ErrorResponse{Error: "status must be 0 (incomplete) or 1 (completed)", Field: "status"}},
		{name: "unhappy: negative status", body: `{"name":"x","status":-1}`, wantStatus: http.StatusBadRequest, wantErr: handler.ErrorResponse{Error: "status must be 0 (incomplete) or 1 (completed)", Field: "status"}},
		{name: "unhappy: status as string", body: `{"name":"x","status":"1"}`, wantStatus: http.StatusBadRequest, wantErr: handler.ErrorResponse{Error: "status must be an integer (0 or 1)", Field: "status"}},
		{name: "unhappy: status as float", body: `{"name":"x","status":0.5}`, wantStatus: http.StatusBadRequest, wantErr: handler.ErrorResponse{Error: "status must be an integer (0 or 1)", Field: "status"}},
		{name: "unhappy: status as bool", body: `{"name":"x","status":true}`, wantStatus: http.StatusBadRequest, wantErr: handler.ErrorResponse{Error: "status must be an integer (0 or 1)", Field: "status"}},
		{name: "unhappy: trailing garbage", body: `{"name":"x","status":0}xyz`, wantStatus: http.StatusBadRequest, wantErr: handler.ErrorResponse{Error: "request body must contain a single JSON object"}},
		{name: "unhappy: two JSON objects", body: `{"name":"x","status":0} {"name":"y"}`, wantStatus: http.StatusBadRequest, wantErr: handler.ErrorResponse{Error: "request body must contain a single JSON object"}},
		{
			name:       "unhappy: body over size limit",
			body:       fmt.Sprintf(`{"name":%q,"status":0}`, strings.Repeat("a", router.MaxBodyBytes)),
			wantStatus: http.StatusRequestEntityTooLarge,
			wantErr:    handler.ErrorResponse{Error: fmt.Sprintf("request body must not exceed %d bytes", router.MaxBodyBytes)},
		},
		{
			name:       "unhappy: oversized trailing data",
			body:       `{"name":"x","status":0}` + strings.Repeat(" ", router.MaxBodyBytes),
			wantStatus: http.StatusRequestEntityTooLarge,
			wantErr:    handler.ErrorResponse{Error: fmt.Sprintf("request body must not exceed %d bytes", router.MaxBodyBytes)},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := newTestServer(t, repository.NewMemoryTaskRepository())

			w := doRequest(r, http.MethodPost, "/tasks", tc.body)

			require.Equal(t, tc.wantStatus, w.Code, "body: %s", w.Body.String())
			if tc.wantBody != "" {
				assert.JSONEq(t, tc.wantBody, w.Body.String())
				return
			}
			assert.Equal(t, tc.wantErr, decode[handler.ErrorResponse](t, w))
			assertNoInternalsLeaked(t, w)

			// A rejected request must not create anything.
			list := doRequest(r, http.MethodGet, "/tasks", "")
			assert.JSONEq(t, `[]`, list.Body.String())
		})
	}

	t.Run("happy: created task appears in list", func(t *testing.T) {
		r := newTestServer(t, repository.NewMemoryTaskRepository())

		doRequest(r, http.MethodPost, "/tasks", `{"name":"a","status":0}`)
		doRequest(r, http.MethodPost, "/tasks", `{"name":"b","status":1}`)
		w := doRequest(r, http.MethodGet, "/tasks", "")

		assert.JSONEq(t, `[{"id":1,"name":"a","status":0},{"id":2,"name":"b","status":1}]`, w.Body.String())
	})

	t.Run("unhappy: storage failure returns 500", func(t *testing.T) {
		r := newTestServer(t, failingRepo{})

		w := doRequest(r, http.MethodPost, "/tasks", `{"name":"x","status":0}`)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestUpdateTask(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		body       string
		wantStatus int
		wantBody   string
		wantErr    handler.ErrorResponse
	}{
		{
			name:       "happy: rename and complete",
			path:       "/tasks/1",
			body:       `{"name":"renamed","status":1}`,
			wantStatus: http.StatusOK,
			wantBody:   `{"id":1,"name":"renamed","status":1}`,
		},
		{
			name:       "happy: mark completed task incomplete",
			path:       "/tasks/2",
			body:       `{"name":"second","status":0}`,
			wantStatus: http.StatusOK,
			wantBody:   `{"id":2,"name":"second","status":0}`,
		},
		{
			name:       "happy: id in body is ignored in favour of path",
			path:       "/tasks/1",
			body:       `{"id":2,"name":"renamed","status":1}`,
			wantStatus: http.StatusOK,
			wantBody:   `{"id":1,"name":"renamed","status":1}`,
		},
		{name: "unhappy: task not found", path: "/tasks/999", body: `{"name":"x","status":1}`, wantStatus: http.StatusNotFound, wantErr: handler.ErrorResponse{Error: "task not found"}},
		{name: "unhappy: non-numeric id", path: "/tasks/abc", body: `{"name":"x","status":1}`, wantStatus: http.StatusBadRequest, wantErr: handler.ErrorResponse{Error: "id must be a positive integer"}},
		{name: "unhappy: zero id", path: "/tasks/0", body: `{"name":"x","status":1}`, wantStatus: http.StatusBadRequest, wantErr: handler.ErrorResponse{Error: "id must be a positive integer"}},
		{name: "unhappy: negative id", path: "/tasks/-1", body: `{"name":"x","status":1}`, wantStatus: http.StatusBadRequest, wantErr: handler.ErrorResponse{Error: "id must be a positive integer"}},
		{name: "unhappy: id overflows int64", path: "/tasks/99999999999999999999", body: `{"name":"x","status":1}`, wantStatus: http.StatusBadRequest, wantErr: handler.ErrorResponse{Error: "id must be a positive integer"}},
		{name: "unhappy: malformed json", path: "/tasks/1", body: `{bad`, wantStatus: http.StatusBadRequest, wantErr: handler.ErrorResponse{Error: "request body must be valid JSON"}},
		{name: "unhappy: empty body", path: "/tasks/1", body: ``, wantStatus: http.StatusBadRequest, wantErr: handler.ErrorResponse{Error: "request body is required"}},
		{name: "unhappy: missing name", path: "/tasks/1", body: `{"status":1}`, wantStatus: http.StatusBadRequest, wantErr: handler.ErrorResponse{Error: "name is required", Field: "name"}},
		{name: "unhappy: missing status (PUT is full replace)", path: "/tasks/1", body: `{"name":"x"}`, wantStatus: http.StatusBadRequest, wantErr: handler.ErrorResponse{Error: "status is required", Field: "status"}},
		{name: "unhappy: empty name", path: "/tasks/1", body: `{"name":"","status":1}`, wantStatus: http.StatusBadRequest, wantErr: handler.ErrorResponse{Error: "name must be a non-empty string of at most 255 characters", Field: "name"}},
		{name: "unhappy: status out of enum", path: "/tasks/1", body: `{"name":"x","status":3}`, wantStatus: http.StatusBadRequest, wantErr: handler.ErrorResponse{Error: "status must be 0 (incomplete) or 1 (completed)", Field: "status"}},
		{name: "unhappy: status as string", path: "/tasks/1", body: `{"name":"x","status":"0"}`, wantStatus: http.StatusBadRequest, wantErr: handler.ErrorResponse{Error: "status must be an integer (0 or 1)", Field: "status"}},
		{name: "unhappy: trailing garbage", path: "/tasks/1", body: `{"name":"x","status":1}xyz`, wantStatus: http.StatusBadRequest, wantErr: handler.ErrorResponse{Error: "request body must contain a single JSON object"}},
	}

	original := `[{"id":1,"name":"first","status":0},{"id":2,"name":"second","status":1}]`

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := newTestServer(t, repository.NewMemoryTaskRepository(), seedTasks...)

			w := doRequest(r, http.MethodPut, tc.path, tc.body)

			require.Equal(t, tc.wantStatus, w.Code, "body: %s", w.Body.String())
			if tc.wantBody != "" {
				assert.JSONEq(t, tc.wantBody, w.Body.String())
				return
			}
			assert.Equal(t, tc.wantErr, decode[handler.ErrorResponse](t, w))
			assertNoInternalsLeaked(t, w)

			// A rejected request must leave existing data untouched.
			list := doRequest(r, http.MethodGet, "/tasks", "")
			assert.JSONEq(t, original, list.Body.String())
		})
	}

	t.Run("happy: update is visible in list", func(t *testing.T) {
		r := newTestServer(t, repository.NewMemoryTaskRepository(), seedTasks...)

		doRequest(r, http.MethodPut, "/tasks/1", `{"name":"done","status":1}`)
		w := doRequest(r, http.MethodGet, "/tasks", "")

		assert.JSONEq(t, `[{"id":1,"name":"done","status":1},{"id":2,"name":"second","status":1}]`, w.Body.String())
	})

	t.Run("unhappy: storage failure returns 500", func(t *testing.T) {
		r := newTestServer(t, failingRepo{})

		w := doRequest(r, http.MethodPut, "/tasks/1", `{"name":"x","status":0}`)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestDeleteTask(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		wantStatus int
		wantList   string
		wantErr    handler.ErrorResponse
	}{
		{
			name:       "happy: existing task",
			path:       "/tasks/1",
			wantStatus: http.StatusNoContent,
			wantList:   `[{"id":2,"name":"second","status":1}]`,
		},
		{
			name:       "unhappy: task not found",
			path:       "/tasks/999",
			wantStatus: http.StatusNotFound,
			wantList:   `[{"id":1,"name":"first","status":0},{"id":2,"name":"second","status":1}]`,
			wantErr:    handler.ErrorResponse{Error: "task not found"},
		},
		{
			name:       "unhappy: non-numeric id",
			path:       "/tasks/abc",
			wantStatus: http.StatusBadRequest,
			wantList:   `[{"id":1,"name":"first","status":0},{"id":2,"name":"second","status":1}]`,
			wantErr:    handler.ErrorResponse{Error: "id must be a positive integer"},
		},
		{
			name:       "unhappy: zero id",
			path:       "/tasks/0",
			wantStatus: http.StatusBadRequest,
			wantList:   `[{"id":1,"name":"first","status":0},{"id":2,"name":"second","status":1}]`,
			wantErr:    handler.ErrorResponse{Error: "id must be a positive integer"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := newTestServer(t, repository.NewMemoryTaskRepository(), seedTasks...)

			w := doRequest(r, http.MethodDelete, tc.path, "")

			require.Equal(t, tc.wantStatus, w.Code, "body: %s", w.Body.String())
			if tc.wantErr.Error != "" {
				assert.Equal(t, tc.wantErr, decode[handler.ErrorResponse](t, w))
				assertNoInternalsLeaked(t, w)
			} else {
				assert.Empty(t, w.Body.String())
			}
			list := doRequest(r, http.MethodGet, "/tasks", "")
			assert.JSONEq(t, tc.wantList, list.Body.String())
		})
	}

	t.Run("unhappy: deleting twice returns 404", func(t *testing.T) {
		r := newTestServer(t, repository.NewMemoryTaskRepository(), seedTasks...)

		assert.Equal(t, http.StatusNoContent, doRequest(r, http.MethodDelete, "/tasks/1", "").Code)
		assert.Equal(t, http.StatusNotFound, doRequest(r, http.MethodDelete, "/tasks/1", "").Code)
	})

	t.Run("unhappy: storage failure returns 500", func(t *testing.T) {
		r := newTestServer(t, failingRepo{})

		w := doRequest(r, http.MethodDelete, "/tasks/1", "")

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestUnsupportedRoutes(t *testing.T) {
	r := newTestServer(t, repository.NewMemoryTaskRepository())

	tests := []struct {
		method     string
		path       string
		wantStatus int
		wantErr    string
		wantAllow  string
	}{
		{http.MethodGet, "/tasks/1", http.StatusMethodNotAllowed, "method not allowed", "PUT, DELETE"},
		{http.MethodPatch, "/tasks/1", http.StatusMethodNotAllowed, "method not allowed", "PUT, DELETE"},
		{http.MethodPut, "/tasks", http.StatusMethodNotAllowed, "method not allowed", "GET, POST"},
		{http.MethodDelete, "/tasks", http.StatusMethodNotAllowed, "method not allowed", "GET, POST"},
		{http.MethodGet, "/unknown", http.StatusNotFound, "route not found", ""},
		{http.MethodGet, "/tasks/1/extra", http.StatusNotFound, "route not found", ""},
	}
	for _, tc := range tests {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			w := doRequest(r, tc.method, tc.path, "")

			assert.Equal(t, tc.wantStatus, w.Code)
			assert.Equal(t, tc.wantAllow, w.Header().Get("Allow"))
			assert.Equal(t, handler.ErrorResponse{Error: tc.wantErr}, decode[handler.ErrorResponse](t, w))
		})
	}
}

// TestConcurrentRequests drives the full HTTP stack from many goroutines.
// Combined with `go test -race`, it verifies the API is safe under load.
func TestConcurrentRequests(t *testing.T) {
	r := newTestServer(t, repository.NewMemoryTaskRepository())

	const workers = 50
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()

			w := doRequest(r, http.MethodPost, "/tasks", fmt.Sprintf(`{"name":"task-%d","status":0}`, i))
			if !assert.Equal(t, http.StatusCreated, w.Code) {
				return
			}
			var created model.Task
			if !assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &created)) {
				return
			}
			path := fmt.Sprintf("/tasks/%d", created.ID)

			assert.Equal(t, http.StatusOK, doRequest(r, http.MethodGet, "/tasks", "").Code)
			assert.Equal(t, http.StatusOK, doRequest(r, http.MethodPut, path, `{"name":"done","status":1}`).Code)
			if i%2 == 0 {
				assert.Equal(t, http.StatusNoContent, doRequest(r, http.MethodDelete, path, "").Code)
			}
		}(i)
	}
	wg.Wait()

	w := doRequest(r, http.MethodGet, fmt.Sprintf("/tasks?limit=%d", service.MaxListLimit), "")
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, strconv.Itoa(workers/2), w.Header().Get("X-Total-Count"))
	tasks := decode[[]model.Task](t, w)
	assert.Len(t, tasks, workers/2)
	for _, task := range tasks {
		assert.Equal(t, model.StatusCompleted, task.Status)
		assert.Equal(t, "done", task.Name)
	}
}
