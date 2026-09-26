-- 001_initial.sql
-- Creates the schema_migrations tracking table used by the migration runner.

CREATE TABLE IF NOT EXISTS schema_migrations (
    version     TEXT PRIMARY KEY,
    applied_at  DATETIME NOT NULL DEFAULT (datetime('now'))
);
