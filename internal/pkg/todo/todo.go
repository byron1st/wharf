package todo

import "time"

// Todo is a flat work item that belongs to one task.
type Todo struct {
	ID          int64
	TaskID      string
	Description string
	IsDone      bool
	// StartDate and EndDate are calendar dates as "YYYY-MM-DD", or empty for
	// none. Text keeps them free of time zones.
	StartDate string
	EndDate   string
	// Reminders are distinct instants in ascending order.
	Reminders []time.Time
	// Position is the index among the todos of the task. When writing, nil
	// means the end of the task, or the current place when the task is kept.
	Position *int
}

// Filter selects todos; an empty or nil field does not filter.
type Filter struct {
	TaskID string
	// Today keeps todos that start on or before it, or have no start date.
	Today string
	// EndDate keeps todos that end on it.
	EndDate string
	IsDone  *bool
}
