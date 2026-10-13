package model

// TaskStatus represents the completion state of a task.
type TaskStatus int

const (
	// StatusIncomplete represents an incomplete task.
	StatusIncomplete TaskStatus = 0
	// StatusCompleted represents a completed task.
	StatusCompleted TaskStatus = 1
)

// Valid reports whether s is one of the allowed enum values [0, 1].
func (s TaskStatus) Valid() bool {
	return s == StatusIncomplete || s == StatusCompleted
}

// Task is the domain entity managed by the API.
type Task struct {
	ID     int64      `json:"id"`
	Name   string     `json:"name"`
	Status TaskStatus `json:"status"`
}
