-- The address users reach a region's machine at (for NodePort URLs). Set by the operator;
-- the control plane cannot reliably detect it from inside Docker or behind NAT.
ALTER TABLE regions ADD COLUMN public_host TEXT NOT NULL DEFAULT '';
