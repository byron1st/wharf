package service_test

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/byron1st/wharf/internal/adapter/db"
	"github.com/byron1st/wharf/internal/pkg/atlassiansync"
	"github.com/byron1st/wharf/internal/pkg/mocks"
	"github.com/byron1st/wharf/internal/pkg/task"
	"github.com/byron1st/wharf/internal/pkg/todo"
	"github.com/byron1st/wharf/internal/service"
)

var tasks = []task.Task{
	{ID: "WHF-1", Originated: task.OriginJira, Title: "first", Status: task.StatusTodo},
	{ID: "WHF-2", Originated: task.OriginJira, Title: "second", Status: task.StatusInProgress},
}

func TestListTasks(t *testing.T) {
	manager := mocks.NewMockManager(t)
	manager.EXPECT().ListTasks(mock.Anything).Return(tasks, nil).Once()
	manager.EXPECT().
		CountTodosByTask(mock.Anything).
		Return(map[string]todo.Count{"WHF-2": {Todos: 5, Done: 2}}, nil).
		Once()
	manager.EXPECT().GetLastAtlassianSync(mock.Anything).Return(atlassiansync.Record{}, nil).Once()

	got, err := service.NewService(mocks.NewMockClient(t), manager).ListTasks(t.Context())
	require.NoError(t, err)
	require.Equal(t, []service.TaskSummary{
		{Task: tasks[0]},
		{Task: tasks[1], Todos: todo.Count{Todos: 5, Done: 2}},
	}, got)
}

func TestListTasksClosedSinceLastSync(t *testing.T) {
	last := time.Date(2026, 10, 26, 9, 0, 0, 0, time.UTC)
	earlier := last.Add(-time.Hour)
	closed := func(id string, status task.Status, syncedAt *time.Time) task.Task {
		return task.Task{ID: id, Originated: task.OriginJira, Status: status, ClosedSyncedAt: syncedAt}
	}
	stored := []task.Task{
		closed("jira-A-1", task.StatusDone, &last),
		closed("jira-A-2", task.StatusCanceled, &last),
		closed("jira-A-3", task.StatusDone, &earlier),
		closed("jira-A-4", task.StatusDone, nil),
		closed("jira-A-5", task.StatusTodo, &last),
	}

	manager := mocks.NewMockManager(t)
	manager.EXPECT().ListTasks(mock.Anything).Return(stored, nil).Once()
	manager.EXPECT().CountTodosByTask(mock.Anything).Return(nil, nil).Once()
	manager.EXPECT().GetLastAtlassianSync(mock.Anything).Return(atlassiansync.Record{SyncedAt: last}, nil).Once()

	got, err := service.NewService(mocks.NewMockClient(t), manager).ListTasks(t.Context())
	require.NoError(t, err)

	flags := make([]bool, 0, len(got))
	for _, s := range got {
		flags = append(flags, s.ClosedSinceLastSync)
	}

	require.Equal(t, []bool{true, true, false, false, false}, flags)
}

func TestListTasksWithoutSync(t *testing.T) {
	never := time.Time{}
	stored := []task.Task{
		{ID: "jira-A-1", Originated: task.OriginJira, Status: task.StatusDone, ClosedSyncedAt: &never},
	}

	manager := mocks.NewMockManager(t)
	manager.EXPECT().ListTasks(mock.Anything).Return(stored, nil).Once()
	manager.EXPECT().CountTodosByTask(mock.Anything).Return(nil, nil).Once()
	manager.EXPECT().
		GetLastAtlassianSync(mock.Anything).
		Return(atlassiansync.Record{}, db.ErrAtlassianSyncNotFound).
		Once()

	got, err := service.NewService(mocks.NewMockClient(t), manager).ListTasks(t.Context())
	require.NoError(t, err)
	require.Equal(t, []service.TaskSummary{{Task: stored[0]}}, got)
}

func TestListTasksFailures(t *testing.T) {
	failure := errors.New("read failed")

	manager := mocks.NewMockManager(t)
	manager.EXPECT().ListTasks(mock.Anything).Return(nil, failure).Once()

	_, err := service.NewService(mocks.NewMockClient(t), manager).ListTasks(t.Context())
	require.ErrorIs(t, err, failure)

	manager = mocks.NewMockManager(t)
	manager.EXPECT().ListTasks(mock.Anything).Return(tasks, nil).Once()
	manager.EXPECT().CountTodosByTask(mock.Anything).Return(nil, failure).Once()

	_, err = service.NewService(mocks.NewMockClient(t), manager).ListTasks(t.Context())
	require.ErrorIs(t, err, failure)

	manager = mocks.NewMockManager(t)
	manager.EXPECT().ListTasks(mock.Anything).Return(tasks, nil).Once()
	manager.EXPECT().CountTodosByTask(mock.Anything).Return(nil, nil).Once()
	manager.EXPECT().GetLastAtlassianSync(mock.Anything).Return(atlassiansync.Record{}, failure).Once()

	_, err = service.NewService(mocks.NewMockClient(t), manager).ListTasks(t.Context())
	require.ErrorIs(t, err, failure)
}

