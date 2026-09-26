CREATE TABLE IF NOT EXISTS bordo_releases (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL,
    image_tag TEXT NOT NULL,
    region TEXT NOT NULL,
    strategy TEXT NOT NULL DEFAULT 'rolling',
    status TEXT NOT NULL DEFAULT 'pending',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
