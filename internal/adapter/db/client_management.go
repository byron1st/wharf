package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"strings"
)

var (
	ErrAlreadyConnected                 = errors.New("db: already connected")
	ErrFailedToOpenDatabase             = errors.New("db: failed to open database")
	ErrFailedToPingDatabase             = errors.New("db: failed to ping database")
	ErrFailedToCloseDatabase            = errors.New("db: failed to close database")
	ErrFailedToReadMigrations           = errors.New("db: failed to read migrations")
	ErrInvalidMigrationFileName         = errors.New("db: invalid migration file name")
	ErrDuplicateMigrationVersion        = errors.New("db: duplicate migration version")
	ErrFailedToReadSchemaMigrationsFile = errors.New("db: failed to read schema_migrations.sql file")
	ErrFailedToCreateMigrationsTable    = errors.New("db: failed to create schema_migrations table")
	ErrFailedToCheckMigrationApplied    = errors.New("db: failed to check migration applied")
	ErrFailedToReadMigrationFile        = errors.New("db: failed to read migration file")
	ErrFailedToBeginMigrationTx         = errors.New("db: failed to begin migration transaction")
	ErrFailedToApplyMigration           = errors.New("db: failed to apply migration")
	ErrFailedToRecordMigration          = errors.New("db: failed to record migration")
	ErrFailedToCommitMigrationTx        = errors.New("db: failed to commit migration transaction")
)

// migrationFileName is "{4-digit version}_{description}.sql". The fixed-width
// version makes file-name order the apply order.
var migrationFileName = regexp.MustCompile(`^(\d{4})_[a-z0-9_]+\.sql$`)

const (
	schemaMigrationsFile = "schema_migrations.sql"
	migrationsDir        = "migrations"
)

// busyTimeoutPragma makes a statement wait up to 5s for a lock held by another
// connection, such as another process, instead of failing with SQLITE_BUSY.
// It does not help a transaction that reads before it writes: BeginTx starts a
// DEFERRED transaction, and SQLite fails its upgrade to a write lock with
// SQLITE_BUSY at once to avoid a deadlock. Transactions here therefore start
// with a write.
const busyTimeoutPragma = "_pragma=busy_timeout(5000)"

// Connect opens the database and verifies the connection. It fails with
// ErrAlreadyConnected while a previous connection is open.
func (m *manager) Connect(ctx context.Context) error {
	if m.db != nil {
		return ErrAlreadyConnected
	}

	separator := "?"
	if strings.Contains(m.path, "?") {
		separator = "&"
	}

	database, err := sql.Open("sqlite", m.path+separator+busyTimeoutPragma)
	if err != nil {
		return errors.Join(ErrFailedToOpenDatabase, err)
	}

	// SQLite allows a single writer; one connection avoids SQLITE_BUSY between
	// pooled connections and keeps ":memory:" a single database.
	database.SetMaxOpenConns(1)

	if err := database.PingContext(ctx); err != nil {
		_ = database.Close()
		return errors.Join(ErrFailedToPingDatabase, err)
	}

	m.db = database
	m.queries = New(database)

	return nil
}

// Close closes the database connection, after which Connect may be called
// again. It is a no-op when not connected.
func (m *manager) Close() error {
	if m.db == nil {
		return nil
	}

	err := m.db.Close()
	m.db, m.queries = nil, nil

	if err != nil {
		return errors.Join(ErrFailedToCloseDatabase, err)
	}

	return nil
}

// Migrate creates the schema_migrations table if missing, then applies the
// migrations not yet applied, in version order. Each migration runs in its own
// transaction together with its record in schema_migrations, so a failed
// migration leaves no partial change.
func (m *manager) Migrate(ctx context.Context) error {
	if err := m.checkConnected(); err != nil {
		return err
	}

	migrations, err := readMigrations(m.migrations)
	if err != nil {
		return err
	}

	schema, err := fs.ReadFile(m.migrations, schemaMigrationsFile)
	if err != nil {
		return errors.Join(ErrFailedToReadSchemaMigrationsFile, err)
	}

	// The file uses CREATE TABLE IF NOT EXISTS, so it is applied on every
	// call without being recorded as a migration.
	if _, err := m.db.ExecContext(ctx, string(schema)); err != nil {
		return errors.Join(ErrFailedToCreateMigrationsTable, err)
	}

	for _, mig := range migrations {
		applied, err := m.queries.ExistsSchemaMigration(ctx, mig.version)
		if err != nil {
			return fmt.Errorf("%w: file=%s", errors.Join(ErrFailedToCheckMigrationApplied, err), mig.name)
		}

		if applied {
			continue
		}

		if err := m.applyMigration(ctx, mig); err != nil {
			return err
		}
	}

	return nil
}

func (m *manager) applyMigration(ctx context.Context, mig migration) error {
	content, err := fs.ReadFile(m.migrations, path.Join(migrationsDir, mig.name))
	if err != nil {
		return fmt.Errorf("%w: file=%s", errors.Join(ErrFailedToReadMigrationFile, err), mig.name)
	}

	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return errors.Join(ErrFailedToBeginMigrationTx, err)
	}

	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, string(content)); err != nil {
		return fmt.Errorf("%w: file=%s", errors.Join(ErrFailedToApplyMigration, err), mig.name)
	}

	if err := m.queries.WithTx(tx).CreateSchemaMigration(ctx, mig.version); err != nil {
		return fmt.Errorf("%w: file=%s", errors.Join(ErrFailedToRecordMigration, err), mig.name)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("%w: file=%s", errors.Join(ErrFailedToCommitMigrationTx, err), mig.name)
	}

	return nil
}

type migration struct {
	version string
	name    string
}

// readMigrations relies on fs.ReadDir returning entries sorted by file name,
// which is version order, so duplicate versions are adjacent.
func readMigrations(fsys fs.FS) ([]migration, error) {
	entries, err := fs.ReadDir(fsys, migrationsDir)
	if err != nil {
		return nil, errors.Join(ErrFailedToReadMigrations, err)
	}

	migrations := make([]migration, 0, len(entries))

	for _, entry := range entries {
		match := migrationFileName.FindStringSubmatch(entry.Name())
		if match == nil {
			return nil, fmt.Errorf("%w: file=%s", ErrInvalidMigrationFileName, entry.Name())
		}

		if len(migrations) > 0 && migrations[len(migrations)-1].version == match[1] {
			return nil, fmt.Errorf("%w: version=%s", ErrDuplicateMigrationVersion, match[1])
		}

		migrations = append(migrations, migration{version: match[1], name: entry.Name()})
	}

	return migrations, nil
}
