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
	"github.com/byron1st/wharf/internal/pkg/task"
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
	title       string
	status      string
	startDate   sql.NullTime
	dueDate     sql.NullTime
	parentID    string
	childrenIDs string
	linkedIDs   string
}

// read loads a stored task through a separate connection, since Manager has
// no read query for task fields.
func read(t *testing.T, path, id string) readTask {
	t.Helper()

	var task readTask

	err := open(t, path).QueryRowContext(
		context.Background(),
		"SELECT title, status, start_date, due_date, parent_id, children_ids, linked_ids FROM tasks WHERE id = ?",
		id,
	).Scan(&task.title, &task.status, &task.startDate, &task.dueDate, &task.parentID, &task.childrenIDs, &task.linkedIDs)
	require.NoError(t, err)

	return task
}

// requireExists checks through a separate connection whether a task with id
// is stored, since Manager has no read query for tasks.
func requireExists(t *testing.T, path, id string, want bool) {
	t.Helper()

	var exists bool

	err := open(t, path).QueryRowContext(context.Background(), "SELECT EXISTS (SELECT 1 FROM tasks WHERE id = ?)", id).
		Scan(&exists)
	require.NoError(t, err)
	require.Equal(t, want, exists, "id=%s", id)
}

func open(t *testing.T, path string) *sql.DB {
	t.Helper()

	conn, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	return conn
}

func newTask(id string) task.Task {
	return task.Task{ID: id, Originated: task.OriginJira, Title: id, Status: task.StatusTodo}
}

func TestUpsertTasks(t *testing.T) {
	manager, path := newFileManager(t, scripts.Migrations())

	start := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	due := time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC)

	full := newTask("WHF-1")
	full.StartDate = &start
	full.DueDate = &due
	full.ParentID = "WHF-0"
	full.ChildrenIDs = []string{"WHF-2", "WHF-3"}
	full.LinkedIDs = []string{"WHF-4"}

	require.NoError(t, manager.UpsertTasks(context.Background(), []task.Task{full, newTask("WHF-2")}))

	got := read(t, path, "WHF-1")
	require.Equal(t, `["WHF-2","WHF-3"]`, got.childrenIDs)
	require.Equal(t, `["WHF-4"]`, got.linkedIDs)
	require.Equal(t, "WHF-0", got.parentID)
	require.True(t, got.startDate.Valid)
	require.True(t, start.Equal(got.startDate.Time))
	require.True(t, got.dueDate.Valid)
	require.True(t, due.Equal(got.dueDate.Time))

	require.Equal(
		t,
		readTask{title: "WHF-2", status: "todo", childrenIDs: "[]", linkedIDs: "[]"},
		read(t, path, "WHF-2"),
	)
	requireExists(t, path, "WHF-9", false)
}

func TestUpsertTasksEmpty(t *testing.T) {
	manager := newManager(t)

	require.NoError(t, manager.UpsertTasks(context.Background(), nil))
	require.NoError(t, manager.UpsertTasks(context.Background(), []task.Task{}))
}

func TestUpsertTasksOverwritesExistingTask(t *testing.T) {
	manager, path := newFileManager(t, scripts.Migrations())

	start := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)

	existing := newTask("WHF-1")
	existing.Title = "old"
	existing.ParentID = "WHF-0"
	existing.StartDate = &start
	require.NoError(t, manager.UpsertTasks(context.Background(), []task.Task{existing}))

	updated := newTask("WHF-1")
	updated.Title = "new"
	updated.Status = task.StatusDone
	require.NoError(t, manager.UpsertTasks(context.Background(), []task.Task{updated, newTask("WHF-2")}))

	require.Equal(
		t,
		readTask{title: "new", status: "done", childrenIDs: "[]", linkedIDs: "[]"},
		read(t, path, "WHF-1"),
	)
	requireExists(t, path, "WHF-2", true)
}

func TestUpsertTasksStoresEmptyIDListsAsJSONArray(t *testing.T) {
	manager, path := newFileManager(t, scripts.Migrations())

	existing := newTask("WHF-1")
	existing.ChildrenIDs = []string{"WHF-2"}
	existing.LinkedIDs = []string{"WHF-3"}
	require.NoError(t, manager.UpsertTasks(context.Background(), []task.Task{existing}))

	// The overwrite sets empty lists and the new task leaves them nil.
	overwrite := newTask("WHF-1")
	overwrite.ChildrenIDs = []string{}
	overwrite.LinkedIDs = []string{}
	require.NoError(t, manager.UpsertTasks(context.Background(), []task.Task{overwrite, newTask("WHF-4")}))

	for _, id := range []string{"WHF-1", "WHF-4"} {
		got := read(t, path, id)
		require.Equal(t, "[]", got.childrenIDs, "id=%s", id)
		require.Equal(t, "[]", got.linkedIDs, "id=%s", id)
	}
}

func TestUpsertTasksRollsBackOnFailure(t *testing.T) {
	// The trigger rejects WHF-FAIL after the earlier tasks in the batch are
	// already written.
	manager, path := newFileManager(t, projectMigrationsWith(t, map[string]string{
		"9999_reject_task.sql": "CREATE TRIGGER reject_task BEFORE INSERT ON tasks WHEN NEW.id = 'WHF-FAIL' " +
			"BEGIN SELECT RAISE(ABORT, 'rejected'); END;",
	}))

	existing := newTask("WHF-1")
	existing.Title = "old"
	require.NoError(t, manager.UpsertTasks(context.Background(), []task.Task{existing}))

	updated := newTask("WHF-1")
	updated.Title = "new"

	err := manager.UpsertTasks(
		context.Background(),
		[]task.Task{updated, newTask("WHF-2"), newTask("WHF-FAIL"), newTask("WHF-3")},
	)
	require.ErrorIs(t, err, db.ErrFailedToUpsertTask)
	require.ErrorContains(t, err, "id=WHF-FAIL")

	require.Equal(t, "old", read(t, path, "WHF-1").title)
	requireExists(t, path, "WHF-2", false)
	requireExists(t, path, "WHF-3", false)
}

func TestUpsertTasksBeginTxFailure(t *testing.T) {
	manager, path := newFileManager(t, scripts.Migrations())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := manager.UpsertTasks(ctx, []task.Task{newTask("WHF-1")})
	require.ErrorIs(t, err, db.ErrFailedToBeginTx)
	requireExists(t, path, "WHF-1", false)
}
