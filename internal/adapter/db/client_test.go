package db_test

import (
	"context"
	"database/sql"
	"errors"
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

// open opens a separate connection to the database file at path, since
// Manager has no read query for task fields or relations.
func open(t *testing.T, path string) *sql.DB {
	t.Helper()

	conn, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	return conn
}

func read(t *testing.T, path, id string) readTask {
	t.Helper()

	var task readTask

	err := open(t, path).
		QueryRowContext(context.Background(), "SELECT originated, title, status, start_date, due_date FROM tasks WHERE id = ?", id).
		Scan(&task.originated, &task.title, &task.status, &task.startDate, &task.dueDate)
	require.NoError(t, err)

	return task
}

// parentOf returns the stored parent of id, or "" when it has none.
func parentOf(t *testing.T, path, id string) string {
	t.Helper()

	var parentID string

	err := open(t, path).
		QueryRowContext(context.Background(), "SELECT parent_id FROM task_parents WHERE child_id = ?", id).
		Scan(&parentID)
	if errors.Is(err, sql.ErrNoRows) {
		return ""
	}

	require.NoError(t, err)

	return parentID
}

// linksOf returns the stored links of id in ID order.
func linksOf(t *testing.T, path, id string) []string {
	t.Helper()

	rows, err := open(t, path).
		QueryContext(context.Background(), "SELECT linked_id FROM task_links WHERE task_id = ? ORDER BY linked_id", id)
	require.NoError(t, err)

	defer func() { _ = rows.Close() }()

	links := []string{}

	for rows.Next() {
		var linkedID string
		require.NoError(t, rows.Scan(&linkedID))

		links = append(links, linkedID)
	}

	require.NoError(t, rows.Err())

	return links
}

func requireExists(t *testing.T, manager db.Manager, id string, want bool) {
	t.Helper()

	exists, err := manager.ExistsTask(context.Background(), id)
	require.NoError(t, err)
	require.Equal(t, want, exists, "id=%s", id)
}

func newTask(id string) task.Task {
	return task.Task{ID: id, Originated: task.OriginJira, Title: id, Status: task.StatusTodo}
}

func TestCreateTasks(t *testing.T) {
	manager, path := newFileManager(t, scripts.Migrations())

	startDate := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	dueDate := time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC)

	full := newTask("jira-WHF-1")
	full.StartDate = &startDate
	full.DueDate = &dueDate
	full.ParentID = "jira-WHF-0"
	full.ChildrenIDs = []string{"jira-WHF-2"}
	// Jira can link the same issue more than once with different link types.
	full.LinkedIDs = []string{"jira-WHF-9", "jira-WHF-8", "jira-WHF-9"}

	// jira-WHF-2 goes first: its empty ParentID would otherwise delete the
	// parent claimed by jira-WHF-1, since the later claim wins.
	require.NoError(t, manager.CreateTasks(context.Background(), []task.Task{newTask("jira-WHF-2"), full}))

	got := read(t, path, "jira-WHF-1")
	require.Equal(t, "jira", got.originated)
	require.Equal(t, "todo", got.status)
	require.True(t, got.startDate.Valid)
	require.True(t, startDate.Equal(got.startDate.Time))
	require.True(t, got.dueDate.Valid)
	require.True(t, dueDate.Equal(got.dueDate.Time))

	empty := read(t, path, "jira-WHF-2")
	require.False(t, empty.startDate.Valid)
	require.False(t, empty.dueDate.Valid)

	require.Equal(t, "jira-WHF-0", parentOf(t, path, "jira-WHF-1"))
	require.Equal(t, "jira-WHF-1", parentOf(t, path, "jira-WHF-2"))
	require.Equal(t, []string{"jira-WHF-8", "jira-WHF-9"}, linksOf(t, path, "jira-WHF-1"))
	require.Empty(t, linksOf(t, path, "jira-WHF-2"))

	requireExists(t, manager, "jira-WHF-9", false)
}

func TestCreateTasksEmpty(t *testing.T) {
	manager := newManager(t)

	require.NoError(t, manager.CreateTasks(context.Background(), nil))
	require.NoError(t, manager.CreateTasks(context.Background(), []task.Task{}))
}

