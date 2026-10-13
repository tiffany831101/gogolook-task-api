package repository

import (
	"context"
	"slices"
	"sync"

	"github.com/tiffany831101/gogolook-task-api/internal/model"
)

// MemoryTaskRepository is a concurrency-safe, in-memory TaskRepository.
//
// Go maps are not safe for concurrent use: a concurrent read and write can
// crash the process with "fatal error: concurrent map read and map write".
// Every access to tasks, ids and nextID is therefore guarded by mu. An
// RWMutex is used so that the read-heavy List endpoint can be served by many
// readers in parallel, while writes remain exclusive.
//
// Tasks are stored and returned by value, so callers never hold a pointer
// into the map and cannot mutate shared state outside the lock.
type MemoryTaskRepository struct {
	mu    sync.RWMutex
	tasks map[int64]model.Task
	// ids holds every stored ID in ascending order so List can serve a page
	// without copying and sorting the whole map. IDs are issued
	// monotonically, so Create only ever appends.
	ids    []int64
	nextID int64
}

// NewMemoryTaskRepository returns an empty repository. IDs start at 1.
func NewMemoryTaskRepository() *MemoryTaskRepository {
	return &MemoryTaskRepository{
		tasks:  make(map[int64]model.Task),
		nextID: 1,
	}
}

// List returns one page of tasks ordered by ID ascending, plus the total
// number of tasks. The cost is proportional to the page size, not the store.
func (r *MemoryTaskRepository) List(_ context.Context, params ListParams) ([]model.Task, int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	total := len(r.ids)
	offset := max(params.Offset, 0)
	if offset >= total {
		return []model.Task{}, total, nil
	}

	end := total
	if params.Limit > 0 && params.Limit < total-offset {
		end = offset + params.Limit
	}

	page := make([]model.Task, 0, end-offset)
	for _, id := range r.ids[offset:end] {
		page = append(page, r.tasks[id])
	}
	return page, total, nil
}

// Create assigns a new unique ID to task and stores it.
func (r *MemoryTaskRepository) Create(_ context.Context, task model.Task) (model.Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	task.ID = r.nextID
	r.nextID++
	r.tasks[task.ID] = task
	r.ids = append(r.ids, task.ID)
	return task, nil
}

// Update replaces an existing task. The existence check and the write happen
// under the same lock, so a concurrent Delete cannot resurrect the task.
func (r *MemoryTaskRepository) Update(_ context.Context, task model.Task) (model.Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.tasks[task.ID]; !ok {
		return model.Task{}, ErrNotFound
	}
	r.tasks[task.ID] = task
	return task, nil
}

// Delete removes the task with the given ID.
func (r *MemoryTaskRepository) Delete(_ context.Context, id int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.tasks[id]; !ok {
		return ErrNotFound
	}
	delete(r.tasks, id)
	if i, found := slices.BinarySearch(r.ids, id); found {
		r.ids = slices.Delete(r.ids, i, i+1)
	}
	return nil
}
