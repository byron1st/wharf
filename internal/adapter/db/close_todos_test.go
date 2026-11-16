package db_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/byron1st/wharf/internal/adapter/db"
	"github.com/byron1st/wharf/internal/pkg/servicedesk"
	"github.com/byron1st/wharf/internal/pkg/task"
	"github.com/byron1st/wharf/internal/pkg/todo"
)

// todoStates is each todo of task id as "description:done", in task order.
func todoStates(t *testing.T, manager db.Manager, id string) []string {
	t.Helper()

	todos, err := manager.ListTodos(context.Background(), todo.Filter{TaskID: id})
	require.NoError(t, err)

	got := []string{}
	for _, td := range todos {
		got = append(got, fmt.Sprintf("%s:%t", td.Description, td.IsDone))
	}

	return got
}

// completedAt is when the todo with description desc of task id was done.
func completedAt(t *testing.T, manager db.Manager, id, desc string) *time.Time {
	t.Helper()

	todos, err := manager.ListTodos(context.Background(), todo.Filter{TaskID: id})
	require.NoError(t, err)

	for _, td := range todos {
		if td.Description == desc {
			return td.CompletedAt
		}
	}

	require.FailNow(t, "todo not stored", "task=%s, description=%s", id, desc)

	return nil
}

// newClosingManager stores task J9 with a todo that has a reminder and a
// service desk request link, so tests can see that closing another task
// leaves it alone.
func newClosingManager(t *testing.T) (db.Manager, string) {
	t.Helper()

	manager, dsn := newTodoManager(t)
	ctx := context.Background()

	require.NoError(t, manager.UpsertTasks(ctx, []task.Task{newTask("J9")}))
	require.NoError(t, manager.UpsertServiceDeskRequests(ctx, []servicedesk.Request{
		{Key: "SD-1", Title: "Mouse", Status: "Open", Created: time.Now(), StatusAt: time.Now()},
	}))

	kept := create(t, manager, todo.Todo{TaskID: "J9", Description: "kept", Reminders: []time.Time{time.Now()}})
	require.NoError(t, manager.LinkServiceDeskRequest(ctx, kept, "SD-1"))

	return manager, dsn
}

// createClosable stores, under task id, an open todo "open" with a reminder
// and a service desk request link, and a todo "done" done at doneAt.
func createClosable(t *testing.T, manager db.Manager, id string) {
	t.Helper()

	ctx := context.Background()

	open := create(t, manager, todo.Todo{TaskID: id, Description: "open", Reminders: []time.Time{time.Now()}})
	require.NoError(t, manager.LinkServiceDeskRequest(ctx, open, "SD-1"))

	done := create(t, manager, todo.Todo{TaskID: id, Description: "done"})
	require.NoError(t, manager.UpdateTodo(ctx, todo.Todo{ID: done, TaskID: id, Description: "done", IsDone: true}))
}

// requireOnlyKept checks that only J9's todo, reminder, and link are stored.
func requireOnlyKept(t *testing.T, manager db.Manager, dsn string) {
	t.Helper()

	todos, reminders := stored(t, dsn)
	require.Len(t, todos, 1)
	require.Equal(t, todos, reminders)
	require.Equal(t, []string{"J9:0:" + todos[0]}, placements(t, dsn))
	require.Equal(t, []string{todos[0]}, texts(t, dsn, "SELECT todo_id FROM todo_service_desk_requests"))
	require.Equal(t, []string{"kept:false"}, todoStates(t, manager, "J9"))
}

func TestUpdateLocalTaskStatusDoneCompletesTodos(t *testing.T) {
	manager, dsn := newClosingManager(t)
	ctx := context.Background()

	id, err := manager.CreateLocalTask(ctx, newTask("ignored"))
	require.NoError(t, err)
	createClosable(t, manager, id)

	doneAt := completedAt(t, manager, id, "done")
	require.NotNil(t, doneAt)

	closed := time.Date(2026, 10, 20, 9, 0, 0, 0, time.FixedZone("KST", 9*3600))
	require.NoError(t, manager.UpdateLocalTaskStatus(ctx, id, task.StatusDone, closed))

	require.Equal(t, []string{"open:true", "done:true"}, todoStates(t, manager, id))
	requireTime(t, closed, completedAt(t, manager, id, "open"))
	requireTime(t, *doneAt, completedAt(t, manager, id, "done"))

	// Staying done leaves a todo added since open.
	create(t, manager, todo.Todo{TaskID: id, Description: "later"})
	require.NoError(t, manager.UpdateLocalTaskStatus(ctx, id, task.StatusDone, closed.Add(time.Hour)))
	require.Equal(t, []string{"open:true", "done:true", "later:false"}, todoStates(t, manager, id))

	// Reopening leaves the todos as they are.
	require.NoError(t, manager.UpdateLocalTaskStatus(ctx, id, task.StatusTodo, closed))
	require.Equal(t, []string{"open:true", "done:true", "later:false"}, todoStates(t, manager, id))

	todos, _ := stored(t, dsn)
	require.Len(t, todos, 4)
	require.Equal(t, []string{"kept:false"}, todoStates(t, manager, "J9"))
}

