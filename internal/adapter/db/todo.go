package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/byron1st/wharf/internal/pkg/todo"
)

var (
	ErrFailedToCreateTodo          = errors.New("db: failed to create todo")
	ErrTaskOfTodoToCreateNotFound  = errors.New("db: task of todo to create not found")
	ErrFailedToCountTaskTodos      = errors.New("db: failed to count todos of task")
	ErrFailedToShiftTaskTodosDown  = errors.New("db: failed to shift todos of task down")
	ErrFailedToStoreTaskTodo       = errors.New("db: failed to store task todo")
	ErrFailedToStoreTodoReminder   = errors.New("db: failed to store todo reminder")
	ErrFailedToListTodos           = errors.New("db: failed to list todos")
	ErrFailedToListTodoReminders   = errors.New("db: failed to list todo reminders")
	ErrFailedToUpdateTodo          = errors.New("db: failed to update todo")
	ErrTodoToUpdateNotFound        = errors.New("db: todo to update not found")
	ErrFailedToCheckTaskOfTodo     = errors.New("db: failed to check task of todo")
	ErrTaskOfTodoToUpdateNotFound  = errors.New("db: task of todo to update not found")
	ErrFailedToDeleteTaskTodo      = errors.New("db: failed to delete task todo")
	ErrFailedToShiftTaskTodosUp    = errors.New("db: failed to shift todos of task up")
	ErrFailedToClearTodoReminders  = errors.New("db: failed to clear todo reminders")
	ErrFailedToDeleteTodo          = errors.New("db: failed to delete todo")
	ErrTodoToDeleteNotFound        = errors.New("db: todo to delete not found")
	ErrFailedToDeleteTodoReminders = errors.New("db: failed to delete todo reminders")
)

// CreateTodo stores t as a new todo of the stored task t.TaskID and returns
// its ID. The todo goes at t.Position, which must not be negative, shifting
// the todos from there down, or at the end when t.Position is nil or past it.
// Duplicate reminders are stored once. t.ID is ignored. It fails with
// ErrTaskOfTodoToCreateNotFound when no task has t.TaskID.
func (m *manager) CreateTodo(ctx context.Context, t todo.Todo) (int64, error) {
	if err := m.checkConnected(); err != nil {
		return 0, err
	}

	var id int64

	err := m.inTx(ctx, func(queries *Queries) error {
		var err error

		// The insert checks the task, keeping the write-first rule described
		// at busyTimeoutPragma.
		id, err = queries.CreateTodo(ctx, CreateTodoParams{
			Description: t.Description,
			IsDone:      t.IsDone,
			StartDate:   toNullString(t.StartDate),
			EndDate:     toNullString(t.EndDate),
			TaskID:      t.TaskID,
		})
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: task_id=%s", ErrTaskOfTodoToCreateNotFound, t.TaskID)
		}

		if err != nil {
			return fmt.Errorf("%w: task_id=%s", errors.Join(ErrFailedToCreateTodo, err), t.TaskID)
		}

		if err := placeTodo(ctx, queries, id, t.TaskID, t.Position); err != nil {
			return err
		}

		return storeReminders(ctx, queries, id, t.Reminders)
	})
	if err != nil {
		return 0, err
	}

	return id, nil
}

// ListTodos returns the todos that match every set field of f, ordered by
// task ID and position, each with its reminders in ascending order.
func (m *manager) ListTodos(ctx context.Context, f todo.Filter) ([]todo.Todo, error) {
	if err := m.checkConnected(); err != nil {
		return nil, err
	}

	params := ListTodosParams{
		TaskID:  toNullString(f.TaskID),
		Today:   toNullString(f.Today),
		EndDate: toNullString(f.EndDate),
	}
	if f.IsDone != nil {
		params.IsDone = sql.NullBool{Bool: *f.IsDone, Valid: true}
	}

	rows, err := m.queries.ListTodos(ctx, params)
	if err != nil {
		return nil, errors.Join(ErrFailedToListTodos, err)
	}

	todos := make([]todo.Todo, 0, len(rows))

	for _, row := range rows {
		position := int(row.Position)
		todos = append(todos, todo.Todo{
			ID:          row.ID,
			TaskID:      row.TaskID,
			Description: row.Description,
			IsDone:      row.IsDone,
			StartDate:   row.StartDate.String,
			EndDate:     row.EndDate.String,
			Position:    &position,
		})
	}

	if len(todos) == 0 {
		return todos, nil
	}

	// The same filters select the reminders of exactly these todos.
	reminders, err := m.queries.ListTodoReminders(ctx, ListTodoRemindersParams(params))
	if err != nil {
		return nil, errors.Join(ErrFailedToListTodoReminders, err)
	}

	byTodo := map[int64][]time.Time{}
	for _, r := range reminders {
		byTodo[r.TodoID] = append(byTodo[r.TodoID], r.RemindAt)
	}

	for i := range todos {
		todos[i].Reminders = byTodo[todos[i].ID]
	}

	return todos, nil
}

