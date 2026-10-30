package service

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/byron1st/wharf/internal/adapter/db"
	"github.com/byron1st/wharf/internal/pkg/todo"
)

var (
	ErrEmptyTodoDescription      = errors.New("service: todo description is empty")
	ErrEmptyTodoTaskID           = errors.New("service: todo task id is empty")
	ErrInvalidTodoDate           = errors.New("service: todo date is not YYYY-MM-DD")
	ErrTodoStartsAfterEnd        = errors.New("service: todo start date is after its end date")
	ErrNegativeTodoPosition      = errors.New("service: todo position is negative")
	ErrTaskOfNewTodoNotFound     = errors.New("service: task of new todo not found")
	ErrTaskOfUpdatedTodoNotFound = errors.New("service: task of updated todo not found")
	ErrTodoToUpdateNotFound      = errors.New("service: todo to update not found")
	ErrTodoToDeleteNotFound      = errors.New("service: todo to delete not found")
)

// ListTodos returns the todos matching every set field of f, ordered by task
// ID and position.
func (s *service) ListTodos(ctx context.Context, f todo.Filter) ([]todo.Todo, error) {
	return s.db.ListTodos(ctx, f)
}

// CreateTodo validates t and stores it as a new todo of the stored task
// t.TaskID, returning its ID. It fails with ErrTaskOfNewTodoNotFound when no
// task has t.TaskID.
func (s *service) CreateTodo(ctx context.Context, t todo.Todo) (int64, error) {
	t, err := normalizeTodo(t)
	if err != nil {
		return 0, err
	}

	id, err := s.db.CreateTodo(ctx, t)
	if errors.Is(err, db.ErrTaskOfTodoToCreateNotFound) {
		return 0, fmt.Errorf("%w: task_id=%s", ErrTaskOfNewTodoNotFound, t.TaskID)
	}

	return id, err
}

// UpdateTodo validates t and replaces the stored todo t.ID with it, moving it
// to t.TaskID or to t.Position when they change. It fails with
// ErrTodoToUpdateNotFound when no todo has t.ID and with
// ErrTaskOfUpdatedTodoNotFound when no task has t.TaskID.
func (s *service) UpdateTodo(ctx context.Context, t todo.Todo) error {
	t, err := normalizeTodo(t)
	if err != nil {
		return err
	}

	err = s.db.UpdateTodo(ctx, t)
	if errors.Is(err, db.ErrTodoToUpdateNotFound) {
		return fmt.Errorf("%w: id=%d", ErrTodoToUpdateNotFound, t.ID)
	}

	if errors.Is(err, db.ErrTaskOfTodoToUpdateNotFound) {
		return fmt.Errorf("%w: id=%d, task_id=%s", ErrTaskOfUpdatedTodoNotFound, t.ID, t.TaskID)
	}

	return err
}

// DeleteTodo deletes the todo with id. It fails with ErrTodoToDeleteNotFound
// when no todo has id.
func (s *service) DeleteTodo(ctx context.Context, id int64) error {
	err := s.db.DeleteTodo(ctx, id)
	if errors.Is(err, db.ErrTodoToDeleteNotFound) {
		return fmt.Errorf("%w: id=%d", ErrTodoToDeleteNotFound, id)
	}

	return err
}

// normalizeTodo checks the fields a client sets and turns the reminders into
// distinct UTC instants in ascending order.
func normalizeTodo(t todo.Todo) (todo.Todo, error) {
	if isBlank(t.Description) {
		return todo.Todo{}, ErrEmptyTodoDescription
	}

	if isBlank(t.TaskID) {
		return todo.Todo{}, ErrEmptyTodoTaskID
	}

	if field, date, ok := firstInvalidDate("start_date", t.StartDate, "end_date", t.EndDate); !ok {
		return todo.Todo{}, fmt.Errorf("%w: %s=%s", ErrInvalidTodoDate, field, date)
	}

	// YYYY-MM-DD text compares in date order.
	if t.StartDate != "" && t.EndDate != "" && t.StartDate > t.EndDate {
		return todo.Todo{}, fmt.Errorf("%w: start_date=%s, end_date=%s", ErrTodoStartsAfterEnd, t.StartDate, t.EndDate)
	}

	if t.Position != nil && *t.Position < 0 {
		return todo.Todo{}, fmt.Errorf("%w: position=%d", ErrNegativeTodoPosition, *t.Position)
	}

	reminders := make([]time.Time, 0, len(t.Reminders))
	for _, at := range t.Reminders {
		reminders = append(reminders, at.UTC())
	}

	slices.SortFunc(reminders, time.Time.Compare)
	t.Reminders = slices.CompactFunc(reminders, time.Time.Equal)

	return t, nil
}

// firstInvalidDate checks two optional dates, given as field-value pairs, in
// order, and returns the first one that is set but not YYYY-MM-DD with
// ok false.
func firstInvalidDate(field1, date1, field2, date2 string) (field, date string, ok bool) {
	for _, d := range [][2]string{{field1, date1}, {field2, date2}} {
		if d[1] == "" {
			continue
		}

		if _, err := time.Parse(time.DateOnly, d[1]); err != nil {
			return d[0], d[1], false
		}
	}

	return "", "", true
}
