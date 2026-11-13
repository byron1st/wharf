package server_test

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/byron1st/wharf/internal/pkg/mocks"
	"github.com/byron1st/wharf/internal/pkg/task"
	"github.com/byron1st/wharf/internal/pkg/todo"
	"github.com/byron1st/wharf/internal/server"
	"github.com/byron1st/wharf/internal/service"
)

// serve sends method path with the request ID "req-1" to a handler backed by
// svc and returns the response and the logs.
func serve(t *testing.T, svc service.Service, method, path string) (*httptest.ResponseRecorder, string) {
	t.Helper()

	return serveBody(t, svc, method, path, nil)
}

// serveBody is serve with a request body.
func serveBody(
	t *testing.T,
	svc service.Service,
	method, path string,
	body io.Reader,
) (*httptest.ResponseRecorder, string) {
	t.Helper()

	var logs bytes.Buffer

	req := httptest.NewRequestWithContext(t.Context(), method, path, body)
	req.Header.Set("X-Request-ID", "req-1")

	rec := httptest.NewRecorder()
	server.NewHandler(svc, mocks.NewMockSettings(t), mocks.NewMockUpdater(t), webApp, slog.New(slog.NewJSONHandler(&logs, nil))).
		ServeHTTP(rec, req)

	return rec, logs.String()
}

func TestHealthz(t *testing.T) {
	rec, _ := serve(t, mocks.NewMockService(t), http.MethodGet, "/api/healthz")

	require.Equal(t, http.StatusOK, rec.Code)
	require.JSONEq(t, `{"status":"ok"}`, rec.Body.String())
}

func TestRemovedRoutes(t *testing.T) {
	for _, tt := range []struct{ method, path string }{
		{method: http.MethodPost, path: "/api/tasks/jira"},
		{method: http.MethodPost, path: "/api/service-desk/requests"},
		{method: http.MethodPost, path: "/tasks"},
		{method: http.MethodGet, path: "/api/healthz/extra"},
	} {
		// No service expectation: the mock fails the test if it is called.
		rec, _ := serve(t, mocks.NewMockService(t), tt.method, tt.path)

		require.Contains(
			t,
			[]int{http.StatusNotFound, http.StatusMethodNotAllowed},
			rec.Code,
			"%s %s",
			tt.method,
			tt.path,
		)
	}
}

func TestRoutesRejectOtherMethods(t *testing.T) {
	for _, tt := range []struct{ method, path string }{
		{method: http.MethodPost, path: "/api/healthz"},
		{method: http.MethodDelete, path: "/api/tasks"},
		{method: http.MethodPatch, path: "/api/tasks/T1"},
		{method: http.MethodDelete, path: "/api/sync/atlassian"},
	} {
		rec, _ := serve(t, mocks.NewMockService(t), tt.method, tt.path)

		require.Equal(t, http.StatusMethodNotAllowed, rec.Code, "%s %s", tt.method, tt.path)
	}
}

func TestListTasks(t *testing.T) {
	due := time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC)
	closed := time.Date(2026, 10, 23, 5, 6, 7, 0, time.UTC)
	long := strings.Repeat("가", 101)

	svc := mocks.NewMockService(t)
	svc.EXPECT().ListTasks(mock.Anything).Return([]service.TaskSummary{
		{
			Task: task.Task{
				ID:          "jira-WHF-1",
				Originated:  task.OriginJira,
				Title:       "first",
				Description: long,
				Status:      task.StatusTodo,
				DueDate:     &due,
				ParentID:    "jira-WHF-0",
				ChildrenIDs: []string{"jira-WHF-2"},
			},
			Todos: todo.Count{Todos: 5, Done: 2},
		},
		{
			Task: task.Task{
				ID:          "local-WHF-001",
				Originated:  task.OriginLocal,
				Title:       "second",
				Description: strings.Repeat("가", 100),
				Status:      task.StatusDone,
				Position:    new(3),
				ClosedAt:    &closed,
			},
		},
		{
			Task: task.Task{
				ID:             "jira-WHF-9",
				Originated:     task.OriginJira,
				Title:          "third",
				Status:         task.StatusCanceled,
				ClosedAt:       &closed,
				ClosedSyncedAt: &closed,
			},
			ClosedSinceLastSync: true,
		},
	}, nil).Once()

	rec, _ := serve(t, svc, http.MethodGet, "/api/tasks")

	require.Equal(t, http.StatusOK, rec.Code)
	require.JSONEq(t, `{"tasks":[
		{"id":"jira-WHF-1","originated":"jira","title":"first","description":"`+strings.Repeat("가", 100)+`…",
		 "status":"todo","start_date":null,"due_date":"2026-10-31T00:00:00Z","todo_count":5,"done_todo_count":2,
		 "position":null,"closed_at":null,"closed_since_last_sync":false},
		{"id":"local-WHF-001","originated":"local","title":"second","description":"`+strings.Repeat("가", 100)+`",
		 "status":"done","start_date":null,"due_date":null,"todo_count":0,"done_todo_count":0,"position":3,
		 "closed_at":"2026-10-23T05:06:07Z","closed_since_last_sync":false},
		{"id":"jira-WHF-9","originated":"jira","title":"third","description":"",
		 "status":"canceled","start_date":null,"due_date":null,"todo_count":0,"done_todo_count":0,"position":null,
		 "closed_at":"2026-10-23T05:06:07Z","closed_since_last_sync":true}
	]}`, rec.Body.String())
}

