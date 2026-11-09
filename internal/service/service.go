package service

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/byron1st/wharf/internal/adapter/atlassian"
	"github.com/byron1st/wharf/internal/adapter/db"
	"github.com/byron1st/wharf/internal/pkg/atlassiansync"
	"github.com/byron1st/wharf/internal/pkg/servicedesk"
	"github.com/byron1st/wharf/internal/pkg/task"
	"github.com/byron1st/wharf/internal/pkg/todo"
)

var (
	ErrEmptyTaskTitle       = errors.New("service: task title is empty")
	ErrUnknownTaskStatus    = errors.New("service: unknown task status")
	ErrEmptyRelatedTaskID   = errors.New("service: related task id is empty")
	ErrTaskReferencesItself = errors.New("service: task cannot reference itself")
	ErrTaskNotFound         = errors.New("service: task not found")
	ErrTaskToUpdateNotFound = errors.New("service: task to update not found")
	// ErrLocalTaskToUpdateNotFound is returned when the task passed the
	// up-front check but was deleted or stopped being local before the write.
	ErrLocalTaskToUpdateNotFound        = errors.New("service: local task to update not found")
	ErrTaskNotLocal                     = errors.New("service: only local tasks can be updated")
	ErrTaskToDeleteNotFound             = errors.New("service: task to delete not found")
	ErrUnknownTaskStatusToSet           = errors.New("service: unknown task status to set")
	ErrLocalTaskToSetStatusNotFound     = errors.New("service: local task to set status not found")
	ErrEmptyTaskIDToReorder             = errors.New("service: task id to reorder is empty")
	ErrDuplicateTaskIDToReorder         = errors.New("service: task id to reorder is duplicated")
	ErrTodoToLinkNotFound               = errors.New("service: todo to link not found")
	ErrServiceDeskRequestToLinkNotFound = errors.New(
		"service: service desk request to link not found",
	)
	ErrServiceDeskRequestLinkToDeleteNotFound = errors.New(
		"service: service desk request link to delete not found",
	)
)

type Service interface {
	SyncAtlassian(ctx context.Context) (AtlassianSync, error)
	GetLastAtlassianSync(ctx context.Context) (atlassiansync.Record, error)
	SyncJiraTask(ctx context.Context, id string) (TaskSync, error)
	ListServiceDeskRequests(ctx context.Context, includeClosed bool) ([]servicedesk.Request, error)
	ListTasks(ctx context.Context) ([]TaskSummary, error)
	GetTask(ctx context.Context, id string) (TaskDetail, error)
	CreateTask(ctx context.Context, t task.Task) (string, error)
	UpdateTask(ctx context.Context, t task.Task) error
	DeleteTask(ctx context.Context, id string) error
	UpdateTaskStatus(ctx context.Context, id string, status task.Status) error
	ReorderTasks(ctx context.Context, ids []string) error
	LinkServiceDeskRequest(ctx context.Context, todoID int64, key string) error
	UnlinkServiceDeskRequest(ctx context.Context, todoID int64, key string) error
	ListTodos(ctx context.Context, f todo.Filter) ([]todo.Todo, error)
	CreateTodo(ctx context.Context, t todo.Todo) (int64, error)
	UpdateTodo(ctx context.Context, t todo.Todo) error
	DeleteTodo(ctx context.Context, id int64) error
}

type service struct {
	jira atlassian.Client
	db   db.Manager
	now  func() time.Time
}

// NewService returns a Service that syncs Atlassian data into db.
func NewService(jira atlassian.Client, db db.Manager) Service {
	return &service{jira: jira, db: db, now: time.Now}
}

// ListTasks returns every stored task in ID order, without relations, each
// with its todo counts.
func (s *service) ListTasks(ctx context.Context) ([]TaskSummary, error) {
	tasks, err := s.db.ListTasks(ctx)
	if err != nil {
		return nil, err
	}

	counts, err := s.db.CountTodosByTask(ctx)
	if err != nil {
		return nil, err
	}

	summaries := make([]TaskSummary, 0, len(tasks))
	for _, t := range tasks {
		summaries = append(summaries, TaskSummary{Task: t, Todos: counts[t.ID]})
	}

	return summaries, nil
}

