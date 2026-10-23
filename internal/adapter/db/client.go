package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"time"

	_ "modernc.org/sqlite"

	"github.com/byron1st/wharf/internal/pkg/task"
)

var (
	ErrNotConnected       = errors.New("db: not connected")
	ErrFailedToBeginTx    = errors.New("db: failed to begin transaction")
	ErrFailedToUpsertTask = errors.New("db: failed to upsert task")
	ErrFailedToCommitTx   = errors.New("db: failed to commit transaction")
)

type Manager interface {
	// DB Manager Methods

	UpsertTasks(ctx context.Context, tasks []task.Task) error

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

// UpsertTasks stores all tasks in one transaction: either every task is
// stored or none is. A task whose ID is already stored has its other fields
// overwritten. Nil dates are stored as NULL and ID lists as JSON arrays, with
// nil or empty lists stored as "[]". An empty list is a no-op.
func (m *manager) UpsertTasks(ctx context.Context, tasks []task.Task) error {
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

	for _, t := range tasks {
		if err := queries.UpsertTask(ctx, toUpsertTaskParams(t)); err != nil {
			return fmt.Errorf("%w: id=%s", errors.Join(ErrFailedToUpsertTask, err), t.ID)
		}
	}

	if err := tx.Commit(); err != nil {
		return errors.Join(ErrFailedToCommitTx, err)
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
		ParentID:    t.ParentID,
		ChildrenIds: toJSONArray(t.ChildrenIDs),
		LinkedIds:   toJSONArray(t.LinkedIDs),
	}
}

func toNullTime(t *time.Time) sql.NullTime {
	if t == nil {
		return sql.NullTime{}
	}

	return sql.NullTime{Time: *t, Valid: true}
}

func toJSONArray(ids []string) string {
	if len(ids) == 0 {
		return "[]"
	}

	// Marshaling a []string cannot fail.
	data, _ := json.Marshal(ids)

	return string(data)
}

func (m *manager) checkConnected() error {
	if m.db == nil {
		return ErrNotConnected
	}

	return nil
}
