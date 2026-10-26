package server

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/byron1st/wharf/internal/pkg/todo"
	"github.com/byron1st/wharf/internal/service"
)

var (
	ErrFailedToDecodeTodoRequest  = errors.New("server: failed to decode todo request")
	ErrTodoRequestHasTrailingData = errors.New("server: todo request has trailing data")
	ErrTodoRequestTooLarge        = errors.New("server: todo request is too large")
	ErrEmptyTodoRequestDate       = errors.New("server: todo request date is an empty string")
	ErrInvalidTodoIsDoneFilter    = errors.New("server: is_done filter is not a bool")
	// ErrTodoIDNotInteger maps to 404: no todo has a non-integer ID.
	ErrTodoIDNotInteger = errors.New("server: todo id is not an integer")
)

// maxTodoRequestBytes bounds a todo request body; a larger one fails with
// ErrTodoRequestTooLarge.
const maxTodoRequestBytes = 1 << 20

// listTodos filters by the task_id, today, end_date, and is_done query
// parameters; an absent or empty one does not filter and others are ignored.
func listTodos(svc service.Service, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		filter := todo.Filter{TaskID: query.Get("task_id"), Today: query.Get("today"), EndDate: query.Get("end_date")}

		if v := query.Get("is_done"); v != "" {
			isDone, err := strconv.ParseBool(v)
			if err != nil {
				writeError(w, r, logger, "failed to list todos", errors.Join(ErrInvalidTodoIsDoneFilter, err))
				return
			}

			filter.IsDone = &isDone
		}

		todos, err := svc.ListTodos(r.Context(), filter)
		if err != nil {
			writeError(w, r, logger, "failed to list todos", err)
			return
		}

		resp := todoListResponse{Todos: make([]todoResponse, 0, len(todos))}
		for _, t := range todos {
			resp.Todos = append(resp.Todos, toTodoResponse(t))
		}

		writeJSON(w, http.StatusOK, resp)
	}
}

func createTodo(svc service.Service, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		t, err := decodeTodo(w, r)
		if err != nil {
			writeError(w, r, logger, "failed to create todo", err)
			return
		}

		id, err := svc.CreateTodo(r.Context(), t)
		if err != nil {
			writeError(w, r, logger, "failed to create todo", err)
			return
		}

		// No Location header: there is no GET /todos/{id}.
		writeJSON(w, http.StatusCreated, createTodoResponse{ID: id})
	}
}

func updateTodo(svc service.Service, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := todoID(r)
		if err != nil {
			writeError(w, r, logger, "failed to update todo", err)
			return
		}

		t, err := decodeTodo(w, r)
		if err != nil {
			writeError(w, r, logger, "failed to update todo", err)
			return
		}

		t.ID = id

		if err := svc.UpdateTodo(r.Context(), t); err != nil {
			writeError(w, r, logger, "failed to update todo", err)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

func deleteTodo(svc service.Service, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := todoID(r)
		if err != nil {
			writeError(w, r, logger, "failed to delete todo", err)
			return
		}

		if err := svc.DeleteTodo(r.Context(), id); err != nil {
			writeError(w, r, logger, "failed to delete todo", err)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

func todoID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		return 0, errors.Join(ErrTodoIDNotInteger, err)
	}

	return id, nil
}

// decodeTodo reads a todoRequest body as decodeTask reads a task request. A
// null date is none, while an empty string is rejected like any other
// malformed date.
func decodeTodo(w http.ResponseWriter, r *http.Request) (todo.Todo, error) {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxTodoRequestBytes))
	decoder.DisallowUnknownFields()

	var req todoRequest

	err := decoder.Decode(&req)
	if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
		return todo.Todo{}, errors.Join(ErrTodoRequestTooLarge, err)
	}

	if err != nil {
		return todo.Todo{}, errors.Join(ErrFailedToDecodeTodoRequest, err)
	}

	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return todo.Todo{}, errors.Join(ErrTodoRequestHasTrailingData, err)
	}

	if req.StartDate != nil && *req.StartDate == "" || req.EndDate != nil && *req.EndDate == "" {
		return todo.Todo{}, ErrEmptyTodoRequestDate
	}

	return todo.Todo{
		TaskID:      req.TaskID,
		Description: req.Description,
		IsDone:      req.IsDone,
		StartDate:   deref(req.StartDate),
		EndDate:     deref(req.EndDate),
		Reminders:   req.Reminders,
		Position:    req.Position,
	}, nil
}

func toTodoResponse(t todo.Todo) todoResponse {
	resp := todoResponse{
		ID:          t.ID,
		TaskID:      t.TaskID,
		Description: t.Description,
		IsDone:      t.IsDone,
		Reminders:   t.Reminders,
		Position:    t.Position,
	}

	if resp.Reminders == nil {
		resp.Reminders = []time.Time{}
	}

	if t.StartDate != "" {
		resp.StartDate = &t.StartDate
	}

	if t.EndDate != "" {
		resp.EndDate = &t.EndDate
	}

	return resp
}

func deref(s *string) string {
	if s == nil {
		return ""
	}

	return *s
}
