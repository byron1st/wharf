package db_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/byron1st/wharf/internal/adapter/db"
	"github.com/byron1st/wharf/internal/pkg/servicedesk"
	"github.com/byron1st/wharf/internal/pkg/task"
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
		{Key: "SD-1", Title: "New mouse", Status: "In Progress", Created: created, StatusAt: later},
	}))
	require.NoError(t, manager.UpsertServiceDeskRequests(context.Background(), nil))

	got := readServiceDeskRequest(t, path, "SD-1")
	require.Equal(t, "New mouse", got.title)
	require.Equal(t, "In Progress", got.status)
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
	title    string
	status   string
	created  time.Time
	statusAt time.Time
}

func readServiceDeskRequest(t *testing.T, path, key string) readServiceDeskRequestRow {
	t.Helper()

	var row readServiceDeskRequestRow

	err := open(t, path).QueryRowContext(
		context.Background(),
		"SELECT title, status, created_at, status_at FROM service_desk_requests WHERE issue_key = ?",
		key,
	).Scan(&row.title, &row.status, &row.created, &row.statusAt)
	require.NoError(t, err)

	return row
}

// serviceDeskLinks is every stored task_service_desk_requests row as
// "task>key", sorted.
func serviceDeskLinks(t *testing.T, path string) []string {
	t.Helper()

	rows, err := open(t, path).QueryContext(
		context.Background(),
		"SELECT task_id || '>' || issue_key FROM task_service_desk_requests ORDER BY 1",
	)
	require.NoError(t, err)

	defer func() { _ = rows.Close() }()

	got := []string{}

	for rows.Next() {
		var link string
		require.NoError(t, rows.Scan(&link))
		got = append(got, link)
	}

	require.NoError(t, rows.Err())

	return got
}

func newRequest(key string) servicedesk.Request {
	return servicedesk.Request{
		Key:      key,
		Title:    "Request " + key,
		Status:   "Open",
		Created:  time.UnixMilli(1444290120000).UTC(),
		StatusAt: time.UnixMilli(1444287660000).UTC(),
	}
}

// seedLinkable stores the Jira task T1, a local task whose ID it returns, and
// the requests SD-1 and SD-2.
func seedLinkable(t *testing.T, manager db.Manager) string {
	t.Helper()

	ctx := context.Background()
	require.NoError(t, manager.UpsertTasks(ctx, []task.Task{newTask("T1")}))

	local, err := manager.CreateLocalTask(ctx, newTask("ignored"))
	require.NoError(t, err)

	require.NoError(
		t,
		manager.UpsertServiceDeskRequests(ctx, []servicedesk.Request{newRequest("SD-1"), newRequest("SD-2")}),
	)

	return local
}

func TestLinkServiceDeskRequest(t *testing.T) {
	manager, path := newFileManager(t, scripts.Migrations())
	local := seedLinkable(t, manager)
	ctx := context.Background()

	require.NoError(t, manager.LinkServiceDeskRequest(ctx, "T1", "SD-2"))
	require.NoError(t, manager.LinkServiceDeskRequest(ctx, "T1", "SD-1"))
	require.NoError(t, manager.LinkServiceDeskRequest(ctx, local, "SD-1"))
	// Linking again is a no-op.
	require.NoError(t, manager.LinkServiceDeskRequest(ctx, "T1", "SD-1"))

	require.Equal(t, []string{"T1>SD-1", "T1>SD-2", local + ">SD-1"}, serviceDeskLinks(t, path))
}

func TestLinkServiceDeskRequestNotFound(t *testing.T) {
	manager, path := newFileManager(t, scripts.Migrations())
	seedLinkable(t, manager)

	ctx := context.Background()

	err := manager.LinkServiceDeskRequest(ctx, "T9", "SD-1")
	require.ErrorIs(t, err, db.ErrTaskToLinkNotFound)
	require.NotErrorIs(t, err, db.ErrServiceDeskRequestToLinkNotFound)

	err = manager.LinkServiceDeskRequest(ctx, "T1", "SD-9")
	require.ErrorIs(t, err, db.ErrServiceDeskRequestToLinkNotFound)
	require.NotErrorIs(t, err, db.ErrTaskToLinkNotFound)

	// A missing task is reported first.
	require.ErrorIs(t, manager.LinkServiceDeskRequest(ctx, "T9", "SD-9"), db.ErrTaskToLinkNotFound)

	require.Equal(t, []string{}, serviceDeskLinks(t, path))
}

