package service_test

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/byron1st/wharf/internal/adapter/db"
	"github.com/byron1st/wharf/internal/pkg/mocks"
	"github.com/byron1st/wharf/internal/pkg/todo"
	"github.com/byron1st/wharf/internal/service"
)

func newTodoService(t *testing.T) (service.Service, *mocks.MockManager) {
	t.Helper()

	manager := mocks.NewMockManager(t)

	return service.NewService(mocks.NewMockClient(t), manager), manager
}

func TestCreateTodoNormalizesReminders(t *testing.T) {
	svc, manager := newTodoService(t)

	seoul := time.FixedZone("KST", 9*60*60)
	early := time.Date(2026, 10, 1, 9, 0, 0, 0, seoul)
	late := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)

	manager.EXPECT().CreateTodo(mock.Anything, todo.Todo{
		TaskID: "jira-WHF-1", Description: "d", StartDate: "2026-10-01", EndDate: "2026-10-01", Position: new(0),
		Reminders: []time.Time{early.UTC(), late},
	}).Return(7, nil).Once()

	id, err := svc.CreateTodo(t.Context(), todo.Todo{
		TaskID: "jira-WHF-1", Description: "d", StartDate: "2026-10-01", EndDate: "2026-10-01", Position: new(0),
		Reminders: []time.Time{late, early, late.In(seoul)},
	})
	require.NoError(t, err)
	require.Equal(t, int64(7), id)
}

func TestCreateTodoRejectsInvalidInput(t *testing.T) {
	valid := todo.Todo{TaskID: "T", Description: "d"}

	for name, tc := range map[string]struct {
		edit func(*todo.Todo)
		want error
	}{
		"blank description": {func(td *todo.Todo) { td.Description = " \t" }, service.ErrEmptyTodoDescription},
		"blank task id":     {func(td *todo.Todo) { td.TaskID = " " }, service.ErrEmptyTodoTaskID},
		"bad start date":    {func(td *todo.Todo) { td.StartDate = "2026-1-01" }, service.ErrInvalidTodoDate},
		"bad end date":      {func(td *todo.Todo) { td.EndDate = "2026-02-30" }, service.ErrInvalidTodoDate},
		"start after end": {func(td *todo.Todo) {
			td.StartDate, td.EndDate = "2026-10-02", "2026-10-01"
		}, service.ErrTodoStartsAfterEnd},
		"negative position": {func(td *todo.Todo) { td.Position = new(-1) }, service.ErrNegativeTodoPosition},
	} {
		t.Run(name, func(t *testing.T) {
			// No manager expectation: nothing is stored.
			svc, _ := newTodoService(t)

			td := valid
			tc.edit(&td)

			_, err := svc.CreateTodo(t.Context(), td)
			require.ErrorIs(t, err, tc.want)

			require.ErrorIs(t, svc.UpdateTodo(t.Context(), td), tc.want)
		})
	}
}

func TestCreateTodoMapsMissingTask(t *testing.T) {
	svc, manager := newTodoService(t)
	manager.EXPECT().CreateTodo(mock.Anything, mock.Anything).Return(0, db.ErrTaskOfTodoToCreateNotFound).Once()

	_, err := svc.CreateTodo(t.Context(), todo.Todo{TaskID: "T", Description: "d"})
	require.ErrorIs(t, err, service.ErrTaskOfNewTodoNotFound)
	require.ErrorContains(t, err, "task_id=T")
}

func TestUpdateTodoMapsErrors(t *testing.T) {
	failure := errors.New("boom")

	for dbErr, want := range map[error]error{
		db.ErrTodoToUpdateNotFound:       service.ErrTodoToUpdateNotFound,
		db.ErrTaskOfTodoToUpdateNotFound: service.ErrTaskOfUpdatedTodoNotFound,
		failure:                          failure,
	} {
		svc, manager := newTodoService(t)
		td := todo.Todo{ID: 3, TaskID: "T", Description: "d", Reminders: []time.Time{}}
		manager.EXPECT().UpdateTodo(mock.Anything, td).Return(dbErr).Once()

		require.ErrorIs(t, svc.UpdateTodo(t.Context(), td), want)
	}
}

func TestDeleteTodo(t *testing.T) {
	svc, manager := newTodoService(t)
	manager.EXPECT().DeleteTodo(mock.Anything, int64(3)).Return(nil).Once()
	manager.EXPECT().DeleteTodo(mock.Anything, int64(4)).Return(db.ErrTodoToDeleteNotFound).Once()

	require.NoError(t, svc.DeleteTodo(t.Context(), 3))

	err := svc.DeleteTodo(t.Context(), 4)
	require.ErrorIs(t, err, service.ErrTodoToDeleteNotFound)
	require.ErrorContains(t, err, "id=4")
}

func TestListTodos(t *testing.T) {
	svc, manager := newTodoService(t)

	filter := todo.Filter{TaskID: "T", Today: "2026-10-01", EndDate: "2026-10-02", IsDone: new(true)}
	want := []todo.Todo{{ID: 1, TaskID: "T", Description: "d"}}
	manager.EXPECT().ListTodos(mock.Anything, filter).Return(want, nil).Once()

	got, err := svc.ListTodos(t.Context(), filter)
	require.NoError(t, err)
	require.Equal(t, want, got)

	_, err = svc.ListTodos(t.Context(), todo.Filter{Today: "today"})
	require.ErrorIs(t, err, service.ErrInvalidTodoFilterDate)

	_, err = svc.ListTodos(t.Context(), todo.Filter{EndDate: "2026/10/01"})
	require.ErrorIs(t, err, service.ErrInvalidTodoFilterDate)
	require.ErrorContains(t, err, "end_date=2026/10/01")
}
