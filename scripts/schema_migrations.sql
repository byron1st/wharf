-- db.Manager sets search_path to the wharf schema, so it must exist before
-- any table is created. checksum is the hex SHA-256 of the migration file as
-- applied, so a file edited after it was applied is detected.
-- CREATE SCHEMA IF NOT EXISTS checks the CREATE privilege on the database even
-- when the schema exists, which a role limited to its own schema lacks, so the
-- schema is created only when missing.
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_namespace WHERE nspname = 'wharf') THEN
        CREATE SCHEMA wharf;
    END IF;
END $$;

CREATE TABLE IF NOT EXISTS schema_migrations (
    version    TEXT PRIMARY KEY NOT NULL,
    checksum   TEXT NOT NULL,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
