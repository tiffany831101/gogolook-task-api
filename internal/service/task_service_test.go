package service

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tiffany831101/gogolook-task-api/internal/model"
	"github.com/tiffany831101/gogolook-task-api/internal/repository"
)

func newService() *TaskService {
	return NewTaskService(repository.NewMemoryTaskRepository())
}

func TestTaskService_List(t *testing.T) {
	tests := []struct {
		name      string
		offset    int
		limit     int
		wantNames []string
		wantErr   error
	}{
		{name: "happy: first page", offset: 0, limit: 2, wantNames: []string{"a", "b"}},
		{name: "happy: second page", offset: 2, limit: 2, wantNames: []string{"c"}},
		{name: "happy: max limit", offset: 0, limit: MaxListLimit, wantNames: []string{"a", "b", "c"}},
		{name: "unhappy: zero limit", offset: 0, limit: 0, wantErr: ErrInvalidLimit},
		{name: "unhappy: negative limit", offset: 0, limit: -1, wantErr: ErrInvalidLimit},
		{name: "unhappy: limit above max", offset: 0, limit: MaxListLimit + 1, wantErr: ErrInvalidLimit},
		{name: "unhappy: negative offset", offset: -1, limit: 10, wantErr: ErrInvalidOffset},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := newService()
			for _, name := range []string{"a", "b", "c"} {
				_, err := svc.Create(context.Background(), name, model.StatusIncomplete)
				require.NoError(t, err)
			}

			tasks, total, err := svc.List(context.Background(), tc.offset, tc.limit)

			if tc.wantErr != nil {
				assert.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, 3, total)
			names := make([]string, 0, len(tasks))
			for _, task := range tasks {
				names = append(names, task.Name)
			}
			assert.Equal(t, tc.wantNames, names)
		})
	}
}

func TestTaskService_Create(t *testing.T) {
	tests := []struct {
		name     string
		taskName string
		status   model.TaskStatus
		want     model.Task
		wantErr  error
	}{
		{
			name:     "happy: incomplete task",
			taskName: "buy milk",
			status:   model.StatusIncomplete,
			want:     model.Task{ID: 1, Name: "buy milk", Status: model.StatusIncomplete},
		},
		{
			name:     "happy: completed task",
			taskName: "buy milk",
			status:   model.StatusCompleted,
			want:     model.Task{ID: 1, Name: "buy milk", Status: model.StatusCompleted},
		},
		{
			name:     "happy: name is trimmed",
			taskName: "  buy milk  ",
			status:   model.StatusIncomplete,
			want:     model.Task{ID: 1, Name: "buy milk", Status: model.StatusIncomplete},
		},
		{
			name:     "happy: name at max length (multibyte counted as one)",
			taskName: strings.Repeat("任", MaxNameLength),
			status:   model.StatusIncomplete,
			want:     model.Task{ID: 1, Name: strings.Repeat("任", MaxNameLength), Status: model.StatusIncomplete},
		},
		{name: "unhappy: empty name", taskName: "", wantErr: ErrInvalidName},
		{name: "unhappy: whitespace-only name", taskName: "   ", wantErr: ErrInvalidName},
		{name: "unhappy: name too long", taskName: strings.Repeat("a", MaxNameLength+1), wantErr: ErrInvalidName},
		{name: "unhappy: status 2", taskName: "x", status: 2, wantErr: ErrInvalidStatus},
		{name: "unhappy: negative status", taskName: "x", status: -1, wantErr: ErrInvalidStatus},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := newService()

			got, err := svc.Create(context.Background(), tc.taskName, tc.status)

			if tc.wantErr != nil {
				assert.ErrorIs(t, err, tc.wantErr)
				all, _, _ := svc.List(context.Background(), 0, MaxListLimit)
				assert.Empty(t, all, "invalid input must not be persisted")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestTaskService_Update(t *testing.T) {
	tests := []struct {
		name     string
		id       int64
		taskName string
		status   model.TaskStatus
		want     model.Task
		wantErr  error
	}{
		{
			name:     "happy: rename and complete",
			id:       1,
			taskName: "renamed",
			status:   model.StatusCompleted,
			want:     model.Task{ID: 1, Name: "renamed", Status: model.StatusCompleted},
		},
		{name: "unhappy: not found", id: 99, taskName: "x", status: model.StatusCompleted, wantErr: ErrNotFound},
		{name: "unhappy: empty name", id: 1, taskName: "", status: model.StatusCompleted, wantErr: ErrInvalidName},
		{name: "unhappy: invalid status", id: 1, taskName: "x", status: 5, wantErr: ErrInvalidStatus},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := newService()
			original, err := svc.Create(context.Background(), "original", model.StatusIncomplete)
			require.NoError(t, err)

			got, err := svc.Update(context.Background(), tc.id, tc.taskName, tc.status)

			if tc.wantErr != nil {
				assert.ErrorIs(t, err, tc.wantErr)
				all, _, _ := svc.List(context.Background(), 0, MaxListLimit)
				assert.Equal(t, []model.Task{original}, all, "failed update must leave data untouched")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestTaskService_Delete(t *testing.T) {
	t.Run("happy: existing task", func(t *testing.T) {
		svc := newService()
		task, _ := svc.Create(context.Background(), "x", model.StatusIncomplete)

		assert.NoError(t, svc.Delete(context.Background(), task.ID))
	})

	t.Run("unhappy: not found", func(t *testing.T) {
		svc := newService()

		assert.ErrorIs(t, svc.Delete(context.Background(), 1), ErrNotFound)
	})
}
