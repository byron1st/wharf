-- Relations move out of JSON columns into their own tables. There are no
-- foreign keys: a related task (an epic not assigned to me, a done subtask)
-- may not be stored in tasks.

-- A task has at most one parent.
CREATE TABLE task_parents (
    child_id  TEXT PRIMARY KEY NOT NULL,
    parent_id TEXT NOT NULL
);

-- Each row is a link claimed by task_id.
CREATE TABLE task_links (
    task_id   TEXT NOT NULL,
    linked_id TEXT NOT NULL,
    PRIMARY KEY (task_id, linked_id)
);

-- Last number handed out per origin; numbers are never reused.
CREATE TABLE task_counters (
    origin TEXT PRIMARY KEY NOT NULL,
    value  INTEGER NOT NULL
);

INSERT INTO task_counters (origin, value) VALUES ('local', 0);

-- Task IDs become "{originated}-{id}". Children claims go in first so a
-- task's own parent claim, inserted next, wins on conflict.
INSERT INTO task_parents (child_id, parent_id)
SELECT t.originated || '-' || c.value, t.originated || '-' || t.id
FROM tasks AS t, json_each(t.children_ids) AS c
WHERE c.value != ''
ON CONFLICT (child_id) DO UPDATE SET parent_id = excluded.parent_id;

INSERT INTO task_parents (child_id, parent_id)
SELECT originated || '-' || id, originated || '-' || parent_id
FROM tasks
WHERE parent_id != ''
ON CONFLICT (child_id) DO UPDATE SET parent_id = excluded.parent_id;

INSERT INTO task_links (task_id, linked_id)
SELECT t.originated || '-' || t.id, t.originated || '-' || l.value
FROM tasks AS t, json_each(t.linked_ids) AS l
WHERE l.value != ''
ON CONFLICT DO NOTHING;

UPDATE tasks SET id = originated || '-' || id;

ALTER TABLE tasks DROP COLUMN parent_id;
ALTER TABLE tasks DROP COLUMN children_ids;
ALTER TABLE tasks DROP COLUMN linked_ids;
