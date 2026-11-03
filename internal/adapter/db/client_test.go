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
		[]string{"id", "originated", "title", "description", "status", "start_date", "due_date", "position"},
		columns("tasks"),
	)
	require.Equal(t, []string{"child_id", "parent_id"}, columns("task_hierarchies"))
	require.Equal(t, []string{"task_id", "linked_id"}, columns("task_links"))
	require.Equal(t, []string{"name", "value"}, columns("counters"))
	require.Equal(
		t,
		[]string{"id", "description", "note", "is_done", "completed_at", "start_date", "end_date"},
		columns("todos"),
	)
	require.Equal(t, []string{"todo_id", "task_id", "position"}, columns("task_todos"))
	require.Equal(t, []string{"todo_id", "remind_at"}, columns("todo_reminders"))
	require.Equal(
		t,
		[]string{"issue_key", "title", "status", "status_category", "created_at", "status_at"},
		columns("service_desk_requests"),
	)
	require.Equal(t, []string{"todo_id", "issue_key"}, columns("todo_service_desk_requests"))

	var linkPK string

	err := conn.QueryRowContext(
		context.Background(),
		"SELECT group_concat(name, ',') FROM "+
			"(SELECT name FROM pragma_table_info('todo_service_desk_requests') WHERE pk > 0 ORDER BY pk)",
	).Scan(&linkPK)
	require.NoError(t, err)
	require.Equal(t, "todo_id,issue_key", linkPK)

	var todoIDIsPK bool

	err = conn.QueryRowContext(
		context.Background(),
		"SELECT pk = 1 FROM pragma_table_info('task_todos') WHERE name = 'todo_id'",
	).Scan(&todoIDIsPK)
	require.NoError(t, err)
	require.True(t, todoIDIsPK)

	var value int64

	err = conn.QueryRowContext(context.Background(), "SELECT value FROM counters WHERE name = 'local_task'").
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

func TestInitialMigrationIndexes(t *testing.T) {
	_, path := newFileManager(t, scripts.Migrations())

	rows, err := open(t, path).QueryContext(
		context.Background(),
		"SELECT tbl_name || '(' || name || ')' FROM sqlite_master WHERE type = 'index' AND sql IS NOT NULL ORDER BY 1",
	)
	require.NoError(t, err)

	defer func() { _ = rows.Close() }()

	var got []string

	for rows.Next() {
		var index string
		require.NoError(t, rows.Scan(&index))
		got = append(got, index)
	}

	require.NoError(t, rows.Err())
	require.Equal(
		t,
		[]string{
			"task_hierarchies(task_hierarchies_parent_id)",
			"task_links(task_links_linked_id)",
			"task_todos(task_todos_task_id_position)",
		},
		got,
	)
}

func TestUpsertTasksKeepsLocalParentOfJiraTask(t *testing.T) {
	manager, path := newFileManager(t, scripts.Migrations())

	local := newTask("ignored")
	local.ChildrenIDs = []string{"jira-J"}

	id, err := manager.CreateLocalTask(context.Background(), local)
	require.NoError(t, err)

	// J declares no parent, so the local parent that claimed it stays.
	require.NoError(t, manager.UpsertTasks(context.Background(), []task.Task{newTask("jira-J")}))
	require.Equal(t, []string{"jira-J<" + id}, relations(t, path))

	// A declared parent still replaces the local one.
	child := newTask("jira-J")
	child.ParentID = "jira-P"
	require.NoError(t, manager.UpsertTasks(context.Background(), []task.Task{child}))
	require.Equal(t, []string{"jira-J<jira-P"}, relations(t, path))

	// A non-local parent is dropped as before once J stops declaring it.
	require.NoError(t, manager.UpsertTasks(context.Background(), []task.Task{newTask("jira-J")}))
	require.Equal(t, []string{}, relations(t, path))
}

func TestListTasks(t *testing.T) {
	manager := newManager(t)

	got, err := manager.ListTasks(context.Background())
	require.NoError(t, err)
	require.Equal(t, []task.Task{}, got)

	due := time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC)

	second := newTask("T2")
	second.Description = "details"
	second.Status = task.StatusDone
	second.DueDate = &due
	second.ParentID = "T1"
	second.LinkedIDs = []string{"T1"}
	require.NoError(t, manager.UpsertTasks(context.Background(), []task.Task{second, newTask("T1")}))

	got, err = manager.ListTasks(context.Background())
	require.NoError(t, err)
	require.Len(t, got, 2)
	require.Equal(t, newTask("T1"), got[0])
	require.NotNil(t, got[1].DueDate)
	require.True(t, due.Equal(*got[1].DueDate))

	got[1].DueDate = nil
	require.Equal(
		t,
		task.Task{ID: "T2", Originated: task.OriginJira, Title: "T2", Description: "details", Status: task.StatusDone},
		got[1],
	)
}

