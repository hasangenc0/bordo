-- Platform settings: encrypted KV store for secrets and configuration
-- entered via the setup wizard or CLI.
CREATE TABLE IF NOT EXISTS platform_settings (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL,  -- AES-256-GCM encrypted, base64-encoded
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
