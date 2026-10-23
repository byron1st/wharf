-- name: UpsertTask :exec
INSERT INTO tasks (
    id, originated, title, description, status, start_date, due_date, parent_id, children_ids, linked_ids
) VALUES (
    ?, ?, ?, ?, ?, ?, ?, ?, ?, ?
)
ON CONFLICT (id) DO UPDATE SET
    originated   = excluded.originated,
    title        = excluded.title,
    description  = excluded.description,
    status       = excluded.status,
    start_date   = excluded.start_date,
    due_date     = excluded.due_date,
    parent_id    = excluded.parent_id,
    children_ids = excluded.children_ids,
    linked_ids   = excluded.linked_ids;

-- name: ExistsSchemaMigration :one
SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = ?);

-- name: CreateSchemaMigration :exec
INSERT INTO schema_migrations (version) VALUES (?);
