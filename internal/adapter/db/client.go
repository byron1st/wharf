package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"regexp"

	_ "modernc.org/sqlite"
)

var (
	ErrFailedToOpenDatabase    = errors.New("db: failed to open database")
	ErrFailedToPingDatabase    = errors.New("db: failed to ping database")
	ErrFailedToCloseDatabase   = errors.New("db: failed to close database")
	ErrFailedToBeginTx         = errors.New("db: failed to begin transaction")
	ErrFailedToCreateTask      = errors.New("db: failed to create task")
	ErrFailedToCommitTx        = errors.New("db: failed to commit transaction")
	ErrFailedToCheckTaskExists = errors.New("db: failed to check task existence")

	ErrFailedToReadMigrations        = errors.New("db: failed to read migrations")
	ErrInvalidMigrationFileName      = errors.New("db: invalid migration file name")
	ErrDuplicateMigrationVersion     = errors.New("db: duplicate migration version")
	ErrFailedToReadMigrationsTable   = errors.New("db: failed to read schema_migrations.sql")
	ErrFailedToCreateMigrationsTable = errors.New("db: failed to create schema_migrations table")
	ErrFailedToCheckMigrationApplied = errors.New("db: failed to check migration applied")
	ErrFailedToReadMigrationFile     = errors.New("db: failed to read migration file")
	ErrFailedToBeginMigrationTx      = errors.New("db: failed to begin migration transaction")
	ErrFailedToApplyMigration        = errors.New("db: failed to apply migration")
	ErrFailedToRecordMigration       = errors.New("db: failed to record migration")
	ErrFailedToCommitMigrationTx     = errors.New("db: failed to commit migration transaction")
)

// migrationFileName is "{4-digit version}_{description}.sql". The fixed-width
// version makes file-name order the apply order.
var migrationFileName = regexp.MustCompile(`^(\d{4})_[a-z0-9_]+\.sql$`)

const (
	schemaMigrationsFile = "schema_migrations.sql"
	migrationsDir        = "migrations"
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
// Connect before using it.
func NewManager(path string, migrations fs.FS) Manager {
	return &manager{path: path, migrations: migrations}
}

// CreateTasks stores all tasks in one transaction: either every task is
// stored or none is. An empty list is a no-op.
func (m *manager) CreateTasks(ctx context.Context, tasks []Task) error {
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
		if err := queries.CreateTask(ctx, CreateTaskParams(task)); err != nil {
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
	exists, err := m.queries.ExistsTask(ctx, id)
	if err != nil {
		return false, errors.Join(ErrFailedToCheckTaskExists, err)
	}

	return exists, nil
}
