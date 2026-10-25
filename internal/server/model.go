package server

import (
	"time"

	"github.com/byron1st/wharf/internal/pkg/task"
)

type syncResponse struct {
	Upserted int  `json:"upserted"`
	Partial  bool `json:"partial"`
}

type errorResponse struct {
	Error string `json:"error"`
}

// taskRequest is the body of POST /tasks and PUT /tasks/{id}.
type taskRequest struct {
	Title       string      `json:"title"`
	Description string      `json:"description"`
	Status      task.Status `json:"status"`
	StartDate   *time.Time  `json:"start_date"`
	DueDate     *time.Time  `json:"due_date"`
	ParentID    string      `json:"parent_id"`
	ChildrenIDs []string    `json:"children_ids"`
	LinkedIDs   []string    `json:"linked_ids"`
}

type taskSummary struct {
	ID          string      `json:"id"`
	Originated  task.Origin `json:"originated"`
	Title       string      `json:"title"`
	Description string      `json:"description"`
	Status      task.Status `json:"status"`
	StartDate   *time.Time  `json:"start_date"`
	DueDate     *time.Time  `json:"due_date"`
}

type taskListResponse struct {
	Tasks []taskSummary `json:"tasks"`
}

// taskDetailResponse carries the full description, unlike the summaries.
type taskDetailResponse struct {
	taskSummary

	ParentID    *string       `json:"parent_id"`
	ChildrenIDs []string      `json:"children_ids"`
	LinkedIDs   []string      `json:"linked_ids"`
	Parent      *taskSummary  `json:"parent"`
	Children    []taskSummary `json:"children"`
	Linked      []taskSummary `json:"linked"`
}

type createTaskResponse struct {
	ID string `json:"id"`
}