// GetTask returns the task with id, its relations, and the stored tasks they
// point at. It fails with ErrTaskNotFound when no task has id.
func (s *service) GetTask(ctx context.Context, id string) (TaskDetail, error) {
	t, err := s.db.GetTask(ctx, id)
	if errors.Is(err, db.ErrTaskNotFound) {
		return TaskDetail{}, fmt.Errorf("%w: id=%s", ErrTaskNotFound, id)
	}

	if err != nil {
		return TaskDetail{}, err
	}

	var ids []string
	if t.ParentID != "" {
		ids = append(ids, t.ParentID)
	}

	ids = append(ids, t.ChildrenIDs...)
	ids = append(ids, t.LinkedIDs...)

	stored := map[string]task.Task{}

	if len(ids) > 0 {
		related, err := s.db.ListTasksByIDs(ctx, ids)
		if err != nil {
			return TaskDetail{}, err
		}

		for _, r := range related {
			stored[r.ID] = r
		}
	}

	detail := TaskDetail{
		Task:     t,
		Children: pick(stored, t.ChildrenIDs),
		Linked:   pick(stored, t.LinkedIDs),
	}
	if parent, ok := stored[t.ParentID]; ok {
		detail.Parent = &parent
	}

	return detail, nil
}

// CreateTask validates t and stores it as a new local task with its
// relations, returning its ID. t.ID and t.Originated are ignored, and an empty
// status becomes todo.
func (s *service) CreateTask(ctx context.Context, t task.Task) (string, error) {
	t, err := normalize(t)
	if err != nil {
		return "", err
	}

	return s.db.CreateLocalTask(ctx, t)
}

// UpdateTask validates t and replaces the stored local task t.ID with it,
// including its parent, children, and links. It fails with
// ErrTaskReferencesItself when a relation points at t.ID, with
// ErrTaskToUpdateNotFound when no task has t.ID, with ErrTaskNotLocal when
// the task is not local, since a sync would overwrite it, and with
// ErrLocalTaskToUpdateNotFound when it stops being a local task before the
// write.
func (s *service) UpdateTask(ctx context.Context, t task.Task) error {
	t, err := normalize(t)
	if err != nil {
		return err
	}

	if t.ParentID == t.ID || slices.Contains(t.ChildrenIDs, t.ID) || slices.Contains(t.LinkedIDs, t.ID) {
		return fmt.Errorf("%w: id=%s", ErrTaskReferencesItself, t.ID)
	}

	// The check reads before UpdateLocalTask writes, so a task deleted in
	// between fails there with ErrLocalTaskToUpdateNotFound instead.
	current, err := s.db.GetTask(ctx, t.ID)
	if errors.Is(err, db.ErrTaskNotFound) {
		return fmt.Errorf("%w: id=%s", ErrTaskToUpdateNotFound, t.ID)
	}

	if err != nil {
		return err
	}

	if current.Originated != task.OriginLocal {
		return fmt.Errorf("%w: id=%s, originated=%s", ErrTaskNotLocal, t.ID, current.Originated)
	}

	err = s.db.UpdateLocalTask(ctx, t)
	if errors.Is(err, db.ErrLocalTaskToUpdateNotFound) {
		return fmt.Errorf("%w: id=%s", ErrLocalTaskToUpdateNotFound, t.ID)
	}

	return err
}

// DeleteTask deletes the task with id and its relations, whatever its origin.
// It fails with ErrTaskToDeleteNotFound when no task has id.
func (s *service) DeleteTask(ctx context.Context, id string) error {
	err := s.db.DeleteTask(ctx, id)
	if errors.Is(err, db.ErrTaskToDeleteNotFound) {
		return fmt.Errorf("%w: id=%s", ErrTaskToDeleteNotFound, id)
	}

	return err
}

