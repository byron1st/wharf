CREATE TABLE tasks (
    id           TEXT PRIMARY KEY NOT NULL,
    originated   TEXT NOT NULL,
    title        TEXT NOT NULL,
    description  TEXT NOT NULL DEFAULT '',
    status       TEXT NOT NULL,
    start_date   DATETIME,
    due_date     DATETIME,
    parent_id    TEXT NOT NULL DEFAULT '',
    children_ids TEXT NOT NULL DEFAULT '[]', -- JSON array of task IDs
    linked_ids   TEXT NOT NULL DEFAULT '[]'  -- JSON array of task IDs
);
