-- 003_create_rbac.sql
-- RBAC: organizations, users, API tokens, and audit log (BRD-080).

CREATE TABLE IF NOT EXISTS organizations (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL UNIQUE,
    created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS users (
    id              TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    email           TEXT NOT NULL UNIQUE,
    role            TEXT NOT NULL CHECK(role IN ('owner','admin','developer','viewer')),
    created_at      DATETIME NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_users_org ON users(organization_id);

-- API tokens scoped to an organization + role.
CREATE TABLE IF NOT EXISTS api_tokens (
    id              TEXT PRIMARY KEY,
    token_hash      TEXT NOT NULL UNIQUE,   -- SHA-256 hex of the raw bearer token
    organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    role            TEXT NOT NULL CHECK(role IN ('owner','admin','developer','viewer')),
    description     TEXT NOT NULL DEFAULT '',
    created_at      DATETIME NOT NULL DEFAULT (datetime('now')),
    expires_at      DATETIME
);
CREATE INDEX IF NOT EXISTS idx_api_tokens_org ON api_tokens(organization_id);

-- Audit log: every mutating API call.
CREATE TABLE IF NOT EXISTS audit_log (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    organization_id TEXT,
    user_id         TEXT,
    token_id        TEXT,
    action          TEXT NOT NULL,
    resource_type   TEXT NOT NULL DEFAULT '',
    resource_id     TEXT NOT NULL DEFAULT '',
    ip_address      TEXT NOT NULL DEFAULT '',
    created_at      DATETIME NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_audit_log_org     ON audit_log(organization_id);
CREATE INDEX IF NOT EXISTS idx_audit_log_created ON audit_log(created_at);
