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

-- name: UpsertTaskParent :exec
INSERT INTO task_parents (child_id, parent_id) VALUES (?, ?)
ON CONFLICT (child_id) DO UPDATE SET parent_id = excluded.parent_id;

-- name: DeleteTaskParent :exec
DELETE FROM task_parents WHERE child_id = ?;

-- name: DeleteTaskLinks :exec
DELETE FROM task_links WHERE task_id = ?;

-- name: CreateTaskLink :exec
INSERT INTO task_links (task_id, linked_id) VALUES (?, ?)
ON CONFLICT DO NOTHING;

-- name: NextTaskNumber :one
UPDATE task_counters SET value = value + 1 WHERE origin = ? RETURNING value;

-- name: ExistsTask :one
SELECT EXISTS (SELECT 1 FROM tasks WHERE id = ?);

-- name: ExistsSchemaMigration :one
SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = ?);

-- name: CreateSchemaMigration :exec
INSERT INTO schema_migrations (version) VALUES (?);
