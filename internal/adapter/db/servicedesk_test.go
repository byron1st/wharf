package db_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

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
