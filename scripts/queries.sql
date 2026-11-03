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

-- name: CountTodosByTask :many
SELECT task_todos.task_id,
       COUNT(*) AS todos,
       CAST(COALESCE(SUM(todos.is_done), 0) AS INTEGER) AS done_todos
FROM task_todos
JOIN todos ON todos.id = task_todos.todo_id
GROUP BY task_todos.task_id;

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

-- name: UpdateLocalTaskStatus :execrows
UPDATE tasks SET status = ? WHERE id = ? AND originated = 'local';

-- name: UpdateTaskPosition :exec
UPDATE tasks SET position = ? WHERE id = ?;

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
INSERT INTO todos (description, note, is_done, completed_at, start_date, end_date)
SELECT sqlc.arg(description), sqlc.arg(note), sqlc.arg(is_done), sqlc.narg(completed_at), sqlc.narg(start_date), sqlc.narg(end_date)
WHERE EXISTS (SELECT 1 FROM tasks WHERE tasks.id = sqlc.arg(task_id))
RETURNING id;

-- name: UpdateTodo :execrows
-- A todo that stays done keeps its completed_at.
UPDATE todos SET
    description  = sqlc.arg(description),
    note         = sqlc.arg(note),
    is_done      = sqlc.arg(is_done),
    completed_at = CASE WHEN sqlc.arg(is_done) THEN COALESCE(completed_at, sqlc.narg(completed_at)) END,
    start_date   = sqlc.narg(start_date),
    end_date     = sqlc.narg(end_date)
WHERE id = sqlc.arg(id);

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
  AND (todos.is_done = sqlc.narg(is_done) OR sqlc.narg(is_done) IS NULL)
  AND (todos.completed_at >= sqlc.narg(completed_since) OR sqlc.narg(completed_since) IS NULL)
  AND (todos.id IN (
          SELECT linked.todo_id FROM todo_service_desk_requests AS linked
          WHERE linked.issue_key = sqlc.narg(service_desk_request)
       ) OR sqlc.narg(service_desk_request) IS NULL)
ORDER BY task_todos.task_id, task_todos.position;

-- name: ListTodoReminders :many
-- Takes the ListTodos filters rather than todo IDs, so the number of bind
-- variables stays fixed however many todos match.
SELECT todo_reminders.todo_id, todo_reminders.remind_at
FROM todo_reminders
JOIN todos ON todos.id = todo_reminders.todo_id
JOIN task_todos ON task_todos.todo_id = todo_reminders.todo_id
WHERE (task_todos.task_id = sqlc.narg(task_id) OR sqlc.narg(task_id) IS NULL)
  AND (todos.is_done = sqlc.narg(is_done) OR sqlc.narg(is_done) IS NULL)
  AND (todos.completed_at >= sqlc.narg(completed_since) OR sqlc.narg(completed_since) IS NULL)
  AND (todos.id IN (
          SELECT linked.todo_id FROM todo_service_desk_requests AS linked
          WHERE linked.issue_key = sqlc.narg(service_desk_request)
       ) OR sqlc.narg(service_desk_request) IS NULL)
ORDER BY todo_reminders.todo_id, todo_reminders.remind_at;

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

-- name: UpsertServiceDeskRequest :exec
INSERT INTO service_desk_requests (
    issue_key, title, status, status_category, created_at, status_at
) VALUES (
    ?, ?, ?, ?, ?, ?
)
ON CONFLICT (issue_key) DO UPDATE SET
    title           = excluded.title,
    status          = excluded.status,
    status_category = excluded.status_category,
    created_at      = excluded.created_at,
    status_at       = excluded.status_at;

-- name: CreateTodoServiceDeskRequest :execrows
-- Inserts nothing when the todo or the request is not stored or the link
-- exists, so the check is a write as the transaction's first statement.
INSERT INTO todo_service_desk_requests (todo_id, issue_key)
SELECT sqlc.arg(todo_id), sqlc.arg(issue_key)
WHERE EXISTS (SELECT 1 FROM todos WHERE todos.id = sqlc.arg(todo_id))
  AND EXISTS (SELECT 1 FROM service_desk_requests WHERE service_desk_requests.issue_key = sqlc.arg(issue_key))
ON CONFLICT (todo_id, issue_key) DO NOTHING;

-- name: ExistsTodo :one
SELECT EXISTS (SELECT 1 FROM todos WHERE id = ?);

-- name: ExistsServiceDeskRequest :one
SELECT EXISTS (SELECT 1 FROM service_desk_requests WHERE issue_key = ?);

-- name: DeleteTodoServiceDeskRequest :execrows
DELETE FROM todo_service_desk_requests WHERE todo_id = ? AND issue_key = ?;

-- name: DeleteTodoServiceDeskRequestsOfTodo :exec
DELETE FROM todo_service_desk_requests WHERE todo_id = ?;

-- name: DeleteTodoServiceDeskRequestsOfTask :exec
DELETE FROM todo_service_desk_requests WHERE todo_id IN (SELECT todo_id FROM task_todos WHERE task_id = ?);

-- name: ListTodoServiceDeskRequests :many
-- Takes the ListTodos filters, as ListTodoReminders does.
SELECT todo_service_desk_requests.todo_id, sqlc.embed(service_desk_requests)
FROM todo_service_desk_requests
JOIN service_desk_requests ON service_desk_requests.issue_key = todo_service_desk_requests.issue_key
JOIN todos ON todos.id = todo_service_desk_requests.todo_id
JOIN task_todos ON task_todos.todo_id = todo_service_desk_requests.todo_id
WHERE (task_todos.task_id = sqlc.narg(task_id) OR sqlc.narg(task_id) IS NULL)
  AND (todos.is_done = sqlc.narg(is_done) OR sqlc.narg(is_done) IS NULL)
  AND (todos.completed_at >= sqlc.narg(completed_since) OR sqlc.narg(completed_since) IS NULL)
  AND (todos.id IN (
          SELECT linked.todo_id FROM todo_service_desk_requests AS linked
          WHERE linked.issue_key = sqlc.narg(service_desk_request)
       ) OR sqlc.narg(service_desk_request) IS NULL)
ORDER BY todo_service_desk_requests.todo_id, service_desk_requests.issue_key;

-- name: ListServiceDeskRequests :many
-- 'DONE' is the closed Jira status category.
SELECT * FROM service_desk_requests
WHERE status_category != 'DONE' OR CAST(sqlc.arg(include_closed) AS BOOLEAN)
ORDER BY issue_key;

-- name: CreateAtlassianSync :exec
INSERT INTO atlassian_syncs (
    synced_at, issues, issue_failures, open_requests, closed_requests, request_failures
) VALUES (
    ?, ?, ?, ?, ?, ?
);

-- name: GetLastAtlassianSync :one
SELECT * FROM atlassian_syncs ORDER BY id DESC LIMIT 1;
