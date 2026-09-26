-- 004_create_regions.sql
-- Region + cluster fleet state (BRD-011).

CREATE TABLE IF NOT EXISTS regions (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL UNIQUE,
    status      TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','healthy','degraded','unreachable')),
    kubeconfig  TEXT NOT NULL DEFAULT '', -- encrypted kubeconfig YAML
    created_at  DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at  DATETIME NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_regions_name ON regions(name);

CREATE TABLE IF NOT EXISTS clusters (
    id          TEXT PRIMARY KEY,
    region_id   TEXT NOT NULL REFERENCES regions(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    node_count  INTEGER NOT NULL DEFAULT 0,
    k8s_version TEXT NOT NULL DEFAULT '',
    status      TEXT NOT NULL DEFAULT 'unknown',
    last_seen   DATETIME,
    created_at  DATETIME NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_clusters_region ON clusters(region_id);
