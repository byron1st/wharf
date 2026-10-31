package db_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/byron1st/wharf/internal/adapter/db"
	"github.com/byron1st/wharf/internal/pkg/task"
	"github.com/byron1st/wharf/internal/pkg/todo"
	"github.com/byron1st/wharf/scripts"
)

// texts returns the rows of query, which selects one text column.
func texts(t *testing.T, path, query string) []string {
	t.Helper()

	rows, err := open(t, path).QueryContext(context.Background(), query)
	require.NoError(t, err)

	defer func() { _ = rows.Close() }()

	got := []string{}

	for rows.Next() {
		var s string
		require.NoError(t, rows.Scan(&s))
		got = append(got, s)
	}

	require.NoError(t, rows.Err())

	return got
}

// placements is every task_todos row as "task:position:todo", sorted.
func placements(t *testing.T, path string) []string {
	t.Helper()

	return texts(
		t,
		path,
		"SELECT task_id || ':' || position || ':' || todo_id FROM task_todos ORDER BY task_id, position",
	)
}

// stored is the IDs in todos and in todo_reminders, as "todos" and "reminders"
// lists of "id" strings, sorted.
func stored(t *testing.T, path string) (todos, reminders []string) {
	t.Helper()

	return texts(t, path, "SELECT id FROM todos ORDER BY id"),
		texts(t, path, "SELECT todo_id FROM todo_reminders ORDER BY todo_id, remind_at")
}

func newTodoManager(t *testing.T) (db.Manager, string) {
	t.Helper()

	manager, path := newFileManager(t, scripts.Migrations())
	require.NoError(t, manager.UpsertTasks(context.Background(), []task.Task{newTask("J1"), newTask("J2")}))

	return manager, path
}

func create(t *testing.T, manager db.Manager, td todo.Todo) int64 {
	t.Helper()

	n, err := manager.CreateTodo(context.Background(), td)
	require.NoError(t, err)

	return n
}

func TestTaskTodosIndexColumns(t *testing.T) {
	_, path := newFileManager(t, scripts.Migrations())
	require.Equal(
		t,
		[]string{"task_id", "position"},
		texts(t, path, "SELECT name FROM pragma_index_info('task_todos_task_id_position') ORDER BY seqno"),
	)
}

func TestCreateTodo(t *testing.T) {
	manager, path := newTodoManager(t)

	localID, err := manager.CreateLocalTask(context.Background(), task.Task{Title: "L", Status: task.StatusTodo})
	require.NoError(t, err)

	at := time.Date(2026, 10, 2, 9, 0, 0, 0, time.UTC)

	a := create(t, manager, todo.Todo{TaskID: "J1", Description: "a", Reminders: []time.Time{at, at}})
	b := create(t, manager, todo.Todo{TaskID: "J1", Description: "b"})
	c := create(t, manager, todo.Todo{TaskID: "J1", Description: "c", Position: new(0)})
	d := create(t, manager, todo.Todo{TaskID: "J1", Description: "d", Position: new(99)})
	e := create(t, manager, todo.Todo{TaskID: localID, Description: "e"})

	require.Equal(t, []string{
		"J1:0:" + fmt.Sprint(c),
		"J1:1:" + fmt.Sprint(a),
		"J1:2:" + fmt.Sprint(b),
		"J1:3:" + fmt.Sprint(d),
		localID + ":0:" + fmt.Sprint(e),
	}, placements(t, path))

	_, reminders := stored(t, path)
	require.Equal(t, []string{fmt.Sprint(a)}, reminders)
}

func TestCreateTodoRejectsMissingTask(t *testing.T) {
	manager, path := newTodoManager(t)

	_, err := manager.CreateTodo(context.Background(), todo.Todo{
		TaskID: "nope", Description: "x", Reminders: []time.Time{time.Now()},
	})
	require.ErrorIs(t, err, db.ErrTaskOfTodoToCreateNotFound)
	require.ErrorContains(t, err, "task_id=nope")

	todos, reminders := stored(t, path)
	require.Empty(t, todos)
	require.Empty(t, reminders)
	require.Empty(t, placements(t, path))
}

