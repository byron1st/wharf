-- db.Manager sets search_path to the wharf schema, so it must exist before
-- any table is created. checksum is the hex SHA-256 of the migration file as
-- applied, so a file edited after it was applied is detected.
CREATE SCHEMA IF NOT EXISTS wharf;

CREATE TABLE IF NOT EXISTS schema_migrations (
    version    TEXT PRIMARY KEY NOT NULL,
    checksum   TEXT NOT NULL,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
