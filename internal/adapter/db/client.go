package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"time"

	"github.com/byron1st/wharf/internal/pkg/task"

	_ "modernc.org/sqlite"
)

var (
	ErrNotConnected               = errors.New("db: not connected")
	ErrFailedToBeginTx            = errors.New("db: failed to begin transaction")
	ErrFailedToCreateTask         = errors.New("db: failed to create task")
	ErrFailedToSetTaskParent      = errors.New("db: failed to set task parent")
	ErrFailedToDeleteTaskParent   = errors.New("db: failed to delete task parent")
	ErrFailedToSetTaskChild       = errors.New("db: failed to set task child")
	ErrFailedToDeleteTaskLinks    = errors.New("db: failed to delete task links")
	ErrFailedToCreateTaskLink     = errors.New("db: failed to create task link")
	ErrFailedToAllocateTaskNumber = errors.New("db: failed to allocate task number")
	ErrFailedToCommitTx           = errors.New("db: failed to commit transaction")
	ErrFailedToCheckTaskExists    = errors.New("db: failed to check task existence")
)

type Manager interface {
	// DB Manager Methods

	CreateTasks(ctx context.Context, tasks []task.Task) error
	CreateLocalTask(ctx context.Context, t task.Task) (string, error)
	ExistsTask(ctx context.Context, id string) (bool, error)

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

// CreateTasks stores all tasks and their relations in one transaction: either
// every task is stored or none is. A task whose ID is already stored has its
// other fields overwritten. Relations are stored as each task claims them:
// its ParentID replaces its stored parent (an empty ParentID deletes it), each
// of its ChildrenIDs gets it as parent, and its LinkedIDs replace its stored
// links. Stored children missing from ChildrenIDs are kept, because Jira
// lists sub-tasks but not stories under an epic. When tasks in the list
// claim conflicting relations, the later one wins. An empty list is a no-op.
func (m *manager) CreateTasks(ctx context.Context, tasks []task.Task) error {
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

// CreateLocalTask stores t as a new local task with its relations, as
// CreateTasks does, and returns its ID. The ID is allocated from a counter
// that is never reused, as "local-WHF-001", "local-WHF-002", and so on; the
// ID and Originated of t are ignored. When storing fails, the allocated
// number is rolled back with the task.
func (m *manager) CreateLocalTask(ctx context.Context, t task.Task) (string, error) {
	if err := m.checkConnected(); err != nil {
		return "", err
	}

	err := m.inTx(ctx, func(queries *Queries) error {
		// The counter update is the first statement, so the transaction
		// starts with a write (see busyTimeoutPragma).
		n, err := queries.NextTaskNumber(ctx, task.OriginLocal)
		if err != nil {
			return errors.Join(ErrFailedToAllocateTaskNumber, err)
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

// inTx runs fn in a transaction and commits it when fn succeeds.
func (m *manager) inTx(ctx context.Context, fn func(queries *Queries) error) error {
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

// storeTask upserts the task row of t, then its relations as described on
// CreateTasks.
func storeTask(ctx context.Context, queries *Queries, t task.Task) error {
	err := queries.UpsertTask(ctx, UpsertTaskParams{
		ID:          t.ID,
		Originated:  string(t.Originated),
		Title:       t.Title,
		Description: t.Description,
		Status:      string(t.Status),
		StartDate:   nullTime(t.StartDate),
		DueDate:     nullTime(t.DueDate),
	})
	if err != nil {
		return fmt.Errorf("%w: id=%s", errors.Join(ErrFailedToCreateTask, err), t.ID)
	}

	if t.ParentID != "" {
		err := queries.UpsertTaskParent(ctx, UpsertTaskParentParams{ChildID: t.ID, ParentID: t.ParentID})
		if err != nil {
			return fmt.Errorf("%w: id=%s", errors.Join(ErrFailedToSetTaskParent, err), t.ID)
		}
	} else if err := queries.DeleteTaskParent(ctx, t.ID); err != nil {
		return fmt.Errorf("%w: id=%s", errors.Join(ErrFailedToDeleteTaskParent, err), t.ID)
	}

	for _, childID := range t.ChildrenIDs {
		err := queries.UpsertTaskParent(ctx, UpsertTaskParentParams{ChildID: childID, ParentID: t.ID})
		if err != nil {
			return fmt.Errorf("%w: id=%s, child=%s", errors.Join(ErrFailedToSetTaskChild, err), t.ID, childID)
		}
	}

	if err := queries.DeleteTaskLinks(ctx, t.ID); err != nil {
		return fmt.Errorf("%w: id=%s", errors.Join(ErrFailedToDeleteTaskLinks, err), t.ID)
	}

	for _, linkedID := range t.LinkedIDs {
		err := queries.CreateTaskLink(ctx, CreateTaskLinkParams{TaskID: t.ID, LinkedID: linkedID})
		if err != nil {
			return fmt.Errorf("%w: id=%s, linked=%s", errors.Join(ErrFailedToCreateTaskLink, err), t.ID, linkedID)
		}
	}

	return nil
}

func nullTime(t *time.Time) sql.NullTime {
	if t == nil {
		return sql.NullTime{}
	}

	return sql.NullTime{Time: *t, Valid: true}
}

// ExistsTask reports whether a task with id is stored.
func (m *manager) ExistsTask(ctx context.Context, id string) (bool, error) {
	if err := m.checkConnected(); err != nil {
		return false, err
	}

	exists, err := m.queries.ExistsTask(ctx, id)
	if err != nil {
		return false, errors.Join(ErrFailedToCheckTaskExists, err)
	}

	return exists, nil
}

func (m *manager) checkConnected() error {
	if m.db == nil {
		return ErrNotConnected
	}

	return nil
}