func TestListTodos(t *testing.T) {
	manager, _ := newTodoManager(t)

	early := time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)
	late := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)

	noStart := create(t, manager, todo.Todo{TaskID: "J2", Description: "no start", EndDate: "2026-10-05"})
	started := create(t, manager, todo.Todo{
		TaskID: "J1", Description: "started", IsDone: true, StartDate: "2026-10-01", EndDate: "2026-10-05",
		Reminders: []time.Time{late, early},
	})
	future := create(t, manager, todo.Todo{TaskID: "J1", Description: "future", StartDate: "2026-10-03"})
	first := create(
		t,
		manager,
		todo.Todo{TaskID: "J1", Description: "first", StartDate: "2026-10-02", Position: new(0)},
	)

	ids := func(f todo.Filter) []int64 {
		todos, err := manager.ListTodos(context.Background(), f)
		require.NoError(t, err)

		got := []int64{}
		for _, td := range todos {
			got = append(got, td.ID)
		}

		return got
	}

	require.Equal(t, []int64{first, started, future, noStart}, ids(todo.Filter{}))
	require.Equal(t, []int64{first, started, future}, ids(todo.Filter{TaskID: "J1"}))
	require.Equal(t, []int64{started}, ids(todo.Filter{IsDone: new(true)}))
	require.Equal(t, []int64{first, future, noStart}, ids(todo.Filter{IsDone: new(false)}))
	require.Equal(t, []int64{first, future}, ids(todo.Filter{TaskID: "J1", IsDone: new(false)}))
	require.Empty(t, ids(todo.Filter{TaskID: "missing"}))

	todos, err := manager.ListTodos(context.Background(), todo.Filter{TaskID: "J1", IsDone: new(true)})
	require.NoError(t, err)
	require.Len(t, todos, 1)

	got := todos[0]
	require.Len(t, got.Reminders, 2)
	require.True(t, early.Equal(got.Reminders[0]))
	require.True(t, late.Equal(got.Reminders[1]))

	require.NotNil(t, got.CompletedAt)

	got.Reminders = nil
	got.CompletedAt = nil
	require.Equal(t, todo.Todo{
		ID: started, TaskID: "J1", Description: "started", IsDone: true,
		StartDate: "2026-10-01", EndDate: "2026-10-05", Position: new(1),
	}, got)
}

func TestUpdateTodo(t *testing.T) {
	manager, path := newTodoManager(t)

	a := create(t, manager, todo.Todo{TaskID: "J1", Description: "a", Reminders: []time.Time{time.Now()}})
	b := create(t, manager, todo.Todo{TaskID: "J1", Description: "b"})
	c := create(t, manager, todo.Todo{TaskID: "J1", Description: "c"})
	x := create(t, manager, todo.Todo{TaskID: "J2", Description: "x"})

	update := func(td todo.Todo) {
		t.Helper()
		require.NoError(t, manager.UpdateTodo(context.Background(), td))
	}

	// Same task without a position keeps the place and replaces the fields.
	at := time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC)
	update(todo.Todo{
		ID: b, TaskID: "J1", Description: "b2", IsDone: true, StartDate: "2026-10-01", EndDate: "2026-10-02",
		Reminders: []time.Time{at},
	})
	require.Equal(
		t,
		[]string{"J1:0:" + fmt.Sprint(a), "J1:1:" + fmt.Sprint(b), "J1:2:" + fmt.Sprint(c), "J2:0:" + fmt.Sprint(x)},
		placements(t, path),
	)

	todos, err := manager.ListTodos(context.Background(), todo.Filter{IsDone: new(true)})
	require.NoError(t, err)
	require.Len(t, todos, 1)
	require.Equal(t, "b2", todos[0].Description)
	require.Equal(t, "2026-10-01", todos[0].StartDate)
	require.Equal(t, "2026-10-02", todos[0].EndDate)
	require.Len(t, todos[0].Reminders, 1)
	require.True(t, at.Equal(todos[0].Reminders[0]))

	// Moving within the task, past the end goes last.
	update(todo.Todo{ID: c, TaskID: "J1", Description: "c", Position: new(0)})
	require.Equal(
		t,
		[]string{"J1:0:" + fmt.Sprint(c), "J1:1:" + fmt.Sprint(a), "J1:2:" + fmt.Sprint(b), "J2:0:" + fmt.Sprint(x)},
		placements(t, path),
	)
	update(todo.Todo{ID: c, TaskID: "J1", Description: "c", Position: new(5)})
	require.Equal(
		t,
		[]string{"J1:0:" + fmt.Sprint(a), "J1:1:" + fmt.Sprint(b), "J1:2:" + fmt.Sprint(c), "J2:0:" + fmt.Sprint(x)},
		placements(t, path),
	)

	// Another task without a position appends there and closes the gap.
	update(todo.Todo{ID: a, TaskID: "J2", Description: "a"})
	require.Equal(
		t,
		[]string{"J1:0:" + fmt.Sprint(b), "J1:1:" + fmt.Sprint(c), "J2:0:" + fmt.Sprint(x), "J2:1:" + fmt.Sprint(a)},
		placements(t, path),
	)

	// Another task with a position inserts there.
	update(todo.Todo{ID: c, TaskID: "J2", Description: "c", Position: new(0)})
	require.Equal(
		t,
		[]string{"J1:0:" + fmt.Sprint(b), "J2:0:" + fmt.Sprint(c), "J2:1:" + fmt.Sprint(x), "J2:2:" + fmt.Sprint(a)},
		placements(t, path),
	)

	_, reminders := stored(t, path)
	require.Equal(t, []string{fmt.Sprint(b)}, reminders, "a's reminders were replaced by none")
}

