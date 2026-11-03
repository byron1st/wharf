package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"time"

	_ "modernc.org/sqlite"

	"github.com/byron1st/wharf/internal/pkg/atlassiansync"
	"github.com/byron1st/wharf/internal/pkg/servicedesk"
	"github.com/byron1st/wharf/internal/pkg/task"
	"github.com/byron1st/wharf/internal/pkg/todo"
)

var (
	ErrNotConnected                         = errors.New("db: not connected")
	ErrFailedToBeginTx                      = errors.New("db: failed to begin transaction")
	ErrFailedToUpsertTask                   = errors.New("db: failed to upsert task")
	ErrFailedToDeleteTaskParent             = errors.New("db: failed to delete task parent")
	ErrFailedToStoreTaskParent              = errors.New("db: failed to store task parent")
	ErrFailedToStoreTaskChild               = errors.New("db: failed to store task child")
	ErrFailedToDeleteTaskLinks              = errors.New("db: failed to delete task links")
	ErrFailedToStoreTaskLink                = errors.New("db: failed to store task link")
	ErrFailedToIncrementLocalTaskCounter    = errors.New("db: failed to increment local task counter")
	ErrFailedToCommitTx                     = errors.New("db: failed to commit transaction")
	ErrFailedToListTasks                    = errors.New("db: failed to list tasks")
	ErrFailedToListTasksByIDs               = errors.New("db: failed to list tasks by ids")
	ErrTaskNotFound                         = errors.New("db: task not found")
	ErrFailedToGetTask                      = errors.New("db: failed to get task")
	ErrFailedToListTaskChildren             = errors.New("db: failed to list task children")
	ErrFailedToListTaskLinks                = errors.New("db: failed to list task links")
	ErrFailedToUpdateLocalTask              = errors.New("db: failed to update local task")
	ErrLocalTaskToUpdateNotFound            = errors.New("db: local task to update not found")
	ErrFailedToUpdateLocalTaskStatus        = errors.New("db: failed to update local task status")
	ErrLocalTaskToSetStatusNotFound         = errors.New("db: local task to set status not found")
	ErrFailedToUpdateTaskPosition           = errors.New("db: failed to update task position")
	ErrFailedToDeleteTaskChildren           = errors.New("db: failed to delete task children")
	ErrFailedToDeleteLocalTaskLinks         = errors.New("db: failed to delete links of local task")
	ErrFailedToDeleteTask                   = errors.New("db: failed to delete task")
	ErrTaskToDeleteNotFound                 = errors.New("db: task to delete not found")
	ErrFailedToDeleteTaskHierarchies        = errors.New("db: failed to delete task hierarchies")
	ErrFailedToDeleteTaskLinksOf            = errors.New("db: failed to delete links of task")
	ErrFailedToDeleteTodoRemindersOfTask    = errors.New("db: failed to delete todo reminders of task")
	ErrFailedToDeleteTodosOfTask            = errors.New("db: failed to delete todos of task")
	ErrFailedToDeleteTaskTodosOfTask        = errors.New("db: failed to delete task todos of task")
	ErrFailedToUpsertServiceDeskRequest     = errors.New("db: failed to upsert service desk request")
	ErrFailedToDeleteServiceDeskLinksOfTask = errors.New(
		"db: failed to delete service desk request links of todos of task",
	)
	ErrFailedToLinkServiceDeskRequest         = errors.New("db: failed to link service desk request")
	ErrFailedToCheckTodoToLink                = errors.New("db: failed to check todo to link")
	ErrTodoToLinkNotFound                     = errors.New("db: todo to link not found")
	ErrFailedToCheckServiceDeskRequestToLink  = errors.New("db: failed to check service desk request to link")
	ErrServiceDeskRequestToLinkNotFound       = errors.New("db: service desk request to link not found")
	ErrFailedToUnlinkServiceDeskRequest       = errors.New("db: failed to unlink service desk request")
	ErrServiceDeskRequestLinkToDeleteNotFound = errors.New("db: service desk request link to delete not found")
	ErrFailedToListServiceDeskRequests        = errors.New("db: failed to list service desk requests")
	ErrFailedToCreateAtlassianSync            = errors.New("db: failed to create atlassian sync")
	ErrAtlassianSyncNotFound                  = errors.New("db: atlassian sync not found")
	ErrFailedToGetAtlassianSync               = errors.New("db: failed to get atlassian sync")
)

