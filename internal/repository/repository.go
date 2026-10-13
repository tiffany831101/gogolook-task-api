package repository

import (
	"context"
	"errors"

	"github.com/tiffany831101/gogolook-task-api/internal/model"
)

// ErrNotFound is returned when a task with the given ID does not exist.
var ErrNotFound = errors.New("task not found")

// ListParams selects a page of tasks ordered by ID ascending.
// A Limit of zero or less means no limit.
type ListParams struct {
	Offset int
	Limit  int
}

// TaskRepository abstracts task persistence so the in-memory store can be
// swapped for a real database without touching the service or handlers.
type TaskRepository interface {
	// List returns the requested page and the total number of tasks.
	List(ctx context.Context, params ListParams) (tasks []model.Task, total int, err error)
	Create(ctx context.Context, task model.Task) (model.Task, error)
	Update(ctx context.Context, task model.Task) (model.Task, error)
	Delete(ctx context.Context, id int64) error
}