func TestCreateTasksOverwritesExistingTask(t *testing.T) {
	manager, path := newFileManager(t, scripts.Migrations())

	existing := newTask("jira-WHF-1")
	existing.Title = "old"
	require.NoError(t, manager.CreateTasks(context.Background(), []task.Task{existing}))

	updated := newTask("jira-WHF-1")
	updated.Title = "new"
	updated.Status = task.StatusDone
	require.NoError(t, manager.CreateTasks(context.Background(), []task.Task{updated, newTask("jira-WHF-2")}))

	require.Equal(t, readTask{originated: "jira", title: "new", status: "done"}, read(t, path, "jira-WHF-1"))
	requireExists(t, manager, "jira-WHF-2", true)
}

func TestCreateTasksReplacesClaimedRelations(t *testing.T) {
	manager, path := newFileManager(t, scripts.Migrations())

	existing := newTask("jira-WHF-1")
	existing.ParentID = "jira-WHF-0"
	existing.ChildrenIDs = []string{"jira-WHF-2"}
	existing.LinkedIDs = []string{"jira-WHF-8", "jira-WHF-9"}
	require.NoError(t, manager.CreateTasks(context.Background(), []task.Task{existing}))

	updated := newTask("jira-WHF-1")
	updated.ChildrenIDs = []string{"jira-WHF-3"}
	require.NoError(t, manager.CreateTasks(context.Background(), []task.Task{updated}))

	// An empty parent and empty links delete the stored ones.
	require.Empty(t, parentOf(t, path, "jira-WHF-1"))
	require.Empty(t, linksOf(t, path, "jira-WHF-1"))
	// A child missing from ChildrenIDs keeps its parent: Jira does not list
	// stories under their epic.
	require.Equal(t, "jira-WHF-1", parentOf(t, path, "jira-WHF-2"))
	require.Equal(t, "jira-WHF-1", parentOf(t, path, "jira-WHF-3"))

	moved := newTask("jira-WHF-1")
	moved.ParentID = "jira-WHF-7"
	moved.LinkedIDs = []string{"jira-WHF-6"}
	require.NoError(t, manager.CreateTasks(context.Background(), []task.Task{moved}))

	require.Equal(t, "jira-WHF-7", parentOf(t, path, "jira-WHF-1"))
	require.Equal(t, []string{"jira-WHF-6"}, linksOf(t, path, "jira-WHF-1"))
}

func TestCreateTasksLaterClaimWins(t *testing.T) {
	manager, path := newFileManager(t, scripts.Migrations())

	first := newTask("jira-WHF-1")
	first.ChildrenIDs = []string{"jira-WHF-3"}

	second := newTask("jira-WHF-2")
	second.ChildrenIDs = []string{"jira-WHF-3"}

	child := newTask("jira-WHF-4")
	child.ParentID = "jira-WHF-1"

	parent := newTask("jira-WHF-2")
	parent.ChildrenIDs = []string{"jira-WHF-4"}

	require.NoError(t, manager.CreateTasks(context.Background(), []task.Task{first, second, child, parent}))

	require.Equal(t, "jira-WHF-2", parentOf(t, path, "jira-WHF-3"))
	require.Equal(t, "jira-WHF-2", parentOf(t, path, "jira-WHF-4"))
}

func TestCreateTasksRollsBackOnFailure(t *testing.T) {
	// The trigger rejects jira-WHF-FAIL after the earlier tasks in the batch
	// are already written.
	manager, path := newFileManager(t, projectMigrationsWith(t, map[string]string{
		"9999_reject_task.sql": "CREATE TRIGGER reject_task BEFORE INSERT ON tasks WHEN NEW.id = 'jira-WHF-FAIL' " +
			"BEGIN SELECT RAISE(ABORT, 'rejected'); END;",
	}))

	existing := newTask("jira-WHF-1")
	existing.Title = "old"
	require.NoError(t, manager.CreateTasks(context.Background(), []task.Task{existing}))

	updated := newTask("jira-WHF-1")
	updated.Title = "new"

	err := manager.CreateTasks(
		context.Background(),
		[]task.Task{updated, newTask("jira-WHF-2"), newTask("jira-WHF-FAIL"), newTask("jira-WHF-3")},
	)
	require.ErrorIs(t, err, db.ErrFailedToCreateTask)
	require.ErrorContains(t, err, "id=jira-WHF-FAIL")

	require.Equal(t, "old", read(t, path, "jira-WHF-1").title)
	requireExists(t, manager, "jira-WHF-2", false)
	requireExists(t, manager, "jira-WHF-3", false)
}