// localTaskCounter is the counters row seeded by the initial migration.
const localTaskCounter = "local_task"

type Manager interface {
	// DB Manager Methods

	UpsertTasks(ctx context.Context, tasks []task.Task) error
	CreateLocalTask(ctx context.Context, t task.Task) (string, error)
	ListTasks(ctx context.Context) ([]task.Task, error)
	ListTasksByIDs(ctx context.Context, ids []string) ([]task.Task, error)
	GetTask(ctx context.Context, id string) (task.Task, error)
	UpdateLocalTask(ctx context.Context, t task.Task) error
	UpdateLocalTaskStatus(ctx context.Context, id string, status task.Status) error
	SetTaskPositions(ctx context.Context, ids []string) error
	DeleteTask(ctx context.Context, id string) error
	CreateTodo(ctx context.Context, t todo.Todo) (int64, error)
	ListTodos(ctx context.Context, f todo.Filter) ([]todo.Todo, error)
	CountTodosByTask(ctx context.Context) (map[string]todo.Count, error)
	UpdateTodo(ctx context.Context, t todo.Todo) error
	DeleteTodo(ctx context.Context, id int64) error
	UpsertServiceDeskRequests(ctx context.Context, requests []servicedesk.Request) error
	LinkServiceDeskRequest(ctx context.Context, todoID int64, key string) error
	UnlinkServiceDeskRequest(ctx context.Context, todoID int64, key string) error
	ListServiceDeskRequests(ctx context.Context, includeClosed bool) ([]servicedesk.Request, error)
	CreateAtlassianSync(ctx context.Context, r atlassiansync.Record) error
	GetLastAtlassianSync(ctx context.Context) (atlassiansync.Record, error)

	// DB Management

	Connect(ctx context.Context) error
	Migrate(ctx context.Context) error
	Close() error
}

type manager struct {
	path       string
	migrations fs.FS
	db         *sql.DB
	queries    *Queries
}

// NewManager creates a manager for the SQLite database at path. migrations
// holds schema_migrations.sql and the migrations directory at its root. Call
// Connect before using it. Connect and Close are meant for program start and
// shutdown and must not run concurrently with other methods.
func NewManager(path string, migrations fs.FS) Manager {
	return &manager{path: path, migrations: migrations}
}

// UpsertTasks stores all tasks with their relations in one transaction:
// either every task is stored or none is. A task whose ID is already stored
// has its other fields overwritten. Nil dates are stored as NULL. An empty
// list is a no-op.
//
// A task owns its parent and its links: storing it replaces the parent and
// the links it declared before. Its children are only added or moved to it,
// since a parent may not list every child (a Jira Epic does not list its
// Stories). Duplicate children and links are stored once.
//
// The exception is a local parent: a non-local task that declares no parent
// keeps a local parent, so a local task can hold a Jira task as its child
// across syncs. A declared parent still replaces it.
func (m *manager) UpsertTasks(ctx context.Context, tasks []task.Task) error {
	if err := m.checkConnected(); err != nil {
		return err
	}

	if len(tasks) == 0 {
		return nil
	}

	return m.inTx(ctx, func(queries *Queries) error {
		for _, t := range tasks {
			if err := storeTask(ctx, queries, t); err != nil {
				return err
			}
		}

		return nil
	})
}

