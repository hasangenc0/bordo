-- ADR-0006 Phase 2 feedback loop: record when a region's on-cluster reconciler
-- last pulled desired state. This is the region's liveness signal — the control
-- plane derives health from it instead of latching "healthy" at join time.
ALTER TABLE regions ADD COLUMN last_pull_at DATETIME;
