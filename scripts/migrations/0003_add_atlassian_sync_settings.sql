-- The one row of Atlassian sync settings, shared by every device.
-- interval_minutes is how long after the last sync, manual or automatic, the
-- server syncs again on its own; 0 turns automatic syncs off.
CREATE TABLE atlassian_sync_settings (
    id               BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK (id),
    interval_minutes BIGINT NOT NULL
);

INSERT INTO atlassian_sync_settings (interval_minutes) VALUES (60);
