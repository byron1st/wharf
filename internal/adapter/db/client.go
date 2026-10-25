package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"time"

	_ "modernc.org/sqlite"

	"github.com/byron1st/wharf/internal/pkg/task"
)

var (
	ErrNotConnected                      = errors.New("db: not connected")
	ErrFailedToBeginTx                   = errors.New("db: failed to begin transaction")
	ErrFailedToUpsertTask                = errors.New("db: failed to upsert task")
	ErrFailedToDeleteTaskParent          = errors.New("db: failed to delete task parent")
	ErrFailedToStoreTaskParent           = errors.New("db: failed to store task parent")
	ErrFailedToStoreTaskChild            = errors.New("db: failed to store task child")
	ErrFailedToDeleteTaskLinks           = errors.New("db: failed to delete task links")
	ErrFailedToStoreTaskLink             = errors.New("db: failed to store task link")
	ErrFailedToIncrementLocalTaskCounter = errors.New("db: failed to increment local task counter")
	ErrFailedToCommitTx                  = errors.New("db: failed to commit transaction")
)

// localTaskCounter is the counters row seeded by the initial migration.
const localTaskCounter = "local_task"

type Manager interface {
	// DB Manager Methods

	UpsertTasks(ctx context.Context, tasks []task.Task) error
	CreateLocalTask(ctx context.Context, t task.Task) (string, error)

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

	if err := queries.DeleteTaskParent(ctx, t.ID); err != nil {
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

func toNullTime(t *time.Time) sql.NullTime {
	if t == nil {
		return sql.NullTime{}
	}

	return sql.NullTime{Time: *t, Valid: true}
}

func (m *manager) checkConnected() error {
	if m.db == nil {
		return ErrNotConnected
	}

	return nil
}
