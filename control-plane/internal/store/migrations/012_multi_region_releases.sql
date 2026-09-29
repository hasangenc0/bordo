ALTER TABLE bordo_releases ADD COLUMN release_group_id TEXT NOT NULL DEFAULT '';
ALTER TABLE bordo_releases ADD COLUMN log TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_releases_group ON bordo_releases(release_group_id);