func TestListTasksEmpty(t *testing.T) {
	svc := mocks.NewMockService(t)
	svc.EXPECT().ListTasks(mock.Anything).Return(nil, nil).Once()

	rec, _ := serve(t, svc, http.MethodGet, "/api/tasks")

	require.Equal(t, http.StatusOK, rec.Code)
	require.JSONEq(t, `{"tasks":[]}`, rec.Body.String())
}

func TestGetTask(t *testing.T) {
	long := strings.Repeat("a", 150)

	svc := mocks.NewMockService(t)
	svc.EXPECT().GetTask(mock.Anything, "local-WHF-001").Return(service.TaskDetail{
		Task: task.Task{
			ID:          "local-WHF-001",
			Originated:  task.OriginLocal,
			Title:       "target",
			Description: long,
			Status:      task.StatusInProgress,
			ParentID:    "jira-P",
			ChildrenIDs: []string{"jira-C1", "jira-C9"},
			LinkedIDs:   []string{"jira-L1"},
		},
		Parent:   &task.Task{ID: "jira-P", Originated: task.OriginJira, Title: "parent", Description: long},
		Children: []task.Task{{ID: "jira-C1", Originated: task.OriginJira, Title: "child"}},
		Linked:   []task.Task{{ID: "jira-L1", Originated: task.OriginJira, Title: "linked"}},
	}, nil).Once()

	rec, _ := serve(t, svc, http.MethodGet, "/api/tasks/local-WHF-001")

	cut := strings.Repeat("a", 100) + "…"

	require.Equal(t, http.StatusOK, rec.Code)
	require.JSONEq(t, `{
		"id":"local-WHF-001","originated":"local","title":"target","description":"`+long+`",
		"status":"in-progress","start_date":null,"due_date":null,
		"parent_id":"jira-P","children_ids":["jira-C1","jira-C9"],"linked_ids":["jira-L1"],
		"parent":{"id":"jira-P","originated":"jira","title":"parent","description":"`+cut+`",
		          "status":"","start_date":null,"due_date":null},
		"children":[{"id":"jira-C1","originated":"jira","title":"child","description":"",
		             "status":"","start_date":null,"due_date":null}],
		"linked":[{"id":"jira-L1","originated":"jira","title":"linked","description":"",
		           "status":"","start_date":null,"due_date":null}]
	}`, rec.Body.String())
}

func TestGetTaskWithoutRelations(t *testing.T) {
	svc := mocks.NewMockService(t)
	svc.EXPECT().
		GetTask(mock.Anything, "jira-WHF-1").
		Return(service.TaskDetail{Task: task.Task{ID: "jira-WHF-1", Originated: task.OriginJira, Title: "alone"}}, nil).
		Once()

	rec, _ := serve(t, svc, http.MethodGet, "/api/tasks/jira-WHF-1")

	require.Equal(t, http.StatusOK, rec.Code)
	require.JSONEq(t, `{
		"id":"jira-WHF-1","originated":"jira","title":"alone","description":"","status":"",
		"start_date":null,"due_date":null,
		"parent_id":null,"children_ids":[],"linked_ids":[],"parent":null,"children":[],"linked":[]
	}`, rec.Body.String())
}

