package db_test

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"path/filepath"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/byron1st/wharf/internal/adapter/db"
	"github.com/byron1st/wharf/scripts"
)

// withMigrations returns the project's schema_migrations.sql with files as the
// migrations directory.
func withMigrations(t *testing.T, files map[string]string) fstest.MapFS {
	t.Helper()

	schema, err := fs.ReadFile(scripts.Migrations(), "schema_migrations.sql")
	require.NoError(t, err)

	fsys := fstest.MapFS{"schema_migrations.sql": {Data: schema}}
	for name, content := range files {
		fsys["migrations/"+name] = &fstest.MapFile{Data: []byte(content)}
	}

	return fsys
}

func TestConnectFailure(t *testing.T) {
	manager := db.NewManager(filepath.Join(t.TempDir(), "missing", "wharf.db"), scripts.Migrations())

	require.ErrorIs(t, manager.Connect(context.Background()), db.ErrFailedToPingDatabase)
	require.NoError(t, manager.Close())
}

func TestCloseWithoutConnect(t *testing.T) {
	require.NoError(t, db.NewManager(":memory:", scripts.Migrations()).Close())
}

func TestMigrateKeepsDataAcrossReconnect(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wharf.db")

	first := db.NewManager(path, scripts.Migrations())
	require.NoError(t, first.Connect(context.Background()))
	require.NoError(t, first.Migrate(context.Background()))
	require.NoError(t, first.CreateTasks(context.Background(), []db.Task{newTask("WHF-1")}))
	require.NoError(t, first.Close())

	// Re-applying 0001 would fail on the existing tasks table.
	second := db.NewManager(path, scripts.Migrations())
	require.NoError(t, second.Connect(context.Background()))
	t.Cleanup(func() { _ = second.Close() })
	require.NoError(t, second.Migrate(context.Background()))
	requireExists(t, second, "WHF-1", true)
}

func TestMigrateIsIdempotent(t *testing.T) {
	manager := newManager(t)

	require.NoError(t, manager.Migrate(context.Background()))
	require.NoError(t, manager.CreateTasks(context.Background(), []db.Task{newTask("WHF-1")}))
	requireExists(t, manager, "WHF-1", true)
}

func TestMigrateAppliesInVersionOrder(t *testing.T) {
	// 0002 fails unless 0001 has already created the table.
	manager := connect(t, withMigrations(t, map[string]string{
		"0002_add_column.sql": "ALTER TABLE items ADD COLUMN name TEXT;",
		"0001_create.sql":     "CREATE TABLE items (id INTEGER PRIMARY KEY);",
	}))

	require.NoError(t, manager.Migrate(context.Background()))
}

func TestMigrateTracksAppliedByVersion(t *testing.T) {
	migrations := withMigrations(t, map[string]string{
		"0001_create.sql": "CREATE TABLE items (id INTEGER PRIMARY KEY);",
	})
	manager := connect(t, migrations)
	require.NoError(t, manager.Migrate(context.Background()))

	// Renaming only the description keeps the version, so the file is not
	// re-applied; re-applying would fail on the existing table.
	content := migrations["migrations/0001_create.sql"]
	delete(migrations, "migrations/0001_create.sql")
	migrations["migrations/0001_create_items.sql"] = content

	require.NoError(t, manager.Migrate(context.Background()))
}

func TestMigrateRollsBackFailedMigration(t *testing.T) {
	migrations := withMigrations(t, map[string]string{
		"0001_create.sql": "CREATE TABLE items (id INTEGER PRIMARY KEY);",
		"0002_broken.sql": "CREATE TABLE others (id INTEGER PRIMARY KEY); NOT SQL;",
	})
	manager := connect(t, migrations)

	err := manager.Migrate(context.Background())
	require.ErrorIs(t, err, db.ErrFailedToApplyMigration)
	require.ErrorContains(t, err, "file=0002_broken.sql")

	// The partial "others" table was rolled back, so a fixed 0002 applies
	// cleanly while 0001 is not re-applied.
	migrations["migrations/0002_broken.sql"] = &fstest.MapFile{
		Data: []byte("CREATE TABLE others (id INTEGER PRIMARY KEY);"),
	}

	require.NoError(t, manager.Migrate(context.Background()))
}

func TestMigrateRejectsInvalidFileName(t *testing.T) {
	manager := connect(t, withMigrations(t, map[string]string{"initial.sql": "SELECT 1;"}))

	err := manager.Migrate(context.Background())
	require.ErrorIs(t, err, db.ErrInvalidMigrationFileName)
	require.ErrorContains(t, err, "file=initial.sql")
}

func TestMigrateRejectsDuplicateVersion(t *testing.T) {
	manager := connect(t, withMigrations(t, map[string]string{
		"0001_a.sql": "SELECT 1;",
		"0001_b.sql": "SELECT 1;",
	}))

	err := manager.Migrate(context.Background())
	require.ErrorIs(t, err, db.ErrDuplicateMigrationVersion)
	require.ErrorContains(t, err, "version=0001")
}

