-- 018_release_expose.sql
-- How a release is exposed: 'none' (ClusterIP, internal only), 'nodeport'
-- (reachable at <node-ip>:<nodeport>), or 'ingress' (HTTP ingress at a host).
ALTER TABLE bordo_releases ADD COLUMN expose TEXT NOT NULL DEFAULT 'none';
ALTER TABLE bordo_releases ADD COLUMN expose_host TEXT NOT NULL DEFAULT '';