// CreateLocalTask stores t as a new local task with its relations, in the way
// UpsertTasks does, and returns its ID. The ID is "local-WHF-{n}" with the
// next local task number, and t.ID and t.Originated are ignored. A failed
// call does not use up the number.
func (m *manager) CreateLocalTask(ctx context.Context, t task.Task) (string, error) {
	if err := m.checkConnected(); err != nil {
		return "", err
	}

	// The counter UPDATE is the transaction's first statement, keeping the
	// write-first rule described at busyTimeoutPragma.
	err := m.inTx(ctx, func(queries *Queries) error {
		n, err := queries.IncrementCounter(ctx, localTaskCounter)
		if err != nil {
			return errors.Join(ErrFailedToIncrementLocalTaskCounter, err)
		}

		t.ID = task.NewID(task.OriginLocal, task.LocalKey(n))
		t.Originated = task.OriginLocal

		return storeTask(ctx, queries, t)
	})
	if err != nil {
		return "", err
	}

	return t.ID, nil
}

// ListTasks returns every stored task in ID order, without relations but
// with their positions.
func (m *manager) ListTasks(ctx context.Context) ([]task.Task, error) {
	if err := m.checkConnected(); err != nil {
		return nil, err
	}

	rows, err := m.queries.ListTasks(ctx)
	if err != nil {
		return nil, errors.Join(ErrFailedToListTasks, err)
	}

	tasks := fromTasks(rows)
	for i, row := range rows {
		if row.Position.Valid {
			position := int(row.Position.Int64)
			tasks[i].Position = &position
		}
	}

	return tasks, nil
}

// ListTasksByIDs returns the stored tasks among ids in ID order, without
// relations. IDs that are not stored are skipped.
func (m *manager) ListTasksByIDs(ctx context.Context, ids []string) ([]task.Task, error) {
	if err := m.checkConnected(); err != nil {
		return nil, err
	}

	rows, err := m.queries.ListTasksByIDs(ctx, ids)
	if err != nil {
		return nil, errors.Join(ErrFailedToListTasksByIDs, err)
	}

	return fromTasks(rows), nil
}

// GetTask returns the task with id and its relations: its parent, the tasks
// stored as its children, and the tasks linked to or from it, in ID order.
// Related IDs may not be stored. It fails with ErrTaskNotFound when no task
// has id.
func (m *manager) GetTask(ctx context.Context, id string) (task.Task, error) {
	if err := m.checkConnected(); err != nil {
		return task.Task{}, err
	}

	row, err := m.queries.GetTask(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return task.Task{}, fmt.Errorf("%w: id=%s", ErrTaskNotFound, id)
	}

	if err != nil {
		return task.Task{}, fmt.Errorf("%w: id=%s", errors.Join(ErrFailedToGetTask, err), id)
	}

	t := fromTask(row.Task)
	t.ParentID = row.ParentID.String

	if t.ChildrenIDs, err = m.queries.ListTaskChildren(ctx, id); err != nil {
		return task.Task{}, fmt.Errorf("%w: id=%s", errors.Join(ErrFailedToListTaskChildren, err), id)
	}

	if t.LinkedIDs, err = m.queries.ListTaskLinks(ctx, id); err != nil {
		return task.Task{}, fmt.Errorf("%w: id=%s", errors.Join(ErrFailedToListTaskLinks, err), id)
	}

	return t, nil
}

