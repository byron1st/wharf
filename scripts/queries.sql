-- name: UpsertTask :exec
INSERT INTO tasks (
    id, originated, title, description, status, start_date, due_date
) VALUES (
    ?, ?, ?, ?, ?, ?, ?
)
ON CONFLICT (id) DO UPDATE SET
    originated  = excluded.originated,
    title       = excluded.title,
    description = excluded.description,
    status      = excluded.status,
    start_date  = excluded.start_date,
    due_date    = excluded.due_date;

-- name: DeleteTaskParent :exec
DELETE FROM task_hierarchies WHERE child_id = ?;

-- name: DeleteTaskNonLocalParent :exec
-- 'local' is task.OriginLocal. A local parent of a non-local task is always
-- stored, since only CreateLocalTask and UpdateLocalTask declare one and
-- DeleteTask removes its rows.
DELETE FROM task_hierarchies
WHERE child_id = ?
  AND parent_id NOT IN (SELECT id FROM tasks WHERE originated = 'local');

-- name: DeleteTaskChildren :exec
DELETE FROM task_hierarchies WHERE parent_id = ?;

-- name: UpsertTaskHierarchy :exec
INSERT INTO task_hierarchies (child_id, parent_id) VALUES (?, ?)
ON CONFLICT (child_id) DO UPDATE SET parent_id = excluded.parent_id;

-- name: DeleteTaskLinks :exec
DELETE FROM task_links WHERE task_id = ?;

-- name: CreateTaskLink :exec
INSERT INTO task_links (task_id, linked_id) VALUES (?, ?)
ON CONFLICT (task_id, linked_id) DO NOTHING;

-- name: ListTasks :many
SELECT * FROM tasks ORDER BY id;

-- name: ListTasksByIDs :many
SELECT * FROM tasks WHERE id IN (sqlc.slice(ids)) ORDER BY id;

-- name: GetTask :one
SELECT sqlc.embed(tasks), task_hierarchies.parent_id
FROM tasks
LEFT JOIN task_hierarchies ON task_hierarchies.child_id = tasks.id
WHERE tasks.id = ?;

-- name: ListTaskChildren :many
SELECT child_id FROM task_hierarchies WHERE parent_id = ? ORDER BY child_id;

-- name: ListTaskLinks :many
SELECT linked_id AS id FROM task_links WHERE task_links.task_id = sqlc.arg(task_id)
UNION
SELECT task_links.task_id AS id FROM task_links WHERE task_links.linked_id = sqlc.arg(task_id)
ORDER BY id;

-- name: UpdateLocalTask :execrows
UPDATE tasks SET
    title       = ?,
    description = ?,
    status      = ?,
    start_date  = ?,
    due_date    = ?
WHERE id = ? AND originated = 'local';

-- name: DeleteTask :execrows
DELETE FROM tasks WHERE id = ?;

-- name: DeleteTaskHierarchies :exec
DELETE FROM task_hierarchies WHERE child_id = sqlc.arg(id) OR parent_id = sqlc.arg(id);

-- name: DeleteTaskLinksOf :exec
DELETE FROM task_links WHERE task_id = sqlc.arg(id) OR linked_id = sqlc.arg(id);

-- name: DeleteTodosOfTask :exec
DELETE FROM todos WHERE id IN (SELECT todo_id FROM task_todos WHERE task_id = ?);

-- name: DeleteTodoRemindersOfTask :exec
DELETE FROM todo_reminders WHERE todo_id IN (SELECT todo_id FROM task_todos WHERE task_id = ?);

-- name: DeleteTaskTodosOfTask :exec
DELETE FROM task_todos WHERE task_id = ?;

-- name: CreateTodo :one
-- Inserts nothing, failing with no rows, when the task is not stored, so the
-- check is a write as the transaction's first statement.
INSERT INTO todos (description, is_done, start_date, end_date)
SELECT sqlc.arg(description), sqlc.arg(is_done), sqlc.narg(start_date), sqlc.narg(end_date)
WHERE EXISTS (SELECT 1 FROM tasks WHERE tasks.id = sqlc.arg(task_id))
RETURNING id;

-- name: UpdateTodo :execrows
UPDATE todos SET
    description = ?,
    is_done     = ?,
    start_date  = ?,
    end_date    = ?
WHERE id = ?;

-- name: DeleteTodo :execrows
DELETE FROM todos WHERE id = ?;

-- name: ExistsTask :one
SELECT EXISTS (SELECT 1 FROM tasks WHERE id = ?);

-- name: DeleteTaskTodo :one
DELETE FROM task_todos WHERE todo_id = ? RETURNING task_id, position;

-- name: CountTaskTodos :one
SELECT COUNT(*) FROM task_todos WHERE task_id = ?;

-- name: ShiftTaskTodosDown :exec
UPDATE task_todos SET position = position + 1
WHERE task_id = sqlc.arg(task_id) AND position >= sqlc.arg(position);

-- name: ShiftTaskTodosUp :exec
UPDATE task_todos SET position = position - 1
WHERE task_id = sqlc.arg(task_id) AND position > sqlc.arg(position);

-- name: CreateTaskTodo :exec
INSERT INTO task_todos (todo_id, task_id, position) VALUES (?, ?, ?);

-- name: ListTodos :many
-- A NULL filter is not applied.
SELECT todos.*, task_todos.task_id, task_todos.position
FROM todos
JOIN task_todos ON task_todos.todo_id = todos.id
WHERE (task_todos.task_id = sqlc.narg(task_id) OR sqlc.narg(task_id) IS NULL)
  AND (todos.start_date <= sqlc.narg(today) OR todos.start_date IS NULL OR sqlc.narg(today) IS NULL)
  AND (todos.end_date = sqlc.narg(end_date) OR sqlc.narg(end_date) IS NULL)
  AND (todos.is_done = sqlc.narg(is_done) OR sqlc.narg(is_done) IS NULL)
ORDER BY task_todos.task_id, task_todos.position;

-- name: ListTodoReminders :many
SELECT todo_id, remind_at FROM todo_reminders
WHERE todo_id IN (sqlc.slice(ids))
ORDER BY todo_id, remind_at;

-- name: CreateTodoReminder :exec
INSERT INTO todo_reminders (todo_id, remind_at) VALUES (?, ?)
ON CONFLICT (todo_id, remind_at) DO NOTHING;

-- name: DeleteTodoReminders :exec
DELETE FROM todo_reminders WHERE todo_id = ?;

-- name: IncrementCounter :one
UPDATE counters SET value = value + 1 WHERE name = ? RETURNING value;

-- name: ExistsSchemaMigration :one
SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = ?);

-- name: CreateSchemaMigration :exec
INSERT INTO schema_migrations (version) VALUES (?);
