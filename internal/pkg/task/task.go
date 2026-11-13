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
	// ClosedAt is when the task was last closed, and nil while it is open or
	// when the time is unknown. Writes use it as db.Manager describes.
	ClosedAt *time.Time
	// ClosedSyncedAt is the time of the full Atlassian sync that saw the
	// stored, open task closed, and nil otherwise. Writes use it as
	// db.Manager.UpsertTasks describes.
	ClosedSyncedAt *time.Time
}