func TestUnlinkServiceDeskRequest(t *testing.T) {
	manager, path := newFileManager(t, scripts.Migrations())
	local := seedLinkable(t, manager)
	ctx := context.Background()

	require.NoError(t, manager.LinkServiceDeskRequest(ctx, "T1", "SD-1"))
	require.NoError(t, manager.LinkServiceDeskRequest(ctx, "T1", "SD-2"))
	require.NoError(t, manager.LinkServiceDeskRequest(ctx, local, "SD-1"))

	require.NoError(t, manager.UnlinkServiceDeskRequest(ctx, "T1", "SD-1"))
	require.Equal(t, []string{"T1>SD-2", local + ">SD-1"}, serviceDeskLinks(t, path))
	require.Equal(t, "Request SD-1", readServiceDeskRequest(t, path, "SD-1").title)
	requireExists(t, path, "T1", true)

	require.ErrorIs(
		t,
		manager.UnlinkServiceDeskRequest(ctx, "T1", "SD-1"),
		db.ErrServiceDeskRequestLinkToDeleteNotFound,
	)
	require.Equal(t, []string{"T1>SD-2", local + ">SD-1"}, serviceDeskLinks(t, path))
}

func TestListTaskServiceDeskRequests(t *testing.T) {
	manager, _ := newFileManager(t, scripts.Migrations())
	local := seedLinkable(t, manager)
	ctx := context.Background()

	require.NoError(t, manager.LinkServiceDeskRequest(ctx, "T1", "SD-2"))
	require.NoError(t, manager.LinkServiceDeskRequest(ctx, "T1", "SD-1"))

	got, err := manager.ListTaskServiceDeskRequests(ctx, "T1")
	require.NoError(t, err)
	require.Len(t, got, 2)

	want := []servicedesk.Request{newRequest("SD-1"), newRequest("SD-2")}
	for i := range want {
		require.Equal(t, want[i].Key, got[i].Key)
		require.Equal(t, want[i].Title, got[i].Title)
		require.Equal(t, want[i].Status, got[i].Status)
		require.True(t, want[i].Created.Equal(got[i].Created))
		require.True(t, want[i].StatusAt.Equal(got[i].StatusAt))
	}

	none, err := manager.ListTaskServiceDeskRequests(ctx, local)
	require.NoError(t, err)
	require.NotNil(t, none)
	require.Empty(t, none)
}

func TestDeleteTaskDeletesItsServiceDeskLinks(t *testing.T) {
	manager, path := newFileManager(t, scripts.Migrations())
	local := seedLinkable(t, manager)
	ctx := context.Background()

	require.NoError(t, manager.LinkServiceDeskRequest(ctx, "T1", "SD-1"))
	require.NoError(t, manager.LinkServiceDeskRequest(ctx, "T1", "SD-2"))
	require.NoError(t, manager.LinkServiceDeskRequest(ctx, local, "SD-1"))

	// Syncs overwrite the task and the request but keep their links.
	require.NoError(t, manager.UpsertTasks(ctx, []task.Task{newTask("T1")}))
	require.NoError(t, manager.UpsertServiceDeskRequests(ctx, []servicedesk.Request{newRequest("SD-1")}))
	require.Equal(t, []string{"T1>SD-1", "T1>SD-2", local + ">SD-1"}, serviceDeskLinks(t, path))

	require.NoError(t, manager.DeleteTask(ctx, "T1"))
	require.Equal(t, []string{local + ">SD-1"}, serviceDeskLinks(t, path))
	require.Equal(t, "Request SD-1", readServiceDeskRequest(t, path, "SD-1").title)
	require.Equal(t, "Request SD-2", readServiceDeskRequest(t, path, "SD-2").title)
}

func TestServiceDeskLinkMethodsCanceled(t *testing.T) {
	manager, path := newFileManager(t, scripts.Migrations())
	seedLinkable(t, manager)
	require.NoError(t, manager.LinkServiceDeskRequest(context.Background(), "T1", "SD-1"))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	require.ErrorIs(t, manager.LinkServiceDeskRequest(ctx, "T1", "SD-2"), context.Canceled)

	err := manager.UnlinkServiceDeskRequest(ctx, "T1", "SD-1")
	require.ErrorIs(t, err, db.ErrFailedToUnlinkServiceDeskRequest)
	require.ErrorIs(t, err, context.Canceled)

	_, err = manager.ListTaskServiceDeskRequests(ctx, "T1")
	require.ErrorIs(t, err, db.ErrFailedToListTaskServiceDeskRequests)
	require.ErrorIs(t, err, context.Canceled)

	require.Equal(t, []string{"T1>SD-1"}, serviceDeskLinks(t, path))
}
