package server_test

import (
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/byron1st/wharf/internal/pkg/mocks"
	"github.com/byron1st/wharf/internal/pkg/todo"
	"github.com/byron1st/wharf/internal/service"
)

func TestListTodos(t *testing.T) {
	at := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)

	svc := mocks.NewMockService(t)
	svc.EXPECT().ListTodos(mock.Anything, todo.Filter{
		TaskID: "jira-WHF-1", Today: "2026-10-01", EndDate: "2026-10-02", IsDone: new(false),
	}).Return([]todo.Todo{
		{
			ID: 1, TaskID: "jira-WHF-1", Description: "a", IsDone: false, StartDate: "2026-10-01",
			EndDate: "2026-10-02", Reminders: []time.Time{at}, Position: new(0),
		},
		{ID: 2, TaskID: "jira-WHF-1", Description: "b", Position: new(1)},
	}, nil).Once()

	rec, _ := serve(t, svc, http.MethodGet,
		"/todos?task_id=jira-WHF-1&today=2026-10-01&end_date=2026-10-02&is_done=0&other=x")

	require.Equal(t, http.StatusOK, rec.Code)
	require.JSONEq(t, `{"todos":[
		{"id":1,"task_id":"jira-WHF-1","description":"a","is_done":false,"start_date":"2026-10-01",
		 "end_date":"2026-10-02","reminders":["2026-10-01T09:00:00Z"],"position":0},
		{"id":2,"task_id":"jira-WHF-1","description":"b","is_done":false,"start_date":null,
		 "end_date":null,"reminders":[],"position":1}
	]}`, rec.Body.String())
}

func TestListTodosWithoutFilter(t *testing.T) {
	svc := mocks.NewMockService(t)
	svc.EXPECT().ListTodos(mock.Anything, todo.Filter{IsDone: new(true)}).Return(nil, nil).Once()
	svc.EXPECT().ListTodos(mock.Anything, todo.Filter{}).Return(nil, nil).Once()

	rec, _ := serve(t, svc, http.MethodGet, "/todos?is_done=true")
	require.Equal(t, http.StatusOK, rec.Code)
	require.JSONEq(t, `{"todos":[]}`, rec.Body.String())

	rec, _ = serve(t, svc, http.MethodGet, "/todos")
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestListTodosRejectsInvalidFilter(t *testing.T) {
	// No ListTodos expectation: the mock fails the test if it is called.
	rec, _ := serve(t, mocks.NewMockService(t), http.MethodGet, "/todos?is_done=maybe")

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "server: is_done filter is not a bool")

	svc := mocks.NewMockService(t)
	svc.EXPECT().ListTodos(mock.Anything, todo.Filter{Today: "x"}).Return(nil, service.ErrInvalidTodoFilterDate).Once()

	rec, _ = serve(t, svc, http.MethodGet, "/todos?today=x")
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestCreateTodo(t *testing.T) {
	at := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)

	svc := mocks.NewMockService(t)
	svc.EXPECT().CreateTodo(mock.Anything, mock.MatchedBy(func(got todo.Todo) bool {
		return got.TaskID == "local-WHF-001" && got.Description == "d" && got.IsDone &&
			got.StartDate == "2026-10-01" && got.EndDate == "" && len(got.Reminders) == 1 &&
			got.Reminders[0].Equal(at) && got.Position != nil && *got.Position == 2
	})).Return(5, nil).Once()

	rec, _ := serveBody(t, svc, http.MethodPost, "/todos", strings.NewReader(`{
		"task_id":"local-WHF-001","description":"d","is_done":true,"start_date":"2026-10-01","end_date":null,
		"reminders":["2026-10-01T18:00:00+09:00"],"position":2
	}`))

	require.Equal(t, http.StatusCreated, rec.Code)
	require.Empty(t, rec.Header().Get("Location"))
	require.JSONEq(t, `{"id":5}`, rec.Body.String())
}

func TestCreateTodoDefaults(t *testing.T) {
	svc := mocks.NewMockService(t)
	svc.EXPECT().CreateTodo(mock.Anything, todo.Todo{TaskID: "T", Description: "d"}).Return(1, nil).Once()

	rec, _ := serveBody(t, svc, http.MethodPost, "/todos", strings.NewReader(`{"task_id":"T","description":"d"}`))

	require.Equal(t, http.StatusCreated, rec.Code)
}

