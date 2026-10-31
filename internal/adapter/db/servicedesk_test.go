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
	"github.com/byron1st/wharf/scripts"
)

func TestUpsertServiceDeskRequests(t *testing.T) {
	manager, path := newFileManager(t, scripts.Migrations())

	created := time.UnixMilli(1444290120000).UTC()
	statusAt := time.UnixMilli(1444287660000).UTC()
	later := statusAt.Add(time.Hour)

	require.NoError(t, manager.UpsertTasks(context.Background(), []task.Task{newTask("WHF-1")}))
	require.NoError(t, manager.UpsertServiceDeskRequests(context.Background(), []servicedesk.Request{
		{Key: "SD-1", Title: "Mouse", Status: "Waiting for Support", Created: created, StatusAt: statusAt},
		{Key: "SD-2", Title: "Account", Status: "Open", Created: created, StatusAt: statusAt},
	}))
	require.NoError(t, manager.UpsertServiceDeskRequests(context.Background(), []servicedesk.Request{
		{Key: "SD-1", Title: "New mouse", Status: "Done", StatusCategory: "DONE", Created: created, StatusAt: later},
	}))
	require.NoError(t, manager.UpsertServiceDeskRequests(context.Background(), nil))

	got := readServiceDeskRequest(t, path, "SD-1")
	require.Equal(t, "New mouse", got.title)
	require.Equal(t, "Done", got.status)
	require.Equal(t, "DONE", got.statusCategory)
	require.True(t, created.Equal(got.created))
	require.True(t, later.Equal(got.statusAt))

	kept := readServiceDeskRequest(t, path, "SD-2")
	require.Equal(t, "Account", kept.title)
	require.Equal(t, "Open", kept.status)

	requireExists(t, path, "WHF-1", true)

	var tasks int

	err := open(t, path).QueryRowContext(context.Background(), "SELECT COUNT(*) FROM tasks").Scan(&tasks)
	require.NoError(t, err)
	require.Equal(t, 1, tasks)
}

func TestUpsertServiceDeskRequestsCanceled(t *testing.T) {
	manager, path := newFileManager(t, scripts.Migrations())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := manager.UpsertServiceDeskRequests(ctx, []servicedesk.Request{
		{
			Key:      "SD-1",
			Title:    "Mouse",
			Status:   "Open",
			Created:  time.UnixMilli(1).UTC(),
			StatusAt: time.UnixMilli(2).UTC(),
		},
	})
	require.ErrorIs(t, err, context.Canceled)

	var count int

	scanErr := open(t, path).QueryRowContext(
		context.Background(),
		"SELECT COUNT(*) FROM service_desk_requests",
	).Scan(&count)
	require.NoError(t, scanErr)
	require.Zero(t, count)
}

type readServiceDeskRequestRow struct {
	title          string
	status         string
	statusCategory string
	created        time.Time
	statusAt       time.Time
}

func readServiceDeskRequest(t *testing.T, path, key string) readServiceDeskRequestRow {
	t.Helper()

	var row readServiceDeskRequestRow

	err := open(t, path).QueryRowContext(
		context.Background(),
		"SELECT title, status, status_category, created_at, status_at FROM service_desk_requests WHERE issue_key = ?",
		key,
	).Scan(&row.title, &row.status, &row.statusCategory, &row.created, &row.statusAt)
	require.NoError(t, err)

	return row
}

// serviceDeskLinks is every stored todo_service_desk_requests row as
// "todo>key", sorted.
func serviceDeskLinks(t *testing.T, path string) []string {
	t.Helper()

	return texts(
		t,
		path,
		"SELECT todo_id || '>' || issue_key FROM todo_service_desk_requests ORDER BY todo_id, issue_key",
	)
}

func newRequest(key string) servicedesk.Request {
	return servicedesk.Request{
		Key:            key,
		Title:          "Request " + key,
		Status:         "Open",
		StatusCategory: "NEW",
		Created:        time.UnixMilli(1444290120000).UTC(),
		StatusAt:       time.UnixMilli(1444287660000).UTC(),
	}
}

// seedLinkable stores the tasks J1 and J2, a todo of each whose IDs it
// returns, and the requests SD-1 and SD-2.
func seedLinkable(t *testing.T, manager db.Manager) (j1Todo, j2Todo int64) {
	t.Helper()

	ctx := context.Background()
	require.NoError(t, manager.UpsertTasks(ctx, []task.Task{newTask("J1"), newTask("J2")}))
	require.NoError(
		t,
		manager.UpsertServiceDeskRequests(ctx, []servicedesk.Request{newRequest("SD-1"), newRequest("SD-2")}),
	)

	return create(t, manager, todo.Todo{TaskID: "J1", Description: "a"}),
		create(t, manager, todo.Todo{TaskID: "J2", Description: "b"})
}

func link(id int64, key string) string {
	return fmt.Sprint(id) + ">" + key
}

