package db_test

import (
	"context"
	"database/sql"
	"io/fs"
	"testing"
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

func TestCreateTasksRollsBackOnFailure(t *testing.T) {
	manager := newManager(t)
	require.NoError(t, manager.CreateTasks(context.Background(), []db.Task{newTask("WHF-1")}))

	err := manager.CreateTasks(context.Background(), []db.Task{newTask("WHF-2"), newTask("WHF-1"), newTask("WHF-3")})
	require.ErrorIs(t, err, db.ErrFailedToCreateTask)
	require.ErrorContains(t, err, "id=WHF-1")

	requireExists(t, manager, "WHF-1", true)
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
