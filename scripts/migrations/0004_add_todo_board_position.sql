-- board_column and board_position place a todo by hand in a column of the
-- Todo view ('late', 'today', 'later' or 'nodate'), across tasks. Both are
-- NULL until it is placed there; the position only counts while the todo is
-- still in that column, so moving out by a date change needs no clean-up.
-- They are separate from task_todos.position, which orders a task's todos.
ALTER TABLE todos ADD COLUMN board_column TEXT;
ALTER TABLE todos ADD COLUMN board_position BIGINT;