// UpdateTaskStatus sets the status of the local task id, leaving its other
// fields and relations as they are. It fails with ErrUnknownTaskStatusToSet
// for a status other than todo, in-progress, done, and canceled, and with
// ErrLocalTaskToSetStatusNotFound when no local task has id, since a sync
// would overwrite the status of any other task.
func (s *service) UpdateTaskStatus(ctx context.Context, id string, status task.Status) error {
	switch status {
	case task.StatusTodo, task.StatusInProgress, task.StatusDone, task.StatusCanceled:
	default:
		return fmt.Errorf("%w: status=%s", ErrUnknownTaskStatusToSet, status)
	}

	err := s.db.UpdateLocalTaskStatus(ctx, id, status)
	if errors.Is(err, db.ErrLocalTaskToSetStatusNotFound) {
		return fmt.Errorf("%w: id=%s", ErrLocalTaskToSetStatusNotFound, id)
	}

	return err
}

// ReorderTasks orders the tasks on Today as ids lists them, whatever their
// origin. ids should list every stored task: IDs that are not stored are
// skipped, and tasks not in ids keep their positions, which may then tie
// with listed ones. It fails with ErrEmptyTaskIDToReorder for a blank ID and
// with ErrDuplicateTaskIDToReorder for an ID listed twice.
func (s *service) ReorderTasks(ctx context.Context, ids []string) error {
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		if isBlank(id) {
			return ErrEmptyTaskIDToReorder
		}

		if seen[id] {
			return fmt.Errorf("%w: id=%s", ErrDuplicateTaskIDToReorder, id)
		}

		seen[id] = true
	}

	return s.db.SetTaskPositions(ctx, ids)
}

// LinkServiceDeskRequest links the stored todo todoID to the stored Service
// Desk request key. Linking them again is a no-op. It fails with
// ErrTodoToLinkNotFound when no todo has todoID and with
// ErrServiceDeskRequestToLinkNotFound when no request has key.
func (s *service) LinkServiceDeskRequest(ctx context.Context, todoID int64, key string) error {
	err := s.db.LinkServiceDeskRequest(ctx, todoID, key)
	if errors.Is(err, db.ErrTodoToLinkNotFound) {
		return fmt.Errorf("%w: todo_id=%d", ErrTodoToLinkNotFound, todoID)
	}

	if errors.Is(err, db.ErrServiceDeskRequestToLinkNotFound) {
		return fmt.Errorf("%w: key=%s", ErrServiceDeskRequestToLinkNotFound, key)
	}

	return err
}

// UnlinkServiceDeskRequest removes the link between the todo todoID and the
// Service Desk request key. It fails with
// ErrServiceDeskRequestLinkToDeleteNotFound when they are not linked.
func (s *service) UnlinkServiceDeskRequest(ctx context.Context, todoID int64, key string) error {
	err := s.db.UnlinkServiceDeskRequest(ctx, todoID, key)
	if errors.Is(err, db.ErrServiceDeskRequestLinkToDeleteNotFound) {
		return fmt.Errorf("%w: todo_id=%d, key=%s", ErrServiceDeskRequestLinkToDeleteNotFound, todoID, key)
	}

	return err
}

// normalize checks the fields a client sets and defaults an empty status to
// todo. An empty ParentID means no parent, but a blank one, like any blank
// child or linked ID, is rejected.
func normalize(t task.Task) (task.Task, error) {
	if isBlank(t.Title) {
		return task.Task{}, ErrEmptyTaskTitle
	}

	if t.ParentID != "" && isBlank(t.ParentID) ||
		slices.ContainsFunc(t.ChildrenIDs, isBlank) ||
		slices.ContainsFunc(t.LinkedIDs, isBlank) {
		return task.Task{}, ErrEmptyRelatedTaskID
	}

	switch t.Status {
	case "":
		t.Status = task.StatusTodo
	case task.StatusTodo, task.StatusInProgress, task.StatusDone, task.StatusCanceled:
	default:
		return task.Task{}, fmt.Errorf("%w: status=%s", ErrUnknownTaskStatus, t.Status)
	}

	return t, nil
}

// pick returns the tasks in stored among ids, in ids order.
func pick(stored map[string]task.Task, ids []string) []task.Task {
	picked := []task.Task{}

	for _, id := range ids {
		if t, ok := stored[id]; ok {
			picked = append(picked, t)
		}
	}

	return picked
}

func isBlank(s string) bool {
	return strings.TrimSpace(s) == ""
}
