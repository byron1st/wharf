-- board_position orders a todo within its column of the Todo view, across
-- tasks, and is NULL until the todo is placed there by hand. It is separate
-- from task_todos.position, which orders the todos of one task.
ALTER TABLE todos ADD COLUMN board_position BIGINT;
