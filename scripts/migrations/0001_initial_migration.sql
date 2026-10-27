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

-- Todos belong to one task through task_todos. Dates are "YYYY-MM-DD" text so
-- text comparison is date comparison. Like the task relations, there are no
-- foreign keys; db.Manager keeps the tables consistent in transactions.
CREATE TABLE todos (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    description TEXT NOT NULL,
    is_done     BOOLEAN NOT NULL DEFAULT 0,
    start_date  TEXT,
    end_date    TEXT
);

-- position orders the todos of a task as 0..n-1 without gaps. It is not
-- unique so shifting positions with one UPDATE cannot collide midway.
CREATE TABLE task_todos (
    todo_id  INTEGER PRIMARY KEY NOT NULL,
    task_id  TEXT NOT NULL,
    position INTEGER NOT NULL
);

CREATE INDEX task_todos_task_id_position ON task_todos (task_id, position);

CREATE TABLE todo_reminders (
    todo_id   INTEGER NOT NULL,
    remind_at DATETIME NOT NULL,
    PRIMARY KEY (todo_id, remind_at)
);

-- Service Desk customer requests are not tasks. issue_key is the Jira issue
-- key. status_at is when the current status was attained, not issue.updated.
CREATE TABLE service_desk_requests (
    issue_key  TEXT PRIMARY KEY NOT NULL,
    title      TEXT NOT NULL,
    status     TEXT NOT NULL,
    created_at DATETIME NOT NULL,
    status_at  DATETIME NOT NULL
);
