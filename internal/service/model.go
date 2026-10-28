package service

import (
	"github.com/byron1st/wharf/internal/pkg/servicedesk"
	"github.com/byron1st/wharf/internal/pkg/task"
)

// SyncResult is the outcome of a sync that stored what its source returned.
type SyncResult struct {
	// Upserted is how many items were passed to storage.
	Upserted int
	// Partial joins item errors from the source. It is nil when every returned
	// item was accepted by the source. Whether a failed item was still stored
	// depends on the sync method.
	Partial error
}

// TaskDetail is a task with its relations, the stored tasks they point at,
// and its linked Service Desk requests.
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
	// ServiceDeskRequests are the stored Service Desk requests linked to the
	// task, in issue key order.
	ServiceDeskRequests []servicedesk.Request
}
