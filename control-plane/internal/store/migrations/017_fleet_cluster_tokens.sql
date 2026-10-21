-- 017_fleet_cluster_tokens.sql
-- Phase 2 of ADR-0006: a revocable, region-scoped credential issued to a cluster
-- at registration. The on-cluster reconciler uses it to PULL desired state from
-- the control plane. Unlike join tokens it is long-lived (renewable/revocable),
-- not single-use. Only the SHA-256 hash is stored.

CREATE TABLE IF NOT EXISTS fleet_cluster_tokens (
    id          TEXT PRIMARY KEY,
    region_name TEXT NOT NULL,
    token_hash  TEXT NOT NULL,
    revoked     INTEGER NOT NULL DEFAULT 0,
    created_at  DATETIME NOT NULL DEFAULT (datetime('now')),
    last_seen   DATETIME
);
CREATE INDEX IF NOT EXISTS idx_fleet_cluster_tokens_region ON fleet_cluster_tokens(region_name);