func TestCreateTask(t *testing.T) {
	start := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)

	svc := mocks.NewMockService(t)
	svc.EXPECT().CreateTask(mock.Anything, mock.MatchedBy(func(got task.Task) bool {
		return got.Title == "new" && got.Description == "desc" && got.Status == task.StatusDone &&
			got.StartDate != nil && got.StartDate.Equal(start) && got.DueDate == nil &&
			got.ParentID == "jira-P" && slices.Equal(got.ChildrenIDs, []string{"jira-C"}) &&
			slices.Equal(got.LinkedIDs, []string{"jira-L"})
	})).Return("local-WHF-001", nil).Once()

	rec, _ := serveBody(t, svc, http.MethodPost, "/api/tasks", strings.NewReader(`{
		"title":"new","description":"desc","status":"done","start_date":"2026-10-01T09:00:00Z","due_date":null,
		"parent_id":"jira-P","children_ids":["jira-C"],"linked_ids":["jira-L"]
	}`))

	require.Equal(t, http.StatusCreated, rec.Code)
	require.Equal(t, "/api/tasks/local-WHF-001", rec.Header().Get("Location"))
	require.JSONEq(t, `{"id":"local-WHF-001"}`, rec.Body.String())
}

func TestCreateTaskRejectsInvalidBody(t *testing.T) {
	bodies := map[string]string{
		"empty":         ``,
		"malformed":     `{"title":`,
		"unknown field": `{"title":"x","owner":"me"}`,
		"id":            `{"title":"x","id":"local-WHF-009"}`,
		"originated":    `{"title":"x","originated":"jira"}`,
		"bad date":      `{"title":"x","due_date":"2026-10-31"}`,
	}

	for name, body := range bodies {
		t.Run(name, func(t *testing.T) {
			// No CreateTask expectation: the mock fails the test if it is called.
			rec, logs := serveBody(t, mocks.NewMockService(t), http.MethodPost, "/api/tasks", strings.NewReader(body))

			require.Equal(t, http.StatusBadRequest, rec.Code)
			require.Contains(t, rec.Body.String(), "server: failed to decode task request")
			require.Contains(t, logs, `"request_id":"req-1"`)
		})
	}
}

func TestCreateTaskRejectsTooLargeBody(t *testing.T) {
	body := `{"title":"` + strings.Repeat("x", 1<<20) + `"}`

	// No CreateTask expectation: the mock fails the test if it is called.
	rec, _ := serveBody(t, mocks.NewMockService(t), http.MethodPost, "/api/tasks", strings.NewReader(body))

	require.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
	require.Contains(t, rec.Body.String(), "server: task request is too large")
}

func TestCreateTaskRejectsTrailingData(t *testing.T) {
	bodies := map[string]string{
		"second object": `{"title":"x"}{"id":"y"}`,
		"garbage":       `{"title":"x"} garbage`,
		"extra brace":   `{"title":"x"}}`,
	}

	for name, body := range bodies {
		t.Run(name, func(t *testing.T) {
			// No CreateTask expectation: the mock fails the test if it is called.
			rec, _ := serveBody(t, mocks.NewMockService(t), http.MethodPost, "/api/tasks", strings.NewReader(body))

			require.Equal(t, http.StatusBadRequest, rec.Code)
			require.Contains(t, rec.Body.String(), "server: task request has trailing data")
		})
	}
}

func TestUpdateTaskRejectsTrailingData(t *testing.T) {
	// No UpdateTask expectation: the mock fails the test if it is called.
	rec, _ := serveBody(
		t,
		mocks.NewMockService(t),
		http.MethodPut,
		"/api/tasks/T1",
		strings.NewReader(`{"title":"x"} {}`),
	)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "server: task request has trailing data")
}

func TestCreateTaskAcceptsTrailingWhitespace(t *testing.T) {
	svc := mocks.NewMockService(t)
	svc.EXPECT().CreateTask(mock.Anything, mock.Anything).Return("local-WHF-001", nil).Once()

	rec, _ := serveBody(t, svc, http.MethodPost, "/api/tasks", strings.NewReader("{\"title\":\"x\"} \n\t\n"))

	require.Equal(t, http.StatusCreated, rec.Code)
}

