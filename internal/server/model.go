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

	ServiceDeskRequests []serviceDeskRequestResponse `json:"service_desk_requests"`
}

// serviceDeskRequestResponse is a Service Desk request linked to a task.
type serviceDeskRequestResponse struct {
	Key       string    `json:"key"`
	Title     string    `json:"title"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	StatusAt  time.Time `json:"status_at"`
}

type createTaskResponse struct {
	ID string `json:"id"`
}

// todoRequest is the body of POST /todos and PUT /todos/{id}.
type todoRequest struct {
	TaskID      string      `json:"task_id"`
	Description string      `json:"description"`
	IsDone      bool        `json:"is_done"`
	StartDate   *string     `json:"start_date"`
	EndDate     *string     `json:"end_date"`
	Reminders   []time.Time `json:"reminders"`
	Position    *int        `json:"position"`
}

type todoResponse struct {
	ID          int64       `json:"id"`
	TaskID      string      `json:"task_id"`
	Description string      `json:"description"`
	IsDone      bool        `json:"is_done"`
	StartDate   *string     `json:"start_date"`
	EndDate     *string     `json:"end_date"`
	Reminders   []time.Time `json:"reminders"`
	Position    *int        `json:"position"`
}

type todoListResponse struct {
	Todos []todoResponse `json:"todos"`
}

type createTodoResponse struct {
	ID int64 `json:"id"`
}