func TestListTasksByIDs(t *testing.T) {
	manager := newManager(t)
	require.NoError(
		t,
		manager.UpsertTasks(context.Background(), []task.Task{newTask("T1"), newTask("T2"), newTask("T3")}),
	)

	got, err := manager.ListTasksByIDs(context.Background(), []string{"T3", "MISSING", "T1"})
	require.NoError(t, err)
	require.Equal(t, []task.Task{newTask("T1"), newTask("T3")}, got)

	got, err = manager.ListTasksByIDs(context.Background(), nil)
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestGetTask(t *testing.T) {
	manager := newManager(t)

	start := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)

	target := newTask("T1")
	target.Description = "full description"
	target.StartDate = &start
	target.ParentID = "P"
	target.ChildrenIDs = []string{"C2", "C1"}
	target.LinkedIDs = []string{"L1", "L2"}

	// L2 links back to T1 and L3 links only to T1, so both directions count
	// and L2 is listed once.
	back := newTask("L2")
	back.LinkedIDs = []string{"T1"}

	other := newTask("L3")
	other.LinkedIDs = []string{"T1"}

	// C1 is stored and C2 is not; both stay children.
	child := newTask("C1")
	child.ParentID = "T1"

	require.NoError(t, manager.UpsertTasks(context.Background(), []task.Task{target, back, other, child}))

	got, err := manager.GetTask(context.Background(), "T1")
	require.NoError(t, err)
	require.NotNil(t, got.StartDate)
	require.True(t, start.Equal(*got.StartDate))
	require.Nil(t, got.DueDate)

	got.StartDate = nil
	require.Equal(t, task.Task{
		ID:          "T1",
		Originated:  task.OriginJira,
		Title:       "T1",
		Description: "full description",
		Status:      task.StatusTodo,
		ParentID:    "P",
		ChildrenIDs: []string{"C1", "C2"},
		LinkedIDs:   []string{"L1", "L2", "L3"},
	}, got)

	got, err = manager.GetTask(context.Background(), "C1")
	require.NoError(t, err)
	require.Equal(
		t,
		task.Task{ID: "C1", Originated: task.OriginJira, Title: "C1", Status: task.StatusTodo, ParentID: "T1"},
		got,
	)

	_, err = manager.GetTask(context.Background(), "MISSING")
	require.ErrorIs(t, err, db.ErrTaskNotFound)
	require.ErrorContains(t, err, "id=MISSING")
}

func TestUpdateLocalTask(t *testing.T) {
	manager, path := newFileManager(t, scripts.Migrations())

	created := newTask("ignored")
	created.Title = "old"
	created.ParentID = "P1"
	created.ChildrenIDs = []string{"C1", "C2"}
	created.LinkedIDs = []string{"L1"}

	id, err := manager.CreateLocalTask(context.Background(), created)
	require.NoError(t, err)

	// C3 moves from its Jira parent to the local task, and the link J1 declared
	// to it is dropped since the update does not list J1.
	jira := newTask("J1")
	jira.ChildrenIDs = []string{"C3"}
	jira.LinkedIDs = []string{id}
	require.NoError(t, manager.UpsertTasks(context.Background(), []task.Task{jira}))

	due := time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC)

	updated := task.Task{
		ID:          id,
		Originated:  task.OriginJira,
		Title:       "new",
		Status:      task.StatusInProgress,
		DueDate:     &due,
		ParentID:    "P2",
		ChildrenIDs: []string{"C3"},
		LinkedIDs:   []string{"L2"},
	}
	require.NoError(t, manager.UpdateLocalTask(context.Background(), updated))

	got := read(t, path, id)
	require.True(t, got.dueDate.Valid)
	require.True(t, due.Equal(got.dueDate.Time))

	got.dueDate = sql.NullTime{}
	require.Equal(t, readTask{originated: "local", title: "new", status: "in-progress"}, got)
	require.Equal(t, []string{"C3<" + id, id + "<P2", id + ">L2"}, relations(t, path))
}