func TestTaskRoutesMapServiceErrors(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		status int
	}{
		{name: "empty title", err: service.ErrEmptyTaskTitle, status: http.StatusBadRequest},
		{
			name:   "unknown status",
			err:    fmt.Errorf("%w: status=x", service.ErrUnknownTaskStatus),
			status: http.StatusBadRequest,
		},
		{name: "self reference", err: service.ErrTaskReferencesItself, status: http.StatusBadRequest},
		{name: "empty related id", err: service.ErrEmptyRelatedTaskID, status: http.StatusBadRequest},
		{name: "not found", err: service.ErrTaskNotFound, status: http.StatusNotFound},
		{name: "update not found", err: service.ErrTaskToUpdateNotFound, status: http.StatusNotFound},
		{name: "local update not found", err: service.ErrLocalTaskToUpdateNotFound, status: http.StatusNotFound},
		{name: "delete not found", err: service.ErrTaskToDeleteNotFound, status: http.StatusNotFound},
		{name: "not local", err: service.ErrTaskNotLocal, status: http.StatusConflict},
		{name: "other", err: errors.New("disk failed"), status: http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := mocks.NewMockService(t)
			svc.EXPECT().ListTasks(mock.Anything).Return(nil, tt.err).Once()
			svc.EXPECT().GetTask(mock.Anything, "T1").Return(service.TaskDetail{}, tt.err).Once()
			svc.EXPECT().CreateTask(mock.Anything, mock.Anything).Return("", tt.err).Once()
			svc.EXPECT().UpdateTask(mock.Anything, mock.Anything).Return(tt.err).Once()
			svc.EXPECT().DeleteTask(mock.Anything, "T1").Return(tt.err).Once()

			for _, req := range []struct{ method, path, body string }{
				{method: http.MethodGet, path: "/api/tasks"},
				{method: http.MethodGet, path: "/api/tasks/T1"},
				{method: http.MethodPost, path: "/api/tasks", body: `{"title":"x"}`},
				{method: http.MethodPut, path: "/api/tasks/T1", body: `{"title":"x"}`},
				{method: http.MethodDelete, path: "/api/tasks/T1"},
			} {
				rec, logs := serveBody(t, svc, req.method, req.path, strings.NewReader(req.body))

				require.Equal(t, tt.status, rec.Code, "%s %s", req.method, req.path)
				require.JSONEq(t, `{"error":`+strconv.Quote(tt.err.Error())+`}`, rec.Body.String())
				require.Contains(t, logs, `"request_id":"req-1"`)
			}
		})
	}
}

func TestUpdateTask(t *testing.T) {
	svc := mocks.NewMockService(t)
	svc.EXPECT().
		UpdateTask(mock.Anything, task.Task{ID: "local-WHF-001", Title: "new", ChildrenIDs: []string{"jira-C"}}).
		Return(nil).
		Once()

	rec, _ := serveBody(
		t,
		svc,
		http.MethodPut,
		"/api/tasks/local-WHF-001",
		strings.NewReader(`{"title":"new","children_ids":["jira-C"]}`),
	)

	require.Equal(t, http.StatusNoContent, rec.Code)
	require.Empty(t, rec.Body.String())
}

