-- name: CreateTask :exec
INSERT INTO tasks (
    id, originated, title, description, status, start_date, due_date, parent_id, children_ids, linked_ids
) VALUES (
    ?, ?, ?, ?, ?, ?, ?, ?, ?, ?
);

-- name: ExistsTask :one
SELECT EXISTS (SELECT 1 FROM tasks WHERE id = ?);

-- name: ExistsSchemaMigration :one
SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = ?);

-- name: CreateSchemaMigration :exec
INSERT INTO schema_migrations (version) VALUES (?);
