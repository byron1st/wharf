-- closed_at is when a task was last closed (done or canceled) and NULL while
-- it is open. A local task gets it from Wharf when its status changes from
-- open to closed; a Jira task from the issue's statuscategorychangedate.
-- Tasks closed before this migration have no known time and stay NULL.
-- closed_synced_at is the synced_at of the full Atlassian sync that first
-- saw a stored, open Jira task closed, and NULL while the task is open or
-- when its closing was seen another way.
ALTER TABLE tasks ADD COLUMN closed_at TIMESTAMPTZ;
ALTER TABLE tasks ADD COLUMN closed_synced_at TIMESTAMPTZ;
