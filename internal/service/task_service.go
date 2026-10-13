package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/tiffany831101/gogolook-task-api/internal/model"
	"github.com/tiffany831101/gogolook-task-api/internal/repository"
)

const (
	// MaxNameLength bounds the task name to keep memory usage predictable.
	MaxNameLength = 255
	// DefaultListLimit is the page size used when the client does not ask for one.
	DefaultListLimit = 100
	// MaxListLimit caps the page size so a single request cannot return the whole store.
	MaxListLimit = 1000
)

var (
	// ErrInvalidName is returned when the name is empty or too long.
	ErrInvalidName = errors.New("name must be a non-empty string of at most 255 characters")
	// ErrInvalidStatus is returned when the status is not 0 or 1.
	ErrInvalidStatus = errors.New("status must be 0 (incomplete) or 1 (completed)")
	// ErrInvalidLimit is returned when the page size is out of range.
	ErrInvalidLimit = fmt.Errorf("limit must be an integer between 1 and %d", MaxListLimit)
	// ErrInvalidOffset is returned when the page offset is negative.
	ErrInvalidOffset = errors.New("offset must be a non-negative integer")
	// ErrNotFound is returned when the requested task does not exist.
	ErrNotFound = repository.ErrNotFound
)

// TaskService holds the business rules for tasks.
type TaskService struct {
	repo repository.TaskRepository
}

// NewTaskService builds a TaskService on top of repo.
func NewTaskService(repo repository.TaskRepository) *TaskService {
	return &TaskService{repo: repo}
}

// List returns one page of tasks ordered by ID, plus the total number of tasks.
func (s *TaskService) List(ctx context.Context, offset, limit int) ([]model.Task, int, error) {
	if limit < 1 || limit > MaxListLimit {
		return nil, 0, ErrInvalidLimit
	}
	if offset < 0 {
		return nil, 0, ErrInvalidOffset
	}
	return s.repo.List(ctx, repository.ListParams{Offset: offset, Limit: limit})
}

// Create validates and stores a new task.
func (s *TaskService) Create(ctx context.Context, name string, status model.TaskStatus) (model.Task, error) {
	task, err := buildTask(0, name, status)
	if err != nil {
		return model.Task{}, err
	}
	return s.repo.Create(ctx, task)
}

// Update validates and replaces the task identified by id.
func (s *TaskService) Update(ctx context.Context, id int64, name string, status model.TaskStatus) (model.Task, error) {
	task, err := buildTask(id, name, status)
	if err != nil {
		return model.Task{}, err
	}
	return s.repo.Update(ctx, task)
}

// Delete removes the task identified by id.
func (s *TaskService) Delete(ctx context.Context, id int64) error {
	return s.repo.Delete(ctx, id)
}

func buildTask(id int64, name string, status model.TaskStatus) (model.Task, error) {
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > MaxNameLength {
		return model.Task{}, ErrInvalidName
	}
	if !status.Valid() {
		return model.Task{}, ErrInvalidStatus
	}
	return model.Task{ID: id, Name: name, Status: status}, nil
}
