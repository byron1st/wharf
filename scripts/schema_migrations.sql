-- checksum is the hex SHA-256 of the migration file as applied, so a file
-- edited after it was applied is detected.
CREATE TABLE IF NOT EXISTS schema_migrations (
    version    TEXT PRIMARY KEY NOT NULL,
    checksum   TEXT NOT NULL,
    applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
