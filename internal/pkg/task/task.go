package task

import "time"

type Task struct {
	ID          string
	Originated  Origin
	Title       string
	Description string
	Status      Status
	StartDate   *time.Time
	DueDate     *time.Time
	ParentID    string
	ChildrenIDs []string
	LinkedIDs   []string
	// Position orders tasks on Today. It is nil until the tasks are first
	// reordered.
	Position *int
}
