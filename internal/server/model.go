package server

import (
	"time"

	"github.com/byron1st/wharf/internal/pkg/task"
)

// errorResponse carries a code when the client must tell the error apart
// from others with the same status.
type errorResponse struct {
	Error string `json:"error"`
	Code  string `json:"code,omitempty"`
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

// taskStatusRequest is the body of PUT /tasks/{id}/status.
type taskStatusRequest struct {
	Status task.Status `json:"status"`
}

// taskOrderRequest is the body of PUT /tasks/order.
type taskOrderRequest struct {
	IDs []string `json:"ids"`
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

// taskListItem is a summary with the task's todo counts and its position on
// Today, for GET /api/tasks.
type taskListItem struct {
	taskSummary

	TodoCount     int  `json:"todo_count"`
	DoneTodoCount int  `json:"done_todo_count"`
	Position      *int `json:"position"`
}

type taskListResponse struct {
	Tasks []taskListItem `json:"tasks"`
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

// serviceDeskRequestResponse is a Service Desk request.
type serviceDeskRequestResponse struct {
	Key            string    `json:"key"`
	Title          string    `json:"title"`
	Status         string    `json:"status"`
	StatusCategory string    `json:"status_category"`
	CreatedAt      time.Time `json:"created_at"`
	StatusAt       time.Time `json:"status_at"`
}

type createTaskResponse struct {
	ID string `json:"id"`
}

// todoRequest is the body of POST /todos and PUT /todos/{id}.
type todoRequest struct {
	TaskID      string      `json:"task_id"`
	Description string      `json:"description"`
	Note        string      `json:"note"`
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
	Note        string      `json:"note"`
	IsDone      bool        `json:"is_done"`
	CompletedAt *time.Time  `json:"completed_at"`
	StartDate   *string     `json:"start_date"`
	EndDate     *string     `json:"end_date"`
	Reminders   []time.Time `json:"reminders"`
	Position    *int        `json:"position"`

	ServiceDeskRequests []serviceDeskRequestResponse `json:"service_desk_requests"`
}

type todoListResponse struct {
	Todos []todoResponse `json:"todos"`
}

type createTodoResponse struct {
	ID int64 `json:"id"`
}

type serviceDeskRequestListResponse struct {
	Requests []serviceDeskRequestResponse `json:"requests"`
}

type atlassianSyncResponse struct {
	SyncedAt        time.Time `json:"synced_at"`
	Issues          int       `json:"issues"`
	IssueFailures   int       `json:"issue_failures"`
	OpenRequests    int       `json:"open_requests"`
	ClosedRequests  int       `json:"closed_requests"`
	RequestFailures int       `json:"request_failures"`
	// Errors is only in the POST response.
	Errors []string `json:"errors,omitempty"`
}

type lastAtlassianSyncResponse struct {
	LastSync *atlassianSyncResponse `json:"last_sync"`
}

type taskSyncResponse struct {
	Task    taskSummary `json:"task"`
	Changes []string    `json:"changes"`
}

// atlassianSettingsBody is the body of GET and PUT /api/settings/atlassian
// and of POST /api/settings/atlassian/test.
type atlassianSettingsBody struct {
	SiteURL  string `json:"site_url"`
	Email    string `json:"email"`
	APIToken string `json:"api_token"`
}

type atlassianCheckResponse struct {
	CheckedAt time.Time `json:"checked_at"`
}

type appSettingsResponse struct {
	ConfigPath        string  `json:"config_path"`
	PendingConfigPath *string `json:"pending_config_path"`
	Addr              string  `json:"addr"`
	DBPath            string  `json:"db_path"`
	Version           string  `json:"version"`
}

type chosenConfigResponse struct {
	PendingConfigPath *string `json:"pending_config_path"`
}