func TestLinkServiceDeskRequest(t *testing.T) {
	manager, path := newFileManager(t, scripts.Migrations())
	a, b := seedLinkable(t, manager)
	ctx := context.Background()

	require.NoError(t, manager.LinkServiceDeskRequest(ctx, a, "SD-2"))
	require.NoError(t, manager.LinkServiceDeskRequest(ctx, a, "SD-1"))
	require.NoError(t, manager.LinkServiceDeskRequest(ctx, b, "SD-1"))
	// Linking again is a no-op.
	require.NoError(t, manager.LinkServiceDeskRequest(ctx, a, "SD-1"))

	require.Equal(t, []string{link(a, "SD-1"), link(a, "SD-2"), link(b, "SD-1")}, serviceDeskLinks(t, path))
}

func TestLinkServiceDeskRequestNotFound(t *testing.T) {
	manager, path := newFileManager(t, scripts.Migrations())
	a, b := seedLinkable(t, manager)
	missing := b + 1

	ctx := context.Background()

	err := manager.LinkServiceDeskRequest(ctx, missing, "SD-1")
	require.ErrorIs(t, err, db.ErrTodoToLinkNotFound)
	require.NotErrorIs(t, err, db.ErrServiceDeskRequestToLinkNotFound)

	err = manager.LinkServiceDeskRequest(ctx, a, "SD-9")
	require.ErrorIs(t, err, db.ErrServiceDeskRequestToLinkNotFound)
	require.NotErrorIs(t, err, db.ErrTodoToLinkNotFound)

	// A missing todo is reported first.
	require.ErrorIs(t, manager.LinkServiceDeskRequest(ctx, missing, "SD-9"), db.ErrTodoToLinkNotFound)

	require.Equal(t, []string{}, serviceDeskLinks(t, path))
}

func TestUnlinkServiceDeskRequest(t *testing.T) {
	manager, path := newFileManager(t, scripts.Migrations())
	a, b := seedLinkable(t, manager)
	ctx := context.Background()

	require.NoError(t, manager.LinkServiceDeskRequest(ctx, a, "SD-1"))
	require.NoError(t, manager.LinkServiceDeskRequest(ctx, a, "SD-2"))
	require.NoError(t, manager.LinkServiceDeskRequest(ctx, b, "SD-1"))

	require.NoError(t, manager.UnlinkServiceDeskRequest(ctx, a, "SD-1"))
	require.Equal(t, []string{link(a, "SD-2"), link(b, "SD-1")}, serviceDeskLinks(t, path))
	require.Equal(t, "Request SD-1", readServiceDeskRequest(t, path, "SD-1").title)

	todos, _ := stored(t, path)
	require.Equal(t, []string{fmt.Sprint(a), fmt.Sprint(b)}, todos)

	require.ErrorIs(
		t,
		manager.UnlinkServiceDeskRequest(ctx, a, "SD-1"),
		db.ErrServiceDeskRequestLinkToDeleteNotFound,
	)
	require.Equal(t, []string{link(a, "SD-2"), link(b, "SD-1")}, serviceDeskLinks(t, path))
}

func TestListTodosWithServiceDeskRequests(t *testing.T) {
	manager, _ := newFileManager(t, scripts.Migrations())
	a, b := seedLinkable(t, manager)
	ctx := context.Background()

	require.NoError(t, manager.LinkServiceDeskRequest(ctx, a, "SD-2"))
	require.NoError(t, manager.LinkServiceDeskRequest(ctx, a, "SD-1"))
	require.NoError(t, manager.LinkServiceDeskRequest(ctx, b, "SD-2"))

	got, err := manager.ListTodos(ctx, todo.Filter{TaskID: "J1"})
	require.NoError(t, err)
	require.Len(t, got, 1)

	requests := got[0].ServiceDeskRequests
	want := []servicedesk.Request{newRequest("SD-1"), newRequest("SD-2")}
	require.Len(t, requests, len(want))

	for i := range want {
		require.Equal(t, want[i].Key, requests[i].Key)
		require.Equal(t, want[i].Title, requests[i].Title)
		require.Equal(t, want[i].Status, requests[i].Status)
		require.Equal(t, want[i].StatusCategory, requests[i].StatusCategory)
		require.True(t, want[i].Created.Equal(requests[i].Created))
		require.True(t, want[i].StatusAt.Equal(requests[i].StatusAt))
	}

	require.NoError(t, manager.UnlinkServiceDeskRequest(ctx, b, "SD-2"))

	got, err = manager.ListTodos(ctx, todo.Filter{TaskID: "J2"})
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Empty(t, got[0].ServiceDeskRequests)
}