func TestCreateTasksRollsBackOnRelationFailure(t *testing.T) {
	// The trigger rejects one link after the earlier tasks and relations in
	// the batch are already written.
	manager, path := newFileManager(t, projectMigrationsWith(t, map[string]string{
		"9999_reject_link.sql": "CREATE TRIGGER reject_link BEFORE INSERT ON task_links WHEN NEW.linked_id = 'jira-WHF-FAIL' " +
			"BEGIN SELECT RAISE(ABORT, 'rejected'); END;",
	}))

	existing := newTask("jira-WHF-1")
	existing.Title = "old"
	existing.ParentID = "jira-WHF-0"
	existing.LinkedIDs = []string{"jira-WHF-8"}
	require.NoError(t, manager.CreateTasks(context.Background(), []task.Task{existing}))

	updated := newTask("jira-WHF-1")
	updated.Title = "new"
	updated.LinkedIDs = []string{"jira-WHF-9"}

	child := newTask("jira-WHF-2")
	child.ParentID = "jira-WHF-1"
	child.ChildrenIDs = []string{"jira-WHF-5"}

	failing := newTask("jira-WHF-3")
	failing.LinkedIDs = []string{"jira-WHF-FAIL"}

	err := manager.CreateTasks(context.Background(), []task.Task{updated, child, failing})
	require.ErrorIs(t, err, db.ErrFailedToCreateTaskLink)
	require.ErrorContains(t, err, "id=jira-WHF-3")

	require.Equal(t, "old", read(t, path, "jira-WHF-1").title)
	require.Equal(t, "jira-WHF-0", parentOf(t, path, "jira-WHF-1"))
	require.Equal(t, []string{"jira-WHF-8"}, linksOf(t, path, "jira-WHF-1"))
	requireExists(t, manager, "jira-WHF-2", false)
	requireExists(t, manager, "jira-WHF-3", false)
	require.Empty(t, parentOf(t, path, "jira-WHF-2"))
	require.Empty(t, parentOf(t, path, "jira-WHF-5"))
}