// UpdateTodo replaces every field of the stored todo t.ID and its reminders.
// When t.TaskID is the current task, a nil t.Position keeps its place;
// otherwise the todo moves to t.TaskID as CreateTodo places it. Both tasks
// renumber their todos from 0 without gaps. It fails with
// ErrTodoToUpdateNotFound when no todo has t.ID and with
// ErrTaskOfTodoToUpdateNotFound when no task has t.TaskID.
func (m *manager) UpdateTodo(ctx context.Context, t todo.Todo) error {
	if err := m.checkConnected(); err != nil {
		return err
	}

	return m.inTx(ctx, func(queries *Queries) error {
		updated, err := queries.UpdateTodo(ctx, UpdateTodoParams{
			Description: t.Description,
			IsDone:      t.IsDone,
			StartDate:   toNullString(t.StartDate),
			EndDate:     toNullString(t.EndDate),
			ID:          t.ID,
		})
		if err != nil {
			return fmt.Errorf("%w: id=%d", errors.Join(ErrFailedToUpdateTodo, err), t.ID)
		}

		if updated == 0 {
			return fmt.Errorf("%w: id=%d", ErrTodoToUpdateNotFound, t.ID)
		}

		exists, err := queries.ExistsTask(ctx, t.TaskID)
		if err != nil {
			return fmt.Errorf("%w: id=%d, task_id=%s", errors.Join(ErrFailedToCheckTaskOfTodo, err), t.ID, t.TaskID)
		}

		if !exists {
			return fmt.Errorf("%w: id=%d, task_id=%s", ErrTaskOfTodoToUpdateNotFound, t.ID, t.TaskID)
		}

		old, err := unlinkTodo(ctx, queries, t.ID)
		if err != nil {
			return err
		}

		position := t.Position
		if position == nil && old.TaskID == t.TaskID {
			current := int(old.Position)
			position = &current
		}

		if err := placeTodo(ctx, queries, t.ID, t.TaskID, position); err != nil {
			return err
		}

		if err := queries.DeleteTodoReminders(ctx, t.ID); err != nil {
			return fmt.Errorf("%w: id=%d", errors.Join(ErrFailedToClearTodoReminders, err), t.ID)
		}

		return storeReminders(ctx, queries, t.ID, t.Reminders)
	})
}

// DeleteTodo deletes the todo with id and its reminders, and moves the later
// todos of its task up. It fails with ErrTodoToDeleteNotFound when no todo
// has id.
func (m *manager) DeleteTodo(ctx context.Context, id int64) error {
	if err := m.checkConnected(); err != nil {
		return err
	}

	return m.inTx(ctx, func(queries *Queries) error {
		deleted, err := queries.DeleteTodo(ctx, id)
		if err != nil {
			return fmt.Errorf("%w: id=%d", errors.Join(ErrFailedToDeleteTodo, err), id)
		}

		if deleted == 0 {
			return fmt.Errorf("%w: id=%d", ErrTodoToDeleteNotFound, id)
		}

		if _, err := unlinkTodo(ctx, queries, id); err != nil {
			return err
		}

		if err := queries.DeleteTodoReminders(ctx, id); err != nil {
			return fmt.Errorf("%w: id=%d", errors.Join(ErrFailedToDeleteTodoReminders, err), id)
		}

		return nil
	})
}

// placeTodo links todo id to taskID at position, or at the end when position
// is nil or past it, shifting the todos from there down.
func placeTodo(ctx context.Context, queries *Queries, id int64, taskID string, position *int) error {
	count, err := queries.CountTaskTodos(ctx, taskID)
	if err != nil {
		return fmt.Errorf("%w: task_id=%s", errors.Join(ErrFailedToCountTaskTodos, err), taskID)
	}

	at := count
	if position != nil && int64(*position) < count {
		at = int64(*position)
	}

	if err := queries.ShiftTaskTodosDown(ctx, ShiftTaskTodosDownParams{TaskID: taskID, Position: at}); err != nil {
		return fmt.Errorf("%w: task_id=%s", errors.Join(ErrFailedToShiftTaskTodosDown, err), taskID)
	}

	if err := queries.CreateTaskTodo(ctx, CreateTaskTodoParams{TodoID: id, TaskID: taskID, Position: at}); err != nil {
		return fmt.Errorf("%w: id=%d, task_id=%s", errors.Join(ErrFailedToStoreTaskTodo, err), id, taskID)
	}

	return nil
}

// unlinkTodo removes todo id from its task, moves the later todos up, and
// returns where it was.
func unlinkTodo(ctx context.Context, queries *Queries, id int64) (DeleteTaskTodoRow, error) {
	old, err := queries.DeleteTaskTodo(ctx, id)
	if err != nil {
		return DeleteTaskTodoRow{}, fmt.Errorf("%w: id=%d", errors.Join(ErrFailedToDeleteTaskTodo, err), id)
	}

	if err := queries.ShiftTaskTodosUp(ctx, ShiftTaskTodosUpParams(old)); err != nil {
		return DeleteTaskTodoRow{}, fmt.Errorf(
			"%w: task_id=%s",
			errors.Join(ErrFailedToShiftTaskTodosUp, err),
			old.TaskID,
		)
	}

	return old, nil
}

func storeReminders(ctx context.Context, queries *Queries, id int64, reminders []time.Time) error {
	for _, at := range reminders {
		if err := queries.CreateTodoReminder(ctx, CreateTodoReminderParams{TodoID: id, RemindAt: at}); err != nil {
			return fmt.Errorf("%w: id=%d, remind_at=%s", errors.Join(ErrFailedToStoreTodoReminder, err), id, at)
		}
	}

	return nil
}

// toNullString stores an empty s as NULL.
func toNullString(s string) sql.NullString {
	return sql.NullString{String: s, Valid: s != ""}
}
