package service

import "github.com/byron1st/wharf/internal/pkg/task"

// SyncResult is the outcome of a sync that stored the tasks it received.
type SyncResult struct {
	// Upserted is the number of tasks stored.
	Upserted int
	// Partial joins the errors of items that failed to load or convert. Items
	// that failed to convert are still stored. It is nil when every item synced.
	Partial error
}

// TaskDetail is a task with its relations and the stored tasks they point at.
type TaskDetail struct {
	// Task holds the task fields and the related IDs, which may include
	// tasks that are not stored.
	Task task.Task
	// Parent is the stored parent, or nil when there is none or it is not
	// stored.
	Parent *task.Task
	// Children and Linked are the stored tasks among Task.ChildrenIDs and
	// Task.LinkedIDs, in the same order.
	Children []task.Task
	Linked   []task.Task
}
