CREATE TABLE tasks (
    id          TEXT PRIMARY KEY NOT NULL,
    originated  TEXT NOT NULL,
    title       TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    status      TEXT NOT NULL,
    start_date  DATETIME,
    due_date    DATETIME
);

-- A task has at most one parent. Relations may point at tasks that are not
-- stored, so there is no foreign key to tasks.
CREATE TABLE task_hierarchies (
    child_id  TEXT PRIMARY KEY NOT NULL,
    parent_id TEXT NOT NULL
);

CREATE INDEX task_hierarchies_parent_id ON task_hierarchies (parent_id);

-- Links are directed by the task that declared them; read both directions to
-- find every task linked to a task.
CREATE TABLE task_links (
    task_id   TEXT NOT NULL,
    linked_id TEXT NOT NULL,
    PRIMARY KEY (task_id, linked_id)
);

CREATE INDEX task_links_linked_id ON task_links (linked_id);

-- Named counters incremented with UPDATE ... RETURNING. local_task numbers
-- local task keys.
CREATE TABLE counters (
    name  TEXT PRIMARY KEY NOT NULL,
    value INTEGER NOT NULL
);

INSERT INTO counters (name, value) VALUES ('local_task', 0);