func TestMigrateFailures(t *testing.T) {
	schema, err := fs.ReadFile(scripts.Migrations(), "schema_migrations.sql")
	require.NoError(t, err)

	tests := []struct {
		name       string
		migrations fstest.MapFS
		wantErr    error
		wantFile   string
	}{
		{
			name:       "missing migrations directory",
			migrations: fstest.MapFS{"schema_migrations.sql": {Data: schema}},
			wantErr:    db.ErrFailedToReadMigrations,
		},
		{
			name:       "missing schema_migrations.sql",
			migrations: fstest.MapFS{"migrations/0001_create.sql": {Data: []byte("SELECT 1;")}},
			wantErr:    db.ErrFailedToReadMigrationsTable,
		},
		{
			name: "invalid schema_migrations.sql",
			migrations: fstest.MapFS{
				"schema_migrations.sql":      {Data: []byte("NOT SQL;")},
				"migrations/0001_create.sql": {Data: []byte("SELECT 1;")},
			},
			wantErr: db.ErrFailedToCreateMigrationsTable,
		},
		{
			name: "schema_migrations without version column",
			migrations: fstest.MapFS{
				"schema_migrations.sql":      {Data: []byte("CREATE TABLE schema_migrations (id INTEGER);")},
				"migrations/0001_create.sql": {Data: []byte("SELECT 1;")},
			},
			wantErr:  db.ErrFailedToCheckMigrationApplied,
			wantFile: "0001_create.sql",
		},
		{
			name: "unreadable migration file",
			migrations: fstest.MapFS{
				"schema_migrations.sql":           {Data: schema},
				"migrations/0001_dir.sql/content": {Data: []byte("SELECT 1;")},
			},
			wantErr:  db.ErrFailedToReadMigrationFile,
			wantFile: "0001_dir.sql",
		},
		{
			name: "migration drops schema_migrations",
			migrations: fstest.MapFS{
				"schema_migrations.sql":    {Data: schema},
				"migrations/0001_drop.sql": {Data: []byte("DROP TABLE schema_migrations;")},
			},
			wantErr:  db.ErrFailedToRecordMigration,
			wantFile: "0001_drop.sql",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := connect(t, tt.migrations).Migrate(context.Background())
			require.ErrorIs(t, err, tt.wantErr)

			if tt.wantFile != "" {
				require.ErrorContains(t, err, "file="+tt.wantFile)
			}
		})
	}
}

func TestConnectWaitsForLockHeldByAnotherConnection(t *testing.T) {
	manager, path := newFileManager(t, scripts.Migrations())

	other, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = other.Close() })

	conn, err := other.Conn(context.Background())
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	_, err = conn.ExecContext(context.Background(), "BEGIN IMMEDIATE")
	require.NoError(t, err)

	released := make(chan error, 1)

	go func() {
		time.Sleep(200 * time.Millisecond)

		_, err := conn.ExecContext(context.Background(), "COMMIT")
		released <- err
	}()

	// Without a busy timeout this fails immediately with SQLITE_BUSY.
	require.NoError(t, manager.CreateTasks(context.Background(), []db.Task{newTask("WHF-1")}))
	require.NoError(t, <-released)
	requireExists(t, manager, "WHF-1", true)
}

func TestMethodsRequireConnection(t *testing.T) {
	notConnected := db.NewManager(":memory:", scripts.Migrations())

	closed := newManager(t)
	require.NoError(t, closed.Close())

	for name, manager := range map[string]db.Manager{"before connect": notConnected, "after close": closed} {
		t.Run(name, func(t *testing.T) {
			require.ErrorIs(
				t,
				manager.CreateTasks(context.Background(), []db.Task{newTask("WHF-1")}),
				db.ErrNotConnected,
			)

			_, err := manager.ExistsTask(context.Background(), "WHF-1")
			require.ErrorIs(t, err, db.ErrNotConnected)

			require.ErrorIs(t, manager.Migrate(context.Background()), db.ErrNotConnected)
			require.NoError(t, manager.Close())
		})
	}
}

func TestConnectTwiceKeepsExistingConnection(t *testing.T) {
	manager := newManager(t)

	require.ErrorIs(t, manager.Connect(context.Background()), db.ErrAlreadyConnected)
	require.NoError(t, manager.CreateTasks(context.Background(), []db.Task{newTask("WHF-1")}))
	requireExists(t, manager, "WHF-1", true)
}

func TestConnectAfterClose(t *testing.T) {
	manager, _ := newFileManager(t, scripts.Migrations())
	require.NoError(t, manager.CreateTasks(context.Background(), []db.Task{newTask("WHF-1")}))
	require.NoError(t, manager.Close())

	require.NoError(t, manager.Connect(context.Background()))
	requireExists(t, manager, "WHF-1", true)
}

func TestCloseWaitsForInFlightOperations(t *testing.T) {
	manager := newManager(t)

	var wg sync.WaitGroup

	errs := make(chan error, 10)

	for i := range 10 {
		wg.Go(func() {
			errs <- manager.CreateTasks(context.Background(), []db.Task{newTask(fmt.Sprintf("WHF-%d", i))})
		})
	}

	require.NoError(t, manager.Close())
	wg.Wait()
	close(errs)

	// Each operation either finished before Close or saw the closed manager;
	// none hit a closed *sql.DB.
	for err := range errs {
		if err != nil {
			require.ErrorIs(t, err, db.ErrNotConnected)
		}
	}
}
