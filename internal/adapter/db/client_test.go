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
	originated string
	title      string
	status     string
	startDate  sql.NullTime
	dueDate    sql.NullTime
}

// read loads a stored task through a separate connection, since Manager has
// no read query for task fields.
func read(t *testing.T, path, id string) readTask {
	t.Helper()

	var task readTask

	err := open(t, path).QueryRowContext(
		context.Background(),
		"SELECT originated, title, status, start_date, due_date FROM tasks WHERE id = ?",
		id,
	).Scan(&task.originated, &task.title, &task.status, &task.startDate, &task.dueDate)
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

// relations is every stored relation, as "child<parent" for task_hierarchies
// and "task>linked" for task_links rows, sorted.
func relations(t *testing.T, path string) []string {
	t.Helper()

	rows, err := open(t, path).QueryContext(
		context.Background(),
		"SELECT child_id || '<' || parent_id FROM task_hierarchies "+
			"UNION ALL SELECT task_id || '>' || linked_id FROM task_links ORDER BY 1",
	)
	require.NoError(t, err)

	defer func() { _ = rows.Close() }()

	got := []string{}

	for rows.Next() {
		var relation string
		require.NoError(t, rows.Scan(&relation))
		got = append(got, relation)
	}

	require.NoError(t, rows.Err())

	return got
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

func TestInitialMigrationSchema(t *testing.T) {
	_, path := newFileManager(t, scripts.Migrations())
	conn := open(t, path)

	columns := func(table string) []string {
		rows, err := conn.QueryContext(
			context.Background(),
			"SELECT name FROM pragma_table_info(?) ORDER BY cid",
			table,
		)
		require.NoError(t, err)

		defer func() { _ = rows.Close() }()

		var names []string

		for rows.Next() {
			var name string
			require.NoError(t, rows.Scan(&name))
			names = append(names, name)
		}

		require.NoError(t, rows.Err())

		return names
	}

	require.Equal(
		t,
		[]string{"id", "originated", "title", "description", "status", "start_date", "due_date"},
		columns("tasks"),
	)
	require.Equal(t, []string{"child_id", "parent_id"}, columns("task_hierarchies"))
	require.Equal(t, []string{"task_id", "linked_id"}, columns("task_links"))
	require.Equal(t, []string{"name", "value"}, columns("counters"))

	var value int64

	err := conn.QueryRowContext(context.Background(), "SELECT value FROM counters WHERE name = 'local_task'").
		Scan(&value)
	require.NoError(t, err)
	require.Zero(t, value)
}

func TestUpsertTasks(t *testing.T) {
	manager, path := newFileManager(t, scripts.Migrations())

	start := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	due := time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC)

	full := newTask("T1")
	full.StartDate = &start
	full.DueDate = &due
	full.ParentID = "T0"
	full.ChildrenIDs = []string{"T2", "T3"}
	full.LinkedIDs = []string{"T4"}

	require.NoError(t, manager.UpsertTasks(context.Background(), []task.Task{full, newTask("T2")}))

	got := read(t, path, "T1")
	require.True(t, got.startDate.Valid)
	require.True(t, start.Equal(got.startDate.Time))
	require.True(t, got.dueDate.Valid)
	require.True(t, due.Equal(got.dueDate.Time))

	// T2 declares no parent, so storing it after T1 removes the T2<T1 row.
	require.Equal(t, []string{"T1<T0", "T1>T4", "T3<T1"}, relations(t, path))

	require.Equal(t, readTask{originated: "jira", title: "T2", status: "todo"}, read(t, path, "T2"))
	requireExists(t, path, "T9", false)
}

func TestUpsertTasksEmpty(t *testing.T) {
	manager := newManager(t)

	require.NoError(t, manager.UpsertTasks(context.Background(), nil))
	require.NoError(t, manager.UpsertTasks(context.Background(), []task.Task{}))
}

func TestUpsertTasksOverwritesExistingTask(t *testing.T) {
	manager, path := newFileManager(t, scripts.Migrations())

	start := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)

	existing := newTask("T1")
	existing.Title = "old"
	existing.StartDate = &start
	require.NoError(t, manager.UpsertTasks(context.Background(), []task.Task{existing}))

	updated := newTask("T1")
	updated.Title = "new"
	updated.Status = task.StatusDone
	require.NoError(t, manager.UpsertTasks(context.Background(), []task.Task{updated, newTask("T2")}))

	require.Equal(t, readTask{originated: "jira", title: "new", status: "done"}, read(t, path, "T1"))
	requireExists(t, path, "T2", true)
}

