CREATE TABLE IF NOT EXISTS bordo_secrets (
    id              TEXT PRIMARY KEY,
    project_id      TEXT NOT NULL,
    key             TEXT NOT NULL,
    encrypted_value TEXT NOT NULL,
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(project_id, key)
);