func TestUpdateLocalTaskStatusCanceledDeletesTodos(t *testing.T) {
	manager, dsn := newClosingManager(t)
	ctx := context.Background()

	id, err := manager.CreateLocalTask(ctx, newTask("ignored"))
	require.NoError(t, err)
	createClosable(t, manager, id)

	require.NoError(t, manager.UpdateLocalTaskStatus(ctx, id, task.StatusCanceled, time.Now()))
	require.Empty(t, todoStates(t, manager, id))
	requireOnlyKept(t, manager, dsn)

	// Staying canceled leaves a todo added since.
	create(t, manager, todo.Todo{TaskID: id, Description: "later"})
	require.NoError(t, manager.UpdateLocalTaskStatus(ctx, id, task.StatusCanceled, time.Now()))
	require.Equal(t, []string{"later:false"}, todoStates(t, manager, id))

	// Done to canceled deletes them too.
	require.NoError(t, manager.UpdateLocalTaskStatus(ctx, id, task.StatusDone, time.Now()))
	require.Equal(t, []string{"later:true"}, todoStates(t, manager, id))
	require.NoError(t, manager.UpdateLocalTaskStatus(ctx, id, task.StatusCanceled, time.Now()))
	require.Empty(t, todoStates(t, manager, id))
	requireOnlyKept(t, manager, dsn)
}

func TestUpdateLocalTaskClosesTodos(t *testing.T) {
	manager, dsn := newClosingManager(t)
	ctx := context.Background()

	done, err := manager.CreateLocalTask(ctx, newTask("ignored"))
	require.NoError(t, err)
	createClosable(t, manager, done)

	canceled, err := manager.CreateLocalTask(ctx, newTask("ignored"))
	require.NoError(t, err)
	createClosable(t, manager, canceled)

	closed := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	update := func(id string, status task.Status) {
		t.Helper()

		u := newTask(id)
		u.Status, u.ClosedAt = status, &closed
		require.NoError(t, manager.UpdateLocalTask(ctx, u))
	}

	update(done, task.StatusDone)
	require.Equal(t, []string{"open:true", "done:true"}, todoStates(t, manager, done))
	requireTime(t, closed, completedAt(t, manager, done, "open"))

	update(canceled, task.StatusCanceled)
	require.Empty(t, todoStates(t, manager, canceled))

	// Saving a done task again leaves a todo added since open.
	create(t, manager, todo.Todo{TaskID: done, Description: "later"})
	update(done, task.StatusDone)
	require.Equal(t, []string{"open:true", "done:true", "later:false"}, todoStates(t, manager, done))

	update(done, task.StatusCanceled)
	require.Empty(t, todoStates(t, manager, done))
	requireOnlyKept(t, manager, dsn)
}

func TestUpsertTasksClosesTodos(t *testing.T) {
	manager, dsn := newClosingManager(t)
	ctx := context.Background()

	createClosable(t, manager, "J1")
	createClosable(t, manager, "J2")

	closed := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	j1, j2 := newTask("J1"), newTask("J2")
	j1.Status, j1.ClosedAt = task.StatusDone, &closed
	j2.Status = task.StatusCanceled

	require.NoError(t, manager.UpsertTasks(ctx, []task.Task{j1, j2, newTask("J9")}))
	require.Equal(t, []string{"open:true", "done:true"}, todoStates(t, manager, "J1"))
	requireTime(t, closed, completedAt(t, manager, "J1", "open"))
	require.Empty(t, todoStates(t, manager, "J2"))

	// A task that stays closed across syncs keeps its todos.
	create(t, manager, todo.Todo{TaskID: "J1", Description: "later"})
	create(t, manager, todo.Todo{TaskID: "J2", Description: "later"})
	require.NoError(t, manager.UpsertTasks(ctx, []task.Task{j1, j2}))
	require.Equal(t, []string{"open:true", "done:true", "later:false"}, todoStates(t, manager, "J1"))
	require.Equal(t, []string{"later:false"}, todoStates(t, manager, "J2"))

	// A task closed without a known closing time completes its todos now.
	before := time.Now()

	j1.Status, j1.ClosedAt = task.StatusTodo, nil
	require.NoError(t, manager.UpsertTasks(ctx, []task.Task{j1}))
	require.Equal(t, []string{"open:true", "done:true", "later:false"}, todoStates(t, manager, "J1"))

	j1.Status = task.StatusDone
	require.NoError(t, manager.UpsertTasks(ctx, []task.Task{j1}))
	require.Equal(t, []string{"open:true", "done:true", "later:true"}, todoStates(t, manager, "J1"))

	later := completedAt(t, manager, "J1", "later")
	require.NotNil(t, later)
	require.False(t, later.Before(before.Add(-time.Minute)), "completed at %s", later)

	require.Equal(t, []string{"kept:false"}, todoStates(t, manager, "J9"))

	todos, _ := stored(t, dsn)
	require.Len(t, todos, 5)
}