func TestUpsertTasksReplacesParent(t *testing.T) {
	manager, path := newFileManager(t, scripts.Migrations())

	child := newTask("T1")
	child.ParentID = "P1"
	require.NoError(t, manager.UpsertTasks(context.Background(), []task.Task{child}))
	require.Equal(t, []string{"T1<P1"}, relations(t, path))

	child.ParentID = "P2"
	require.NoError(t, manager.UpsertTasks(context.Background(), []task.Task{child}))
	require.Equal(t, []string{"T1<P2"}, relations(t, path))

	child.ParentID = ""
	require.NoError(t, manager.UpsertTasks(context.Background(), []task.Task{child}))
	require.Equal(t, []string{}, relations(t, path))
}

func TestUpsertTasksAddsAndMovesChildren(t *testing.T) {
	manager, path := newFileManager(t, scripts.Migrations())

	first := newTask("P1")
	first.ChildrenIDs = []string{"C1", "C2"}
	require.NoError(t, manager.UpsertTasks(context.Background(), []task.Task{first}))

	// P2 takes C2 from P1. P1 no longer lists C1 but keeps it, since a
	// parent's children list may be partial.
	second := newTask("P2")
	second.ChildrenIDs = []string{"C2"}
	first.ChildrenIDs = []string{"C3"}
	require.NoError(t, manager.UpsertTasks(context.Background(), []task.Task{second, first}))

	require.Equal(t, []string{"C1<P1", "C2<P2", "C3<P1"}, relations(t, path))
}

func TestUpsertTasksKeepsStoryUnderEpicWithoutChildren(t *testing.T) {
	manager, path := newFileManager(t, scripts.Migrations())

	story := newTask("STORY")
	story.ParentID = "EPIC"
	require.NoError(t, manager.UpsertTasks(context.Background(), []task.Task{story}))

	// Jira lists only sub-tasks as subtasks, so the Epic declares no children.
	require.NoError(t, manager.UpsertTasks(context.Background(), []task.Task{newTask("EPIC")}))
	require.NoError(t, manager.UpsertTasks(context.Background(), []task.Task{newTask("EPIC"), story}))

	require.Equal(t, []string{"STORY<EPIC"}, relations(t, path))
}

func TestUpsertTasksReplacesOnlyDeclaredLinks(t *testing.T) {
	manager, path := newFileManager(t, scripts.Migrations())

	local := newTask("L1")
	local.LinkedIDs = []string{"T1"}

	jira := newTask("T1")
	jira.LinkedIDs = []string{"T2", "T3"}
	require.NoError(t, manager.UpsertTasks(context.Background(), []task.Task{local, jira}))

	jira.LinkedIDs = []string{"T3", "T4"}
	require.NoError(t, manager.UpsertTasks(context.Background(), []task.Task{jira}))
	require.Equal(t, []string{"L1>T1", "T1>T3", "T1>T4"}, relations(t, path))

	jira.LinkedIDs = []string{}
	require.NoError(t, manager.UpsertTasks(context.Background(), []task.Task{jira}))
	require.Equal(t, []string{"L1>T1"}, relations(t, path))
}

func TestUpsertTasksStoresDuplicateRelationsOnce(t *testing.T) {
	manager, path := newFileManager(t, scripts.Migrations())

	dup := newTask("T1")
	dup.ChildrenIDs = []string{"C1", "C1"}
	dup.LinkedIDs = []string{"T2", "T2"}
	require.NoError(t, manager.UpsertTasks(context.Background(), []task.Task{dup}))

	require.Equal(t, []string{"C1<T1", "T1>T2"}, relations(t, path))
}

func TestUpsertTasksRollsBackOnFailure(t *testing.T) {
	// Each trigger rejects one write so that the failure comes after the
	// earlier writes of the batch.
	tests := []struct {
		name    string
		trigger string
		fail    task.Task
		wantErr error
	}{
		{
			name:    "task",
			trigger: "BEFORE INSERT ON tasks WHEN NEW.id = 'FAIL'",
			fail:    newTask("FAIL"),
			wantErr: db.ErrFailedToUpsertTask,
		},
		{
			name:    "delete parent",
			trigger: "BEFORE DELETE ON task_hierarchies WHEN OLD.child_id = 'T1'",
			fail:    newTask("T1"),
			wantErr: db.ErrFailedToDeleteTaskParent,
		},
		{
			name:    "parent",
			trigger: "BEFORE INSERT ON task_hierarchies WHEN NEW.parent_id = 'FAIL'",
			fail:    task.Task{ID: "T9", ParentID: "FAIL"},
			wantErr: db.ErrFailedToStoreTaskParent,
		},
		{
			name:    "child",
			trigger: "BEFORE INSERT ON task_hierarchies WHEN NEW.child_id = 'FAIL'",
			fail:    task.Task{ID: "T9", ChildrenIDs: []string{"FAIL"}},
			wantErr: db.ErrFailedToStoreTaskChild,
		},
		{
			name:    "delete links",
			trigger: "BEFORE DELETE ON task_links WHEN OLD.task_id = 'T1'",
			fail:    newTask("T1"),
			wantErr: db.ErrFailedToDeleteTaskLinks,
		},
		{
			name:    "link",
			trigger: "BEFORE INSERT ON task_links WHEN NEW.linked_id = 'FAIL'",
			fail:    task.Task{ID: "T9", LinkedIDs: []string{"FAIL"}},
			wantErr: db.ErrFailedToStoreTaskLink,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager, path := newFileManager(t, projectMigrationsWith(t, map[string]string{
				"9999_reject.sql": "CREATE TRIGGER reject " + tt.trigger + " BEGIN SELECT RAISE(ABORT, 'rejected'); END;",
			}))

			existing := newTask("T1")
			existing.Title = "old"
			existing.ParentID = "P1"
			existing.LinkedIDs = []string{"L1"}
			require.NoError(t, manager.UpsertTasks(context.Background(), []task.Task{existing}))

			updated := newTask("T2")
			updated.ParentID = "P2"
			updated.ChildrenIDs = []string{"T1"}
			updated.LinkedIDs = []string{"L2"}

			err := manager.UpsertTasks(context.Background(), []task.Task{updated, tt.fail, newTask("T3")})
			require.ErrorIs(t, err, tt.wantErr)
			require.ErrorContains(t, err, "id="+tt.fail.ID)

			require.Equal(t, "old", read(t, path, "T1").title)
			requireExists(t, path, "T2", false)
			requireExists(t, path, "T3", false)
			require.Equal(t, []string{"T1<P1", "T1>L1"}, relations(t, path))
		})
	}
}

