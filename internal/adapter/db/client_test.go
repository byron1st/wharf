package db_test

import (
	"context"
	"database/sql"
	"io/fs"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/byron1st/wharf/internal/adapter/db"
	"github.com/byron1st/wharf/scripts"
)

// newManager connects a manager to a fresh in-memory database with the
// project migrations applied.
func newManager(t *testing.T) db.Manager {
	t.Helper()

	manager := connect(t, scripts.Migrations())
	require.NoError(t, manager.Migrate(context.Background()))

	return manager
}

func connect(t *testing.T, migrations fs.FS) db.Manager {
	t.Helper()

	manager := db.NewManager(":memory:", migrations)
	require.NoError(t, manager.Connect(context.Background()))
	t.Cleanup(func() { _ = manager.Close() })

	return manager
}

// newFileManager connects a manager to a fresh database file migrated with
// migrations, and returns the file path so tests can read rows directly.
func newFileManager(t *testing.T, migrations fs.FS) (db.Manager, string) {
	t.Helper()

	path := filepath.Join(t.TempDir(), "wharf.db")

	manager := db.NewManager(path, migrations)
	require.NoError(t, manager.Connect(context.Background()))
	t.Cleanup(func() { _ = manager.Close() })
	require.NoError(t, manager.Migrate(context.Background()))

	return manager, path
}

// projectMigrationsWith returns the project migrations plus extra files in
// the migrations directory.
func projectMigrationsWith(t *testing.T, extra map[string]string) fstest.MapFS {
	t.Helper()

	fsys := fstest.MapFS{}
	err := fs.WalkDir(scripts.Migrations(), ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}

		data, err := fs.ReadFile(scripts.Migrations(), name)
		fsys[name] = &fstest.MapFile{Data: data}

		return err
	})
	require.NoError(t, err)

	for name, content := range extra {
		fsys["migrations/"+name] = &fstest.MapFile{Data: []byte(content)}
	}

	return fsys
}

type readTask struct {
	title    string
	status   string
	parentID string
}

// read loads a stored task through a separate connection, since Manager has
// no read query for task fields.
func read(t *testing.T, path, id string) readTask {
	t.Helper()

	conn, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	var task readTask

	err = conn.QueryRowContext(context.Background(), "SELECT title, status, parent_id FROM tasks WHERE id = ?", id).
		Scan(&task.title, &task.status, &task.parentID)
	require.NoError(t, err)

	return task
}

func requireExists(t *testing.T, manager db.Manager, id string, want bool) {
	t.Helper()

	exists, err := manager.ExistsTask(context.Background(), id)
	require.NoError(t, err)
	require.Equal(t, want, exists, "id=%s", id)
}

func newTask(id string) db.Task {
	return db.Task{ID: id, Originated: "jira", Title: id, Status: "todo", ChildrenIds: "[]", LinkedIds: "[]"}
}

func TestCreateTasks(t *testing.T) {
	manager := newManager(t)

	full := newTask("WHF-1")
	full.StartDate = sql.NullTime{Time: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), Valid: true}
	full.DueDate = sql.NullTime{Time: time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC), Valid: true}
	full.ParentID = "WHF-0"
	full.ChildrenIds = `["WHF-2"]`

	require.NoError(t, manager.CreateTasks(context.Background(), []db.Task{full, newTask("WHF-2")}))

	requireExists(t, manager, "WHF-1", true)
	requireExists(t, manager, "WHF-2", true)
	requireExists(t, manager, "WHF-9", false)
}

func TestCreateTasksEmpty(t *testing.T) {
	manager := newManager(t)

	require.NoError(t, manager.CreateTasks(context.Background(), nil))
	require.NoError(t, manager.CreateTasks(context.Background(), []db.Task{}))
}

func TestCreateTasksOverwritesExistingTask(t *testing.T) {
	manager, path := newFileManager(t, scripts.Migrations())

	existing := newTask("WHF-1")
	existing.Title = "old"
	existing.ParentID = "WHF-0"
	require.NoError(t, manager.CreateTasks(context.Background(), []db.Task{existing}))

	updated := newTask("WHF-1")
	updated.Title = "new"
	updated.Status = "done"
	require.NoError(t, manager.CreateTasks(context.Background(), []db.Task{updated, newTask("WHF-2")}))

	require.Equal(t, readTask{title: "new", status: "done", parentID: ""}, read(t, path, "WHF-1"))
	requireExists(t, manager, "WHF-2", true)
}

func TestCreateTasksRollsBackOnFailure(t *testing.T) {
	// The trigger rejects WHF-FAIL after the earlier tasks in the batch are
	// already written.
	manager, path := newFileManager(t, projectMigrationsWith(t, map[string]string{
		"9999_reject_task.sql": "CREATE TRIGGER reject_task BEFORE INSERT ON tasks WHEN NEW.id = 'WHF-FAIL' " +
			"BEGIN SELECT RAISE(ABORT, 'rejected'); END;",
	}))

	existing := newTask("WHF-1")
	existing.Title = "old"
	require.NoError(t, manager.CreateTasks(context.Background(), []db.Task{existing}))

	updated := newTask("WHF-1")
	updated.Title = "new"

	err := manager.CreateTasks(
		context.Background(),
		[]db.Task{updated, newTask("WHF-2"), newTask("WHF-FAIL"), newTask("WHF-3")},
	)
	require.ErrorIs(t, err, db.ErrFailedToCreateTask)
	require.ErrorContains(t, err, "id=WHF-FAIL")

	require.Equal(t, "old", read(t, path, "WHF-1").title)
	requireExists(t, manager, "WHF-2", false)
	requireExists(t, manager, "WHF-3", false)
}

func TestCreateTasksBeginTxFailure(t *testing.T) {
	manager := newManager(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := manager.CreateTasks(ctx, []db.Task{newTask("WHF-1")})
	require.ErrorIs(t, err, db.ErrFailedToBeginTx)
	requireExists(t, manager, "WHF-1", false)
}

func TestExistsTaskFailure(t *testing.T) {
	manager := newManager(t)
	require.NoError(t, manager.Close())

	exists, err := manager.ExistsTask(context.Background(), "WHF-1")
	require.ErrorIs(t, err, db.ErrFailedToCheckTaskExists)
	require.False(t, exists)
}