func TestListTodosCompletedSince(t *testing.T) {
	manager, _ := newTodoManager(t)
	ctx := context.Background()

	early := create(t, manager, todo.Todo{TaskID: "J1", Description: "early", IsDone: true})
	since := time.Now()
	late := create(
		t,
		manager,
		todo.Todo{TaskID: "J2", Description: "late", IsDone: true, Reminders: []time.Time{since}},
	)
	create(t, manager, todo.Todo{TaskID: "J1", Description: "open"})

	got, err := manager.ListTodos(ctx, todo.Filter{CompletedSince: &since})
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, late, got[0].ID)
	require.Len(t, got[0].Reminders, 1, "reminders follow the same filter")

	before := since.Add(-time.Hour)

	got, err = manager.ListTodos(ctx, todo.Filter{CompletedSince: &before})
	require.NoError(t, err)
	require.Len(t, got, 2)
	require.Equal(t, early, got[0].ID)
}

func TestCountTodosByTask(t *testing.T) {
	manager, _ := newTodoManager(t)
	ctx := context.Background()

	create(t, manager, todo.Todo{TaskID: "J1", Description: "a", IsDone: true})
	create(t, manager, todo.Todo{TaskID: "J1", Description: "b"})
	create(t, manager, todo.Todo{TaskID: "J1", Description: "c", IsDone: true})
	create(t, manager, todo.Todo{TaskID: "J2", Description: "d"})

	got, err := manager.CountTodosByTask(ctx)
	require.NoError(t, err)
	require.Equal(t, map[string]todo.Count{"J1": {Todos: 3, Done: 2}, "J2": {Todos: 1}}, got)

	empty, _ := newTodoManager(t)

	got, err = empty.CountTodosByTask(ctx)
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestTodoNoteAndCompletedAt(t *testing.T) {
	manager, _ := newTodoManager(t)
	ctx := context.Background()

	get := func(id int64) todo.Todo {
		t.Helper()

		todos, err := manager.ListTodos(ctx, todo.Filter{})
		require.NoError(t, err)

		for _, td := range todos {
			if td.ID == id {
				return td
			}
		}

		require.FailNow(t, "todo not found", "id=%d", id)

		return todo.Todo{}
	}

	before := time.Now().Add(-time.Second)
	a := create(t, manager, todo.Todo{TaskID: "J1", Description: "a", Note: "first", IsDone: true})
	b := create(t, manager, todo.Todo{TaskID: "J1", Description: "b"})

	created := get(a)
	require.Equal(t, "first", created.Note)
	require.NotNil(t, created.CompletedAt)
	require.WithinRange(t, *created.CompletedAt, before, time.Now())
	require.Nil(t, get(b).CompletedAt)

	// Staying done keeps the completion time, and the note is replaced.
	require.NoError(
		t,
		manager.UpdateTodo(ctx, todo.Todo{ID: a, TaskID: "J1", Description: "a", Note: "", IsDone: true}),
	)
	require.Empty(t, get(a).Note)
	require.True(t, created.CompletedAt.Equal(*get(a).CompletedAt))

	// Undoing clears it; becoming done sets it.
	require.NoError(t, manager.UpdateTodo(ctx, todo.Todo{ID: a, TaskID: "J1", Description: "a"}))
	require.Nil(t, get(a).CompletedAt)

	before = time.Now().Add(-time.Second)

	require.NoError(t, manager.UpdateTodo(ctx, todo.Todo{ID: b, TaskID: "J1", Description: "b", IsDone: true}))
	require.NotNil(t, get(b).CompletedAt)
	require.WithinRange(t, *get(b).CompletedAt, before, time.Now())
}

func TestUpdateTodoRejectsMissingTodoOrTask(t *testing.T) {
	manager, path := newTodoManager(t)

	a := create(t, manager, todo.Todo{TaskID: "J1", Description: "a", Reminders: []time.Time{time.Now()}})

	err := manager.UpdateTodo(context.Background(), todo.Todo{ID: a + 1, TaskID: "J1", Description: "z"})
	require.ErrorIs(t, err, db.ErrTodoToUpdateNotFound)
	require.ErrorContains(t, err, "id="+fmt.Sprint(a+1))

	err = manager.UpdateTodo(context.Background(), todo.Todo{ID: a, TaskID: "nope", Description: "z"})
	require.ErrorIs(t, err, db.ErrTaskOfTodoToUpdateNotFound)
	require.ErrorContains(t, err, "task_id=nope")

	todos, err := manager.ListTodos(context.Background(), todo.Filter{})
	require.NoError(t, err)
	require.Len(t, todos, 1)
	require.Equal(t, "a", todos[0].Description)
	require.Equal(t, []string{"J1:0:" + fmt.Sprint(a)}, placements(t, path))

	_, reminders := stored(t, path)
	require.Equal(t, []string{fmt.Sprint(a)}, reminders)
}

func TestDeleteTodo(t *testing.T) {
	manager, path := newTodoManager(t)

	a := create(t, manager, todo.Todo{TaskID: "J1", Description: "a"})
	b := create(t, manager, todo.Todo{TaskID: "J1", Description: "b", Reminders: []time.Time{time.Now()}})
	c := create(t, manager, todo.Todo{TaskID: "J1", Description: "c", Reminders: []time.Time{time.Now()}})

	require.NoError(t, manager.DeleteTodo(context.Background(), b))
	require.Equal(t, []string{"J1:0:" + fmt.Sprint(a), "J1:1:" + fmt.Sprint(c)}, placements(t, path))

	todos, reminders := stored(t, path)
	require.Equal(t, []string{fmt.Sprint(a), fmt.Sprint(c)}, todos)
	require.Equal(t, []string{fmt.Sprint(c)}, reminders)

	err := manager.DeleteTodo(context.Background(), b)
	require.ErrorIs(t, err, db.ErrTodoToDeleteNotFound)
	require.ErrorContains(t, err, "id="+fmt.Sprint(b))
}

func TestDeleteTaskDeletesItsTodos(t *testing.T) {
	manager, path := newTodoManager(t)

	create(t, manager, todo.Todo{TaskID: "J1", Description: "a", Reminders: []time.Time{time.Now()}})
	create(t, manager, todo.Todo{TaskID: "J1", Description: "b"})
	kept := create(t, manager, todo.Todo{TaskID: "J2", Description: "c", Reminders: []time.Time{time.Now()}})

	// A sync stores the tasks again without touching their todos.
	require.NoError(t, manager.UpsertTasks(context.Background(), []task.Task{newTask("J1"), newTask("J2")}))
	require.Len(t, placements(t, path), 3)

	require.NoError(t, manager.DeleteTask(context.Background(), "J1"))

	todos, reminders := stored(t, path)
	require.Equal(t, []string{fmt.Sprint(kept)}, todos)
	require.Equal(t, []string{fmt.Sprint(kept)}, reminders)
	require.Equal(t, []string{"J2:0:" + fmt.Sprint(kept)}, placements(t, path))
}