func TestUpsertTasksBeginTxFailure(t *testing.T) {
	manager, path := newFileManager(t, scripts.Migrations())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := manager.UpsertTasks(ctx, []task.Task{newTask("T1")})
	require.ErrorIs(t, err, db.ErrFailedToBeginTx)
	requireExists(t, path, "T1", false)
}

func TestCreateLocalTask(t *testing.T) {
	manager, path := newFileManager(t, scripts.Migrations())

	first := newTask("ignored")
	first.ParentID = "jira-WHF-1"
	first.LinkedIDs = []string{"jira-WHF-2"}

	id, err := manager.CreateLocalTask(context.Background(), first)
	require.NoError(t, err)
	require.Equal(t, "local-WHF-001", id)

	id, err = manager.CreateLocalTask(context.Background(), newTask("ignored"))
	require.NoError(t, err)
	require.Equal(t, "local-WHF-002", id)

	require.Equal(t, readTask{originated: "local", title: "ignored", status: "todo"}, read(t, path, "local-WHF-001"))
	requireExists(t, path, "ignored", false)
	require.Equal(t, []string{"local-WHF-001<jira-WHF-1", "local-WHF-001>jira-WHF-2"}, relations(t, path))
}

func TestCreateLocalTaskPastThreeDigits(t *testing.T) {
	manager, path := newFileManager(t, scripts.Migrations())

	_, err := open(
		t,
		path,
	).ExecContext(context.Background(), "UPDATE counters SET value = 999 WHERE name = 'local_task'")
	require.NoError(t, err)

	id, err := manager.CreateLocalTask(context.Background(), newTask("ignored"))
	require.NoError(t, err)
	require.Equal(t, "local-WHF-1000", id)
}

func TestCreateLocalTaskFailureKeepsNumber(t *testing.T) {
	manager, path := newFileManager(t, projectMigrationsWith(t, map[string]string{
		"9999_reject.sql": "CREATE TRIGGER reject BEFORE INSERT ON task_links WHEN NEW.linked_id = 'FAIL' " +
			"BEGIN SELECT RAISE(ABORT, 'rejected'); END;",
	}))

	id, err := manager.CreateLocalTask(context.Background(), newTask("first"))
	require.NoError(t, err)
	require.Equal(t, "local-WHF-001", id)

	failing := newTask("failing")
	failing.LinkedIDs = []string{"FAIL"}

	id, err = manager.CreateLocalTask(context.Background(), failing)
	require.ErrorIs(t, err, db.ErrFailedToStoreTaskLink)
	require.ErrorContains(t, err, "id=local-WHF-002")
	require.Empty(t, id)
	requireExists(t, path, "local-WHF-002", false)

	id, err = manager.CreateLocalTask(context.Background(), newTask("second"))
	require.NoError(t, err)
	require.Equal(t, "local-WHF-002", id)
	require.Equal(t, "second", read(t, path, "local-WHF-002").title)
}

func TestCreateLocalTaskCounterFailure(t *testing.T) {
	manager, path := newFileManager(t, scripts.Migrations())

	_, err := open(t, path).ExecContext(context.Background(), "DELETE FROM counters")
	require.NoError(t, err)

	id, err := manager.CreateLocalTask(context.Background(), newTask("ignored"))
	require.ErrorIs(t, err, db.ErrFailedToIncrementLocalTaskCounter)
	require.ErrorIs(t, err, sql.ErrNoRows)
	require.Empty(t, id)
}