// UpdateLocalTask replaces every field of the stored local task t.ID except
// its ID and origin, together with its relations: its parent, and unlike
// UpsertTasks all of its children and its links in both directions, which is
// how GetTask reads them. A link another task declared to t.ID is dropped
// unless t.LinkedIDs lists it; a Jira task declares it again on its next sync.
// It fails with ErrLocalTaskToUpdateNotFound when no local task has t.ID.
func (m *manager) UpdateLocalTask(ctx context.Context, t task.Task) error {
	if err := m.checkConnected(); err != nil {
		return err
	}

	t.Originated = task.OriginLocal

	return m.inTx(ctx, func(queries *Queries) error {
		updated, err := queries.UpdateLocalTask(ctx, UpdateLocalTaskParams{
			Title:       t.Title,
			Description: t.Description,
			Status:      string(t.Status),
			StartDate:   toNullTime(t.StartDate),
			DueDate:     toNullTime(t.DueDate),
			ID:          t.ID,
		})
		if err != nil {
			return fmt.Errorf("%w: id=%s", errors.Join(ErrFailedToUpdateLocalTask, err), t.ID)
		}

		if updated == 0 {
			return fmt.Errorf("%w: id=%s", ErrLocalTaskToUpdateNotFound, t.ID)
		}

		if err := queries.DeleteTaskChildren(ctx, t.ID); err != nil {
			return fmt.Errorf("%w: id=%s", errors.Join(ErrFailedToDeleteTaskChildren, err), t.ID)
		}

		if err := queries.DeleteTaskLinksOf(ctx, t.ID); err != nil {
			return fmt.Errorf("%w: id=%s", errors.Join(ErrFailedToDeleteLocalTaskLinks, err), t.ID)
		}

		return storeRelations(ctx, queries, t)
	})
}

// UpdateLocalTaskStatus sets the status of the stored local task id, leaving
// its other fields and relations as they are. It fails with
// ErrLocalTaskToSetStatusNotFound when no local task has id.
func (m *manager) UpdateLocalTaskStatus(ctx context.Context, id string, status task.Status) error {
	if err := m.checkConnected(); err != nil {
		return err
	}

	updated, err := m.queries.UpdateLocalTaskStatus(ctx, UpdateLocalTaskStatusParams{Status: string(status), ID: id})
	if err != nil {
		return fmt.Errorf("%w: id=%s", errors.Join(ErrFailedToUpdateLocalTaskStatus, err), id)
	}

	if updated == 0 {
		return fmt.Errorf("%w: id=%s", ErrLocalTaskToSetStatusNotFound, id)
	}

	return nil
}

// SetTaskPositions stores the index of each ID in ids as the position of
// that task, in one transaction. IDs that are not stored are skipped, and
// tasks not in ids keep their positions.
func (m *manager) SetTaskPositions(ctx context.Context, ids []string) error {
	if err := m.checkConnected(); err != nil {
		return err
	}

	return m.inTx(ctx, func(queries *Queries) error {
		for i, id := range ids {
			params := UpdateTaskPositionParams{Position: sql.NullInt64{Int64: int64(i), Valid: true}, ID: id}
			if err := queries.UpdateTaskPosition(ctx, params); err != nil {
				return fmt.Errorf("%w: id=%s", errors.Join(ErrFailedToUpdateTaskPosition, err), id)
			}
		}

		return nil
	})
}

// DeleteTask deletes the task with id and every relation it is part of, as
// child, parent, or either side of a link, together with its todos, their
// reminders, and their links to Service Desk requests. Other tasks, their
// todos and links, and the requests are kept. It fails with
// ErrTaskToDeleteNotFound when no task has id.
func (m *manager) DeleteTask(ctx context.Context, id string) error {
	if err := m.checkConnected(); err != nil {
		return err
	}

	return m.inTx(ctx, func(queries *Queries) error {
		deleted, err := queries.DeleteTask(ctx, id)
		if err != nil {
			return fmt.Errorf("%w: id=%s", errors.Join(ErrFailedToDeleteTask, err), id)
		}

		if deleted == 0 {
			return fmt.Errorf("%w: id=%s", ErrTaskToDeleteNotFound, id)
		}

		if err := queries.DeleteTaskHierarchies(ctx, id); err != nil {
			return fmt.Errorf("%w: id=%s", errors.Join(ErrFailedToDeleteTaskHierarchies, err), id)
		}

		if err := queries.DeleteTaskLinksOf(ctx, id); err != nil {
			return fmt.Errorf("%w: id=%s", errors.Join(ErrFailedToDeleteTaskLinksOf, err), id)
		}

		// Links, reminders, and todos are found through task_todos, so it goes
		// last.
		if err := queries.DeleteTodoServiceDeskRequestsOfTask(ctx, id); err != nil {
			return fmt.Errorf("%w: id=%s", errors.Join(ErrFailedToDeleteServiceDeskLinksOfTask, err), id)
		}

		if err := queries.DeleteTodoRemindersOfTask(ctx, id); err != nil {
			return fmt.Errorf("%w: id=%s", errors.Join(ErrFailedToDeleteTodoRemindersOfTask, err), id)
		}

		if err := queries.DeleteTodosOfTask(ctx, id); err != nil {
			return fmt.Errorf("%w: id=%s", errors.Join(ErrFailedToDeleteTodosOfTask, err), id)
		}

		if err := queries.DeleteTaskTodosOfTask(ctx, id); err != nil {
			return fmt.Errorf("%w: id=%s", errors.Join(ErrFailedToDeleteTaskTodosOfTask, err), id)
		}

		return nil
	})
}

