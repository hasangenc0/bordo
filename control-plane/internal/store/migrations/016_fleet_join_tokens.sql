-- 016_fleet_join_tokens.sql
-- Phase 1 of ADR-0006 (hardened deployment): single-use, short-lived node-join
-- tokens. Only the SHA-256 hash of each token is stored — the secret is shown
-- once at mint time and is never recoverable, so a DB read cannot leak it.

CREATE TABLE IF NOT EXISTS fleet_join_tokens (
    id          TEXT PRIMARY KEY,
    region_name TEXT NOT NULL,
    token_hash  TEXT NOT NULL,
    expires_at  DATETIME NOT NULL,
    used_at     DATETIME,
    revoked     INTEGER NOT NULL DEFAULT 0,
    created_at  DATETIME NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_fleet_join_tokens_region ON fleet_join_tokens(region_name);
