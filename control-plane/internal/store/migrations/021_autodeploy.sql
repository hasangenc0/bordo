-- Opt-in deploy-on-merge. One row per project; the watcher polls the repo's default branch.
CREATE TABLE IF NOT EXISTS bordo_autodeploy (
  project_id        TEXT PRIMARY KEY REFERENCES projects(id) ON DELETE CASCADE,
  region            TEXT NOT NULL,
  expose            TEXT NOT NULL DEFAULT 'none',
  require_approval  INTEGER NOT NULL DEFAULT 1,
  last_sha          TEXT NOT NULL DEFAULT '',
  pending_sha       TEXT NOT NULL DEFAULT '',
  pending_image     TEXT NOT NULL DEFAULT '',
  pending_approval  TEXT NOT NULL DEFAULT '',
  last_result       TEXT NOT NULL DEFAULT '',
  updated_at        DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