// UpsertServiceDeskRequests stores requests in one transaction: either every
// request is stored or none is. A request whose issue key is already stored
// has its other fields overwritten. Requests absent from this call are kept.
// An empty list is a no-op.
func (m *manager) UpsertServiceDeskRequests(ctx context.Context, requests []servicedesk.Request) error {
	if err := m.checkConnected(); err != nil {
		return err
	}

	if len(requests) == 0 {
		return nil
	}

	return m.inTx(ctx, func(queries *Queries) error {
		for _, request := range requests {
			err := queries.UpsertServiceDeskRequest(ctx, UpsertServiceDeskRequestParams{
				IssueKey:       request.Key,
				Title:          request.Title,
				Status:         request.Status,
				StatusCategory: request.StatusCategory,
				CreatedAt:      request.Created,
				StatusAt:       request.StatusAt,
			})
			if err != nil {
				return fmt.Errorf("%w: key=%s", errors.Join(ErrFailedToUpsertServiceDeskRequest, err), request.Key)
			}
		}

		return nil
	})
}

// LinkServiceDeskRequest stores a relates-to link between the stored todo
// todoID and the stored Service Desk request key. Linking them again is a
// no-op. It fails with ErrTodoToLinkNotFound when no todo has todoID and with
// ErrServiceDeskRequestToLinkNotFound when no request has key, storing
// nothing.
func (m *manager) LinkServiceDeskRequest(ctx context.Context, todoID int64, key string) error {
	if err := m.checkConnected(); err != nil {
		return err
	}

	return m.inTx(ctx, func(queries *Queries) error {
		// The insert checks both sides, keeping the write-first rule described
		// at busyTimeoutPragma. Only when it inserts nothing do the reads below
		// tell a missing side from an existing link.
		params := CreateTodoServiceDeskRequestParams{TodoID: todoID, IssueKey: key}

		created, err := queries.CreateTodoServiceDeskRequest(ctx, params)
		if err != nil {
			return fmt.Errorf(
				"%w: todo_id=%d, key=%s",
				errors.Join(ErrFailedToLinkServiceDeskRequest, err),
				todoID,
				key,
			)
		}

		if created > 0 {
			return nil
		}

		todoExists, err := queries.ExistsTodo(ctx, todoID)
		if err != nil {
			return fmt.Errorf("%w: todo_id=%d", errors.Join(ErrFailedToCheckTodoToLink, err), todoID)
		}

		if !todoExists {
			return fmt.Errorf("%w: todo_id=%d", ErrTodoToLinkNotFound, todoID)
		}

		requestExists, err := queries.ExistsServiceDeskRequest(ctx, key)
		if err != nil {
			return fmt.Errorf("%w: key=%s", errors.Join(ErrFailedToCheckServiceDeskRequestToLink, err), key)
		}

		if !requestExists {
			return fmt.Errorf("%w: key=%s", ErrServiceDeskRequestToLinkNotFound, key)
		}

		return nil
	})
}