func TestTodoRoutesRejectInvalidBody(t *testing.T) {
	for name, tc := range map[string]struct {
		body, want string
		status     int
	}{
		"malformed":     {`{"task_id":`, "server: failed to decode todo request", http.StatusBadRequest},
		"unknown field": {`{"task_id":"T","id":3}`, "server: failed to decode todo request", http.StatusBadRequest},
		"bad reminder":  {`{"reminders":["2026-10-01"]}`, "server: failed to decode todo request", http.StatusBadRequest},
		"bad position":  {`{"position":1.5}`, "server: failed to decode todo request", http.StatusBadRequest},
		"trailing":      {`{"task_id":"T"} {}`, "server: todo request has trailing data", http.StatusBadRequest},
		"empty date":    {`{"start_date":""}`, "server: todo request date is an empty string", http.StatusBadRequest},
		"too large": {
			`{"description":"` + strings.Repeat("x", 1<<20) + `"}`,
			"server: todo request is too large",
			http.StatusRequestEntityTooLarge,
		},
	} {
		for _, req := range []struct{ method, path string }{
			{http.MethodPost, "/todos"},
			{http.MethodPut, "/todos/1"},
		} {
			t.Run(name+" "+req.method, func(t *testing.T) {
				// No service expectation: the mock fails the test if it is called.
				rec, _ := serveBody(t, mocks.NewMockService(t), req.method, req.path, strings.NewReader(tc.body))

				require.Equal(t, tc.status, rec.Code)
				require.Contains(t, rec.Body.String(), tc.want)
			})
		}
	}
}

func TestUpdateTodo(t *testing.T) {
	svc := mocks.NewMockService(t)
	svc.EXPECT().UpdateTodo(mock.Anything, todo.Todo{
		ID: 3, TaskID: "T", Description: "d", EndDate: "2026-10-02", Reminders: []time.Time{},
	}).Return(nil).Once()

	rec, _ := serveBody(t, svc, http.MethodPut, "/todos/3",
		strings.NewReader(`{"task_id":"T","description":"d","end_date":"2026-10-02","reminders":[]}`))

	require.Equal(t, http.StatusNoContent, rec.Code)
	require.Empty(t, rec.Body.String())
}

func TestDeleteTodo(t *testing.T) {
	svc := mocks.NewMockService(t)
	svc.EXPECT().DeleteTodo(mock.Anything, int64(3)).Return(nil).Once()

	rec, _ := serve(t, svc, http.MethodDelete, "/todos/3")

	require.Equal(t, http.StatusNoContent, rec.Code)
}

func TestTodoRoutesRejectNonIntegerID(t *testing.T) {
	for _, method := range []string{http.MethodPut, http.MethodDelete} {
		// No service expectation: the mock fails the test if it is called.
		rec, _ := serveBody(t, mocks.NewMockService(t), method, "/todos/abc", strings.NewReader(`{}`))

		require.Equal(t, http.StatusNotFound, rec.Code, method)
		require.Contains(t, rec.Body.String(), "server: todo id is not an integer")
	}
}

func TestTodoRoutesMapServiceErrors(t *testing.T) {
	body := `{"task_id":"T","description":"d"}`

	for err, status := range map[error]int{
		service.ErrEmptyTodoDescription:      http.StatusBadRequest,
		service.ErrEmptyTodoTaskID:           http.StatusBadRequest,
		service.ErrInvalidTodoDate:           http.StatusBadRequest,
		service.ErrTodoStartsAfterEnd:        http.StatusBadRequest,
		service.ErrNegativeTodoPosition:      http.StatusBadRequest,
		service.ErrTaskOfNewTodoNotFound:     http.StatusBadRequest,
		service.ErrTaskOfUpdatedTodoNotFound: http.StatusBadRequest,
		service.ErrTodoToUpdateNotFound:      http.StatusNotFound,
		errors.New("db down"):                http.StatusInternalServerError,
	} {
		svc := mocks.NewMockService(t)
		svc.EXPECT().CreateTodo(mock.Anything, mock.Anything).Return(0, err).Once()
		svc.EXPECT().UpdateTodo(mock.Anything, mock.Anything).Return(err).Once()

		rec, _ := serveBody(t, svc, http.MethodPost, "/todos", strings.NewReader(body))
		require.Equal(t, status, rec.Code, "POST %v", err)

		rec, _ = serveBody(t, svc, http.MethodPut, "/todos/1", strings.NewReader(body))
		require.Equal(t, status, rec.Code, "PUT %v", err)
	}

	svc := mocks.NewMockService(t)
	svc.EXPECT().DeleteTodo(mock.Anything, int64(9)).Return(service.ErrTodoToDeleteNotFound).Once()

	rec, _ := serve(t, svc, http.MethodDelete, "/todos/9")
	require.Equal(t, http.StatusNotFound, rec.Code)
}
