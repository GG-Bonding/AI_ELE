-- V3.3: Durable Skill Execution + approval actor fields.

ALTER TABLE skill_executions
    ADD COLUMN IF NOT EXISTS step_cursor INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS lease_owner TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS lease_until TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS heartbeat_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS requester_id TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS spec_snapshot TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_skill_executions_stale_running
    ON skill_executions (status, lease_until)
    WHERE status = 'RUNNING';

ALTER TABLE skill_approval_requests
    ADD COLUMN IF NOT EXISTS requester_id TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS approved_by TEXT NOT NULL DEFAULT '';

COMMENT ON COLUMN skill_executions.step_cursor IS 'Next Spec.Steps index to execute (crash recovery).';
COMMENT ON COLUMN skill_executions.lease_until IS 'Exclusive recovery lease expiry.';