// UnlinkServiceDeskRequest deletes the link between the todo todoID and the
// Service Desk request key. The todo, the request, and their other links are
// kept. It fails with ErrServiceDeskRequestLinkToDeleteNotFound when they are
// not linked.
func (m *manager) UnlinkServiceDeskRequest(ctx context.Context, todoID int64, key string) error {
	if err := m.checkConnected(); err != nil {
		return err
	}

	params := DeleteTodoServiceDeskRequestParams{TodoID: todoID, IssueKey: key}

	deleted, err := m.queries.DeleteTodoServiceDeskRequest(ctx, params)
	if err != nil {
		return fmt.Errorf("%w: todo_id=%d, key=%s", errors.Join(ErrFailedToUnlinkServiceDeskRequest, err), todoID, key)
	}

	if deleted == 0 {
		return fmt.Errorf("%w: todo_id=%d, key=%s", ErrServiceDeskRequestLinkToDeleteNotFound, todoID, key)
	}

	return nil
}

// ListServiceDeskRequests returns the stored requests in issue key order:
// those not in the DONE status category, or every one when includeClosed.
func (m *manager) ListServiceDeskRequests(ctx context.Context, includeClosed bool) ([]servicedesk.Request, error) {
	if err := m.checkConnected(); err != nil {
		return nil, err
	}

	rows, err := m.queries.ListServiceDeskRequests(ctx, includeClosed)
	if err != nil {
		return nil, errors.Join(ErrFailedToListServiceDeskRequests, err)
	}

	requests := make([]servicedesk.Request, 0, len(rows))
	for _, row := range rows {
		requests = append(requests, fromServiceDeskRequest(row))
	}

	return requests, nil
}

// CreateAtlassianSync records r as the latest Atlassian sync.
func (m *manager) CreateAtlassianSync(ctx context.Context, r atlassiansync.Record) error {
	if err := m.checkConnected(); err != nil {
		return err
	}

	err := m.queries.CreateAtlassianSync(ctx, CreateAtlassianSyncParams{
		SyncedAt:        r.SyncedAt,
		Issues:          int64(r.Issues),
		IssueFailures:   int64(r.IssueFailures),
		OpenRequests:    int64(r.OpenRequests),
		ClosedRequests:  int64(r.ClosedRequests),
		RequestFailures: int64(r.RequestFailures),
	})
	if err != nil {
		return errors.Join(ErrFailedToCreateAtlassianSync, err)
	}

	return nil
}

// GetLastAtlassianSync returns the latest recorded Atlassian sync. It fails
// with ErrAtlassianSyncNotFound when none is recorded.
func (m *manager) GetLastAtlassianSync(ctx context.Context) (atlassiansync.Record, error) {
	if err := m.checkConnected(); err != nil {
		return atlassiansync.Record{}, err
	}

	row, err := m.queries.GetLastAtlassianSync(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return atlassiansync.Record{}, ErrAtlassianSyncNotFound
	}

	if err != nil {
		return atlassiansync.Record{}, errors.Join(ErrFailedToGetAtlassianSync, err)
	}

	return atlassiansync.Record{
		SyncedAt:        row.SyncedAt,
		Issues:          int(row.Issues),
		IssueFailures:   int(row.IssueFailures),
		OpenRequests:    int(row.OpenRequests),
		ClosedRequests:  int(row.ClosedRequests),
		RequestFailures: int(row.RequestFailures),
	}, nil
}

// inTx runs fn in a transaction and commits it when fn succeeds.
func (m *manager) inTx(ctx context.Context, fn func(*Queries) error) error {
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return errors.Join(ErrFailedToBeginTx, err)
	}

	// Rollback after a successful Commit is a no-op returning sql.ErrTxDone.
	defer func() { _ = tx.Rollback() }()

	if err := fn(m.queries.WithTx(tx)); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return errors.Join(ErrFailedToCommitTx, err)
	}

	return nil
}

