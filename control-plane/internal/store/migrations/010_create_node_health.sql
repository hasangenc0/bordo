CREATE TABLE IF NOT EXISTS bordo_node_health (
    id TEXT PRIMARY KEY,
    region_name TEXT NOT NULL,
    node_name TEXT NOT NULL,
    ready INTEGER NOT NULL DEFAULT 0,
    last_heartbeat DATETIME NOT NULL,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(region_name, node_name)
);
