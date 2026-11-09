package db

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"database/sql/driver"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/stdlib"
)

var (
	ErrAlreadyConnected                 = errors.New("db: already connected")
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
	ErrMigrationChecksumMismatch        = errors.New("db: applied migration file was changed")
	ErrFailedToListAppliedMigrations    = errors.New("db: failed to list applied migrations")
	ErrUnknownAppliedMigration          = errors.New("db: database has a migration unknown to this binary")
	ErrDBURLNotSet                      = errors.New("db: database url is not set")
	ErrFailedToParseDBURL               = errors.New("db: failed to parse database url")
	ErrFailedToGetMigrationConn         = errors.New("db: failed to get connection for migrations")
	ErrFailedToLockMigrations           = errors.New("db: failed to lock migrations")
)

// migrationFileName is "{4-digit version}_{description}.sql". The fixed-width
// version makes file-name order the apply order.
var migrationFileName = regexp.MustCompile(`^(\d{4})_[a-z0-9_]+\.sql$`)

const (
	schemaMigrationsFile = "schema_migrations.sql"
	migrationsDir        = "migrations"
)

// searchPath makes the unqualified tables of the migrations and queries those
// of the wharf schema. It is set with SET on every new connection rather than
// as a startup parameter, which a connection pooler may not pass on.
const searchPath = "SET search_path TO wharf"

// migrationLockKey names the session advisory lock Migrate holds, so that
// wharf processes starting together on several machines migrate one at a time.
const migrationLockKey int64 = 0x7768617266 // "wharf"

// Connect opens the database and verifies the connection. It fails with
// ErrDBURLNotSet when the URL is empty and with ErrAlreadyConnected while a
// previous connection is open.
func (m *manager) Connect(ctx context.Context) error {
	if m.db != nil {
		return ErrAlreadyConnected
	}

	if m.url == "" {
		return ErrDBURLNotSet
	}

	// pgx redacts the password in parse errors, and the URL carries none.
	config, err := pgx.ParseConfig(m.url)
	if err != nil {
		return errors.Join(ErrFailedToParseDBURL, err)
	}

	config.Password = m.password

	database := stdlib.OpenDB(*config, stdlib.OptionAfterConnect(func(ctx context.Context, conn *pgx.Conn) error {
		// Times are read back at UTC, as they are stored, rather than in the
		// local zone.
		conn.TypeMap().RegisterType(&pgtype.Type{
			Name:  "timestamptz",
			OID:   pgtype.TimestamptzOID,
			Codec: &pgtype.TimestamptzCodec{ScanLocation: time.UTC},
		})

		_, err := conn.Exec(ctx, searchPath)

		return err
	}))

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

// Migrate creates the wharf schema and the schema_migrations table if
// missing, then applies the migrations not yet applied, in version order. Each
// migration runs in its own transaction together with its record in
// schema_migrations, so a failed migration leaves no partial change. It holds
// an advisory lock throughout, so concurrent calls, from this process or
// another, apply each migration once.
//
// Before applying anything it fails with ErrMigrationChecksumMismatch when an
// applied migration file was edited, and with ErrUnknownAppliedMigration when
// the database has a migration this binary does not know, such as one applied
// by a newer version.
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

	// A session lock belongs to one connection, so every statement below runs
	// on conn.
	conn, err := m.db.Conn(ctx)
	if err != nil {
		return errors.Join(ErrFailedToGetMigrationConn, err)
	}

	defer func() { _ = conn.Close() }()

	if _, err := conn.ExecContext(ctx, "SELECT pg_advisory_lock($1)", migrationLockKey); err != nil {
		return errors.Join(ErrFailedToLockMigrations, err)
	}

	// Unlocking runs even when ctx is done; were it to fail, closing conn ends
	// the session and the lock with it.
	defer func() {
		_, err := conn.ExecContext(context.WithoutCancel(ctx), "SELECT pg_advisory_unlock($1)", migrationLockKey)
		if err != nil {
			_ = conn.Raw(func(any) error { return driver.ErrBadConn })
		}
	}()

	// The file uses IF NOT EXISTS, so it is applied on every call without
	// being recorded as a migration.
	if _, err := conn.ExecContext(ctx, string(schema)); err != nil {
		return errors.Join(ErrFailedToCreateMigrationsTable, err)
	}

	queries := New(conn)

	pending, err := pendingMigrations(ctx, queries, m.migrations, migrations)
	if err != nil {
		return err
	}

	applied, err := queries.ListSchemaMigrationVersions(ctx)
	if err != nil {
		return errors.Join(ErrFailedToListAppliedMigrations, err)
	}

	known := make(map[string]bool, len(migrations))
	for _, mig := range migrations {
		known[mig.version] = true
	}

	for _, version := range applied {
		if !known[version] {
			return fmt.Errorf("%w: version=%s", ErrUnknownAppliedMigration, version)
		}
	}

	for _, mig := range pending {
		if err := applyMigration(ctx, conn, mig); err != nil {
			return err
		}
	}

	return nil
}

// pendingMigrations reads every migration file and returns those not yet
// applied, checking that the applied ones are unchanged.
func pendingMigrations(
	ctx context.Context,
	queries *Queries,
	fsys fs.FS,
	migrations []migration,
) ([]migration, error) {
	var pending []migration

	for _, mig := range migrations {
		content, err := fs.ReadFile(fsys, path.Join(migrationsDir, mig.name))
		if err != nil {
			return nil, fmt.Errorf("%w: file=%s", errors.Join(ErrFailedToReadMigrationFile, err), mig.name)
		}

		sum := sha256.Sum256(content)
		mig.content, mig.checksum = string(content), hex.EncodeToString(sum[:])

		applied, err := queries.GetSchemaMigrationChecksum(ctx, mig.version)
		if errors.Is(err, sql.ErrNoRows) {
			pending = append(pending, mig)
			continue
		}

		if err != nil {
			return nil, fmt.Errorf("%w: file=%s", errors.Join(ErrFailedToCheckMigrationApplied, err), mig.name)
		}

		if applied != mig.checksum {
			return nil, fmt.Errorf("%w: file=%s", ErrMigrationChecksumMismatch, mig.name)
		}
	}

	return pending, nil
}

func applyMigration(ctx context.Context, conn *sql.Conn, mig migration) error {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return errors.Join(ErrFailedToBeginMigrationTx, err)
	}

	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, mig.content); err != nil {
		return fmt.Errorf("%w: file=%s", errors.Join(ErrFailedToApplyMigration, err), mig.name)
	}

	err = New(tx).CreateSchemaMigration(ctx, CreateSchemaMigrationParams{
		Version:  mig.version,
		Checksum: mig.checksum,
	})
	if err != nil {
		return fmt.Errorf("%w: file=%s", errors.Join(ErrFailedToRecordMigration, err), mig.name)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("%w: file=%s", errors.Join(ErrFailedToCommitMigrationTx, err), mig.name)
	}

	return nil
}

type migration struct {
	version  string
	name     string
	content  string
	checksum string
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