// storeTask upserts t and writes its relations as UpsertTasks describes.
func storeTask(ctx context.Context, queries *Queries, t task.Task) error {
	if err := queries.UpsertTask(ctx, toUpsertTaskParams(t)); err != nil {
		return fmt.Errorf("%w: id=%s", errors.Join(ErrFailedToUpsertTask, err), t.ID)
	}

	return storeRelations(ctx, queries, t)
}

// storeRelations replaces the parent and links of t and adds its children, as
// UpsertTasks describes.
func storeRelations(ctx context.Context, queries *Queries, t task.Task) error {
	deleteParent := queries.DeleteTaskParent
	if t.Originated != task.OriginLocal && t.ParentID == "" {
		deleteParent = queries.DeleteTaskNonLocalParent
	}

	if err := deleteParent(ctx, t.ID); err != nil {
		return fmt.Errorf("%w: id=%s", errors.Join(ErrFailedToDeleteTaskParent, err), t.ID)
	}

	if t.ParentID != "" {
		params := UpsertTaskHierarchyParams{ChildID: t.ID, ParentID: t.ParentID}
		if err := queries.UpsertTaskHierarchy(ctx, params); err != nil {
			return fmt.Errorf("%w: id=%s, parent=%s", errors.Join(ErrFailedToStoreTaskParent, err), t.ID, t.ParentID)
		}
	}

	for _, child := range t.ChildrenIDs {
		params := UpsertTaskHierarchyParams{ChildID: child, ParentID: t.ID}
		if err := queries.UpsertTaskHierarchy(ctx, params); err != nil {
			return fmt.Errorf("%w: id=%s, child=%s", errors.Join(ErrFailedToStoreTaskChild, err), t.ID, child)
		}
	}

	if err := queries.DeleteTaskLinks(ctx, t.ID); err != nil {
		return fmt.Errorf("%w: id=%s", errors.Join(ErrFailedToDeleteTaskLinks, err), t.ID)
	}

	for _, linked := range t.LinkedIDs {
		params := CreateTaskLinkParams{TaskID: t.ID, LinkedID: linked}
		if err := queries.CreateTaskLink(ctx, params); err != nil {
			return fmt.Errorf("%w: id=%s, linked=%s", errors.Join(ErrFailedToStoreTaskLink, err), t.ID, linked)
		}
	}

	return nil
}

func toUpsertTaskParams(t task.Task) UpsertTaskParams {
	return UpsertTaskParams{
		ID:          t.ID,
		Originated:  string(t.Originated),
		Title:       t.Title,
		Description: t.Description,
		Status:      string(t.Status),
		StartDate:   toNullTime(t.StartDate),
		DueDate:     toNullTime(t.DueDate),
	}
}

func fromTasks(rows []Task) []task.Task {
	tasks := make([]task.Task, 0, len(rows))
	for _, row := range rows {
		tasks = append(tasks, fromTask(row))
	}

	return tasks
}

func fromTask(row Task) task.Task {
	return task.Task{
		ID:          row.ID,
		Originated:  task.Origin(row.Originated),
		Title:       row.Title,
		Description: row.Description,
		Status:      task.Status(row.Status),
		StartDate:   fromNullTime(row.StartDate),
		DueDate:     fromNullTime(row.DueDate),
	}
}

func fromNullTime(t sql.NullTime) *time.Time {
	if !t.Valid {
		return nil
	}

	return &t.Time
}

// toNullTime stores t at UTC. SQLite keeps times as text and compares them
// as strings, so every stored or compared time must share one zone.
func toNullTime(t *time.Time) sql.NullTime {
	if t == nil {
		return sql.NullTime{}
	}

	return sql.NullTime{Time: t.UTC(), Valid: true}
}

func (m *manager) checkConnected() error {
	if m.db == nil {
		return ErrNotConnected
	}

	return nil
}
