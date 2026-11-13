package service

import (
	"github.com/byron1st/wharf/internal/pkg/atlassiansync"
	"github.com/byron1st/wharf/internal/pkg/task"
	"github.com/byron1st/wharf/internal/pkg/todo"
)

// AtlassianSync is the outcome of SyncAtlassian.
type AtlassianSync struct {
	Record atlassiansync.Record
	// Errors are the messages of the issues and requests that failed to read,
	// issues first.
	Errors []string
}

// TaskSync is the outcome of SyncJiraTask.
type TaskSync struct {
	// Task is the task as stored now, without relations.
	Task task.Task
	// Changes names the fields that differ from the copy stored before:
	// title, description, status, start_date, and due_date, in that order.
	Changes []string
}

// TaskSummary is a stored task with how many todos it has.
type TaskSummary struct {
	Task  task.Task
	Todos todo.Count
	// ClosedSinceLastSync is whether the latest Atlassian sync saw the task,
	// stored and open before, close.
	ClosedSinceLastSync bool
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
