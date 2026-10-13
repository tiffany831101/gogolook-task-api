package repository

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tiffany831101/gogolook-task-api/internal/model"
)

// listAll returns every stored task, ignoring the total.
func listAll(repo *MemoryTaskRepository) ([]model.Task, error) {
	tasks, _, err := repo.List(context.Background(), ListParams{})
	return tasks, err
}

func TestMemoryTaskRepository_List(t *testing.T) {
	ctx := context.Background()

	t.Run("empty repository returns empty non-nil slice", func(t *testing.T) {
		repo := NewMemoryTaskRepository()

		tasks, err := listAll(repo)

		require.NoError(t, err)
		assert.NotNil(t, tasks)
		assert.Empty(t, tasks)
	})

	t.Run("returns tasks ordered by id", func(t *testing.T) {
		repo := NewMemoryTaskRepository()
		for _, name := range []string{"a", "b", "c"} {
			_, err := repo.Create(ctx, model.Task{Name: name})
			require.NoError(t, err)
		}

		tasks, err := listAll(repo)

		require.NoError(t, err)
		require.Len(t, tasks, 3)
		assert.Equal(t, []int64{1, 2, 3}, []int64{tasks[0].ID, tasks[1].ID, tasks[2].ID})
	})

	t.Run("returned slice is a copy", func(t *testing.T) {
		repo := NewMemoryTaskRepository()
		_, err := repo.Create(ctx, model.Task{Name: "original"})
		require.NoError(t, err)

		tasks, _ := listAll(repo)
		tasks[0].Name = "mutated"

		again, _ := listAll(repo)
		assert.Equal(t, "original", again[0].Name)
	})
}

// newRepoWith returns a repository holding n tasks with IDs 1..n.
func newRepoWith(t *testing.T, n int) *MemoryTaskRepository {
	t.Helper()
	repo := NewMemoryTaskRepository()
	for i := 0; i < n; i++ {
		_, err := repo.Create(context.Background(), model.Task{Name: "t"})
		require.NoError(t, err)
	}
	return repo
}

func taskIDs(tasks []model.Task) []int64 {
	out := make([]int64, 0, len(tasks))
	for _, task := range tasks {
		out = append(out, task.ID)
	}
	return out
}

func TestMemoryTaskRepository_ListPagination(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name    string
		params  ListParams
		wantIDs []int64
	}{
		{name: "first page", params: ListParams{Offset: 0, Limit: 2}, wantIDs: []int64{1, 2}},
		{name: "middle page", params: ListParams{Offset: 2, Limit: 2}, wantIDs: []int64{3, 4}},
		{name: "last partial page", params: ListParams{Offset: 4, Limit: 2}, wantIDs: []int64{5}},
		{name: "offset past the end", params: ListParams{Offset: 10, Limit: 2}, wantIDs: []int64{}},
		{name: "limit larger than store", params: ListParams{Offset: 0, Limit: 100}, wantIDs: []int64{1, 2, 3, 4, 5}},
		{name: "zero limit means no limit", params: ListParams{Offset: 1}, wantIDs: []int64{2, 3, 4, 5}},
		{name: "negative offset treated as zero", params: ListParams{Offset: -3, Limit: 1}, wantIDs: []int64{1}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := newRepoWith(t, 5)

			tasks, total, err := repo.List(ctx, tc.params)

			require.NoError(t, err)
			assert.Equal(t, 5, total)
			assert.NotNil(t, tasks)
			assert.Equal(t, tc.wantIDs, taskIDs(tasks))
		})
	}

	t.Run("order and total stay correct after deletes", func(t *testing.T) {
		repo := newRepoWith(t, 5)
		require.NoError(t, repo.Delete(ctx, 2))
		require.NoError(t, repo.Delete(ctx, 5))
		_, err := repo.Create(ctx, model.Task{Name: "new"})
		require.NoError(t, err)

		tasks, total, err := repo.List(ctx, ListParams{})

		require.NoError(t, err)
		assert.Equal(t, 4, total)
		assert.Equal(t, []int64{1, 3, 4, 6}, taskIDs(tasks))
	})
}

func TestMemoryTaskRepository_Create(t *testing.T) {
	ctx := context.Background()
	repo := NewMemoryTaskRepository()

	first, err := repo.Create(ctx, model.Task{Name: "first", Status: model.StatusIncomplete})
	require.NoError(t, err)
	second, err := repo.Create(ctx, model.Task{ID: 999, Name: "second", Status: model.StatusCompleted})
	require.NoError(t, err)

	assert.Equal(t, model.Task{ID: 1, Name: "first", Status: model.StatusIncomplete}, first)
	assert.Equal(t, int64(2), second.ID, "caller-supplied ID must be ignored")
}

