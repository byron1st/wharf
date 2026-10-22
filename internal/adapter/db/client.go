package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"

	_ "modernc.org/sqlite"
)

var (
	ErrNotConnected            = errors.New("db: not connected")
	ErrFailedToBeginTx         = errors.New("db: failed to begin transaction")
	ErrFailedToCreateTask      = errors.New("db: failed to create task")
	ErrFailedToCommitTx        = errors.New("db: failed to commit transaction")
	ErrFailedToCheckTaskExists = errors.New("db: failed to check task existence")
)

type Manager interface {
	// DB Manager Methods

	CreateTasks(ctx context.Context, tasks []Task) error
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

// CreateTasks stores all tasks in one transaction: either every task is
// stored or none is. A task whose ID is already stored has its other fields
// overwritten. Empty ChildrenIds and LinkedIds are stored as "[]" so the
// columns always hold a JSON array. An empty list is a no-op.
func (m *manager) CreateTasks(ctx context.Context, tasks []Task) error {
	if err := m.checkConnected(); err != nil {
		return err
	}

	if len(tasks) == 0 {
		return nil
	}

	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return errors.Join(ErrFailedToBeginTx, err)
	}

	// Rollback after a successful Commit is a no-op returning sql.ErrTxDone.
	defer func() { _ = tx.Rollback() }()

	queries := m.queries.WithTx(tx)

	for _, task := range tasks {
		task.ChildrenIds = orEmptyJSONArray(task.ChildrenIds)
		task.LinkedIds = orEmptyJSONArray(task.LinkedIds)

		if err := queries.UpsertTask(ctx, UpsertTaskParams(task)); err != nil {
			return fmt.Errorf("%w: id=%s", errors.Join(ErrFailedToCreateTask, err), task.ID)
		}
	}

	if err := tx.Commit(); err != nil {
		return errors.Join(ErrFailedToCommitTx, err)
	}

	return nil
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

func orEmptyJSONArray(ids string) string {
	if ids == "" {
		return "[]"
	}

	return ids
}
