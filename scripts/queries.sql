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
DELETE FROM task_hierarchies WHERE child_id = ? AND parent_id NOT LIKE 'local-%';

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

-- name: IncrementCounter :one
UPDATE counters SET value = value + 1 WHERE name = ? RETURNING value;

-- name: ExistsSchemaMigration :one
SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = ?);

-- name: CreateSchemaMigration :exec
INSERT INTO schema_migrations (version) VALUES (?);