// closedBetween matches a task equal to want except for a ClosedAt at UTC
// between from and the time of the match.
func closedBetween(want task.Task, from time.Time) any {
	return mock.MatchedBy(func(got task.Task) bool {
		at := got.ClosedAt
		if at == nil || at.Location() != time.UTC || at.Before(from) || at.After(time.Now()) {
			return false
		}

		got.ClosedAt = nil

		return assert.ObjectsAreEqual(want, got)
	})
}

func TestGetTask(t *testing.T) {
	stored := task.Task{
		ID:          "T1",
		Title:       "target",
		ParentID:    "P",
		ChildrenIDs: []string{"C2", "C1", "C9"},
		LinkedIDs:   []string{"L1", "L9"},
	}
	parent := task.Task{ID: "P", Title: "parent"}
	c1 := task.Task{ID: "C1", Title: "c1"}
	c2 := task.Task{ID: "C2", Title: "c2"}
	l1 := task.Task{ID: "L1", Title: "l1"}

	manager := mocks.NewMockManager(t)
	manager.EXPECT().GetTask(mock.Anything, "T1").Return(stored, nil).Once()
	manager.EXPECT().
		ListTasksByIDs(mock.Anything, []string{"P", "C2", "C1", "C9", "L1", "L9"}).
		Return([]task.Task{c1, c2, l1, parent}, nil).
		Once()

	got, err := service.NewService(mocks.NewMockClient(t), manager).GetTask(t.Context(), "T1")
	require.NoError(t, err)
	require.Equal(t, service.TaskDetail{
		Task:     stored,
		Parent:   &parent,
		Children: []task.Task{c2, c1},
		Linked:   []task.Task{l1},
	}, got)
}

func TestGetTaskWithoutStoredRelations(t *testing.T) {
	stored := task.Task{ID: "T1", ParentID: "P"}

	manager := mocks.NewMockManager(t)
	manager.EXPECT().GetTask(mock.Anything, "T1").Return(stored, nil).Once()
	manager.EXPECT().ListTasksByIDs(mock.Anything, []string{"P"}).Return([]task.Task{}, nil).Once()

	got, err := service.NewService(mocks.NewMockClient(t), manager).GetTask(t.Context(), "T1")
	require.NoError(t, err)
	require.Equal(t, service.TaskDetail{
		Task:     stored,
		Children: []task.Task{},
		Linked:   []task.Task{},
	}, got)
}

func TestGetTaskWithoutRelations(t *testing.T) {
	stored := task.Task{ID: "T1"}

	// No ListTasksByIDs expectation: there is nothing to look up.
	manager := mocks.NewMockManager(t)
	manager.EXPECT().GetTask(mock.Anything, "T1").Return(stored, nil).Once()

	got, err := service.NewService(mocks.NewMockClient(t), manager).GetTask(t.Context(), "T1")
	require.NoError(t, err)
	require.Equal(t, service.TaskDetail{
		Task:     stored,
		Children: []task.Task{},
		Linked:   []task.Task{},
	}, got)
}

func TestGetTaskFailures(t *testing.T) {
	failure := errors.New("list failed")

	manager := mocks.NewMockManager(t)
	manager.EXPECT().GetTask(mock.Anything, "MISSING").Return(task.Task{}, db.ErrTaskNotFound).Once()
	manager.EXPECT().GetTask(mock.Anything, "T1").Return(task.Task{ID: "T1", ParentID: "P"}, nil).Once()
	manager.EXPECT().ListTasksByIDs(mock.Anything, mock.Anything).Return(nil, failure).Once()

	svc := service.NewService(mocks.NewMockClient(t), manager)

	_, err := svc.GetTask(t.Context(), "MISSING")
	require.ErrorIs(t, err, service.ErrTaskNotFound)
	require.ErrorContains(t, err, "id=MISSING")

	_, err = svc.GetTask(t.Context(), "T1")
	require.ErrorIs(t, err, failure)
}

