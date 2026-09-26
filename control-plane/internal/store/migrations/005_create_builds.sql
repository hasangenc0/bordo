-- 005_create_builds.sql
-- Build records for the container build orchestrator (BRD-007).

CREATE TABLE IF NOT EXISTS builds (
    id           TEXT PRIMARY KEY,
    project_id   TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    status       TEXT NOT NULL DEFAULT 'queued' CHECK(status IN ('queued','running','success','failed')),
    image_name   TEXT NOT NULL DEFAULT '',
    image_tag    TEXT NOT NULL DEFAULT '',
    image_digest TEXT NOT NULL DEFAULT '',
    error_message TEXT NOT NULL DEFAULT '',
    started_at   DATETIME,
    completed_at DATETIME,
    created_at   DATETIME NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_builds_project ON builds(project_id);
CREATE INDEX IF NOT EXISTS idx_builds_status  ON builds(status);

CREATE TABLE IF NOT EXISTS build_logs (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    build_id   TEXT NOT NULL REFERENCES builds(id) ON DELETE CASCADE,
    line       TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_build_logs_build ON build_logs(build_id);
