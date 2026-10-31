package service_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/byron1st/wharf/internal/adapter/db"
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

	got, err := service.NewService(mocks.NewMockClient(t), manager).ListTasks(t.Context())
	require.NoError(t, err)
	require.Equal(t, []service.TaskSummary{
		{Task: tasks[0]},
		{Task: tasks[1], Todos: todo.Count{Todos: 5, Done: 2}},
	}, got)
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
		CreateLocalTask(mock.Anything, task.Task{Title: "done", Status: task.StatusDone}).
		Return("local-WHF-002", nil).
		Once()

	svc := service.NewService(mocks.NewMockClient(t), manager)

	id, err := svc.CreateTask(t.Context(), task.Task{Title: "new", ChildrenIDs: []string{"jira-J"}})
	require.NoError(t, err)
	require.Equal(t, "local-WHF-001", id)

	id, err = svc.CreateTask(t.Context(), task.Task{Title: "done", Status: task.StatusDone})
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