func TestCreateTask(t *testing.T) {
	manager := mocks.NewMockManager(t)
	manager.EXPECT().
		CreateLocalTask(mock.Anything, task.Task{Title: "new", Status: task.StatusTodo, ChildrenIDs: []string{"jira-J"}}).
		Return("local-WHF-001", nil).
		Once()
	manager.EXPECT().
		CreateLocalTask(mock.Anything, closedBetween(task.Task{Title: "done", Status: task.StatusDone}, time.Now())).
		Return("local-WHF-002", nil).
		Once()

	svc := service.NewService(mocks.NewMockClient(t), manager)

	// Closing times given by the caller are ignored.
	stray := time.Unix(1, 0)

	id, err := svc.CreateTask(t.Context(), task.Task{
		Title: "new", ChildrenIDs: []string{"jira-J"}, ClosedAt: &stray, ClosedSyncedAt: &stray,
	})
	require.NoError(t, err)
	require.Equal(t, "local-WHF-001", id)

	id, err = svc.CreateTask(t.Context(), task.Task{Title: "done", Status: task.StatusDone, ClosedSyncedAt: &stray})
	require.NoError(t, err)
	require.Equal(t, "local-WHF-002", id)
}

func TestCreateTaskRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name    string
		task    task.Task
		wantErr error
	}{
		{name: "empty title", task: task.Task{}, wantErr: service.ErrEmptyTaskTitle},
		{name: "blank title", task: task.Task{Title: " \t\n"}, wantErr: service.ErrEmptyTaskTitle},
		{name: "unknown status", task: task.Task{Title: "x", Status: "blocked"}, wantErr: service.ErrUnknownTaskStatus},
		{name: "blank parent", task: task.Task{Title: "x", ParentID: " "}, wantErr: service.ErrEmptyRelatedTaskID},
		{
			name:    "empty child",
			task:    task.Task{Title: "x", ChildrenIDs: []string{"C", ""}},
			wantErr: service.ErrEmptyRelatedTaskID,
		},
		{
			name:    "blank link",
			task:    task.Task{Title: "x", LinkedIDs: []string{"\t"}},
			wantErr: service.ErrEmptyRelatedTaskID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// No CreateLocalTask expectation: nothing may be stored.
			_, err := service.NewService(mocks.NewMockClient(t), mocks.NewMockManager(t)).
				CreateTask(t.Context(), tt.task)
			require.ErrorIs(t, err, tt.wantErr)
		})
	}
}

func TestUpdateTask(t *testing.T) {
	update := task.Task{ID: "local-WHF-001", Title: "new", ParentID: "P", ChildrenIDs: []string{"C"}}
	want := update
	want.Status = task.StatusTodo

	manager := mocks.NewMockManager(t)
	manager.EXPECT().
		GetTask(mock.Anything, "local-WHF-001").
		Return(task.Task{ID: "local-WHF-001", Originated: task.OriginLocal}, nil).
		Once()
	manager.EXPECT().UpdateLocalTask(mock.Anything, want).Return(nil).Once()

	require.NoError(t, service.NewService(mocks.NewMockClient(t), manager).UpdateTask(t.Context(), update))
}

func TestUpdateTaskClosing(t *testing.T) {
	stray := time.Unix(1, 0)
	update := task.Task{ID: "local-WHF-001", Title: "new", Status: task.StatusCanceled, ClosedSyncedAt: &stray}
	want := update
	want.ClosedSyncedAt = nil

	manager := mocks.NewMockManager(t)
	manager.EXPECT().
		GetTask(mock.Anything, "local-WHF-001").
		Return(task.Task{ID: "local-WHF-001", Originated: task.OriginLocal}, nil).
		Once()
	manager.EXPECT().UpdateLocalTask(mock.Anything, closedBetween(want, time.Now())).Return(nil).Once()

	require.NoError(t, service.NewService(mocks.NewMockClient(t), manager).UpdateTask(t.Context(), update))
}

func TestUpdateTaskRejectsInvalidInput(t *testing.T) {
	const id = "local-WHF-001"

	tests := []struct {
		name    string
		task    task.Task
		wantErr error
	}{
		{name: "blank title", task: task.Task{ID: id, Title: " "}, wantErr: service.ErrEmptyTaskTitle},
		{
			name:    "unknown status",
			task:    task.Task{ID: id, Title: "x", Status: "x"},
			wantErr: service.ErrUnknownTaskStatus,
		},
		{
			name:    "self parent",
			task:    task.Task{ID: id, Title: "x", ParentID: id},
			wantErr: service.ErrTaskReferencesItself,
		},
		{
			name:    "self child",
			task:    task.Task{ID: id, Title: "x", ChildrenIDs: []string{"C", id}},
			wantErr: service.ErrTaskReferencesItself,
		},
		{
			name:    "self link",
			task:    task.Task{ID: id, Title: "x", LinkedIDs: []string{id}},
			wantErr: service.ErrTaskReferencesItself,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// No expectations: nothing may be read or stored.
			err := service.NewService(mocks.NewMockClient(t), mocks.NewMockManager(t)).UpdateTask(t.Context(), tt.task)
			require.ErrorIs(t, err, tt.wantErr)
		})
	}
}

