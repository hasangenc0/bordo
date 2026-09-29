CREATE TABLE IF NOT EXISTS bordo_approvals (
  id TEXT PRIMARY KEY,
  kind TEXT NOT NULL,
  target TEXT NOT NULL,
  payload TEXT NOT NULL DEFAULT '{}',
  status TEXT NOT NULL DEFAULT 'pending',
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  resolved_at DATETIME
);
CREATE INDEX IF NOT EXISTS idx_approvals_status ON bordo_approvals(status);