func TestMemoryTaskRepository_Update(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name    string
		input   model.Task
		want    model.Task
		wantErr error
	}{
		{
			name:  "happy: existing task is replaced",
			input: model.Task{ID: 1, Name: "updated", Status: model.StatusCompleted},
			want:  model.Task{ID: 1, Name: "updated", Status: model.StatusCompleted},
		},
		{
			name:    "unhappy: unknown id returns ErrNotFound",
			input:   model.Task{ID: 42, Name: "ghost"},
			wantErr: ErrNotFound,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := NewMemoryTaskRepository()
			_, err := repo.Create(ctx, model.Task{Name: "original"})
			require.NoError(t, err)

			got, err := repo.Update(ctx, tc.input)

			if tc.wantErr != nil {
				assert.ErrorIs(t, err, tc.wantErr)
				all, _ := listAll(repo)
				assert.Len(t, all, 1, "failed update must not create a task")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
			all, _ := listAll(repo)
			assert.Equal(t, []model.Task{tc.want}, all)
		})
	}
}

func TestMemoryTaskRepository_Delete(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name      string
		id        int64
		wantErr   error
		remaining int
	}{
		{name: "happy: existing task is removed", id: 1, remaining: 0},
		{name: "unhappy: unknown id returns ErrNotFound", id: 42, wantErr: ErrNotFound, remaining: 1},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := NewMemoryTaskRepository()
			_, err := repo.Create(ctx, model.Task{Name: "task"})
			require.NoError(t, err)

			err = repo.Delete(ctx, tc.id)

			if tc.wantErr != nil {
				assert.ErrorIs(t, err, tc.wantErr)
			} else {
				assert.NoError(t, err)
			}
			all, _ := listAll(repo)
			assert.Len(t, all, tc.remaining)
		})
	}

	t.Run("unhappy: deleting twice returns ErrNotFound", func(t *testing.T) {
		repo := NewMemoryTaskRepository()
		created, _ := repo.Create(ctx, model.Task{Name: "task"})

		require.NoError(t, repo.Delete(ctx, created.ID))
		assert.ErrorIs(t, repo.Delete(ctx, created.ID), ErrNotFound)
	})
}

// TestMemoryTaskRepository_Concurrency hammers the repository from many
// goroutines. Run with `go test -race` to prove there is no data race on the
// underlying map; without the mutex this would also trip Go's runtime
// "concurrent map writes" detector.
func TestMemoryTaskRepository_Concurrency(t *testing.T) {
	ctx := context.Background()
	repo := NewMemoryTaskRepository()

	const workers = 50
	const perWorker = 20

	var wg sync.WaitGroup
	ids := make(chan int64, workers*perWorker)

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perWorker; i++ {
				task, err := repo.Create(ctx, model.Task{Name: "t"})
				if !assert.NoError(t, err) {
					return
				}
				ids <- task.ID

				if i%5 == 0 {
					_, _ = listAll(repo)
				}
				_, _ = repo.Update(ctx, model.Task{ID: task.ID, Name: "u", Status: model.StatusCompleted})
			}
		}()
	}
	wg.Wait()
	close(ids)

	seen := make(map[int64]struct{}, workers*perWorker)
	for id := range ids {
		_, dup := seen[id]
		require.False(t, dup, "duplicate id %d issued", id)
		seen[id] = struct{}{}
	}
	assert.Len(t, seen, workers*perWorker)

	all, err := listAll(repo)
	require.NoError(t, err)
	assert.Len(t, all, workers*perWorker)

	// Concurrently delete everything, with racing readers.
	n := 0
	for id := range seen {
		wg.Add(1)
		go func(id int64) {
			defer wg.Done()
			assert.NoError(t, repo.Delete(ctx, id))
		}(id)
		if n%20 == 0 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, _ = listAll(repo)
			}()
		}
		n++
	}
	wg.Wait()

	all, _ = listAll(repo)
	assert.Empty(t, all)
}

// TestMemoryTaskRepository_ConcurrentDeleteSameID verifies that exactly one of
// many racing deletes for the same ID succeeds.
func TestMemoryTaskRepository_ConcurrentDeleteSameID(t *testing.T) {
	ctx := context.Background()
	repo := NewMemoryTaskRepository()
	task, _ := repo.Create(ctx, model.Task{Name: "contended"})

	const racers = 50
	var wg sync.WaitGroup
	var mu sync.Mutex
	successes := 0

	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := repo.Delete(ctx, task.ID); err == nil {
				mu.Lock()
				successes++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	assert.Equal(t, 1, successes)
}