func TestCreateTasksRelationStepFailures(t *testing.T) {
	tests := []struct {
		name    string
		trigger string
		seed    bool
		input   func() task.Task
		want    error
		wantMsg string
	}{
		{
			name: "set parent",
			trigger: "CREATE TRIGGER reject BEFORE INSERT ON task_parents WHEN NEW.parent_id = 'jira-WHF-FAIL' " +
				"BEGIN SELECT RAISE(ABORT, 'rejected'); END;",
			input: func() task.Task {
				x := newTask("jira-WHF-1")
				x.ParentID = "jira-WHF-FAIL"

				return x
			},
			want:    db.ErrFailedToSetTaskParent,
			wantMsg: "id=jira-WHF-1",
		},
		{
			name: "delete parent",
			trigger: "CREATE TRIGGER reject BEFORE DELETE ON task_parents WHEN OLD.child_id = 'jira-WHF-1' " +
				"BEGIN SELECT RAISE(ABORT, 'rejected'); END;",
			seed:    true,
			input:   func() task.Task { return newTask("jira-WHF-1") },
			want:    db.ErrFailedToDeleteTaskParent,
			wantMsg: "id=jira-WHF-1",
		},
		{
			name: "set child",
			trigger: "CREATE TRIGGER reject BEFORE INSERT ON task_parents WHEN NEW.child_id = 'jira-WHF-FAIL' " +
				"BEGIN SELECT RAISE(ABORT, 'rejected'); END;",
			input: func() task.Task {
				x := newTask("jira-WHF-1")
				x.ChildrenIDs = []string{"jira-WHF-FAIL"}

				return x
			},
			want:    db.ErrFailedToSetTaskChild,
			wantMsg: "child=jira-WHF-FAIL",
		},
		{
			name: "delete links",
			trigger: "CREATE TRIGGER reject BEFORE DELETE ON task_links WHEN OLD.task_id = 'jira-WHF-1' " +
				"BEGIN SELECT RAISE(ABORT, 'rejected'); END;",
			seed:    true,
			input:   func() task.Task { return newTask("jira-WHF-1") },
			want:    db.ErrFailedToDeleteTaskLinks,
			wantMsg: "id=jira-WHF-1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// The trigger is created by a later migration so seeding is not rejected.
			manager, path := newFileManager(
				t,
				projectMigrationsWith(t, map[string]string{"9999_reject.sql": tt.trigger}),
			)

			if tt.seed {
				seeded := newTask("jira-WHF-1")
				seeded.ParentID = "jira-WHF-0"
				seeded.LinkedIDs = []string{"jira-WHF-8"}
				require.NoError(t, manager.CreateTasks(context.Background(), []task.Task{seeded}))
			}

			input := tt.input()
			input.Title = "changed"

			err := manager.CreateTasks(context.Background(), []task.Task{input})
			require.ErrorIs(t, err, tt.want)
			require.ErrorContains(t, err, tt.wantMsg)

			if tt.seed {
				require.Equal(t, "jira-WHF-1", read(t, path, "jira-WHF-1").title)
				require.Equal(t, "jira-WHF-0", parentOf(t, path, "jira-WHF-1"))
				require.Equal(t, []string{"jira-WHF-8"}, linksOf(t, path, "jira-WHF-1"))
			} else {
				requireExists(t, manager, "jira-WHF-1", false)
			}
		})
	}
}

func TestCreateLocalTask(t *testing.T) {
	// The trigger rejects tasks titled FAIL after their number is allocated.
	manager, path := newFileManager(t, projectMigrationsWith(t, map[string]string{
		"9999_reject_task.sql": "CREATE TRIGGER reject_task BEFORE INSERT ON tasks WHEN NEW.title = 'FAIL' " +
			"BEGIN SELECT RAISE(ABORT, 'rejected'); END;",
	}))

	// ID and Originated are ignored.
	first := newTask("jira-WHF-1")
	first.Title = "first"
	first.ParentID = "local-WHF-000"
	first.LinkedIDs = []string{"jira-WHF-9"}

	id, err := manager.CreateLocalTask(context.Background(), first)
	require.NoError(t, err)
	require.Equal(t, "local-WHF-001", id)
	require.Equal(t, readTask{originated: "local", title: "first", status: "todo"}, read(t, path, id))
	require.Equal(t, "local-WHF-000", parentOf(t, path, id))
	require.Equal(t, []string{"jira-WHF-9"}, linksOf(t, path, id))
	requireExists(t, manager, "jira-WHF-1", false)

	failing := newTask("")
	failing.Title = "FAIL"

	id, err = manager.CreateLocalTask(context.Background(), failing)
	require.ErrorIs(t, err, db.ErrFailedToCreateTask)
	require.ErrorContains(t, err, "id=local-WHF-002")
	require.Empty(t, id)

	// The failed call did not consume its number.
	id, err = manager.CreateLocalTask(context.Background(), newTask(""))
	require.NoError(t, err)
	require.Equal(t, "local-WHF-002", id)

	// Numbers continue after reconnecting.
	require.NoError(t, manager.Close())
	require.NoError(t, manager.Connect(context.Background()))

	id, err = manager.CreateLocalTask(context.Background(), newTask(""))
	require.NoError(t, err)
	require.Equal(t, "local-WHF-003", id)
}

func TestCreateLocalTaskWithoutCounter(t *testing.T) {
	manager := connect(t, projectMigrationsWith(t, map[string]string{
		"9999_drop_counter.sql": "DELETE FROM task_counters;",
	}))
	require.NoError(t, manager.Migrate(context.Background()))

	id, err := manager.CreateLocalTask(context.Background(), newTask(""))
	require.ErrorIs(t, err, db.ErrFailedToAllocateTaskNumber)
	require.Empty(t, id)
}

