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

-- name: UpsertTaskHierarchy :exec
INSERT INTO task_hierarchies (child_id, parent_id) VALUES (?, ?)
ON CONFLICT (child_id) DO UPDATE SET parent_id = excluded.parent_id;

-- name: DeleteTaskLinks :exec
DELETE FROM task_links WHERE task_id = ?;

-- name: CreateTaskLink :exec
INSERT INTO task_links (task_id, linked_id) VALUES (?, ?)
ON CONFLICT (task_id, linked_id) DO NOTHING;

-- name: IncrementCounter :one
UPDATE counters SET value = value + 1 WHERE name = ? RETURNING value;

-- name: ExistsSchemaMigration :one
SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = ?);

-- name: CreateSchemaMigration :exec
INSERT INTO schema_migrations (version) VALUES (?);
