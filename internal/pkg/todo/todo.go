package todo

import (
	"time"

	"github.com/byron1st/wharf/internal/pkg/servicedesk"
)

// Todo is a flat work item that belongs to one task.
type Todo struct {
	ID          int64
	TaskID      string
	Description string
	// Note is free text kept only in wharf.
	Note   string
	IsDone bool
	// CompletedAt is when the todo was marked done, or nil while it is not
	// done. Storage sets it; it is ignored when writing.
	CompletedAt *time.Time
	// StartDate and EndDate are calendar dates as "YYYY-MM-DD", or empty for
	// none. Text keeps them free of time zones.
	StartDate string
	EndDate   string
	// Reminders are distinct instants in ascending order.
	Reminders []time.Time
	// Position is the index among the todos of the task. When writing, nil
	// means the end of the task, or the current place when the task is kept.
	Position *int
	// ServiceDeskRequests are the stored requests linked to the todo, in issue
	// key order. They are ignored when writing.
	ServiceDeskRequests []servicedesk.Request
}

// Filter selects todos; an empty or nil field does not filter.
type Filter struct {
	TaskID string
	IsDone *bool
	// CompletedSince keeps todos completed at or after it.
	CompletedSince *time.Time
	// ServiceDeskRequest keeps todos linked to the request with this key.
	ServiceDeskRequest string
}

// Count is how many todos a task has, and how many of them are done.
type Count struct {
	Todos int
	Done  int
}
