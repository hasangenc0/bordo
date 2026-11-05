-- 022_region_disk.sql
-- Disk usage of a region's machine, as last reported by its on-cluster reconciler on a
-- desired-state pull. A full disk is the usual reason a reconciler stops (the node taints itself
-- disk-pressure and evicts pods), so the control plane keeps the last reading to show it.
ALTER TABLE regions ADD COLUMN disk_total_bytes INTEGER;
ALTER TABLE regions ADD COLUMN disk_free_bytes INTEGER;
ALTER TABLE regions ADD COLUMN disk_reported_at DATETIME;