func TestUpdateTaskRejectsInvalidBody(t *testing.T) {
	rec, _ := serveBody(
		t,
		mocks.NewMockService(t),
		http.MethodPut,
		"/api/tasks/local-WHF-001",
		strings.NewReader(`{"title":"x","id":"local-WHF-002"}`),
	)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestUpdateTaskStatus(t *testing.T) {
	svc := mocks.NewMockService(t)
	svc.EXPECT().
		UpdateTaskStatus(mock.Anything, "local-WHF-001", task.Status(task.StatusDone), (*time.Time)(nil)).
		Return(nil).
		Once()

	rec, _ := serveBody(
		t,
		svc,
		http.MethodPut,
		"/api/tasks/local-WHF-001/status",
		strings.NewReader(`{"status":"done"}`),
	)

	require.Equal(t, http.StatusNoContent, rec.Code)
	require.Empty(t, rec.Body.String())
}

func TestUpdateTaskStatusWithClosedAt(t *testing.T) {
	closedAt := time.Date(2026, 10, 21, 0, 30, 0, 0, time.UTC)

	svc := mocks.NewMockService(t)
	svc.EXPECT().
		UpdateTaskStatus(mock.Anything, "local-WHF-001", task.Status(task.StatusCanceled), mock.MatchedBy(
			func(at *time.Time) bool { return at != nil && at.Equal(closedAt) },
		)).
		Return(nil).
		Once()

	rec, _ := serveBody(
		t,
		svc,
		http.MethodPut,
		"/api/tasks/local-WHF-001/status",
		strings.NewReader(`{"status":"canceled","closed_at":"2026-10-21T09:30:00+09:00"}`),
	)

	require.Equal(t, http.StatusNoContent, rec.Code)
}

func TestReorderTasks(t *testing.T) {
	svc := mocks.NewMockService(t)
	svc.EXPECT().ReorderTasks(mock.Anything, []string{"local-WHF-001", "jira-WHF-1"}).Return(nil).Once()

	rec, _ := serveBody(
		t,
		svc,
		http.MethodPut,
		"/api/tasks/order",
		strings.NewReader(`{"ids":["local-WHF-001","jira-WHF-1"]}`),
	)

	require.Equal(t, http.StatusNoContent, rec.Code)
	require.Empty(t, rec.Body.String())
}

func TestTaskStatusAndOrderRoutesRejectInvalidBody(t *testing.T) {
	for _, req := range []struct{ path, body string }{
		{path: "/api/tasks/local-WHF-001/status", body: `{"status":"done","title":"x"}`},
		{path: "/api/tasks/local-WHF-001/status", body: `{"status":"done"} {}`},
		{path: "/api/tasks/local-WHF-001/status", body: `{"status":"done","closed_at":"yesterday"}`},
		{path: "/api/tasks/order", body: `{"ids":"T1"}`},
	} {
		rec, _ := serveBody(t, mocks.NewMockService(t), http.MethodPut, req.path, strings.NewReader(req.body))

		require.Equal(t, http.StatusBadRequest, rec.Code, req.body)
	}
}

func TestTaskStatusAndOrderRoutesMapServiceErrors(t *testing.T) {
	tests := []struct {
		err    error
		status int
	}{
		{err: service.ErrUnknownTaskStatusToSet, status: http.StatusBadRequest},
		{err: service.ErrClosedAtForOpenStatus, status: http.StatusBadRequest},
		{err: service.ErrLocalTaskToSetStatusNotFound, status: http.StatusNotFound},
		{err: service.ErrEmptyTaskIDToReorder, status: http.StatusBadRequest},
		{err: service.ErrDuplicateTaskIDToReorder, status: http.StatusBadRequest},
		{err: errors.New("disk failed"), status: http.StatusInternalServerError},
	}

	for _, tt := range tests {
		svc := mocks.NewMockService(t)
		svc.EXPECT().UpdateTaskStatus(mock.Anything, "T1", mock.Anything, mock.Anything).Return(tt.err).Once()
		svc.EXPECT().ReorderTasks(mock.Anything, mock.Anything).Return(tt.err).Once()

		for _, req := range []struct{ path, body string }{
			{path: "/api/tasks/T1/status", body: `{"status":"x"}`},
			{path: "/api/tasks/order", body: `{"ids":["T1"]}`},
		} {
			rec, _ := serveBody(t, svc, http.MethodPut, req.path, strings.NewReader(req.body))

			require.Equal(t, tt.status, rec.Code, "%s: %v", req.path, tt.err)
			require.JSONEq(t, `{"error":`+strconv.Quote(tt.err.Error())+`}`, rec.Body.String())
		}
	}
}

func TestDeleteTask(t *testing.T) {
	svc := mocks.NewMockService(t)
	svc.EXPECT().DeleteTask(mock.Anything, "jira-WHF-1").Return(nil).Once()

	rec, _ := serve(t, svc, http.MethodDelete, "/api/tasks/jira-WHF-1")

	require.Equal(t, http.StatusNoContent, rec.Code)
	require.Empty(t, rec.Body.String())
}