func TestUpdateTaskRejectsMissingOrNonLocalTask(t *testing.T) {
	failure := errors.New("get failed")

	manager := mocks.NewMockManager(t)
	manager.EXPECT().GetTask(mock.Anything, "MISSING").Return(task.Task{}, db.ErrTaskNotFound).Once()
	manager.EXPECT().
		GetTask(mock.Anything, "jira-WHF-1").
		Return(task.Task{ID: "jira-WHF-1", Originated: task.OriginJira}, nil).
		Once()
	manager.EXPECT().GetTask(mock.Anything, "BROKEN").Return(task.Task{}, failure).Once()

	svc := service.NewService(mocks.NewMockClient(t), manager)

	err := svc.UpdateTask(t.Context(), task.Task{ID: "MISSING", Title: "x"})
	require.ErrorIs(t, err, service.ErrTaskToUpdateNotFound)
	require.ErrorContains(t, err, "id=MISSING")

	err = svc.UpdateTask(t.Context(), task.Task{ID: "jira-WHF-1", Title: "x"})
	require.ErrorIs(t, err, service.ErrTaskNotLocal)
	require.ErrorContains(t, err, "originated=jira")

	err = svc.UpdateTask(t.Context(), task.Task{ID: "BROKEN", Title: "x"})
	require.ErrorIs(t, err, failure)
}

func TestUpdateTaskRejectsTaskGoneBeforeWrite(t *testing.T) {
	update := task.Task{ID: "local-WHF-001", Title: "new", Status: task.StatusTodo}

	manager := mocks.NewMockManager(t)
	manager.EXPECT().
		GetTask(mock.Anything, "local-WHF-001").
		Return(task.Task{ID: "local-WHF-001", Originated: task.OriginLocal}, nil).
		Once()
	manager.EXPECT().UpdateLocalTask(mock.Anything, update).Return(db.ErrLocalTaskToUpdateNotFound).Once()

	err := service.NewService(mocks.NewMockClient(t), manager).UpdateTask(t.Context(), update)
	require.ErrorIs(t, err, service.ErrLocalTaskToUpdateNotFound)
	require.ErrorContains(t, err, "id=local-WHF-001")
}

func TestUpdateTaskPropagatesWriteFailure(t *testing.T) {
	failure := errors.New("write failed")
	update := task.Task{ID: "local-WHF-001", Title: "new", Status: task.StatusTodo}

	manager := mocks.NewMockManager(t)
	manager.EXPECT().
		GetTask(mock.Anything, "local-WHF-001").
		Return(task.Task{ID: "local-WHF-001", Originated: task.OriginLocal}, nil).
		Once()
	manager.EXPECT().UpdateLocalTask(mock.Anything, update).Return(failure).Once()

	err := service.NewService(mocks.NewMockClient(t), manager).UpdateTask(t.Context(), update)
	require.ErrorIs(t, err, failure)
	require.NotErrorIs(t, err, service.ErrLocalTaskToUpdateNotFound)
}

func TestUpdateTaskStatus(t *testing.T) {
	failure := errors.New("write failed")
	before := time.Now()
	sinceBefore := mock.MatchedBy(func(at time.Time) bool { return !at.Before(before) && !at.After(time.Now()) })

	manager := mocks.NewMockManager(t)
	manager.EXPECT().
		UpdateLocalTaskStatus(mock.Anything, "local-WHF-001", task.Status(task.StatusDone), sinceBefore).
		Return(nil).
		Once()
	manager.EXPECT().
		UpdateLocalTaskStatus(mock.Anything, "jira-WHF-1", task.Status(task.StatusDone), sinceBefore).
		Return(db.ErrLocalTaskToSetStatusNotFound).
		Once()
	manager.EXPECT().
		UpdateLocalTaskStatus(mock.Anything, "BROKEN", task.Status(task.StatusDone), sinceBefore).
		Return(failure).
		Once()

	svc := service.NewService(mocks.NewMockClient(t), manager)

	require.NoError(t, svc.UpdateTaskStatus(t.Context(), "local-WHF-001", task.StatusDone, nil))

	err := svc.UpdateTaskStatus(t.Context(), "jira-WHF-1", task.StatusDone, nil)
	require.ErrorIs(t, err, service.ErrLocalTaskToSetStatusNotFound)
	require.ErrorContains(t, err, "id=jira-WHF-1")

	err = svc.UpdateTaskStatus(t.Context(), "BROKEN", task.StatusDone, nil)
	require.ErrorIs(t, err, failure)
	require.NotErrorIs(t, err, service.ErrLocalTaskToSetStatusNotFound)
}

