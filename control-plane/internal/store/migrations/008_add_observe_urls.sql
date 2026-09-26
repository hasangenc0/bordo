-- 008_add_observe_urls.sql
-- Add observe backend URLs to each region (BRD-031).

ALTER TABLE regions ADD COLUMN vm_url    TEXT NOT NULL DEFAULT '';
ALTER TABLE regions ADD COLUMN loki_url  TEXT NOT NULL DEFAULT '';
ALTER TABLE regions ADD COLUMN tempo_url TEXT NOT NULL DEFAULT '';
