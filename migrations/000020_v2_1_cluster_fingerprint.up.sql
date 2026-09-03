-- V2.1-5: cluster fingerprint for automatic generalization dedupe.
ALTER TABLE patterns
    ADD COLUMN IF NOT EXISTS cluster_fingerprint TEXT NOT NULL DEFAULT '';

CREATE UNIQUE INDEX IF NOT EXISTS idx_patterns_tenant_cluster_fingerprint
    ON patterns (tenant_id, cluster_fingerprint)
    WHERE cluster_fingerprint <> '';