func TestUpdateLocalTaskRejectsMissingOrNonLocalTask(t *testing.T) {
	manager, path := newFileManager(t, scripts.Migrations())

	jira := newTask("J1")
	jira.Title = "old"
	jira.ChildrenIDs = []string{"C1"}
	require.NoError(t, manager.UpsertTasks(context.Background(), []task.Task{jira}))

	for _, id := range []string{"J1", "local-WHF-404"} {
		err := manager.UpdateLocalTask(context.Background(), task.Task{ID: id, Title: "new", ParentID: "P"})
		require.ErrorIs(t, err, db.ErrLocalTaskToUpdateNotFound)
		require.ErrorContains(t, err, "id="+id)
	}

	require.Equal(t, "old", read(t, path, "J1").title)
	requireExists(t, path, "local-WHF-404", false)
	require.Equal(t, []string{"C1<J1"}, relations(t, path))
}

func TestUpdateLocalTaskStatus(t *testing.T) {
	manager, path := newFileManager(t, scripts.Migrations())

	created := newTask("ignored")
	created.Title = "local"
	created.ChildrenIDs = []string{"C1"}

	id, err := manager.CreateLocalTask(context.Background(), created)
	require.NoError(t, err)
	require.NoError(t, manager.UpsertTasks(context.Background(), []task.Task{newTask("J1")}))

	require.NoError(t, manager.UpdateLocalTaskStatus(context.Background(), id, task.StatusDone))
	require.Equal(t, readTask{originated: "local", title: "local", status: "done"}, read(t, path, id))
	require.Equal(t, []string{"C1<" + id}, relations(t, path))

	for _, missing := range []string{"J1", "local-WHF-404"} {
		err := manager.UpdateLocalTaskStatus(context.Background(), missing, task.StatusDone)
		require.ErrorIs(t, err, db.ErrLocalTaskToSetStatusNotFound)
		require.ErrorContains(t, err, "id="+missing)
	}

	require.Equal(t, "todo", read(t, path, "J1").status)
}

func TestSetTaskPositions(t *testing.T) {
	manager := newManager(t)
	require.NoError(
		t,
		manager.UpsertTasks(context.Background(), []task.Task{newTask("T1"), newTask("T2"), newTask("T3")}),
	)

	positions := func() map[string]*int {
		tasks, err := manager.ListTasks(context.Background())
		require.NoError(t, err)

		got := map[string]*int{}
		for _, t := range tasks {
			got[t.ID] = t.Position
		}

		return got
	}

	require.Equal(t, map[string]*int{"T1": nil, "T2": nil, "T3": nil}, positions())

	require.NoError(t, manager.SetTaskPositions(context.Background(), []string{"T3", "MISSING", "T1"}))
	require.Equal(t, map[string]*int{"T1": new(2), "T2": nil, "T3": new(0)}, positions())

	got, err := manager.GetTask(context.Background(), "T1")
	require.NoError(t, err)
	require.Equal(t, new(2), got.Position)

	// A sync keeps the positions, and tasks left out keep theirs.
	require.NoError(t, manager.UpsertTasks(context.Background(), []task.Task{newTask("T3")}))
	require.NoError(t, manager.SetTaskPositions(context.Background(), []string{"T2"}))
	require.Equal(t, map[string]*int{"T1": new(2), "T2": new(0), "T3": new(0)}, positions())
}

func TestDeleteTask(t *testing.T) {
	manager, path := newFileManager(t, scripts.Migrations())

	target := newTask("T1")
	target.ParentID = "P"
	target.ChildrenIDs = []string{"C1"}
	target.LinkedIDs = []string{"L1"}

	back := newTask("L2")
	back.LinkedIDs = []string{"T1", "L1"}

	sibling := newTask("S1")
	sibling.ParentID = "P"

	child := newTask("C1")
	child.ParentID = "T1"

	require.NoError(t, manager.UpsertTasks(context.Background(), []task.Task{target, back, sibling, child}))
	require.Equal(t, []string{"C1<T1", "L2>L1", "L2>T1", "S1<P", "T1<P", "T1>L1"}, relations(t, path))
	require.NoError(t, manager.DeleteTask(context.Background(), "T1"))

	requireExists(t, path, "T1", false)
	requireExists(t, path, "C1", true)
	requireExists(t, path, "L2", true)
	requireExists(t, path, "S1", true)
	require.Equal(t, []string{"L2>L1", "S1<P"}, relations(t, path))

	err := manager.DeleteTask(context.Background(), "T1")
	require.ErrorIs(t, err, db.ErrTaskToDeleteNotFound)
	require.ErrorContains(t, err, "id=T1")
}