func TestUpdateTaskStatusWithClosedAt(t *testing.T) {
	closedAt := time.Date(2026, 10, 21, 9, 30, 0, 0, time.UTC)

	manager := mocks.NewMockManager(t)
	manager.EXPECT().
		UpdateLocalTaskStatus(mock.Anything, "local-WHF-001", task.Status(task.StatusCanceled), closedAt).
		Return(nil).
		Once()

	svc := service.NewService(mocks.NewMockClient(t), manager)
	require.NoError(t, svc.UpdateTaskStatus(t.Context(), "local-WHF-001", task.StatusCanceled, &closedAt))
}

func TestUpdateTaskStatusRejectsClosedAtForOpenStatus(t *testing.T) {
	closedAt := time.Date(2026, 10, 21, 9, 30, 0, 0, time.UTC)

	// No expectations: nothing may be stored.
	svc := service.NewService(mocks.NewMockClient(t), mocks.NewMockManager(t))

	for _, status := range []task.Status{task.StatusTodo, task.StatusInProgress} {
		err := svc.UpdateTaskStatus(t.Context(), "local-WHF-001", status, &closedAt)
		require.ErrorIs(t, err, service.ErrClosedAtForOpenStatus)
		require.ErrorContains(t, err, "status="+string(status))
	}
}

func TestUpdateTaskStatusRejectsUnknownStatus(t *testing.T) {
	// No expectations: nothing may be stored.
	svc := service.NewService(mocks.NewMockClient(t), mocks.NewMockManager(t))

	for _, status := range []task.Status{"", "x"} {
		err := svc.UpdateTaskStatus(t.Context(), "local-WHF-001", status, nil)
		require.ErrorIs(t, err, service.ErrUnknownTaskStatusToSet)
	}
}

func TestReorderTasks(t *testing.T) {
	failure := errors.New("write failed")

	manager := mocks.NewMockManager(t)
	manager.EXPECT().SetTaskPositions(mock.Anything, []string{"T2", "T1"}).Return(nil).Once()
	manager.EXPECT().SetTaskPositions(mock.Anything, []string(nil)).Return(failure).Once()

	svc := service.NewService(mocks.NewMockClient(t), manager)

	require.NoError(t, svc.ReorderTasks(t.Context(), []string{"T2", "T1"}))
	require.ErrorIs(t, svc.ReorderTasks(t.Context(), nil), failure)
}

func TestReorderTasksRejectsInvalidIDs(t *testing.T) {
	// No expectations: nothing may be stored.
	svc := service.NewService(mocks.NewMockClient(t), mocks.NewMockManager(t))

	require.ErrorIs(t, svc.ReorderTasks(t.Context(), []string{"T1", " "}), service.ErrEmptyTaskIDToReorder)

	err := svc.ReorderTasks(t.Context(), []string{"T1", "T2", "T1"})
	require.ErrorIs(t, err, service.ErrDuplicateTaskIDToReorder)
	require.ErrorContains(t, err, "id=T1")
}

func TestDeleteTask(t *testing.T) {
	failure := errors.New("delete failed")

	manager := mocks.NewMockManager(t)
	manager.EXPECT().DeleteTask(mock.Anything, "T1").Return(nil).Once()
	manager.EXPECT().DeleteTask(mock.Anything, "MISSING").Return(db.ErrTaskToDeleteNotFound).Once()
	manager.EXPECT().DeleteTask(mock.Anything, "BROKEN").Return(failure).Once()

	svc := service.NewService(mocks.NewMockClient(t), manager)

	require.NoError(t, svc.DeleteTask(t.Context(), "T1"))

	err := svc.DeleteTask(t.Context(), "MISSING")
	require.ErrorIs(t, err, service.ErrTaskToDeleteNotFound)
	require.ErrorContains(t, err, "id=MISSING")

	require.ErrorIs(t, svc.DeleteTask(t.Context(), "BROKEN"), failure)
}