func TestListTodosByServiceDeskRequest(t *testing.T) {
	manager, _ := newFileManager(t, scripts.Migrations())
	a, b := seedLinkable(t, manager)
	done := create(t, manager, todo.Todo{TaskID: "J2", Description: "done", IsDone: true})
	ctx := context.Background()

	require.NoError(t, manager.LinkServiceDeskRequest(ctx, a, "SD-1"))
	require.NoError(t, manager.LinkServiceDeskRequest(ctx, a, "SD-2"))
	require.NoError(t, manager.LinkServiceDeskRequest(ctx, done, "SD-1"))
	require.NoError(t, manager.LinkServiceDeskRequest(ctx, b, "SD-2"))

	got, err := manager.ListTodos(ctx, todo.Filter{ServiceDeskRequest: "SD-1"})
	require.NoError(t, err)
	require.Len(t, got, 2)
	require.Equal(t, a, got[0].ID)
	require.Equal(t, done, got[1].ID)
	// A matching todo still lists every request it is linked to.
	require.Len(t, got[0].ServiceDeskRequests, 2)

	got, err = manager.ListTodos(ctx, todo.Filter{ServiceDeskRequest: "SD-1", IsDone: new(false)})
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, a, got[0].ID)

	got, err = manager.ListTodos(ctx, todo.Filter{ServiceDeskRequest: "SD-9"})
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestListServiceDeskRequests(t *testing.T) {
	manager, _ := newFileManager(t, scripts.Migrations())
	ctx := context.Background()

	closed := newRequest("SD-1")
	closed.Status, closed.StatusCategory = "Done", "DONE"
	undefined := newRequest("SD-3")
	undefined.StatusCategory = "UNDEFINED"

	require.NoError(
		t,
		manager.UpsertServiceDeskRequests(ctx, []servicedesk.Request{undefined, closed, newRequest("SD-2")}),
	)

	keys := func(includeClosed bool) []string {
		t.Helper()

		requests, err := manager.ListServiceDeskRequests(ctx, includeClosed)
		require.NoError(t, err)

		got := []string{}
		for _, r := range requests {
			got = append(got, r.Key)
		}

		return got
	}

	require.Equal(t, []string{"SD-2", "SD-3"}, keys(false))
	require.Equal(t, []string{"SD-1", "SD-2", "SD-3"}, keys(true))

	requests, err := manager.ListServiceDeskRequests(ctx, true)
	require.NoError(t, err)
	require.Equal(t, "Done", requests[0].Status)
	require.Equal(t, "DONE", requests[0].StatusCategory)
	require.True(t, closed.Created.Equal(requests[0].Created))
	require.True(t, closed.StatusAt.Equal(requests[0].StatusAt))
}

func TestDeleteTodoAndTaskDeleteServiceDeskLinks(t *testing.T) {
	manager, path := newFileManager(t, scripts.Migrations())
	a, b := seedLinkable(t, manager)
	c := create(t, manager, todo.Todo{TaskID: "J2", Description: "c"})
	ctx := context.Background()

	require.NoError(t, manager.LinkServiceDeskRequest(ctx, a, "SD-1"))
	require.NoError(t, manager.LinkServiceDeskRequest(ctx, a, "SD-2"))
	require.NoError(t, manager.LinkServiceDeskRequest(ctx, b, "SD-1"))
	require.NoError(t, manager.LinkServiceDeskRequest(ctx, c, "SD-2"))

	// Syncs and todo updates overwrite the task, the request, and the todo
	// but keep their links.
	require.NoError(t, manager.UpsertTasks(ctx, []task.Task{newTask("J1")}))
	require.NoError(t, manager.UpsertServiceDeskRequests(ctx, []servicedesk.Request{newRequest("SD-1")}))
	require.NoError(t, manager.UpdateTodo(ctx, todo.Todo{ID: a, TaskID: "J2", Description: "moved"}))
	require.Len(t, serviceDeskLinks(t, path), 4)

	require.NoError(t, manager.DeleteTodo(ctx, c))
	require.Equal(t, []string{link(a, "SD-1"), link(a, "SD-2"), link(b, "SD-1")}, serviceDeskLinks(t, path))

	// a moved to J2, so deleting J1 keeps every link and deleting J2 drops them.
	require.NoError(t, manager.DeleteTask(ctx, "J1"))
	require.Len(t, serviceDeskLinks(t, path), 3)

	require.NoError(t, manager.DeleteTask(ctx, "J2"))
	require.Equal(t, []string{}, serviceDeskLinks(t, path))
	require.Equal(t, "Request SD-1", readServiceDeskRequest(t, path, "SD-1").title)
	require.Equal(t, "Request SD-2", readServiceDeskRequest(t, path, "SD-2").title)
}

func TestServiceDeskLinkMethodsCanceled(t *testing.T) {
	manager, path := newFileManager(t, scripts.Migrations())
	a, _ := seedLinkable(t, manager)
	require.NoError(t, manager.LinkServiceDeskRequest(context.Background(), a, "SD-1"))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	require.ErrorIs(t, manager.LinkServiceDeskRequest(ctx, a, "SD-2"), context.Canceled)

	err := manager.UnlinkServiceDeskRequest(ctx, a, "SD-1")
	require.ErrorIs(t, err, db.ErrFailedToUnlinkServiceDeskRequest)
	require.ErrorIs(t, err, context.Canceled)

	require.Equal(t, []string{link(a, "SD-1")}, serviceDeskLinks(t, path))
}