func TestMigrateCreatesRelationTables(t *testing.T) {
	_, path := newFileManager(t, scripts.Migrations())
	conn := open(t, path)

	rows, err := conn.QueryContext(context.Background(), "SELECT name FROM pragma_table_info('tasks') ORDER BY cid")
	require.NoError(t, err)

	defer func() { _ = rows.Close() }()

	var columns []string

	for rows.Next() {
		var name string
		require.NoError(t, rows.Scan(&name))

		columns = append(columns, name)
	}

	require.NoError(t, rows.Err())
	require.Equal(t, []string{"id", "originated", "title", "description", "status", "start_date", "due_date"}, columns)

	for _, table := range []string{"task_parents", "task_links", "task_counters"} {
		var count int

		err := conn.QueryRowContext(
			context.Background(), "SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?", table,
		).Scan(&count)
		require.NoError(t, err)
		require.Equal(t, 1, count, "table=%s", table)
	}

	var value int

	err = conn.QueryRowContext(context.Background(), "SELECT value FROM task_counters WHERE origin = 'local'").
		Scan(&value)
	require.NoError(t, err)
	require.Equal(t, 0, value)
}

func TestMigrateConvertsRelationsOfExistingTasks(t *testing.T) {
	migrations := projectMigrationsWith(t, nil)
	relations := migrations["migrations/0002_task_relations.sql"]
	delete(migrations, "migrations/0002_task_relations.sql")

	manager, path := newFileManager(t, migrations)

	// Rows as stored by the JSON-column schema of 0001.
	_, err := open(t, path).ExecContext(context.Background(), `
		INSERT INTO tasks (id, originated, title, status, parent_id, children_ids, linked_ids) VALUES
			('WHF-1', 'jira', 'epic', 'todo', 'WHF-0', '["WHF-2","WHF-3"]', '["WHF-9","WHF-8","WHF-9"]'),
			('WHF-3', 'jira', 'story', 'todo', 'WHF-7', '[]', '[]'),
			('N-1', 'notion', 'note', 'todo', '', '[]', '[]');
	`)
	require.NoError(t, err)

	migrations["migrations/0002_task_relations.sql"] = relations

	require.NoError(t, manager.Migrate(context.Background()))

	requireExists(t, manager, "WHF-1", false)
	require.Equal(t, readTask{originated: "jira", title: "epic", status: "todo"}, read(t, path, "jira-WHF-1"))
	requireExists(t, manager, "jira-WHF-3", true)
	requireExists(t, manager, "notion-N-1", true)

	require.Equal(t, "jira-WHF-0", parentOf(t, path, "jira-WHF-1"))
	require.Equal(t, "jira-WHF-1", parentOf(t, path, "jira-WHF-2"))
	// The task's own parent claim wins over the children list of another.
	require.Equal(t, "jira-WHF-7", parentOf(t, path, "jira-WHF-3"))
	require.Empty(t, parentOf(t, path, "notion-N-1"))
	require.Equal(t, []string{"jira-WHF-8", "jira-WHF-9"}, linksOf(t, path, "jira-WHF-1"))
	require.Empty(t, linksOf(t, path, "notion-N-1"))
}

func TestCreateTasksBeginTxFailure(t *testing.T) {
	manager := newManager(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := manager.CreateTasks(ctx, []task.Task{newTask("jira-WHF-1")})
	require.ErrorIs(t, err, db.ErrFailedToBeginTx)

	id, err := manager.CreateLocalTask(ctx, newTask(""))
	require.ErrorIs(t, err, db.ErrFailedToBeginTx)
	require.Empty(t, id)

	requireExists(t, manager, "jira-WHF-1", false)
	requireExists(t, manager, "local-WHF-001", false)
}

func TestExistsTaskFailure(t *testing.T) {
	// These migrations do not create the tasks table, so the query fails.
	manager := connect(t, withMigrations(t, map[string]string{"0001_empty.sql": "SELECT 1;"}))
	require.NoError(t, manager.Migrate(context.Background()))

	exists, err := manager.ExistsTask(context.Background(), "WHF-1")
	require.ErrorIs(t, err, db.ErrFailedToCheckTaskExists)
	require.False(t, exists)
}
